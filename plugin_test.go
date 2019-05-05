package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/spiffe/spire/pkg/common/bundleutil"
	"github.com/spiffe/spire/pkg/common/util"
	"github.com/spiffe/spire/proto/spire/common"
	spi "github.com/spiffe/spire/proto/spire/common/plugin"
	"github.com/spiffe/spire/proto/spire/server/datastore"
	"github.com/spiffe/spire/test/spiretest"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/summerwind/spire-plugin-datastore-k8s/pkg/apis/v1alpha1"
)

var (
	ctx       = context.Background()
	namespace = "default"

	svidPath    = "test/fixture/certs/svid.pem"
	svidKeyPath = "test/fixture/certs/svid_key.pem"
	caPath      = "test/fixture/certs/ca.pem"
	caKeyPath   = "test/fixture/certs/ca_key.pem"
	bundlePath  = "test/fixture/certs/bundle.der"
)

func TestPlugin(t *testing.T) {
	spiretest.Run(t, new(PluginSuite))
}

type PluginSuite struct {
	spiretest.Suite
	cert   *x509.Certificate
	env    *envtest.Environment
	config *rest.Config
	plugin *Plugin
}

func (s *PluginSuite) SetupSuite() {
	var err error

	s.cert, _, err = loadSVIDFixture()
	s.Require().NoError(err)

	s.env = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("manifests", "crds")},
		ControlPlaneStartTimeout: 60 * time.Second,
	}

	kubeConfig, err = s.env.Start()
	s.Require().NoError(err)
}

func (s *PluginSuite) TearDownSuite() {
	s.env.Stop()
}

func (s *PluginSuite) SetupTest() {
	p := New()

	_, err := p.Configure(ctx, &spi.ConfigureRequest{
		Configuration: fmt.Sprintf(`namespace = "%s"`, namespace),
	})
	s.Require().NoError(err)

	s.plugin = p
}

func (s *PluginSuite) TearDownTest() {
	var err error

	bundleList := v1alpha1.BundleList{}
	err = s.plugin.List(ctx, &bundleList, client.InNamespace(namespace))
	s.Require().NoError(err)

	for _, bundle := range bundleList.Items {
		err = s.plugin.Delete(ctx, &bundle)
		s.Require().NoError(err)
	}

	entryList := v1alpha1.RegistrationEntryList{}
	err = s.plugin.List(ctx, &entryList, client.InNamespace(namespace))
	s.Require().NoError(err)

	for _, entry := range entryList.Items {
		err = s.plugin.Delete(ctx, &entry)
		s.Require().NoError(err)
	}

	nodeList := v1alpha1.AttestedNodeList{}
	err = s.plugin.List(ctx, &nodeList, client.InNamespace(namespace))
	s.Require().NoError(err)

	for _, node := range nodeList.Items {
		err = s.plugin.Delete(ctx, &node)
		s.Require().NoError(err)
	}
}

func (s *PluginSuite) TestDeleteBundleRestrictedByRegistrationEntries() {
	var err error

	// Create a bundle
	_, err = s.plugin.CreateBundle(ctx, &datastore.CreateBundleRequest{
		Bundle: bundleutil.BundleProtoFromRootCA("spiffe://otherdomain.org", s.cert),
	})
	s.Require().NoError(err)

	// Create an associated entry
	_, err = s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{
		Entry: &common.RegistrationEntry{
			Selectors: []*common.Selector{
				{Type: "Type1", Value: "Value1"},
			},
			SpiffeId:      "spiffe://example.org/foo",
			FederatesWith: []string{"spiffe://otherdomain.org"},
		},
	})
	s.Require().NoError(err)

	// Delete the bundle in RESTRICTED mode.
	_, err = s.plugin.DeleteBundle(context.Background(), &datastore.DeleteBundleRequest{
		TrustDomainId: "spiffe://otherdomain.org",
	})
	s.RequireErrorContains(err, "cannot delete bundle; federated with 1 registration entries")
}

