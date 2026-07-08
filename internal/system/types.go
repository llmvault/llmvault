package system

// LLMRequest is the upstream-shape request the forwarder sends. OpenAI
// chat-completions wire shape (other providers added later).
type LLMRequest struct {
	Model          string         `json:"model"`
	Messages       []LLMMessage   `json:"messages"`
	MaxTokens      int            `json:"max_tokens,omitempty"`
	Temperature    *float32       `json:"temperature,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
	ResponseFormat *responseSpec  `json:"response_format,omitempty"`
	StreamOptions  *streamOptions `json:"stream_options,omitempty"`
	Reasoning      *reasoningSpec `json:"reasoning,omitempty"`
}

type reasoningSpec struct {
	Effort string `json:"effort,omitempty"`
}

type LLMMessage struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	Parts   []LLMPart `json:"-"`
}

type LLMPart struct {
	Kind        string
	Text        string
	ContentType string
}

const (
	LLMPartText  = "text"
	LLMPartMedia = "media"
)

func JSONResponseSpec() *responseSpec {
	return &responseSpec{Type: "json_object"}
}

type responseSpec struct {
	Type string `json:"type"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Usage is the normalised token usage we surface to callers and store in the
// cache. Mirrors observe.UsageData but lives here to keep the system package
// importable from tasks without pulling observe.
type Usage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// CompletionResult is what the forwarder returns on a non-streaming call and
// what the cache stores. Both branches produce one of these at completion.
type CompletionResult struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
	Model string `json:"model"`
}
