package v1alpha1

import (
	"github.com/spiffe/spire/proto/spire/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RegistrationEntry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RegistrationEntrySpec   `json:"spec,omitempty"`
	Status RegistrationEntryStatus `json:"status,omitempty"`
}

func NewRegistrationEntry(entry *common.RegistrationEntry) *RegistrationEntry {
	re := NewRegistrationEntrySpec(entry)

	if re.EntryID == "" {
		re.EntryID = NewName()
	}

	return &RegistrationEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name: re.EntryID,
			Labels: map[string]string{
				LabelSpiffeID: EncodeID(re.SpiffeID),
				LabelParentID: EncodeID(re.ParentID),
			},
		},
		Spec: *re,
	}
}

func (re *RegistrationEntry) Proto() (*common.RegistrationEntry, error) {
	return re.Spec.Proto()
}

type RegistrationEntrySpec struct {
	EntryID       string      `json:"entryID"`
	EntryExpiry   int64       `json:"entryExpiry"`
	ParentID      string      `json:"parentID"`
	SpiffeID      string      `json:"spiffeID"`
	TTL           int32       `json:"ttl"`
	Selectors     []*Selector `json:"selectors"`
	FederatesWith []string    `json:"feredatesWith"`
	DNSNames      []string    `json:"dnsNames"`
	Admin         bool        `json:"admin"`
	Downstream    bool        `json:"downstream"`
}

func NewRegistrationEntrySpec(entry *common.RegistrationEntry) *RegistrationEntrySpec {
	re := RegistrationEntrySpec{
		ParentID:      entry.ParentId,
		SpiffeID:      entry.SpiffeId,
		EntryID:       entry.EntryId,
		EntryExpiry:   entry.EntryExpiry,
		TTL:           entry.Ttl,
		FederatesWith: entry.FederatesWith,
		DNSNames:      entry.DnsNames,
		Admin:         entry.Admin,
		Downstream:    entry.Downstream,
	}

	for _, selector := range entry.Selectors {
		re.Selectors = append(re.Selectors, NewSelector(selector))
	}

	return &re
}

func (re *RegistrationEntrySpec) Proto() (*common.RegistrationEntry, error) {
	entry := common.RegistrationEntry{
		ParentId:      re.ParentID,
		SpiffeId:      re.SpiffeID,
		EntryId:       re.EntryID,
		EntryExpiry:   re.EntryExpiry,
		Ttl:           re.TTL,
		FederatesWith: re.FederatesWith,
		DnsNames:      re.DNSNames,
		Admin:         re.Admin,
		Downstream:    re.Downstream,
	}

	for _, s := range re.Selectors {
		selector, err := s.Proto()
		if err != nil {
			return nil, err
		}
		entry.Selectors = append(entry.Selectors, selector)
	}

	return &entry, nil
}

type RegistrationEntryStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type RegistrationEntryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RegistrationEntry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RegistrationEntry{}, &RegistrationEntryList{})
}
