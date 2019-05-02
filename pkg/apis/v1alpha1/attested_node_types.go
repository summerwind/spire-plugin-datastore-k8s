package v1alpha1

import (
	"github.com/spiffe/spire/proto/spire/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AttestedNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AttestedNodeSpec   `json:"spec,omitempty"`
	Status AttestedNodeStatus `json:"status,omitempty"`
}

func NewAttestedNode(node *common.AttestedNode) *AttestedNode {
	an := NewAttestedNodeSpec(node)

	return &AttestedNode{
		ObjectMeta: metav1.ObjectMeta{
			Name: NewName(),
			Labels: map[string]string{
				LabelSpiffeID: EncodeID(an.SpiffeID),
			},
		},
		Spec: *an,
	}
}

func (an *AttestedNode) Proto() (*common.AttestedNode, error) {
	return an.Spec.Proto()
}

type AttestedNodeSpec struct {
	SpiffeID            string `json:"spiffeID"`
	AttestationDataType string `json:"attestationDataType"`
	CertSerialNumber    string `json:"certSerialNumber"`
	CertNotAfter        int64  `json:"certNotAfter"`
}

func NewAttestedNodeSpec(node *common.AttestedNode) *AttestedNodeSpec {
	an := AttestedNodeSpec{
		SpiffeID:            node.SpiffeId,
		AttestationDataType: node.AttestationDataType,
		CertSerialNumber:    node.CertSerialNumber,
		CertNotAfter:        node.CertNotAfter,
	}

	return &an
}

func (ans *AttestedNodeSpec) Proto() (*common.AttestedNode, error) {
	node := common.AttestedNode{
		SpiffeId:            ans.SpiffeID,
		AttestationDataType: ans.AttestationDataType,
		CertSerialNumber:    ans.CertSerialNumber,
		CertNotAfter:        ans.CertNotAfter,
	}

	return &node, nil
}

type AttestedNodeStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AttestedNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AttestedNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AttestedNode{}, &AttestedNodeList{})
}
