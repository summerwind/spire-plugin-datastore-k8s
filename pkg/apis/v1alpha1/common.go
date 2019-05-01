package v1alpha1

import "github.com/spiffe/spire/proto/spire/common"

type Selector struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func NewSelector(selector *common.Selector) *Selector {
	return &Selector{
		Type:  selector.Type,
		Value: selector.Value,
	}
}

func (s *Selector) Proto() (*common.Selector, error) {
	return &common.Selector{
		Type:  s.Type,
		Value: s.Value,
	}, nil
}