func (s *PluginSuite) TestDeleteBundleDeleteRegistrationEntries() {
	var err error

	// Create a bundle
	_, err = s.plugin.CreateBundle(ctx, &datastore.CreateBundleRequest{
		Bundle: bundleutil.BundleProtoFromRootCA("spiffe://otherdomain.org", s.cert),
	})
	s.Require().NoError(err)

	// Create an associated entry
	entry, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{
		Entry: &common.RegistrationEntry{
			Selectors: []*common.Selector{
				{Type: "Type1", Value: "Value1"},
			},
			SpiffeId:      "spiffe://example.org/foo",
			FederatesWith: []string{"spiffe://otherdomain.org"},
		},
	})
	s.Require().NoError(err)

	// Create an unrelated registration entry to make sure the delete
	// operation only deletes associated registration entries.
	unrelated, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{
		Entry: &common.RegistrationEntry{
			Selectors: []*common.Selector{
				{Type: "Type2", Value: "Value2"},
			},
			SpiffeId: "spiffe://example.org/foo",
		},
	})
	s.Require().NoError(err)

	// delete the bundle in DELETE mode
	_, err = s.plugin.DeleteBundle(context.Background(), &datastore.DeleteBundleRequest{
		TrustDomainId: "spiffe://otherdomain.org",
		Mode:          datastore.DeleteBundleRequest_DELETE,
	})
	s.Require().NoError(err)

	// verify that the registeration entry has been deleted
	res, err := s.plugin.FetchRegistrationEntry(context.Background(), &datastore.FetchRegistrationEntryRequest{
		EntryId: entry.Entry.EntryId,
	})
	s.Require().NoError(err)
	s.Require().Nil(res.Entry)

	// make sure the unrelated entry still exists
	res, err = s.plugin.FetchRegistrationEntry(ctx, &datastore.FetchRegistrationEntryRequest{
		EntryId: unrelated.Entry.EntryId,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res.Entry)
}

func (s *PluginSuite) TestDeleteBundleDissociateRegistrationEntries() {
	var err error

	// Create a bundle
	_, err = s.plugin.CreateBundle(ctx, &datastore.CreateBundleRequest{
		Bundle: bundleutil.BundleProtoFromRootCA("spiffe://otherdomain.org", s.cert),
	})
	s.Require().NoError(err)

	// Create an associated entry
	entry, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{
		Entry: &common.RegistrationEntry{
			Selectors: []*common.Selector{
				{Type: "Type1", Value: "Value1"},
			},
			SpiffeId:      "spiffe://example.org/foo",
			FederatesWith: []string{"spiffe://otherdomain.org"},
		},
	})
	s.Require().NoError(err)

	// delete the bundle in DISSOCIATE mode
	_, err = s.plugin.DeleteBundle(context.Background(), &datastore.DeleteBundleRequest{
		TrustDomainId: "spiffe://otherdomain.org",
		Mode:          datastore.DeleteBundleRequest_DISSOCIATE,
	})
	s.Require().NoError(err)

	// make sure the entry still exists, albeit without an associated bundle
	res, err := s.plugin.FetchRegistrationEntry(ctx, &datastore.FetchRegistrationEntryRequest{
		EntryId: entry.Entry.EntryId,
	})
	s.Require().NoError(err)
	s.Require().NotNil(res.Entry)
}

