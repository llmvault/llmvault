package main

const (
	artifactTypeWebPage      = "web_page"
	artifactTypePresentation = "presentation"
)

type canvasArtifactManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Project       string                `json:"project"`
	Slug          string                `json:"slug"`
	Name          string                `json:"name"`
	Type          string                `json:"type"`
	Entrypoint    string                `json:"entrypoint,omitempty"`
	Files         []canvasArtifactFile  `json:"files"`
	Slides        []canvasArtifactSlide `json:"slides,omitempty"`
	Metadata      map[string]any        `json:"metadata,omitempty"`
	CreatedAt     string                `json:"created_at,omitempty"`
	UpdatedAt     string                `json:"updated_at,omitempty"`
}

type canvasArtifactFile struct {
	Path        string `json:"path"`
	Role        string `json:"role,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type canvasArtifactSlide struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type artifactValidationResult struct {
	Valid        bool                       `json:"valid"`
	ArtifactPath string                     `json:"artifact_path"`
	ManifestPath string                     `json:"manifest_path"`
	Type         string                     `json:"type"`
	Name         string                     `json:"name"`
	Files        []string                   `json:"files"`
	HTML         []artifactHTMLReport       `json:"html,omitempty"`
	Issues       []artifactValidationIssue  `json:"issues,omitempty"`
	Guidance     []artifactValidationAdvice `json:"guidance,omitempty"`
}

type artifactHTMLReport struct {
	Path                 string `json:"path"`
	CanvasIDCount        int    `json:"canvas_id_count"`
	SemanticElementCount int    `json:"semantic_element_count"`
	TopLevelSectionCount int    `json:"top_level_section_count"`
	MaxDepth             int    `json:"max_depth"`
}

type artifactValidationIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Element  string `json:"element,omitempty"`
	NodePath string `json:"node_path,omitempty"`
	CanvasID string `json:"canvas_id,omitempty"`
	Message  string `json:"message"`
	Fix      string `json:"fix"`
}

type artifactValidationAdvice struct {
	Topic  string `json:"topic"`
	Detail string `json:"detail"`
}

type artifactSyncPayload struct {
	SessionID string                 `json:"session_id,omitempty"`
	Project   artifactProjectPayload `json:"project"`
	Artifact  artifactDetailsPayload `json:"artifact"`
	Files     []artifactFilePayload  `json:"files"`
}

type artifactProjectPayload struct {
	Ref  string `json:"ref"`
	Slug string `json:"slug,omitempty"`
	ID   string `json:"id,omitempty"`
}

type artifactDetailsPayload struct {
	Slug     string         `json:"slug"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Manifest map[string]any `json:"manifest"`
}

type artifactFilePayload struct {
	Path        string `json:"path"`
	Role        string `json:"role,omitempty"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	Content     string `json:"content"`
}
