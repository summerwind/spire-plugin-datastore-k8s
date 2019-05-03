package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/spiffe/spire/proto/spire/common"
	spi "github.com/spiffe/spire/proto/spire/common/plugin"
	"github.com/spiffe/spire/proto/spire/server/datastore"
	"github.com/spiffe/spire/test/spiretest"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	ctx       = context.Background()
	namespace = "default"
)

func TestPlugin(t *testing.T) {
	spiretest.Run(t, new(PluginSuite))
}

type PluginSuite struct {
	spiretest.Suite
	env    *envtest.Environment
	config *rest.Config
	plugin *Plugin
}

func (s *PluginSuite) SetupSuite() {
	var err error

	s.env = &envtest.Environment{
		CRDDirectoryPaths:        []string{filepath.Join("manifests", "crds")},
		ControlPlaneStartTimeout: 60 * time.Second,
	}

	kubeConfig, err = s.env.Start()
	if err != nil {
		log.Fatal(err)
	}
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
			name: "get_nodes_by_expire_before_get_only_page_third_page_no_resultds",
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
			t.Logf("Last Token: %s", lastToken)

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
