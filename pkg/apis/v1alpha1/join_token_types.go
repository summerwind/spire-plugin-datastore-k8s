package v1alpha1

import (
	"crypto/sha256"
	"fmt"

	"github.com/spiffe/spire/proto/spire/server/datastore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type JoinToken struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   JoinTokenSpec   `json:"spec,omitempty"`
	Status JoinTokenStatus `json:"status,omitempty"`
}

func NewJoinToken(token *datastore.JoinToken) *JoinToken {
	jt := NewJoinTokenSpec(token)

	return &JoinToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: GetJoinTokenName(jt.Token),
		},
		Spec: *jt,
	}
}

func GetJoinTokenName(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}

func (jt *JoinToken) Proto() (*datastore.JoinToken, error) {
	return jt.Spec.Proto()
}

type JoinTokenSpec struct {
	Token  string `json:"token"`
	Expiry int64  `json:"expiry"`
}

func NewJoinTokenSpec(token *datastore.JoinToken) *JoinTokenSpec {
	return &JoinTokenSpec{
		Token:  token.Token,
		Expiry: token.Expiry,
	}
}

func (jts *JoinTokenSpec) Proto() (*datastore.JoinToken, error) {
	token := datastore.JoinToken{
		Token:  jts.Token,
		Expiry: jts.Expiry,
	}

	return &token, nil
}

type JoinTokenStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type JoinTokenList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JoinToken `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JoinToken{}, &JoinTokenList{})
}