func (s *PluginSuite) TestFetchAttestedNodesWithPagination() {
	// Create all necessary nodes
	aNode1 := &common.AttestedNode{
		SpiffeId:            "node1",
		AttestationDataType: "aws-tag",
		CertSerialNumber:    "badcafe",
		CertNotAfter:        time.Now().Add(-time.Hour).Unix(),
	}

	aNode2 := &common.AttestedNode{
		SpiffeId:            "node2",
		AttestationDataType: "aws-tag",
		CertSerialNumber:    "deadbeef",
		CertNotAfter:        time.Now().Add(time.Hour).Unix(),
	}

	aNode3 := &common.AttestedNode{
		SpiffeId:            "node3",
		AttestationDataType: "aws-tag",
		CertSerialNumber:    "badcafe",
		CertNotAfter:        time.Now().Add(-time.Hour).Unix(),
	}

	aNode4 := &common.AttestedNode{
		SpiffeId:            "node4",
		AttestationDataType: "aws-tag",
		CertSerialNumber:    "badcafe",
		CertNotAfter:        time.Now().Add(-time.Hour).Unix(),
	}

	_, err := s.plugin.CreateAttestedNode(ctx, &datastore.CreateAttestedNodeRequest{Node: aNode1})
	s.Require().NoError(err)
	_, err = s.plugin.CreateAttestedNode(ctx, &datastore.CreateAttestedNodeRequest{Node: aNode2})
	s.Require().NoError(err)
	_, err = s.plugin.CreateAttestedNode(ctx, &datastore.CreateAttestedNodeRequest{Node: aNode3})
	s.Require().NoError(err)
	_, err = s.plugin.CreateAttestedNode(ctx, &datastore.CreateAttestedNodeRequest{Node: aNode4})
	s.Require().NoError(err)

	tests := []struct {
		name               string
		pagination         *datastore.Pagination
		byExpiresBefore    *wrappers.Int64Value
		useLastToken       bool
		expectedList       []*common.AttestedNode
		expectedPagination *datastore.Pagination
	}{
		{
			name: "pagination_without_token",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			expectedList: []*common.AttestedNode{aNode1, aNode2},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "pagination_not_null_but_page_size_is_zero",
			pagination: &datastore.Pagination{
				PageSize: 0,
			},
			expectedList: []*common.AttestedNode{aNode1, aNode2, aNode3, aNode4},
			expectedPagination: &datastore.Pagination{
				PageSize: 0,
			},
		},
		{
			name: "get_all_nodes_first_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			expectedList: []*common.AttestedNode{aNode1, aNode2},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_all_nodes_second_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			useLastToken: true,
			expectedList: []*common.AttestedNode{aNode3, aNode4},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name:         "get_all_nodes_third_page_no_results",
			expectedList: []*common.AttestedNode{},
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			useLastToken: true,
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_nodes_by_expire_before_get_only_page_fist_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			byExpiresBefore: &wrappers.Int64Value{
				Value: time.Now().Unix(),
			},
			expectedList: []*common.AttestedNode{aNode1, aNode3},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_nodes_by_expire_before_get_only_page_second_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			byExpiresBefore: &wrappers.Int64Value{
				Value: time.Now().Unix(),
			},
			useLastToken: true,
			expectedList: []*common.AttestedNode{aNode4},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_nodes_by_expire_before_get_only_page_third_page_no_results",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			byExpiresBefore: &wrappers.Int64Value{
				Value: time.Now().Unix(),
			},
			useLastToken: true,
			expectedList: []*common.AttestedNode{},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
	}

	var lastToken string
	for _, test := range tests {
		s.T().Run(test.name, func(t *testing.T) {
			if test.useLastToken {
				test.pagination.Token = lastToken
			}

			resp, err := s.plugin.ListAttestedNodes(ctx, &datastore.ListAttestedNodesRequest{
				ByExpiresBefore: test.byExpiresBefore,
				Pagination:      test.pagination,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)

			spiretest.RequireProtoListEqual(t, test.expectedList, resp.Nodes)
			require.Equal(t, test.expectedPagination.PageSize, resp.Pagination.PageSize)

			lastToken = resp.Pagination.Token
		})
	}

	// with invalid token
	resp, err := s.plugin.ListAttestedNodes(ctx, &datastore.ListAttestedNodesRequest{
		Pagination: &datastore.Pagination{
			Token:    "invalid token",
			PageSize: 10,
		},
	})
	s.Require().Nil(resp)
	s.Require().Error(err, "invalid token")
}

func (s *PluginSuite) TestFetchRegistrationEntriesWithPagination() {
	entry1 := &common.RegistrationEntry{
		Selectors: []*common.Selector{
			{Type: "Type1", Value: "Value1"},
			{Type: "Type2", Value: "Value2"},
			{Type: "Type3", Value: "Value3"},
		},
		SpiffeId: "spiffe://example.org/foo",
		ParentId: "spiffe://example.org/bar",
		Ttl:      1,
	}

	entry2 := &common.RegistrationEntry{
		Selectors: []*common.Selector{
			{Type: "Type3", Value: "Value3"},
			{Type: "Type4", Value: "Value4"},
			{Type: "Type5", Value: "Value5"},
		},
		SpiffeId: "spiffe://example.org/baz",
		ParentId: "spiffe://example.org/bat",
		Ttl:      2,
	}

	entry3 := &common.RegistrationEntry{
		Selectors: []*common.Selector{
			{Type: "Type1", Value: "Value1"},
			{Type: "Type2", Value: "Value2"},
			{Type: "Type3", Value: "Value3"},
		},
		SpiffeId: "spiffe://example.org/tez",
		ParentId: "spiffe://example.org/taz",
		Ttl:      2,
	}

	selectors := []*common.Selector{
		{Type: "Type1", Value: "Value1"},
		{Type: "Type2", Value: "Value2"},
		{Type: "Type3", Value: "Value3"},
	}

	res, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry1})
	s.Require().NoError(err)
	entry1 = res.Entry
	res, err = s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry2})
	s.Require().NoError(err)
	entry2 = res.Entry
	res, err = s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry3})
	s.Require().NoError(err)
	entry3 = res.Entry

	tests := []struct {
		name               string
		pagination         *datastore.Pagination
		selectors          []*common.Selector
		useLastToken       bool
		expectedList       []*common.RegistrationEntry
		expectedPagination *datastore.Pagination
	}{
		{
			name: "pagination_without_token",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			expectedList: []*common.RegistrationEntry{entry1, entry2},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "pagination_not_null_but_page_size_is_zero",
			pagination: &datastore.Pagination{
				PageSize: 0,
			},
			expectedList: []*common.RegistrationEntry{entry1, entry2, entry3},
			expectedPagination: &datastore.Pagination{
				PageSize: 0,
			},
		},
		{
			name: "get_all_entries_first_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			expectedList: []*common.RegistrationEntry{entry1, entry2},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_all_entries_second_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			useLastToken: true,
			expectedList: []*common.RegistrationEntry{entry3},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_all_entries_third_page_no_results",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			useLastToken: true,
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_entries_by_selector_get_only_page_fist_page",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			selectors:    selectors,
			expectedList: []*common.RegistrationEntry{entry1, entry3},
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_entries_by_selector_get_only_page_second_page_no_results",
			pagination: &datastore.Pagination{
				PageSize: 2,
			},
			selectors:    selectors,
			useLastToken: true,
			expectedPagination: &datastore.Pagination{
				PageSize: 2,
			},
		},
		{
			name: "get_entries_by_selector_fist_page",
			pagination: &datastore.Pagination{
				PageSize: 1,
			},
			selectors:    selectors,
			expectedList: []*common.RegistrationEntry{entry1},
			expectedPagination: &datastore.Pagination{
				PageSize: 1,
			},
		},
		{
			name: "get_entries_by_selector_second_page",
			pagination: &datastore.Pagination{
				PageSize: 1,
			},
			selectors:    selectors,
			useLastToken: true,
			expectedList: []*common.RegistrationEntry{entry3},
			expectedPagination: &datastore.Pagination{
				PageSize: 1,
			},
		},
		{
			name: "get_entries_by_selector_third_page_no_results",
			pagination: &datastore.Pagination{
				PageSize: 1,
			},
			selectors:    selectors,
			useLastToken: true,
			expectedPagination: &datastore.Pagination{
				PageSize: 1,
			},
		},
	}

	var lastToken string
	for _, test := range tests {
		s.T().Run(test.name, func(t *testing.T) {
			if test.useLastToken {
				test.pagination.Token = lastToken
			}

			resp, err := s.plugin.ListRegistrationEntries(ctx, &datastore.ListRegistrationEntriesRequest{
				BySelectors: &datastore.BySelectors{
					Selectors: test.selectors,
				},
				Pagination: test.pagination,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)

			spiretest.RequireProtoListEqual(t, test.expectedList, resp.Entries)
			require.Equal(t, test.expectedPagination.PageSize, resp.Pagination.PageSize)

			lastToken = resp.Pagination.Token
		})
	}

	// with invalid token
	resp, err := s.plugin.ListRegistrationEntries(ctx, &datastore.ListRegistrationEntriesRequest{
		Pagination: &datastore.Pagination{
			Token:    "invalid token",
			PageSize: 10,
		},
	})
	s.Require().Nil(resp)
	s.Require().Error(err, "invalid token")
}

