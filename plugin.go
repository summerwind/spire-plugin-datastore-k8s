package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/hcl"
	"github.com/spiffe/spire/pkg/common/bundleutil"
	"github.com/spiffe/spire/pkg/common/selector"
	"github.com/spiffe/spire/proto/spire/common"
	spi "github.com/spiffe/spire/proto/spire/common/plugin"
	"github.com/spiffe/spire/proto/spire/server/datastore"
	"github.com/summerwind/spire-plugin-datastore-kubernetes/pkg/apis"
	"github.com/summerwind/spire-plugin-datastore-kubernetes/pkg/apis/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	ErrNotFound = errors.New("resource not found")

	kubeConfig *rest.Config
)

type Plugin struct {
	client.Client
	config *PluginConfig
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Configure(ctx context.Context, req *spi.ConfigureRequest) (*spi.ConfigureResponse, error) {
	var err error

	c := PluginConfig{}
	err = hcl.Decode(&c, req.Configuration)
	if err != nil {
		return nil, err
	}

	err = c.Validate()
	if err != nil {
		return nil, err
	}

	p.config = &c

	if kubeConfig == nil {
		if c.KubeConfig != "" && os.Getenv("KUBECONFIG") == "" {
			err = os.Setenv("KUBECONFIG", c.KubeConfig)
			if err != nil {
				return nil, err
			}
		}

		kubeConfig, err = config.GetConfig()
		if err != nil {
			return nil, err
		}
	}

	s := scheme.Scheme
	err = apis.AddToScheme(s)
	if err != nil {
		return nil, err
	}

	mapper, err := apiutil.NewDiscoveryRESTMapper(kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := client.New(kubeConfig, client.Options{Scheme: s, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	p.Client = kubeClient

	return &spi.ConfigureResponse{}, nil
}

func (p *Plugin) GetPluginInfo(ctx context.Context, req *spi.GetPluginInfoRequest) (*spi.GetPluginInfoResponse, error) {
	return &spi.GetPluginInfoResponse{}, nil
}

func (p *Plugin) CreateBundle(ctx context.Context, req *datastore.CreateBundleRequest) (*datastore.CreateBundleResponse, error) {
	b := v1alpha1.NewBundle(req.Bundle)
	b.Namespace = p.config.Namespace

	err := p.Create(ctx, b)
	if err != nil {
		return nil, err
	}

	bundle, err := b.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.CreateBundleResponse{
		Bundle: bundle,
	}, nil
}

func (p *Plugin) UpdateBundle(ctx context.Context, req *datastore.UpdateBundleRequest) (*datastore.UpdateBundleResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.Bundle.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	bs := v1alpha1.NewBundleSpec(req.Bundle)
	instance.Spec = *bs

	err = p.Update(ctx, &instance)
	if err != nil {
		return nil, err
	}

	return &datastore.UpdateBundleResponse{
		Bundle: req.Bundle,
	}, nil
}

func (p *Plugin) AppendBundle(ctx context.Context, req *datastore.AppendBundleRequest) (*datastore.AppendBundleResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.Bundle.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		b := v1alpha1.NewBundle(req.Bundle)
		b.Namespace = p.config.Namespace

		err = p.Create(ctx, b)
		if err != nil {
			return nil, err
		}

		bundle, err := b.Proto()
		if err != nil {
			return nil, err
		}

		return &datastore.AppendBundleResponse{
			Bundle: bundle,
		}, nil
	}

	bundle, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	bundle, changed := bundleutil.MergeBundles(bundle, req.Bundle)
	if changed {
		bs := v1alpha1.NewBundleSpec(bundle)
		instance.Spec = *bs

		err = p.Update(ctx, &instance)
		if err != nil {
			return nil, err
		}
	}

	return &datastore.AppendBundleResponse{
		Bundle: bundle,
	}, nil
}

func (p *Plugin) DeleteBundle(ctx context.Context, req *datastore.DeleteBundleRequest) (*datastore.DeleteBundleResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	labels := v1alpha1.GetFederationLabels([]string{instance.Spec.TrustDomainID})
	entryList := v1alpha1.RegistrationEntryList{}
	err = p.List(ctx, &entryList, client.InNamespace(p.config.Namespace), client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}

	if len(entryList.Items) > 0 {
		switch req.Mode {
		case datastore.DeleteBundleRequest_DELETE:
			for _, re := range entryList.Items {
				err = p.Delete(ctx, &re)
				if err != nil {
					return nil, err
				}
			}
		case datastore.DeleteBundleRequest_DISSOCIATE:
			// Nothing to do.
		default:
			return nil, fmt.Errorf("cannot delete bundle; federated with %d registration entries", len(entryList.Items))
		}
	}

	err = p.Delete(ctx, &instance)
	if err != nil {
		return nil, err
	}

	bundle, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.DeleteBundleResponse{
		Bundle: bundle,
	}, nil
}

func (p *Plugin) FetchBundle(ctx context.Context, req *datastore.FetchBundleRequest) (*datastore.FetchBundleResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &datastore.FetchBundleResponse{}, nil
		}
		return nil, err
	}

	bundle, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.FetchBundleResponse{
		Bundle: bundle,
	}, nil
}

func (p *Plugin) ListBundles(ctx context.Context, req *datastore.ListBundlesRequest) (*datastore.ListBundlesResponse, error) {
	bundleList := v1alpha1.BundleList{}
	err := p.List(ctx, &bundleList, client.InNamespace(p.config.Namespace))
	if err != nil {
		return nil, err
	}

	res := datastore.ListBundlesResponse{}
	for i := range bundleList.Items {
		bundle, err := bundleList.Items[i].Proto()
		if err != nil {
			return nil, err
		}
		res.Bundles = append(res.Bundles, bundle)
	}

	return &res, nil
}

func (p *Plugin) CreateAttestedNode(ctx context.Context, req *datastore.CreateAttestedNodeRequest) (*datastore.CreateAttestedNodeResponse, error) {
	an := v1alpha1.NewAttestedNode(req.Node)
	an.Namespace = p.config.Namespace

	err := p.Create(ctx, an)
	if err != nil {
		return nil, err
	}

	node, err := an.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.CreateAttestedNodeResponse{
		Node: node,
	}, nil
}

func (p *Plugin) FetchAttestedNode(ctx context.Context, req *datastore.FetchAttestedNodeRequest) (*datastore.FetchAttestedNodeResponse, error) {
	instance, err := p.getAttestedNode(ctx, req.SpiffeId)
	if err != nil {
		if err == ErrNotFound {
			return &datastore.FetchAttestedNodeResponse{}, nil
		}
		return nil, err
	}

	node, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.FetchAttestedNodeResponse{
		Node: node,
	}, nil
}

func (p *Plugin) ListAttestedNodes(ctx context.Context, req *datastore.ListAttestedNodesRequest) (*datastore.ListAttestedNodesResponse, error) {
	page := req.Pagination

	nodeList := v1alpha1.AttestedNodeList{}
	err := p.List(ctx, &nodeList, client.InNamespace(p.config.Namespace))
	if err != nil {
		return nil, err
	}

	if len(nodeList.Items) == 0 {
		return &datastore.ListAttestedNodesResponse{}, nil
	}

	nodes := []v1alpha1.AttestedNode{}
	start := 0

	for _, node := range nodeList.Items {
		if req.ByExpiresBefore != nil && node.Spec.CertNotAfter >= req.ByExpiresBefore.Value {
			continue
		}

		nodes = append(nodes, node)
		if page != nil && page.Token == node.Name {
			start = len(nodes)
		}
	}

	if page != nil && page.PageSize > 0 {
		if page.Token != "" && start == 0 {
			return nil, errors.New("invalid token")
		}

		last := start + int(page.PageSize)
		if last > len(nodes) {
			last = len(nodes)
		}

		nodes = nodes[start:last]
		if len(nodes) != 0 {
			length := len(nodes)
			page.Token = nodes[length-1].Name
		}
	}

	res := datastore.ListAttestedNodesResponse{
		Nodes:      make([]*common.AttestedNode, 0, len(nodes)),
		Pagination: page,
	}

	for _, n := range nodes {
		node, err := n.Proto()
		if err != nil {
			return nil, err
		}
		res.Nodes = append(res.Nodes, node)
	}

	return &res, nil
}

func (p *Plugin) UpdateAttestedNode(ctx context.Context, req *datastore.UpdateAttestedNodeRequest) (*datastore.UpdateAttestedNodeResponse, error) {
	instance, err := p.getAttestedNode(ctx, req.SpiffeId)
	if err != nil {
		return nil, err
	}

	instance.Spec.CertSerialNumber = req.CertSerialNumber
	instance.Spec.CertNotAfter = req.CertNotAfter

	err = p.Update(ctx, instance)
	if err != nil {
		return nil, err
	}

	node, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.UpdateAttestedNodeResponse{
		Node: node,
	}, nil
}

func (p *Plugin) DeleteAttestedNode(ctx context.Context, req *datastore.DeleteAttestedNodeRequest) (*datastore.DeleteAttestedNodeResponse, error) {
	instance, err := p.getAttestedNode(ctx, req.SpiffeId)
	if err != nil {
		return nil, err
	}

	err = p.Delete(ctx, instance)
	if err != nil {
		return nil, err
	}

	node, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.DeleteAttestedNodeResponse{
		Node: node,
	}, nil
}

func (p *Plugin) getAttestedNode(ctx context.Context, spiffeID string) (*v1alpha1.AttestedNode, error) {
	nodeList := v1alpha1.AttestedNodeList{}
	labels := map[string]string{
		v1alpha1.LabelSpiffeID: v1alpha1.EncodeID(spiffeID),
	}

	err := p.List(ctx, &nodeList, client.InNamespace(p.config.Namespace), client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}

	if len(nodeList.Items) == 0 {
		return nil, ErrNotFound
	}

	return &(nodeList.Items[0]), nil
}

func (p *Plugin) SetNodeSelectors(ctx context.Context, req *datastore.SetNodeSelectorsRequest) (*datastore.SetNodeSelectorsResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.Selectors.SpiffeId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.NodeSelectors{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}

		ns := v1alpha1.NewNodeSelectors(req.Selectors)
		ns.Namespace = p.config.Namespace

		err = p.Create(ctx, ns)
		if err != nil {
			return nil, err
		}

		return &datastore.SetNodeSelectorsResponse{}, nil
	}

	nss := v1alpha1.NewNodeSelectorsSpec(req.Selectors)
	instance.Spec = *nss

	err = p.Update(ctx, &instance)
	if err != nil {
		return nil, err
	}

	return &datastore.SetNodeSelectorsResponse{}, nil
}

