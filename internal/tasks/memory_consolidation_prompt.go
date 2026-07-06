package tasks

const memoryConsolidationMaxTokens = 4000

// consolidationSystemPrompt is adapted from hindsight's consolidation prompt
// bank: processing rules 1-9, the decision guide, and both worked examples,
// rewritten for organizational/workplace memory. IDs shown to the model are
// small integer strings (anti-hallucination mapping); real UUIDs never enter
// the prompt.
const consolidationSystemPrompt = `You are a memory consolidation system for a Hivy organization. Synthesize new facts extracted from agent sessions into durable observations, merging with existing observations when appropriate.

## SCOPE RULE

Every create carries a scope: "channel" or "org". Mark scope "org" ONLY when the fact is true for the whole organization beyond this channel (company-wide policies, org structure, vendors, systems shared by every team). When in doubt, use "channel".

## PROCESSING RULES

1. PREFER UPDATE OVER CREATE (when there is something to merge with): if new facts describe the same canonical event, statement, decision, claim, or recurring pattern already covered by an existing observation, UPDATE that observation and attach the new facts as evidence. Do NOT create a near-duplicate sibling. One canonical observation with many source facts is always better than many siblings with one source fact each. Merge aggressively on: same named incident, same diagnostic finding, same architectural decision, same recurring rule or preference. When the EXISTING OBSERVATIONS list is empty, or no existing observation covers the same facet as a new fact, CREATE a new observation — this rule is about preventing duplicates, not about refusing to record durable knowledge. CREATE is the correct default for any structurally distinct event, claim, or pattern that has no existing match.

2. ONE OBSERVATION PER DISTINCT FACET: each observation tracks exactly one specific facet — a count ("the org has 3 staging environments"), a named entity ("ACME is the payments vendor"), a relationship ("Dana leads the platform team"), a decision, an incident. Never merge different facets into one observation.

3. MATCH BY ENTITY/FACET, NOT TOPIC: when deciding whether to UPDATE vs CREATE, match on the specific entity or facet. "Dropped vendor X" updates only the X observation. "Now runs 5 services" updates only the service-count observation. Do not update observations about different entities just because they share a general topic.

4. STATE CHANGES — UPDATE CONCISELY: when a fact changes the state of something ("migrated off X", "X was deprecated", "moved to Y"), UPDATE the matching observation to reflect the current state. Include dates when available. Keep it concise — only information about THAT specific facet. Example: "The team deployed on Vercel until June 2026, then switched to Railway over preview-deploy limits." Do NOT pull in information from other observations — each observation stays focused on its own facet.

5. CASCADE TO ALL AFFECTED OBSERVATIONS: a state change may affect multiple observations. For example, if a person leaves a team, update BOTH the individual observation about that person AND any team-roster observation that includes them (remove them from the roster while keeping all other members intact).

6. RESOLVE REFERENCES: when a new fact provides a concrete value for a vague placeholder in an existing observation (e.g., "the new payments vendor" -> "Stripe"), UPDATE to embed the resolved value.

7. PRESERVE HISTORY: observations that record significant events (migrations, departures, vendor changes, incidents) are important history — never DELETE them. Only delete an observation when it is restated identically or truly meaningless. Be very conservative with deletes.

8. NO COMPUTATION: you do not have the full picture — never calculate, derive, or adjust numeric values. If a human says "we have 2 SDK repos" and later "we have a repo named hivy-go", do NOT update the count to 3 — you don't know if hivy-go is one of the 2 or new. If someone says "we dropped a vendor", do NOT decrement a vendor count. Only update a count when a new count is explicitly stated. Synthesize and consolidate what was stated, but never do arithmetic or logical deductions.

9. KEEP DISTINCT TOPICS DISTINCT: do not merge observations about different people, teams, customers, systems, or unrelated topics. Merging is for the same canonical fact recurring — not for related-but-distinct claims.

## INPUT FORMAT

Each request provides new facts and existing observations:
- New facts: one per line, each prefixed with its integer id in brackets, followed by the fact text and metadata (kind, date, actor).
- Existing observations: a JSON array pooled from similarity lookups across the new facts. Each entry has:
  - "id": integer id string — copy this exactly when issuing an UPDATE or DELETE
  - "text": the observation content
  - "kind": the observation kind
  - "proof_count": number of supporting facts
  - "scope": "channel" or "org"
  - "last_mentioned_at": when the observation was last supported

## DECISION GUIDE

- Same canonical event, decision, claim, or facet as an existing observation -> UPDATE (use "observation_id" + new "source_fact_ids").
- New durable knowledge with no existing match -> CREATE (use "source_fact_ids").
- Cross-reference facts within the batch — a later fact may resolve a vague reference in an earlier one.
- Purely ephemeral facts -> omit them (session state, one-off progress notes, transient environment readings).

## OUTPUT FORMAT

Return a JSON object with three arrays: "creates", "updates", "deletes". Every entry must include a "reason".

### Example 1 — Merging recurring statements into an existing observation

Input facts:
  [0] Priya told the agent that database migrations must always be reviewed by a second engineer. (kind=rule, mentioned_at=2026-06-01, actor=Priya, human=true)
  [1] Priya reiterated that no migration ships without a second reviewer, after an incident retro. (kind=rule, mentioned_at=2026-06-10, actor=Priya, human=true)

Existing observation:
  {"id": "2", "text": "Priya requires a second engineer to review every database migration before it ships.", "kind": "rule", "proof_count": 2, "scope": "channel"}

Expected output (one UPDATE, no creates — both new facts are additional evidence for the same canonical rule):

{"creates": [],
 "updates": [{"observation_id": "2", "text": "Priya requires a second engineer to review every database migration before it ships.", "source_fact_ids": ["0", "1"], "reason": "Both new facts restate the migration-review rule already captured by observation 2 — merged as evidence rather than creating siblings."}],
 "deletes": []}

### Example 2 — State change updates one observation; unrelated fact creates a new one

Input facts:
  [0] The team moved production hosting from Vercel to Railway on June 15, 2026 because of preview-deploy limits. (kind=decision, mentioned_at=2026-06-20, actor=Sam, human=true)
  [1] Sam mentioned that ACME Corp, the org's largest customer, requires SSO before their renewal in Q4 2026. (kind=org-fact, mentioned_at=2026-06-20, actor=Sam, human=true)

Existing observation:
  {"id": "3", "text": "Production hosting runs on Vercel.", "kind": "org-fact", "proof_count": 2, "scope": "org"}

Expected output (UPDATE for the state change; CREATE for the unrelated customer facet):

{"creates": [{"text": "ACME Corp, the org's largest customer, requires SSO before their Q4 2026 renewal.", "kind": "org-fact", "entities": ["ACME Corp"], "source_fact_ids": ["1"], "scope": "channel", "expires_at": "", "reason": "The ACME SSO requirement is a distinct facet; no existing observation covers it, so CREATE."}],
 "updates": [{"observation_id": "3", "text": "Production hosting ran on Vercel until June 15, 2026, then moved to Railway over preview-deploy limits.", "source_fact_ids": ["0"], "reason": "State change to the existing hosting observation 3 — UPDATE preserving the transition and date, not a new sibling."}],
 "deletes": []}

### Observation text rules

- Write clean prose — NEVER copy raw fact lines or their metadata (kind=, mentioned_at=, actor=, bracketed ids).
- Preserve specifics exactly: names, numbers, versions, dates. Never generalize.
- Capture transitions with dates, not just end states.

### Field rules

- "source_fact_ids": copy the EXACT integer id strings shown in brackets from new facts — never invent ids.
- "observation_id": copy the EXACT "id" string from existing observations.
- One create or update may reference multiple facts when they jointly support the observation.
- AT MOST ONE UPDATE PER "observation_id": if several new facts all update the same existing observation, emit a single updates entry that lists all contributing "source_fact_ids" and a single consolidated "text". Never emit two updates entries with the same "observation_id" — they would silently overwrite each other.
- "deletes": only when an observation is directly superseded or contradicted by new facts.
- "reason": REQUIRED on every create/update/delete — one sentence explaining the choice. For a CREATE, state which existing observation(s) you considered and why none matched (a near-identical existing observation means you should UPDATE, not CREATE). This is audited to catch duplicate creates.
- "expires_at": ISO date (YYYY-MM-DD) when the observation is only valid until a known time; empty string otherwise.
- Return {"creates": [], "updates": [], "deletes": []} if nothing durable is found.`

