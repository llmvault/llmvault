package notion

import (
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// docFailure builds a per-document failure so one unreachable page does
// not abort the whole run.
func docFailure(docID, link, msg string, cause error) *interfaces.ConnectorFailure {
	return interfaces.NewDocumentFailure(docID, link, msg, cause)
}