func (p *Plugin) GetNodeSelectors(ctx context.Context, req *datastore.GetNodeSelectorsRequest) (*datastore.GetNodeSelectorsResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.SpiffeId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.NodeSelectors{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &datastore.GetNodeSelectorsResponse{
				Selectors: &datastore.NodeSelectors{
					SpiffeId: req.SpiffeId,
				},
			}, nil
		}
		return nil, err
	}

	ns, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.GetNodeSelectorsResponse{
		Selectors: ns,
	}, nil
}

func (p *Plugin) CreateRegistrationEntry(ctx context.Context, req *datastore.CreateRegistrationEntryRequest) (*datastore.CreateRegistrationEntryResponse, error) {
	re := v1alpha1.NewRegistrationEntry(req.Entry)
	re.Namespace = p.config.Namespace

	err := p.Create(ctx, re)
	if err != nil {
		return nil, err
	}

	entry, err := re.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.CreateRegistrationEntryResponse{
		Entry: entry,
	}, nil
}

func (p *Plugin) FetchRegistrationEntry(ctx context.Context, req *datastore.FetchRegistrationEntryRequest) (*datastore.FetchRegistrationEntryResponse, error) {
	nn := types.NamespacedName{
		Name:      req.EntryId,
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.RegistrationEntry{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &datastore.FetchRegistrationEntryResponse{}, nil
		}
		return nil, err
	}

	entry, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.FetchRegistrationEntryResponse{
		Entry: entry,
	}, nil
}

