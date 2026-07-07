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
	sessionReflectionMaxMemories = 10
	sessionReflectionMinScore    = 0.7
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

const sessionReflectionPromptIntro = `You extract durable organizational memories from a Hivy agent session.
Be SELECTIVE — most sessions contain 0–5 memories worth keeping. Extract only
what a teammate joining this channel would still need to know weeks from now.`

const sessionReflectionPromptRules = `ONLY extract:
✅ Stated preferences, corrections, and rules from humans ("always X", "stop doing Y", "we prefer Z")
✅ Decisions WITH their rationale ("chose Railway over Fly because ...")
✅ Durable project/org conventions (naming, process, review rules, tooling choices)
✅ Facts about the organization: people and roles, customers, vendors, systems, integrations
✅ Recurring workarounds for real problems (include the problem and the fix)
✅ Commitments and plans with owners and dates
✅ Hard-won findings that would cost real effort to rediscover (root causes, gotchas in THIS org's systems)

DO NOT extract:
❌ Filesystem/directory listings, file contents, repo structure observations
❌ Machine/environment readings: disk space, RAM, CPU, installed versions, sandbox specs
❌ One-off command output or its interpretation
❌ Mid-task state and progress ("user is setting up X", "pending approval")
❌ In-flight work: this session may STILL BE RUNNING. Extract only SETTLED facts —
   things a human stated, decisions explicitly made, findings confirmed. If ongoing
   work could still change the fact, it is not settled; it will be re-offered later.
❌ What the agent did this session, unless it established a durable convention or decision
❌ Anything true only inside this session's sandbox or that expires when the session ends
❌ Restatements of anything in the Existing Memories list
❌ Secrets, tokens, credentials, hidden reasoning
❌ Individual end-customer queries, requests, or personal details (names, contact info,
   account specifics of the org's customers' end users). Recurring PATTERNS across
   customers ARE channel knowledge ("password-reset emails land in spam for Outlook
   users") — individual interactions are not. The channel mission may explicitly
   override this for dedicated account channels.

LITMUS TEST: "Will this still be true and useful to someone in this channel in 6 months?"
If unsure → omit. A missed triviality costs nothing; stored noise pollutes every future prompt.

MEMORY QUALITY:
- Contextually rich, not atomic: 1–2 sentences (15–60 words) with who/what/why.
  Capture transitions ("switched from Vercel to Railway in June 2026 after preview-deploy
  limits"), not just end states.
- Preserve specifics exactly: names, ids, numbers, versions, titles. Never generalize.
- Resolve ALL relative dates against the Session Date to absolute dates
  ("yesterday" → "on July 5, 2026"). Never write relative time into a memory.
- Resolve coreferences: "the client" + "ACME" → "ACME (client)".
- entities: list people, teams, customers, systems, projects mentioned (for linking).
- If a memory is only valid until a known time, set expires_at (ISO date).

Return strict JSON: {"memories":[...]}. Empty array is a GOOD result for routine windows.`

const sessionReflectionPromptOutput = `OUTPUT FIELDS:
Each memory object must include content, kind, tags, confidence, entities, expires_at, source_event_ids, actor_display_name, and actor_external_ref.
- kind: one of preference, rule, decision, convention, org-fact, person, workaround, commitment, finding.
- tags: lowercase kebab-case.
- confidence: 0.0–1.0. Memories below 0.7 are discarded, so only emit what you are confident is settled.
- entities: people, teams, customers, systems, projects the memory mentions.
- expires_at: ISO date (YYYY-MM-DD) when the memory stops being valid, or "" when indefinite.
- source_event_ids: transcript event UUIDs (from the [event:UUID] markers) that evidence the memory.
- actor_display_name / actor_external_ref: from the transcript Actor line when the evidence is a human statement; empty strings otherwise.
Never store secrets, tokens, passwords, private keys, API keys, raw credentials, or hidden reasoning.
Return compact minified JSON with no prose.`

