package main

// OperationSelector identifies one OpenAPI operation by its original path and HTTP method.
// It is used for APIs where a curated, small action surface is preferable to every endpoint.
type OperationSelector struct {
	Method string
	Path   string
}

// ResourceFilterConfig defines a resource and the path patterns used to filter actions for it.
type ResourceFilterConfig struct {
	// Display metadata (output to JSON)
	DisplayName string
	Description string
	IDField     string
	NameField   string
	Icon        string

	// List endpoint configuration for resource discovery
	ListAction        string
	ListRequestConfig *RequestConfig

	// Ref bindings — maps action param names to $refs for auto-filling context action params.
	// When a context action says ref: "issue", the system finds this resource and uses these bindings.
	RefBindings map[string]string

	// ResourceKeyTemplate — $refs.x template producing a stable identifier
	// used by the trigger dispatcher to decide continue-vs-new-conversation.
	// Empty means "no continuation" (always new conversation per event).
	ResourceKeyTemplate string

	// Action filtering — actions matching these paths belong to this resource
	PathPrefixes []string // any action path starting with these prefixes
	ExactPaths   []string // any action path exactly equal to these
}