func (p *Plugin) ListRegistrationEntries(ctx context.Context, req *datastore.ListRegistrationEntriesRequest) (*datastore.ListRegistrationEntriesResponse, error) {
	labels := map[string]string{}
	if req.ByParentId != nil {
		labels[v1alpha1.LabelParentID] = v1alpha1.EncodeID(req.ByParentId.Value)
	}
	if req.BySpiffeId != nil {
		labels[v1alpha1.LabelSpiffeID] = v1alpha1.EncodeID(req.BySpiffeId.Value)
	}

	entryList := v1alpha1.RegistrationEntryList{}
	err := p.List(ctx, &entryList, client.InNamespace(p.config.Namespace), client.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}

	if len(entryList.Items) == 0 {
		return &datastore.ListRegistrationEntriesResponse{}, nil
	}

	res := &datastore.ListRegistrationEntriesResponse{
		Entries:    make([]*common.RegistrationEntry, 0, len(entryList.Items)),
		Pagination: req.Pagination,
	}

	if req.BySelectors == nil || len(req.BySelectors.Selectors) == 0 {
		for _, re := range entryList.Items {
			entry, err := re.Proto()
			if err != nil {
				return nil, err
			}
			res.Entries = append(res.Entries, entry)
		}
	} else {
		// Generate selector index.
		selectorIndex := map[string]map[string]bool{}
		for _, re := range entryList.Items {
			selectorIndex[re.Name] = map[string]bool{}
			for _, selector := range re.Spec.Selectors {
				key := fmt.Sprintf("%s/%s", selector.Type, selector.Value)
				selectorIndex[re.Name][key] = true
			}
		}

		// Generate selectors list based on request.
		var selectorsList [][]*common.Selector
		selectorSet := selector.NewSetFromRaw(req.BySelectors.Selectors)
		switch req.BySelectors.Match {
		case datastore.BySelectors_MATCH_SUBSET:
			for combination := range selectorSet.Power() {
				selectorsList = append(selectorsList, combination.Raw())
			}
		case datastore.BySelectors_MATCH_EXACT:
			selectorsList = append(selectorsList, selectorSet.Raw())
		default:
			return nil, fmt.Errorf("unhandled match behavior %q", req.BySelectors.Match)
		}

		// Extract entry by using selector list and selector index.
		for _, re := range entryList.Items {
			for _, selectors := range selectorsList {
				if len(re.Spec.Selectors) != len(selectors) {
					continue
				}

				matched := true
				for _, selector := range selectors {
					key := fmt.Sprintf("%s/%s", selector.Type, selector.Value)
					_, ok := selectorIndex[re.Name][key]
					if !ok {
						matched = false
						break
					}
				}

				if matched {
					entry, err := re.Proto()
					if err != nil {
						return nil, err
					}
					res.Entries = append(res.Entries, entry)
				}
			}
		}
	}

	page := req.Pagination
	if page == nil || page.PageSize == 0 {
		return res, nil
	}

	start := 0
	for i, entry := range res.Entries {
		if page.Token == entry.EntryId {
			start = i + 1
		}
	}

	if page.Token != "" && start == 0 {
		return nil, errors.New("invalid token")
	}

	last := start + int(page.PageSize)
	if last > len(res.Entries) {
		last = len(res.Entries)
	}

	res.Entries = res.Entries[start:last]
	if len(res.Entries) != 0 {
		res.Pagination.Token = res.Entries[len(res.Entries)-1].EntryId
	}

	return res, nil
}