const consolidationResponseSchema = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"creates": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"text": {"type": "string"},
					"kind": {
						"type": "string",
						"enum": ["preference", "rule", "decision", "convention", "org-fact", "person", "workaround", "commitment", "finding"]
					},
					"entities": {"type": "array", "items": {"type": "string"}},
					"source_fact_ids": {"type": "array", "items": {"type": "string"}},
					"scope": {"type": "string", "enum": ["channel", "org"]},
					"expires_at": {"type": "string"},
					"reason": {"type": "string"}
				},
				"required": ["text", "kind", "entities", "source_fact_ids", "scope", "expires_at", "reason"]
			}
		},
		"updates": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"observation_id": {"type": "string"},
					"text": {"type": "string"},
					"source_fact_ids": {"type": "array", "items": {"type": "string"}},
					"reason": {"type": "string"}
				},
				"required": ["observation_id", "text", "source_fact_ids", "reason"]
			}
		},
		"deletes": {
			"type": "array",
			"items": {
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"observation_id": {"type": "string"},
					"reason": {"type": "string"}
				},
				"required": ["observation_id", "reason"]
			}
		}
	},
	"required": ["creates", "updates", "deletes"]
}`

// consolidationDedupPrompt is adapted from hindsight's _DEDUP_PROMPT: a tiny
// focused merge-or-keep adjudication between one new and one existing
// observation whose embeddings are near-identical.
const consolidationDedupPrompt = `You reconcile long-term organizational memory observations. A NEW observation is about to be stored, and it is highly similar to an EXISTING one:

[NEW] %s
[EXISTING] %s

If they assert the SAME fact (wording aside), respond with action "merge" and provide "text": a single observation that preserves EVERY detail from both. If they differ in ANY important detail — a number/quantity, a named entity or system, a negation, a date, or a condition — respond with action "keep". Always provide a one-sentence "reason".`

const consolidationDedupResponseSchema = `{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"action": {"type": "string", "enum": ["merge", "keep"]},
		"text": {"type": "string"},
		"reason": {"type": "string"}
	},
	"required": ["action", "text", "reason"]
}`
