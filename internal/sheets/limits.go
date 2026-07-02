package sheets

// Production guardrails (plan §2d). Enforced on every write path with typed
// LimitError values — violations are rejected, never silently truncated.
const (
	// MaxCellValueBytes caps a single JSON-encoded cell value.
	MaxCellValueBytes = 16 * 1024
	// MaxRowDataBytes caps a row's whole JSON-encoded data payload.
	MaxRowDataBytes = 128 * 1024
	// MaxFieldsPerPage caps columns per page.
	MaxFieldsPerPage = 200
	// MaxPagesPerSheet caps tabs per sheet.
	MaxPagesPerSheet = 100
	// MaxSelectOptionsPerField caps select/multi_select choices.
	MaxSelectOptionsPerField = 500
	// MaxAttachmentsPerCell caps object keys in one attachment cell.
	MaxAttachmentsPerCell = 10
	// MaxRelationLinksPerCell caps linked row IDs in one relation cell.
	MaxRelationLinksPerCell = 100
	// MaxRowsPerPage is the soft cap on rows per page.
	MaxRowsPerPage = 200_000
	// MaxRowsPerWriteMCP caps rows per rows_write MCP call.
	MaxRowsPerWriteMCP = 100
	// MaxRowsPerWriteREST caps rows per REST batch write.
	MaxRowsPerWriteREST = 500
	// QueryLimitMCP is the rows_query limit clamp for MCP callers.
	QueryLimitMCP = 100
	// QueryLimitREST is the rows query limit clamp for REST callers.
	QueryLimitREST = 200
	// DefaultQueryLimit is used when the caller does not pass a limit.
	DefaultQueryLimit = 50
	// MaxSheetsPerOrg is the soft cap on sheets per org.
	MaxSheetsPerOrg = 1000
	// MaxCSVUploadBytes caps CSV import uploads.
	MaxCSVUploadBytes = 50 * 1024 * 1024
	// MaxAttachmentFileBytes caps a single attachment file.
	MaxAttachmentFileBytes = 25 * 1024 * 1024

	// operationRetentionPerPage is the count-capped undo-log window: inside
	// the same transaction that records a new operation, older operations
	// beyond the newest N for that page are pruned.
	operationRetentionPerPage = 25

	// maxFilterConditions and maxFilterDepth bound the filter AST so a
	// hostile payload cannot compile into a pathological SQL statement.
	maxFilterConditions = 100
	maxFilterDepth      = 5
	// maxInValues bounds the size of an `in` op's value list.
	maxInValues = 100
)

// OperationRetentionPerPage exposes the undo-log window to later phases
// (handlers, MCP tools) without letting them redefine it.
const OperationRetentionPerPage = operationRetentionPerPage

// ClampLimit normalizes a caller-provided page size: non-positive becomes
// DefaultQueryLimit, and the result never exceeds max.
func ClampLimit(limit, max int) int {
	if max <= 0 {
		max = DefaultQueryLimit
	}
	if limit <= 0 {
		limit = DefaultQueryLimit
	}
	if limit > max {
		return max
	}
	return limit
}
