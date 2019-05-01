package v1alpha1

import (
	"crypto/sha256"
	"fmt"

	"github.com/spiffe/spire/proto/spire/server/datastore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeSelectors struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSelectorsSpec   `json:"spec,omitempty"`
	Status NodeSelectorsStatus `json:"status,omitempty"`
}

func NewNodeSelectors(selectors *datastore.NodeSelectors) *NodeSelectors {
	ns := NewNodeSelectorsSpec(selectors)

	return &NodeSelectors{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetNodeSelectorsName(ns.SpiffeID),
		},
		Spec: *ns,
	}
}

func GetNodeSelectorsName(spiffeID string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(spiffeID)))
}

func (ns *NodeSelectors) Proto() (*datastore.NodeSelectors, error) {
	return ns.Spec.Proto()
}

type NodeSelectorsSpec struct {
	SpiffeID  string      `json:"spiffeID"`
	Selectors []*Selector `json:"selectors,omitempty"`
}

func NewNodeSelectorsSpec(selectors *datastore.NodeSelectors) *NodeSelectorsSpec {
	ns := NodeSelectorsSpec{
		SpiffeID: selectors.SpiffeId,
	}

	for _, selector := range selectors.Selectors {
		ns.Selectors = append(ns.Selectors, NewSelector(selector))
	}

	return &ns
}

func (ns *NodeSelectorsSpec) Proto() (*datastore.NodeSelectors, error) {
	selectors := datastore.NodeSelectors{
		SpiffeId: ns.SpiffeID,
	}

	for _, s := range ns.Selectors {
		selector, err := s.Proto()
		if err != nil {
			return nil, err
		}
		selectors.Selectors = append(selectors.Selectors, selector)
	}

	return &selectors, nil
}

type NodeSelectorsStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NodeSelectorsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSelectors `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeSelectors{}, &NodeSelectorsList{})
}