func (s *PluginSuite) TestListParentIDEntries() {
	allEntries := getRegistrationEntries("entries.json")

	for _, entry := range allEntries {
		r, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry})
		s.Require().NoError(err)
		s.Require().NotNil(r)
		entry.EntryId = r.Entry.EntryId
	}

	tests := []struct {
		name         string
		parentID     string
		expectedList []*common.RegistrationEntry
	}{
		{
			name:         "test_parentID_found",
			parentID:     "spiffe://parent",
			expectedList: allEntries[:2],
		},
		{
			name:         "test_parentID_notfound",
			parentID:     "spiffe://imnoparent",
			expectedList: nil,
		},
	}

	for _, test := range tests {
		s.T().Run(test.name, func(t *testing.T) {
			result, err := s.plugin.ListRegistrationEntries(ctx, &datastore.ListRegistrationEntriesRequest{
				ByParentId: &wrappers.StringValue{
					Value: test.parentID,
				},
			})
			require.NoError(t, err)
			spiretest.RequireProtoListEqual(t, test.expectedList, result.Entries)
		})
	}
}

func (s *PluginSuite) TestListSelectorEntries() {
	allEntries := getRegistrationEntries("entries.json")

	for _, entry := range allEntries {
		r, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry})
		s.Require().NoError(err)
		s.Require().NotNil(r)
		entry.EntryId = r.Entry.EntryId
	}

	tests := []struct {
		name         string
		selectors    []*common.Selector
		expectedList []*common.RegistrationEntry
	}{
		{
			name: "entries_by_selector_found",
			selectors: []*common.Selector{
				{Type: "a", Value: "1"},
				{Type: "b", Value: "2"},
				{Type: "c", Value: "3"},
			},
			expectedList: []*common.RegistrationEntry{allEntries[0]},
		},
		{
			name: "entries_by_selector_not_found",
			selectors: []*common.Selector{
				{Type: "e", Value: "0"},
			},
			expectedList: nil,
		},
	}

	for _, test := range tests {
		s.T().Run(test.name, func(t *testing.T) {
			result, err := s.plugin.ListRegistrationEntries(ctx, &datastore.ListRegistrationEntriesRequest{
				BySelectors: &datastore.BySelectors{
					Selectors: test.selectors,
				},
			})
			require.NoError(t, err)
			spiretest.RequireProtoListEqual(t, test.expectedList, result.Entries)
		})
	}
}

