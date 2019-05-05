package v1alpha1

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/oklog/ulid"
	"github.com/spiffe/spire/proto/spire/common"
)

const (
	LabelSpiffeID         = "spire.summerwind.dev/spiffe-id"
	LabelParentID         = "spire.summerwind.dev/parent-id"
	LabelPrefixFederation = "federation.spire.summerwind.dev"
)

var (
	entropy *rand.Rand
)

func init() {
	t := time.Now()
	entropy = rand.New(rand.NewSource(t.UnixNano()))
}

func NewName() string {
	id := ulid.MustNew(ulid.Now(), entropy)
	return strings.ToLower(id.String())
}

func EncodeID(id string) string {
	return fmt.Sprintf("%x", sha256.Sum224([]byte(id)))
}

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
