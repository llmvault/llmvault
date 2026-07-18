package memory

// Agent categories select a default memory mission for catalog-installed
// agents when no explicit agent mission is configured.
const (
	AgentCategoryCustomerSupport = "customer-support"
	AgentCategoryAccount         = "account"
	AgentCategoryEngineering     = "engineering"
	AgentCategoryOperations      = "operations"
	AgentCategorySales           = "sales"
	AgentCategoryMarketing       = "marketing"
	AgentCategoryPeople          = "people"
	AgentCategoryGeneral         = "general"
)

// AgentCategories lists every supported agent category, in display order.
var AgentCategories = []string{
	AgentCategoryCustomerSupport,
	AgentCategoryAccount,
	AgentCategoryEngineering,
	AgentCategoryOperations,
	AgentCategorySales,
	AgentCategoryMarketing,
	AgentCategoryPeople,
	AgentCategoryGeneral,
}

// ValidAgentCategory reports whether category is a known agent category.
func ValidAgentCategory(category string) bool {
	for _, known := range AgentCategories {
		if category == known {
			return true
		}
	}
	return false
}

// Curated memory-mission template per category. Hand-written and versioned in
// code — this is the quality lever for what each kind of agent retains. The
// template is selected when agents.memory_mission is empty and is fully
// user-editable afterward.
// 'general' has no template: the base extraction guidelines apply unchanged.
//
// Each mission is written as direct instructions to the extraction model and
// is injected as the FOCUS section of the extraction prompt, taking priority
// over the general guidelines.
var missionTemplates = map[string]string{
	AgentCategoryCustomerSupport: "This agent handles customer support. " +
		"Do NOT retain individual customer queries, requests, or personal details — no names, contact information, account specifics, or one-off issue reports tied to a single end customer. " +
		"Recurring PATTERNS across customers are useful agent knowledge: retain a pattern once independent reports point the same way (\"password-reset emails land in spam for Outlook users\"), without naming the individuals behind it. " +
		"Retain product gaps and bugs discovered through support conversations: the symptom, the affected area, and any confirmed workaround. " +
		"Retain resolution playbooks that actually worked — the problem, the fix, and the preconditions for applying it. " +
		"Retain escalation paths, SLA rules, and support-process decisions, with who set them and when.",
	AgentCategoryAccount: "This agent is dedicated to one client, so retaining client-specific detail is appropriate and explicitly overrides the general exclusion of individual customer information. " +
		"Retain this client's stated preferences, corrections, and rules exactly as they expressed them. " +
		"Retain commitments made in BOTH directions — what was promised to the client and what the client committed to — each with its owner and date. " +
		"Retain durable facts about the client's systems, environments, and constraints: their stack, integrations, limits, and approval or procurement processes. " +
		"Retain their key contacts with roles and areas of responsibility, and update these when people change. " +
		"Do NOT retain the client's secrets or credentials, and do NOT retain personal details about the client's own end customers beyond recurring patterns.",
	AgentCategoryEngineering: "This agent works on engineering. " +
		"Retain architecture and tooling decisions together with their rationale (\"chose Railway over Fly because preview-deploy limits kept blocking PRs\"), including options that were rejected and why. " +
		"Retain code, review, and process conventions the team has agreed on: naming rules, branching and deploy flow, testing expectations. " +
		"Retain incident root causes with their fixes, and recurring gotchas in this org's systems that would cost real effort to rediscover. " +
		"Capture transitions with dates, not just end states, and preserve names, versions, and numbers exactly. " +
		"Do NOT retain machine or sandbox environment readings (disk space, RAM, installed versions), one-off command output, mid-task progress, or repository structure observations.",
	AgentCategoryOperations: "This agent works on operations. " +
		"Retain business processes and especially how and why they change: capture the transition, the date, and who decided. " +
		"Retain durable vendor and tool facts — which vendors are used for what, stated contract or pricing constraints, and switches between providers with their reasons. " +
		"Retain ownership and responsibilities: who owns which process, system, or relationship, updated when responsibilities move. " +
		"Retain deadlines, recurring schedules, and compliance obligations with all dates resolved to absolute form. " +
		"Do NOT retain one-off task chatter or in-flight work that has not settled into a process, decision, or owned responsibility.",
	AgentCategorySales: "This agent works on sales. " +
		"Retain pricing and discounting decisions with their rationale and who approved them. " +
		"Retain deal learnings and objection patterns: which arguments, materials, or concessions moved deals forward, and which objections recur by segment. " +
		"Retain durable prospect and segment insights that generalize beyond one conversation, and competitor facts learned in the field — positioning, pricing, win/loss reasons. " +
		"Retain commitments made in the sales process, by us or by the prospect, each with owner and date. " +
		"Do NOT retain speculation about a deal's mood or odds, and do NOT retain personal details about individual contacts beyond their name, role, and stated preferences relevant to the deal.",
	AgentCategoryMarketing: "This agent works on marketing. " +
		"Retain positioning and messaging decisions with their rationale, including directions that were considered and rejected. " +
		"Retain campaign outcomes as settled facts: what ran, what worked, what failed, and the numbers behind the conclusion. " +
		"Retain audience and segment learnings that will steer future campaigns, and marketing performance facts (which outreach venues convert, at what cost) with dates. " +
		"Retain brand and content conventions the team has agreed to follow: tone, terminology, visual rules. " +
		"Do NOT retain in-flight campaign chatter or draft iterations that have not produced a settled decision or measured outcome.",
	AgentCategoryPeople: "This agent works on people/HR. " +
		"Retain policies and their changes — what changed, when, and why — with relative dates resolved to absolute ones. " +
		"Retain team structure and roles: who leads what, reporting lines, and role changes over time. " +
		"Retain recurring people processes (review cycles, hiring stages, onboarding steps) and the decisions that reshape them. " +
		"Do NOT retain individual personal, medical, or compensation details under any circumstance: no salaries, health information, performance assessments, or personal circumstances of specific people. This exclusion is absolute and cannot be overridden. " +
		"Facts about a person's role and responsibilities are fine; private facts about them are not.",
}

// MissionTemplate returns the curated memory-mission template for a category,
// or "" when the category has none ('general' and unknown values).
func MissionTemplate(category string) string {
	return missionTemplates[category]
}