func (s *PluginSuite) TestListMatchingEntries() {
	allEntries := getRegistrationEntries("entries.json")

	for _, entry := range allEntries {
		r, err := s.plugin.CreateRegistrationEntry(ctx, &datastore.CreateRegistrationEntryRequest{Entry: entry})
		s.Require().NoError(err)
		s.Require().NotNil(r)
		entry.EntryId = r.Entry.EntryId
	}

	tests := []struct {
		name         string
		selectors    []*common.Selector
		expectedList []*common.RegistrationEntry
	}{
		{
			name: "test1",
			selectors: []*common.Selector{
				{Type: "a", Value: "1"},
				{Type: "b", Value: "2"},
				{Type: "c", Value: "3"},
			},
			expectedList: []*common.RegistrationEntry{
				allEntries[0],
				allEntries[1],
				allEntries[2],
			},
		},
		{
			name: "test2",
			selectors: []*common.Selector{
				{Type: "d", Value: "4"},
			},
			expectedList: nil,
		},
	}

	for _, test := range tests {
		s.T().Run(test.name, func(t *testing.T) {
			result, err := s.plugin.ListRegistrationEntries(ctx, &datastore.ListRegistrationEntriesRequest{
				BySelectors: &datastore.BySelectors{
					Selectors: test.selectors,
					Match:     datastore.BySelectors_MATCH_SUBSET,
				},
			})
			s.Require().NoError(err)
			util.SortRegistrationEntries(test.expectedList)
			util.SortRegistrationEntries(result.Entries)
			s.RequireProtoListEqual(test.expectedList, result.Entries)
		})
	}
}

func LoadPEM(path string) (*pem.Block, error) {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	blk, rest := pem.Decode(dat)
	if len(rest) > 0 {
		return nil, fmt.Errorf("error decoding certificate at %s", path)
	}

	return blk, nil
}

func LoadCert(path string) (*x509.Certificate, error) {
	block, err := LoadPEM(path)
	if err != nil {
		return nil, err
	}

	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return crt, nil
}

func LoadKey(path string) (*ecdsa.PrivateKey, error) {
	block, err := LoadPEM(path)
	if err != nil {
		return nil, err
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return key, nil
}

func loadCertAndKey(crtPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	crt, err := LoadCert(crtPath)
	if err != nil {
		return crt, nil, err
	}

	key, err := LoadKey(keyPath)
	return crt, key, err
}

func loadSVIDFixture() (svid *x509.Certificate, key *ecdsa.PrivateKey, err error) {
	return loadCertAndKey(svidPath, svidKeyPath)
}

func getRegistrationEntries(fileName string) []*common.RegistrationEntry {
	entries := &common.RegistrationEntries{}
	p := path.Join("test/fixture/registration/", fileName)
	buf, _ := ioutil.ReadFile(p)
	json.Unmarshal(buf, &entries)
	return entries.Entries
}
