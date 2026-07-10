package agentruntime

const naturalExtremelyConciseProfileID = "natural_extremely_concise_v1"

type communicationProfile struct {
	ID      string
	Content string
}

// resolveCommunicationProfile centralizes user-facing writing policy. The
// model argument is intentional: profiles can diverge by model later without
// spreading model checks through prompt compilation.
func resolveCommunicationProfile(modelID string) communicationProfile {
	_ = modelID
	return communicationProfile{
		ID:      naturalExtremelyConciseProfileID,
		Content: "Use plain language. Lead with the answer. Keep replies to at most two short paragraphs unless the user asks for technical detail or another format. Skip filler.",
	}
}
