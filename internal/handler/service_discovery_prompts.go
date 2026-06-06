package handler

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func serviceDiscoveryPrompt(provider string, conn model.Connection) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "linear":
		return linearServiceDiscoveryPrompt(conn)
	case "notion":
		return notionServiceDiscoveryPrompt(conn)
	case "railway":
		return railwayServiceDiscoveryPrompt(conn)
	case "slack":
		return slackServiceDiscoveryPrompt(conn)
	case "vercel":
		return vercelServiceDiscoveryPrompt(conn)
	default:
		return ""
	}
}

func discoveryPromptHeader(provider string, conn model.Connection) string {
	displayName := strings.TrimSpace(conn.Integration.DisplayName)
	if displayName == "" {
		displayName = provider
	}
	return fmt.Sprintf(`You are running a Hivy system-managed service discovery job for the %s integration.

Connection context:
- Provider: %s
- Display name: %s
- Connection ID: %s

Use only the Hivy-provided skill and proxy environment variables for this provider. Do not use raw credentials or external accounts directly.
Persist durable discoveries with memory_retain. Use concise, factual memories. Prefer many focused memories over one huge dump when facts are independently useful.
Tag or describe retained memories with provider="%s", source="service_discovery", and the resource type when the memory tool supports that metadata.
Before retaining new facts, recall recent memories for this provider and update, replace, or avoid duplicating stale facts.
Never retain secrets, access tokens, private credentials, raw API payloads, or full unbounded lists.
If the provider is disconnected or unavailable, retain nothing and finish with a short explanation.
`, displayName, provider, displayName, conn.ID.String(), provider)
}

func railwayServiceDiscoveryPrompt(conn model.Connection) string {
	return discoveryPromptHeader("railway", conn) + `
Railway discovery checklist:
1. Load the Railway skill instructions and use the Hivy Railway proxy.
2. Discover production-relevant Railway projects and environments. Focus on production/staging/default environments, not every historical environment.
3. For each relevant project/environment, identify service IDs, service names, domains, deployment source repository/image when visible, current deployed image/tag when visible, volumes, databases, and important environment names.
4. Infer each service purpose from its name, domains, repository, image, and adjacent database/volume relationships. Mark inference clearly as inferred.
5. Retain operational notes that would help future work, such as "API service is project X service Y in environment Z" or "production Postgres database is service ID ...".
6. Avoid destructive actions. Do not mutate services, deployments, variables, databases, volumes, domains, or environments.
7. Keep retained memories compact. Do not retain low-value deployment history or full GraphQL/REST payloads.
`
}

func vercelServiceDiscoveryPrompt(conn model.Connection) string {
	return discoveryPromptHeader("vercel", conn) + `
Vercel discovery checklist:
1. Load the Vercel skill instructions and use the Hivy Vercel proxy.
2. Discover the team/account context, production-relevant projects, project IDs, names, framework/build settings where visible, domains, and GitHub repository links.
3. For each important project, identify latest production deployment status, production domain, connected Git repository, branch, and environment target names where visible.
4. Retain concise memories that help future deploy/debug work, such as project IDs, domain-to-project mappings, repo-to-project mappings, and production deployment conventions.
5. Avoid destructive actions. Do not delete projects, deployments, domains, aliases, environment variables, teams, or integrations.
6. Do not retain full deployment logs unless a short operational note is clearly useful.
`
}

func notionServiceDiscoveryPrompt(conn model.Connection) string {
	return discoveryPromptHeader("notion", conn) + `
Notion discovery checklist:
1. Load the Notion skill instructions and use the Hivy Notion proxy.
2. Discover important top-level pages, high-signal team/project docs, and important databases. Prioritize docs/databases likely to answer business, product, operations, support, roadmap, or project questions.
3. For databases, identify database IDs, titles, core properties, relation/rollup shape where visible, and inferred purpose. Include enough schema detail for future agents to query them without rediscovery.
4. Retain concise memories for important page IDs, database IDs, schema summaries, and when to use each resource.
5. Avoid mutating pages/databases. Do not archive, delete, move, or rewrite content.
6. Do not retain large page bodies; retain resource identity, purpose, and navigation/query hints.
`
}

func linearServiceDiscoveryPrompt(conn model.Connection) string {
	return discoveryPromptHeader("linear", conn) + `
Linear discovery checklist:
1. Load the Linear skill instructions and use the Hivy Linear proxy.
2. Discover teams, team IDs, project IDs/names, workflow states, labels, cycles if relevant, and important owner/assignee/user mappings visible from active work.
3. Identify conventions that help future triage, such as which teams own product areas, which workflow states mean done/blocked/backlog, and labels used for priority or customer impact.
4. Retain concise memories for team IDs, project IDs, workflow state names/IDs, label conventions, and owner mappings that future agents can use directly.
5. Avoid mutations. Do not create, update, archive, delete, assign, or reprioritize issues during discovery.
6. Avoid retaining full issue lists; keep only durable structure and important conventions.
`
}

func slackServiceDiscoveryPrompt(conn model.Connection) string {
	return discoveryPromptHeader("slack", conn) + `
Slack discovery checklist:
1. Load the Slack skill instructions and use the Hivy Slack proxy.
2. Discover only high-signal workspace context. Do not scan or retain every channel/user.
3. Identify up to 20 hottest channels by visible activity, member count, or operational importance. For each, retain channel ID, channel name, and a short inferred purpose if clear.
4. From those high-signal channels only, identify relevant user ID/name mappings that future agents are likely to mention or need for routing. Prefer executives, admins, managers, operators, or frequent collaborators over exhaustive user lists.
5. Retain concise memories that help future Slack work, such as "engineering channel ID is C..." or "Kim's Slack user ID is U...".
6. Avoid reading private content unnecessarily. Do not retain sensitive conversation contents, full transcripts, or private messages.
7. Avoid mutations. Do not send messages, change channel membership, archive channels, or edit workspace settings during discovery.
`
}
