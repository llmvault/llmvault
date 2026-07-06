package scheduler

import (
	"encoding/json"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

type CapabilityCheck func(kind string) bool

func HasSlimCapability(kind string) bool {
	factory, err := interfaces.Lookup(kind)
	if err != nil {
		return false
	}
	c, err := factory(nilSource{kind: kind}, interfaces.BuildDeps{})
	if err != nil || c == nil {
		return false
	}
	_, ok := c.(interfaces.SlimConnector)
	return ok
}

type nilSource struct{ kind string }

func (s nilSource) SourceID() string        { return "" }
func (s nilSource) OrgID() string           { return "" }
func (s nilSource) SourceKind() string      { return s.kind }
func (s nilSource) Config() json.RawMessage { return json.RawMessage(`{}`) }