func (p *Plugin) UpdateRegistrationEntry(ctx context.Context, req *datastore.UpdateRegistrationEntryRequest) (*datastore.UpdateRegistrationEntryResponse, error) {
	nn := types.NamespacedName{
		Name:      req.Entry.EntryId,
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.RegistrationEntry{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	re := v1alpha1.NewRegistrationEntrySpec(req.Entry)
	instance.Spec = *re

	err = p.Update(ctx, &instance)
	if err != nil {
		return nil, err
	}

	return &datastore.UpdateRegistrationEntryResponse{
		Entry: req.Entry,
	}, nil
}

func (p *Plugin) DeleteRegistrationEntry(ctx context.Context, req *datastore.DeleteRegistrationEntryRequest) (*datastore.DeleteRegistrationEntryResponse, error) {
	nn := types.NamespacedName{
		Name:      req.EntryId,
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.RegistrationEntry{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	err = p.Delete(ctx, &instance)
	if err != nil {
		return nil, err
	}

	entry, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.DeleteRegistrationEntryResponse{
		Entry: entry,
	}, nil
}

func (p *Plugin) PruneRegistrationEntries(ctx context.Context, req *datastore.PruneRegistrationEntriesRequest) (*datastore.PruneRegistrationEntriesResponse, error) {
	entryList := v1alpha1.RegistrationEntryList{}
	err := p.List(ctx, &entryList, client.InNamespace(p.config.Namespace))
	if err != nil {
		return nil, err
	}

	for _, re := range entryList.Items {
		if re.Spec.EntryExpiry < req.ExpiresBefore {
			err = p.Delete(ctx, &re)
			if err != nil {
				return nil, err
			}
		}
	}

	return &datastore.PruneRegistrationEntriesResponse{}, nil
}

func (p *Plugin) CreateJoinToken(ctx context.Context, req *datastore.CreateJoinTokenRequest) (*datastore.CreateJoinTokenResponse, error) {
	jt := v1alpha1.NewJoinToken(req.JoinToken)
	jt.Namespace = p.config.Namespace

	err := p.Create(ctx, jt)
	if err != nil {
		return nil, err
	}

	token, err := jt.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.CreateJoinTokenResponse{
		JoinToken: token,
	}, nil
}

func (p *Plugin) FetchJoinToken(ctx context.Context, req *datastore.FetchJoinTokenRequest) (*datastore.FetchJoinTokenResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.Token),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.JoinToken{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	token, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.FetchJoinTokenResponse{
		JoinToken: token,
	}, nil
}

func (p *Plugin) DeleteJoinToken(ctx context.Context, req *datastore.DeleteJoinTokenRequest) (*datastore.DeleteJoinTokenResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.EncodeID(req.Token),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.JoinToken{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	err = p.Delete(ctx, &instance)
	if err != nil {
		return nil, err
	}

	token, err := instance.Proto()
	if err != nil {
		return nil, err
	}

	return &datastore.DeleteJoinTokenResponse{
		JoinToken: token,
	}, nil
}

func (p *Plugin) PruneJoinTokens(ctx context.Context, req *datastore.PruneJoinTokensRequest) (*datastore.PruneJoinTokensResponse, error) {
	tokenList := v1alpha1.JoinTokenList{}
	err := p.List(ctx, &tokenList, client.InNamespace(p.config.Namespace))
	if err != nil {
		return nil, err
	}

	for _, jt := range tokenList.Items {
		if jt.Spec.Expiry < req.ExpiresBefore {
			err = p.Delete(ctx, &jt)
			if err != nil {
				return nil, err
			}
		}
	}

	return &datastore.PruneJoinTokensResponse{}, nil
}
