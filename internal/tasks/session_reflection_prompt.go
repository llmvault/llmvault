package tasks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/trigger/hivy"
)

const (
	sessionReflectionMaxTokens   = 3000
	sessionReflectionMaxMemories = 5
	sessionReflectionMinScore    = 0.85
)

const sessionReflectionResponseSchema = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"memories": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"content": {"type": "string"},
					"kind": {
						"type": "string",
						"enum": ["preference", "rule", "decision", "convention", "org-fact", "person", "workaround", "commitment", "finding"]
					},
					"tags": {"type": "array", "items": {"type": "string"}},
					"confidence": {"type": "number"},
					"entities": {"type": "array", "items": {"type": "string"}},
					"expires_at": {"type": "string"},
					"source_event_ids": {"type": "array", "items": {"type": "string"}},
					"actor_display_name": {"type": "string"},
					"actor_external_ref": {"type": "string"}
				},
				"required": [
					"content",
					"kind",
					"tags",
					"confidence",
					"entities",
					"expires_at",
					"source_event_ids",
					"actor_display_name",
					"actor_external_ref"
				]
			}
		}
	},
	"required": ["memories"]
}`

type sessionReflectionResult struct {
	Memories []reflectionMemoryCandidate `json:"memories"`
}

type reflectionMemoryCandidate struct {
	Content          string   `json:"content"`
	Kind             string   `json:"kind"`
	Tags             []string `json:"tags"`
	Confidence       float64  `json:"confidence"`
	Entities         []string `json:"entities"`
	ExpiresAt        string   `json:"expires_at"`
	SourceEventIDs   []string `json:"source_event_ids"`
	ActorDisplayName string   `json:"actor_display_name"`
	ActorExternalRef string   `json:"actor_external_ref"`
}

const sessionReflectionPromptIntro = `You are a high-precision memory gate for a Hivy agent.
Your job is to retain the smallest possible set of durable facts that will materially improve
this agent's future work. False positives are much more harmful than omissions. Most transcript
windows contain no memory worth storing. Return an empty memories array by default.`

const sessionReflectionPromptRules = `A candidate is a memory only when EVERY gate below passes:

1. AUTHORITATIVE EVIDENCE
   - A Role: Human event explicitly states, corrects, approves, or confirms the fact; OR
   - for a finding/workaround only, tool evidence demonstrates the problem and a Role: Agent
     conclusion confirms the diagnosis or fix.
   - Role: Agent text is never authoritative evidence for an org fact, preference, rule,
     convention, decision, commitment, person, system, integration, permission, or capability.
2. DURABILITY
   - It is expected to remain true for at least six months, or it has a meaningful known expiry.
3. FUTURE UTILITY
   - Remembering it will change a future answer, decision, workflow, or action.
4. ORGANIZATIONAL SPECIFICITY
   - It is specific to this organization, its people, customers, projects, policies, or systems.
5. SETTLED STATE
   - It is explicit and complete, not inferred from exploration, a test, or unfinished work.
6. NON-DERIVABILITY
   - It would be costly or unreliable to rediscover when needed. Facts cheaply obtained by
     listing, searching, reading current configuration, or calling an availability endpoint
     are not memories.

HIGH-VALUE MEMORY TYPES:
- Human-stated preferences, corrections, prohibitions, and standing rules.
- Human-confirmed decisions with rationale, rejected alternatives, and effective dates.
- Stable conventions or policies the organization deliberately follows.
- Durable ownership, role, customer, vendor, or system facts explicitly confirmed by a human.
- Commitments with a concrete owner and date.
- Hard-won findings and recurring workarounds that include the symptom, verified cause,
  applicable conditions, and confirmed resolution.

ALWAYS RETURN NO MEMORY FOR:
- Anything the agent did, tried, searched, listed, fetched, read, generated, posted, tested,
  verified access to, or reported during this session.
- Agent self-narration or capability claims: "the agent can", "the agent could see",
  "the agent has access", "the agent returned", "the agent reported", or equivalent wording.
- Tool catalogs, tool names, available commands, MCP servers, skills, model capabilities,
  result-count limitations, search behavior, or runtime features.
- Inventories of Slack channels, teams, repositories, files, integrations, connections,
  credentials, providers, routes, resources, or other objects discovered by listing/searching.
