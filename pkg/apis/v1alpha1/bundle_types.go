package v1alpha1

import (
	"encoding/base64"

	"github.com/spiffe/spire/proto/spire/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleSpec   `json:"spec,omitempty"`
	Status BundleStatus `json:"status,omitempty"`
}

func NewBundle(bundle *common.Bundle) *Bundle {
	bs := NewBundleSpec(bundle)

	return &Bundle{
		ObjectMeta: metav1.ObjectMeta{
			Name: EncodeID(bs.TrustDomainID),
		},
		Spec: *bs,
	}
}

func (b *Bundle) Proto() (*common.Bundle, error) {
	return b.Spec.Proto()
}

type BundleSpec struct {
	TrustDomainID  string         `json:"trustDomainID,omitempty"`
	RootCAs        []*Certificate `json:"rootCAs,omitempty"`
	JWTSigningKeys []*PublicKey   `json:"jwtSigningKeys"`
}

func NewBundleSpec(bundle *common.Bundle) *BundleSpec {
	bs := BundleSpec{
		TrustDomainID:  bundle.TrustDomainId,
		RootCAs:        []*Certificate{},
		JWTSigningKeys: []*PublicKey{},
	}

	for _, cert := range bundle.RootCas {
		bs.RootCAs = append(bs.RootCAs, NewCertificate(cert))
	}

	for _, key := range bundle.JwtSigningKeys {
		bs.JWTSigningKeys = append(bs.JWTSigningKeys, NewPublicKey(key))
	}

	return &bs
}

func (bs *BundleSpec) Proto() (*common.Bundle, error) {
	bundle := common.Bundle{
		TrustDomainId:  bs.TrustDomainID,
		RootCas:        []*common.Certificate{},
		JwtSigningKeys: []*common.PublicKey{},
	}

	for _, cert := range bs.RootCAs {
		rootCA, err := cert.Proto()
		if err != nil {
			return nil, err
		}
		bundle.RootCas = append(bundle.RootCas, rootCA)
	}

	for _, pk := range bs.JWTSigningKeys {
		key, err := pk.Proto()
		if err != nil {
			return nil, err
		}
		bundle.JwtSigningKeys = append(bundle.JwtSigningKeys, key)
	}

	return &bundle, nil
}

type Certificate struct {
	DER string `json:"der"`
}

func NewCertificate(cert *common.Certificate) *Certificate {
	return &Certificate{
		DER: base64.StdEncoding.EncodeToString(cert.DerBytes),
	}
}

func (c *Certificate) Proto() (*common.Certificate, error) {
	buf, err := base64.StdEncoding.DecodeString(c.DER)
	if err != nil {
		return nil, err
	}

	return &common.Certificate{
		DerBytes: buf,
	}, nil
}

type PublicKey struct {
	PKIX     string `json:"pkix"`
	Kid      string `json:"kid"`
	NotAfter int64  `json:"notAfter"`
}

func NewPublicKey(key *common.PublicKey) *PublicKey {
	return &PublicKey{
		PKIX:     base64.StdEncoding.EncodeToString(key.PkixBytes),
		Kid:      key.Kid,
		NotAfter: key.NotAfter,
	}
}

func (pk *PublicKey) Proto() (*common.PublicKey, error) {
	buf, err := base64.StdEncoding.DecodeString(pk.PKIX)
	if err != nil {
		return nil, err
	}

	return &common.PublicKey{
		PkixBytes: buf,
		Kid:       pk.Kid,
		NotAfter:  pk.NotAfter,
	}, nil
}

type BundleStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bundle{}, &BundleList{})
}
