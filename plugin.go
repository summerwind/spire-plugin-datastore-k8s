package main

import (
	"context"

	"github.com/hashicorp/hcl"
	"github.com/spiffe/spire/pkg/common/bundleutil"
	spi "github.com/spiffe/spire/proto/spire/common/plugin"
	"github.com/spiffe/spire/proto/spire/server/datastore"
	"github.com/summerwind/spire-plugin-datastore-kubernetes/pkg/apis"
	"github.com/summerwind/spire-plugin-datastore-kubernetes/pkg/apis/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

type Plugin struct {
	client.Client
	config *PluginConfig
}

func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Configure(ctx context.Context, req *spi.ConfigureRequest) (*spi.ConfigureResponse, error) {
	c := PluginConfig{}
	err := hcl.Decode(&c, req.Configuration)
	if err != nil {
		return nil, err
	}

	err = c.Validate()
	if err != nil {
		return nil, err
	}

	kc, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	sc := scheme.Scheme
	err = apis.AddToScheme(sc)
	if err != nil {
		return nil, err
	}

	mapper, err := apiutil.NewDiscoveryRESTMapper(kc)
	if err != nil {
		return nil, err
	}

	kclient, err := client.New(kc, client.Options{Scheme: sc, Mapper: mapper})
	if err != nil {
		return nil, err
	}

	p.config = &c
	p.Client = kclient

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
		Name:      v1alpha1.GetBundleName(req.Bundle.TrustDomainId),
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
		Name:      v1alpha1.GetBundleName(req.Bundle.TrustDomainId),
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
		Name:      v1alpha1.GetBundleName(req.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	// TODO: Process mode value

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
		Name:      v1alpha1.GetBundleName(req.TrustDomainId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.Bundle{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
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
	nn := types.NamespacedName{
		Name:      v1alpha1.GetAttestedNodeName(req.SpiffeId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.AttestedNode{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
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

func (p *Plugin) ListAttestedNodes(context.Context, *datastore.ListAttestedNodesRequest) (*datastore.ListAttestedNodesResponse, error) {
	// TODO
	return &datastore.ListAttestedNodesResponse{}, nil
}

func (p *Plugin) UpdateAttestedNode(ctx context.Context, req *datastore.UpdateAttestedNodeRequest) (*datastore.UpdateAttestedNodeResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.GetAttestedNodeName(req.SpiffeId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.AttestedNode{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	instance.Spec.CertSerialNumber = req.CertSerialNumber
	instance.Spec.CertNotAfter = req.CertNotAfter

	err = p.Update(ctx, &instance)
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
	nn := types.NamespacedName{
		Name:      v1alpha1.GetAttestedNodeName(req.SpiffeId),
		Namespace: p.config.Namespace,
	}
	instance := v1alpha1.AttestedNode{}

	err := p.Get(ctx, nn, &instance)
	if err != nil {
		return nil, err
	}

	err = p.Delete(ctx, &instance)
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

func (p *Plugin) SetNodeSelectors(ctx context.Context, req *datastore.SetNodeSelectorsRequest) (*datastore.SetNodeSelectorsResponse, error) {
	nn := types.NamespacedName{
		Name:      v1alpha1.GetNodeSelectorsName(req.Selectors.SpiffeId),
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
		Name:      v1alpha1.GetNodeSelectorsName(req.SpiffeId),
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

func (p *Plugin) ListRegistrationEntries(context.Context, *datastore.ListRegistrationEntriesRequest) (*datastore.ListRegistrationEntriesResponse, error) {
	// TODO
	return &datastore.ListRegistrationEntriesResponse{}, nil
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
		Name:      v1alpha1.GetJoinTokenName(req.Token),
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
		Name:      v1alpha1.GetJoinTokenName(req.Token),
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