- Connection IDs, channel IDs, internal UUIDs, tool-call syntax, API method names, and other
  implementation identifiers unless a human explicitly made the identifier part of a standing
  rule and future work genuinely requires it.
- Successful hello/test messages, access probes, authentication checks, smoke tests, and
  demonstrations that an integration or permission worked once.
- One-off tool or command output, including a summary or interpretation of that output.
- Filesystem listings, file contents, repository structure, machine readings, sandbox details,
  installed versions, environment capacity, and transient configuration.
- Requests, questions, task instructions, work performed, progress, plans under discussion,
  pending approvals, drafts, hypotheses, and unresolved or in-flight work.
- General knowledge, obvious facts, documentation summaries, and facts easily queried again.
- Restatements or paraphrases of Existing Memories.
- Secrets, credentials, hidden reasoning, or personal details about an end customer.
- A fact included only because it is recent, specific, or confidently phrased.

SOURCE RULES:
- Cite only event UUIDs that directly prove the memory.
- Preferences, rules, decisions, conventions, commitments, people, and org facts require at
  least one cited Role: Human event.
- Findings/workarounds without a human statement require BOTH cited tool/error evidence and a
  cited Role: Agent conclusion. Raw output alone is not a finding.
- Never treat an Actor attached to Role: Agent or Role: Tool/Event as the speaker.
- If the evidence roles are ambiguous, contradictory, or incomplete, omit the memory.

MEMORY WRITING:
- State the durable fact directly. Never write "the agent saw/reported/found/can/could".
- Use one or two information-dense sentences, normally 20–60 words, with the useful why or
  operating condition. Do not write a session recap.
- Include a date only when it explains a transition, deadline, commitment, or validity period.
  Do not prefix static facts with the session date merely because the event was timestamped.
- Resolve relative dates against Session Date. Preserve meaningful names, numbers, and versions.
- Set expires_at only when the memory has a known end date.

Before emitting each candidate, try to disqualify it using every gate and exclusion above.
If any exclusion applies or any gate is uncertain, omit it. A clean empty result is success.`

const sessionReflectionPromptOutput = `OUTPUT FIELDS:
Each memory object must include content, kind, tags, confidence, entities, expires_at, source_event_ids, actor_display_name, and actor_external_ref.
- kind: one of preference, rule, decision, convention, org-fact, person, workaround, commitment, finding.
- tags: lowercase kebab-case.
- confidence: 0.0–1.0. Memories below 0.85 are discarded. Confidence measures evidence quality
  after every gate passes; it cannot rescue a low-value or excluded fact.
- entities: people, teams, customers, systems, projects the memory mentions.
- expires_at: ISO date (YYYY-MM-DD) when the memory stops being valid, or "" when indefinite.
- source_event_ids: transcript event UUIDs (from the [event:UUID] markers) that evidence the memory.
- actor_display_name / actor_external_ref: from the transcript Actor line when the evidence is a human statement; empty strings otherwise.
Never store secrets, tokens, passwords, private keys, API keys, raw credentials, or hidden reasoning.
Return compact minified JSON with no prose.`

const sessionReflectionPromptExamples = `EXAMPLES:

Example 1 — integration exploration produces NO memories (Session Date: 2026-07-06):
Transcript (abridged):
[event:6f0a1b2c-0000-0000-0000-000000000001] Role: Human — "Check what Slack access and tools you have."
[event:6f0a1b2c-0000-0000-0000-000000000002] Role: Tool/Event — Result: Slack channel inventory — all-hive, social, engineering, qa
[event:6f0a1b2c-0000-0000-0000-000000000003] Role: Agent — "I can see four public channels and these twelve Slack tools."
[event:6f0a1b2c-0000-0000-0000-000000000004] Role: Tool/Event — Result: chat_post_message ok — hello
[event:6f0a1b2c-0000-0000-0000-000000000005] Role: Agent — "The hello message proves I can post to Slack."

Output — empty because channel/tool inventories, access probes, and agent capability reports are derivable operational state:
{"memories":[]}

Example 2 — human-confirmed decision (Session Date: 2026-07-06):
Transcript (abridged):
[event:7a1b2c3d-0000-0000-0000-000000000001] Role: Human Actor: Priya — "Decision from yesterday's infra review: preview deploys move from Fly to Railway because Fly's three-app preview limit kept blocking PR previews. Production stays on Hetzner."

