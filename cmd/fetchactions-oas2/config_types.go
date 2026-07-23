package main

import "encoding/json"

// OperationSelector identifies one Swagger operation by its original path and
// HTTP method. Use it when a provider should expose a small, reviewed subset of
// a larger API.
type OperationSelector struct {
	Method          string
	Path            string
	AllowDeprecated bool
}

// ActionOverride replaces generated metadata for one Swagger operation. Map
// entries are keyed by the source operationId so regeneration remains stable
// when summaries or descriptions change upstream.
type ActionOverride struct {
	Key         string
	DisplayName string
	Description string
	Access      string
	Parameters  json.RawMessage
}