const sessionReflectionPromptExamples = `EXAMPLES:

Example 1 — noisy exploration session (Session Date: 2026-07-06, Channel: engineering):
Transcript (abridged):
[event:6f0a1b2c-0000-0000-0000-000000000001] Actor: Dana — "Set up Playwright e2e tests for the dashboard repo."
[event:6f0a1b2c-0000-0000-0000-000000000002] Result: bash ok — npm install: added 312 packages in 41s; disk 3.9 GB free
[event:6f0a1b2c-0000-0000-0000-000000000003] Result: bash error — chromium launch failed: dbus connection refused; /dev/shm size 64M too small
[event:6f0a1b2c-0000-0000-0000-000000000004] Agent — "Fixed: Chromium only launches in our sandboxes with --disable-dev-shm-usage; tests pass now."
[event:6f0a1b2c-0000-0000-0000-000000000005] Actor: Dana — "Nice. Going forward keep test artifacts out of git — add test-results/ to .gitignore in every repo."

Output — ONLY 2 memories. Explicitly skipped: the 3.9 GB disk reading (environment reading), the dbus warning and npm install output (one-off command output), "setting up Playwright" (mid-task state):
{"memories":[{"content":"Playwright/Chromium in Hivy sandboxes must launch with --disable-dev-shm-usage because /dev/shm (64M) is too small; without the flag browser startup fails.","kind":"workaround","tags":["playwright","chromium","sandbox"],"confidence":0.85,"entities":["Playwright","Chromium","Hivy sandbox"],"expires_at":"","source_event_ids":["6f0a1b2c-0000-0000-0000-000000000003","6f0a1b2c-0000-0000-0000-000000000004"],"actor_display_name":"","actor_external_ref":""},{"content":"Dana's rule (stated July 6, 2026): keep test artifacts out of git — add test-results/ to .gitignore in every repo.","kind":"rule","tags":["git","testing","conventions"],"confidence":0.95,"entities":["Dana"],"expires_at":"","source_event_ids":["6f0a1b2c-0000-0000-0000-000000000005"],"actor_display_name":"Dana","actor_external_ref":""}]}

Example 2 — decision-rich session (Session Date: 2026-07-06, Channel: operations):
Transcript (abridged):
[event:7a1b2c3d-0000-0000-0000-000000000001] Actor: Priya — "Decision from yesterday's infra review: preview deploys move from Fly to Railway — Fly's 3-app preview limit kept blocking PR previews. Prod stays on Hetzner."
[event:7a1b2c3d-0000-0000-0000-000000000002] Actor: Priya — "Also FYI invoicing goes through Paystack now, not Stripe."

Output — 2 memories ("yesterday" resolved against the Session Date):
{"memories":[{"content":"On July 5, 2026 the team decided to move preview deploys from Fly to Railway because Fly's 3-app preview limit kept blocking PR previews; production stays on Hetzner.","kind":"decision","tags":["infra","deploys","railway"],"confidence":0.95,"entities":["Priya","Fly","Railway","Hetzner"],"expires_at":"","source_event_ids":["7a1b2c3d-0000-0000-0000-000000000001"],"actor_display_name":"Priya","actor_external_ref":""},{"content":"Invoicing runs through Paystack, switched from Stripe (noted by Priya on July 6, 2026).","kind":"org-fact","tags":["billing","vendors"],"confidence":0.9,"entities":["Paystack","Stripe","Priya"],"expires_at":"","source_event_ids":["7a1b2c3d-0000-0000-0000-000000000002"],"actor_display_name":"Priya","actor_external_ref":""}]}

Example 3 — routine window (Session Date: 2026-07-06):
Transcript: the human asks what the deploy pipeline does; the agent reads existing docs and explains. Nothing new was stated, decided, or confirmed.

Output:
{"memories":[]}`

// buildSessionReflectionSystemPrompt assembles the extraction system prompt.
// channelMission is the per-channel memory mission (Phase 3); when empty the
// mission section is omitted entirely.
func buildSessionReflectionSystemPrompt(channelMission string) string {
	sections := make([]string, 0, 5)
	sections = append(sections, sessionReflectionPromptIntro)
	if mission := strings.TrimSpace(channelMission); mission != "" {
		sections = append(sections,
			"FOCUS — what to retain for this channel (takes priority over the general guidelines):\n"+mission)
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
func generateSessionReflection(ctx context.Context, client hivy.CompletionClient, modelID string, temperature float64, transcript, existing, channelMission string) (sessionReflectionResult, string, error) {
	userPrompt := "Existing reflection memories for this session:\n" + emptyMarker(existing) +
		"\n\nNew transcript range:\n" + emptyMarker(transcript)
	reqTemperature := float32(temperature)
	req := hivy.CompletionRequest{
		Model: modelID,
		Messages: []hivy.Message{
			{Role: "system", Content: buildSessionReflectionSystemPrompt(channelMission)},
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