Output — one memory; "yesterday" is resolved and the rationale is retained:
{"memories":[{"content":"On July 5, 2026 the team decided to move preview deploys from Fly to Railway because Fly's three-app preview limit repeatedly blocked PR previews; production remains on Hetzner.","kind":"decision","tags":["infra","preview-deploys"],"confidence":0.97,"entities":["Priya","Fly","Railway","Hetzner"],"expires_at":"","source_event_ids":["7a1b2c3d-0000-0000-0000-000000000001"],"actor_display_name":"Priya","actor_external_ref":""}]}

Example 3 — verified recurring workaround without a human statement:
Transcript (abridged):
[event:8b2c3d4e-0000-0000-0000-000000000001] Role: Tool/Event — Result: deploy error — release fails only when generated manifest exceeds 1 MiB
[event:8b2c3d4e-0000-0000-0000-000000000002] Role: Tool/Event — Result: deploy ok — pruning source maps reduces manifest to 740 KiB
[event:8b2c3d4e-0000-0000-0000-000000000003] Role: Agent — "Confirmed across both retries: manifests above 1 MiB fail; pruning source maps fixes the release."

Output:
{"memories":[{"content":"Deployments fail when the generated manifest exceeds 1 MiB; prune source maps before release to keep the manifest below that limit.","kind":"workaround","tags":["deploys","manifest"],"confidence":0.93,"entities":[],"expires_at":"","source_event_ids":["8b2c3d4e-0000-0000-0000-000000000001","8b2c3d4e-0000-0000-0000-000000000002","8b2c3d4e-0000-0000-0000-000000000003"],"actor_display_name":"","actor_external_ref":""}]}

Example 4 — routine explanation:
Transcript: a human asks what the deploy pipeline does; the agent reads current documentation and explains it. No human states or confirms a new durable fact.

Output:
{"memories":[]}`

// buildSessionReflectionSystemPrompt assembles the extraction system prompt.
// agentMission is the per-agent memory mission; when empty the
// mission section is omitted entirely.
func buildSessionReflectionSystemPrompt(agentMission string) string {
	sections := make([]string, 0, 5)
	sections = append(sections, sessionReflectionPromptIntro)
	if mission := strings.TrimSpace(agentMission); mission != "" {
		sections = append(sections,
			"FOCUS — what to retain for this agent (takes priority over the general guidelines):\n"+mission)
	}
	sections = append(sections,
		sessionReflectionPromptRules,
		sessionReflectionPromptOutput,
		sessionReflectionPromptExamples,
	)
	return strings.Join(sections, "\n\n")
}

// generateSessionReflection runs the extraction prompt and returns the kept
// candidates plus the raw model response (for the replay harness).
func generateSessionReflection(ctx context.Context, client hivy.CompletionClient, modelID string, temperature float64, transcript, existing, agentMission string) (sessionReflectionResult, string, error) {
	userPrompt := "Existing reflection memories for this session:\n" + emptyMarker(existing) +
		"\n\nNew transcript range:\n" + emptyMarker(transcript)
	reqTemperature := float32(temperature)
	req := hivy.CompletionRequest{
		Model: modelID,
		Messages: []hivy.Message{
			{Role: "system", Content: buildSessionReflectionSystemPrompt(agentMission)},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   sessionReflectionMaxTokens,
		Temperature: &reqTemperature,
		ResponseFormat: &hivy.ResponseFormat{
			Type: hivy.ResponseFormatJSONSchema,
			JSONSchema: &hivy.ResponseJSONSchema{
				Name:   "session_reflection",
				Schema: json.RawMessage(sessionReflectionResponseSchema),
				Strict: true,
			},
		},
	}
	resp, err := client.ChatCompletion(ctx, req)
	if err != nil {
		return sessionReflectionResult{}, "", err
	}
	raw := resp.Message.Content
	result, err := parseSessionReflectionResponse(raw)
	if err != nil {
		return sessionReflectionResult{}, raw, err
	}
	if len(result.Memories) > sessionReflectionMaxMemories {
		logging.FromContext(ctx).WarnContext(ctx, "session reflection exceeded memory cap; truncating",
			"returned", len(result.Memories),
			"cap", sessionReflectionMaxMemories,
		)
		result.Memories = result.Memories[:sessionReflectionMaxMemories]
	}
	return result, raw, nil
}
