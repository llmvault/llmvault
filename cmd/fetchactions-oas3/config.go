package main

// ServiceConfig defines how to fetch and parse an OpenAPI spec for a service.
type ServiceConfig struct {
	Name       string // service name (maps to metadata.json key)
	SpecSource string // URL to the OpenAPI spec file
	// AdditionalSpecSources are merged into the same generated provider files.
	// This supports APIs that publish separate OpenAPI documents by domain.
	AdditionalSpecSources []string
	NangoProviders        []string // nango provider IDs that share this API surface
	PathFilters           []string // include only paths matching these prefixes (empty = all)
	PathExcludes          []string // exclude paths matching these prefixes
	TagFilters            []string // include only operations with these tags (empty = all)
	// OperationSelectors limits generation to exact method/path pairs when set.
	OperationSelectors []OperationSelector
	BasePathStrip      string            // strip this prefix from paths before output
	ExtraHeaders       map[string]string // added to every action's execution.headers
	// TagResourceMap maps OpenAPI tags to resource_type values.
	// e.g. {"Issues": "repo", "Pull Requests": "repo"}
	// Ignored when Resources is set.
	TagResourceMap map[string]string

	// Resources defines the resources for this provider and how to filter actions.
	// When set, replaces PathFilters, PathExcludes, and TagResourceMap entirely.
	// Only actions matching a resource's path patterns are included.
	Resources map[string]ResourceFilterConfig
}

// AllServices returns the full registry of OpenAPI 3.x providers.
func AllServices() []ServiceConfig {
	return []ServiceConfig{
		// --- Phase 2a: Simple, well-structured OAS 3.0 specs ---
		{
			Name:           "jira",
			SpecSource:     "https://developer.atlassian.com/cloud/jira/platform/swagger-v3.v3.json",
			NangoProviders: []string{"jira", "jira-basic", "jira-data-center", "jira-data-center-api-key", "jira-data-center-basic"},
			PathFilters:    []string{"/rest/api/3/"},
			PathExcludes:   []string{"/rest/api/3/app", "/rest/api/3/auditing", "/rest/api/3/configuration", "/rest/api/3/jql"},
		},
		{
			Name:           "confluence",
			SpecSource:     "https://developer.atlassian.com/cloud/confluence/swagger.v3.json",
			NangoProviders: []string{"confluence", "confluence-basic", "confluence-data-center"},
		},
		{
			Name:           "asana",
			SpecSource:     "https://raw.githubusercontent.com/Asana/openapi/refs/heads/master/defs/asana_oas.yaml",
			NangoProviders: []string{"asana", "asana-mcp"},
		},
		{
			Name:           "pagerduty",
			SpecSource:     "https://raw.githubusercontent.com/PagerDuty/api-schema/refs/heads/main/reference/REST/openapiv3.json",
			NangoProviders: []string{"pagerduty"},
		},
		{
			Name:           "intercom",
			SpecSource:     "https://raw.githubusercontent.com/intercom/Intercom-OpenAPI/refs/heads/main/descriptions/2.15/api.intercom.io.yaml",
			NangoProviders: []string{"intercom"},
		},
		{
			Name:           "box",
			SpecSource:     "https://raw.githubusercontent.com/box/box-openapi/refs/heads/main/openapi/openapi.json",
			NangoProviders: []string{"box"},
		},
		{
			Name:           "sentry",
			SpecSource:     "https://raw.githubusercontent.com/getsentry/sentry-api-schema/refs/heads/main/openapi-derefed.json",
			NangoProviders: []string{"sentry", "sentry-oauth"},
		},
		{
			Name:           "bugsink",
			SpecSource:     "https://sentry.usehivy.com/api/canonical/0/schema/",
			NangoProviders: []string{"bugsink"},
		},
		{
			Name:           "glitchtip",
			SpecSource:     "https://app.glitchtip.com/api/openapi.json",
			NangoProviders: []string{"glitchtip"},
		},
		{
			Name:           "zendesk",
			SpecSource:     "https://developer.zendesk.com/zendesk/oas.yaml",
			NangoProviders: []string{"zendesk"},
		},

		// --- Phase 2b: Large/complex OAS 3.0 specs ---
		{
			Name:       "hubspot",
			SpecSource: "https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Contacts/Rollouts/424/v3/contacts.json",
			AdditionalSpecSources: []string{
				"https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Companies/Rollouts/424/v3/companies.json",
				"https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Deals/Rollouts/424/v3/deals.json",
				"https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Tickets/Rollouts/424/v3/tickets.json",
				"https://raw.githubusercontent.com/HubSpot/HubSpot-public-api-spec-collection/refs/heads/main/PublicApiSpecs/CRM/Products/Rollouts/424/v3/products.json",
			},
			NangoProviders: []string{"hubspot"},
			OperationSelectors: append(
				append(
					append(
						append(hubspotCRMObjectSelectors("contacts", "contactId"), hubspotCRMObjectSelectors("companies", "companyId")...),
						hubspotCRMObjectSelectors("0-3", "dealId")...),
					hubspotCRMObjectSelectors("tickets", "ticketId")...),
				hubspotCRMObjectSelectors("products", "productId")...),
		},
		{
			Name:           "datadog",
			SpecSource:     "https://raw.githubusercontent.com/DataDog/datadog-api-client-go/refs/heads/master/.generator/schemas/v1/openapi.yaml",
			NangoProviders: []string{"datadog"},
			BasePathStrip:  "/api",
			OperationSelectors: []OperationSelector{
				{Method: "GET", Path: "/api/v1/monitor"},
				{Method: "POST", Path: "/api/v1/monitor"},
				{Method: "GET", Path: "/api/v1/monitor/{monitor_id}"},
				{Method: "PUT", Path: "/api/v1/monitor/{monitor_id}"},
				{Method: "DELETE", Path: "/api/v1/monitor/{monitor_id}"},
				{Method: "GET", Path: "/api/v1/monitor/can_delete"},
				{Method: "GET", Path: "/api/v1/monitor/groups/search"},
				{Method: "GET", Path: "/api/v1/monitor/search"},
				{Method: "POST", Path: "/api/v1/monitor/validate"},
				{Method: "GET", Path: "/api/v1/dashboard"},
				{Method: "POST", Path: "/api/v1/dashboard"},
				{Method: "GET", Path: "/api/v1/dashboard/{dashboard_id}"},
				{Method: "PUT", Path: "/api/v1/dashboard/{dashboard_id}"},
				{Method: "DELETE", Path: "/api/v1/dashboard/{dashboard_id}"},
				{Method: "GET", Path: "/api/v1/dashboard/lists/manual"},
				{Method: "POST", Path: "/api/v1/dashboard/lists/manual"},
				{Method: "GET", Path: "/api/v1/dashboard/lists/manual/{list_id}"},
				{Method: "PUT", Path: "/api/v1/dashboard/lists/manual/{list_id}"},
				{Method: "DELETE", Path: "/api/v1/dashboard/lists/manual/{list_id}"},
				{Method: "GET", Path: "/api/v1/slo"},
				{Method: "POST", Path: "/api/v1/slo"},
				{Method: "GET", Path: "/api/v1/slo/{slo_id}"},
				{Method: "PUT", Path: "/api/v1/slo/{slo_id}"},
				{Method: "DELETE", Path: "/api/v1/slo/{slo_id}"},
				{Method: "GET", Path: "/api/v1/events"},
				{Method: "GET", Path: "/api/v1/events/{event_id}"},
				{Method: "GET", Path: "/api/v1/hosts"},
				{Method: "GET", Path: "/api/v1/hosts/totals"},
				{Method: "POST", Path: "/api/v1/logs-queries/list"},
				{Method: "GET", Path: "/api/v1/logs/config/indexes"},
				{Method: "POST", Path: "/api/v1/logs/config/indexes"},
				{Method: "GET", Path: "/api/v1/logs/config/indexes/{name}"},
				{Method: "PUT", Path: "/api/v1/logs/config/indexes/{name}"},
				{Method: "DELETE", Path: "/api/v1/logs/config/indexes/{name}"},
				{Method: "GET", Path: "/api/v1/logs/config/pipelines"},
				{Method: "POST", Path: "/api/v1/logs/config/pipelines"},
				{Method: "GET", Path: "/api/v1/logs/config/pipelines/{pipeline_id}"},
				{Method: "PUT", Path: "/api/v1/logs/config/pipelines/{pipeline_id}"},
				{Method: "DELETE", Path: "/api/v1/logs/config/pipelines/{pipeline_id}"},
				{Method: "GET", Path: "/api/v1/metrics"},
				{Method: "GET", Path: "/api/v1/metrics/{metric_name}"},
				{Method: "PUT", Path: "/api/v1/metrics/{metric_name}"},
				{Method: "GET", Path: "/api/v1/notebooks"},
				{Method: "POST", Path: "/api/v1/notebooks"},
				{Method: "GET", Path: "/api/v1/notebooks/{notebook_id}"},
				{Method: "PUT", Path: "/api/v1/notebooks/{notebook_id}"},
				{Method: "DELETE", Path: "/api/v1/notebooks/{notebook_id}"},
				{Method: "GET", Path: "/api/v1/usage/billable-summary"},
				{Method: "GET", Path: "/api/v1/usage/summary"},
				{Method: "GET", Path: "/api/v1/usage/top_avg_metrics"},
			},
		},
		{
			Name:           "resend",
			SpecSource:     "https://raw.githubusercontent.com/resend/resend-openapi/refs/heads/main/resend.yaml",
			NangoProviders: []string{"resend"},
			ExtraHeaders:   map[string]string{"User-Agent": "Hivy MCP"},
			OperationSelectors: []OperationSelector{
				{Method: "POST", Path: "/emails"}, {Method: "GET", Path: "/emails"}, {Method: "GET", Path: "/emails/{email_id}"}, {Method: "PATCH", Path: "/emails/{email_id}"}, {Method: "POST", Path: "/emails/{email_id}/cancel"}, {Method: "POST", Path: "/emails/batch"}, {Method: "GET", Path: "/emails/{email_id}/attachments"}, {Method: "GET", Path: "/emails/{email_id}/attachments/{attachment_id}"}, {Method: "GET", Path: "/emails/receiving"}, {Method: "GET", Path: "/emails/receiving/{email_id}"},
				{Method: "POST", Path: "/domains"}, {Method: "GET", Path: "/domains"}, {Method: "GET", Path: "/domains/{domain_id}"}, {Method: "PATCH", Path: "/domains/{domain_id}"}, {Method: "DELETE", Path: "/domains/{domain_id}"}, {Method: "POST", Path: "/domains/{domain_id}/verify"}, {Method: "POST", Path: "/domains/claim"},
				{Method: "POST", Path: "/templates"}, {Method: "GET", Path: "/templates"}, {Method: "GET", Path: "/templates/{id}"}, {Method: "PATCH", Path: "/templates/{id}"}, {Method: "DELETE", Path: "/templates/{id}"}, {Method: "POST", Path: "/templates/{id}/publish"}, {Method: "POST", Path: "/templates/{id}/duplicate"},
				{Method: "POST", Path: "/contacts"}, {Method: "GET", Path: "/contacts"}, {Method: "GET", Path: "/contacts/{id}"}, {Method: "PATCH", Path: "/contacts/{id}"}, {Method: "DELETE", Path: "/contacts/{id}"}, {Method: "GET", Path: "/contacts/imports"}, {Method: "GET", Path: "/contacts/imports/{id}"}, {Method: "GET", Path: "/emails/receiving/{email_id}/attachments"},
				{Method: "POST", Path: "/broadcasts"}, {Method: "GET", Path: "/broadcasts"}, {Method: "GET", Path: "/broadcasts/{id}"}, {Method: "PATCH", Path: "/broadcasts/{id}"}, {Method: "DELETE", Path: "/broadcasts/{id}"}, {Method: "POST", Path: "/broadcasts/{id}/send"},
				{Method: "POST", Path: "/webhooks"}, {Method: "GET", Path: "/webhooks"}, {Method: "GET", Path: "/webhooks/{webhook_id}"}, {Method: "PATCH", Path: "/webhooks/{webhook_id}"}, {Method: "DELETE", Path: "/webhooks/{webhook_id}"},
				{Method: "POST", Path: "/segments"}, {Method: "GET", Path: "/segments"}, {Method: "GET", Path: "/segments/{id}"}, {Method: "DELETE", Path: "/segments/{id}"},
				{Method: "POST", Path: "/topics"}, {Method: "GET", Path: "/topics"}, {Method: "GET", Path: "/topics/{id}"},
			},
		},
		{
			Name:           "stripe",
			SpecSource:     "https://raw.githubusercontent.com/stripe/openapi/refs/heads/master/openapi/spec3.json",
			NangoProviders: []string{"stripe", "stripe-api-key", "stripe-app", "stripe-app-sandbox", "stripe-express"},
			PathFilters:    []string{"/v1/charges", "/v1/customers", "/v1/subscriptions", "/v1/invoices", "/v1/payment_intents", "/v1/products", "/v1/prices", "/v1/refunds", "/v1/payment_methods", "/v1/checkout", "/v1/billing_portal"},
		},
		{
			Name:           "cloudflare",
			SpecSource:     "https://raw.githubusercontent.com/cloudflare/api-schemas/refs/heads/main/openapi.json",
			NangoProviders: []string{"cloudflare"},
			PathFilters:    []string{"/zones", "/dns", "/workers"},
		},
		{
			Name:           "vercel",
			SpecSource:     "https://openapi.vercel.sh/",
			NangoProviders: []string{"vercel"},
		},
		{
			Name:           "twilio",
			SpecSource:     "https://raw.githubusercontent.com/twilio/twilio-oai/refs/heads/main/spec/json/twilio_api_v2010.json",
			NangoProviders: []string{"twilio"},
		},

		// --- Phase 2c: OAS 3.1 specs (libopenapi handles 3.1 natively) ---
		{
			Name:           "github",
			SpecSource:     "https://raw.githubusercontent.com/github/rest-api-description/refs/heads/main/descriptions/api.github.com/api.github.com.json",
			NangoProviders: []string{"github", "github-app", "github-app-oauth", "github-pat"},
			Resources:      githubResources(),
		},
		{
			Name:           "figma",
			SpecSource:     "https://raw.githubusercontent.com/figma/rest-api-spec/refs/heads/main/openapi/openapi.yaml",
			NangoProviders: []string{"figma"},
		},
		{
			Name:           "discord",
			SpecSource:     "https://raw.githubusercontent.com/discord/discord-api-spec/refs/heads/main/specs/openapi.json",
			NangoProviders: []string{"discord"},
		},
		{
			Name:           "posthog",
			SpecSource:     "https://eu.posthog.com/api/schema/",
			NangoProviders: []string{"posthog"},
			// A focused, read-only analytics surface. Keep this list explicit so
			// changes to PostHog's very large schema cannot expand it unexpectedly.
			OperationSelectors: []OperationSelector{
				{Method: "GET", Path: "/api/projects/{project_id}/dashboards/"},
				{Method: "GET", Path: "/api/projects/{project_id}/dashboards/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/dashboards/{id}/run_insights/"},
				{Method: "GET", Path: "/api/projects/{project_id}/dashboards/{id}/run_widgets/"},
				{Method: "GET", Path: "/api/projects/{project_id}/event_definitions/"},
				{Method: "GET", Path: "/api/projects/{project_id}/events/values/"},
				{Method: "GET", Path: "/api/projects/{project_id}/insights/"},
				{Method: "GET", Path: "/api/projects/{project_id}/insights/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/insights/{id}/analyze/"},
				{Method: "GET", Path: "/api/projects/{project_id}/insights/trending/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/{id}/activity/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/funnel/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/lifecycle/"},
				{Method: "GET", Path: "/api/projects/{project_id}/persons/trends/"},
				{Method: "GET", Path: "/api/projects/{project_id}/web_analytics/recap/"},
				{Method: "GET", Path: "/api/projects/{project_id}/cohorts/"},
				{Method: "GET", Path: "/api/projects/{project_id}/cohorts/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/experiments/"},
				{Method: "POST", Path: "/api/projects/{project_id}/dashboards/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/dashboards/{id}/"},
				{Method: "DELETE", Path: "/api/projects/{project_id}/dashboards/{id}/"},
				{Method: "POST", Path: "/api/projects/{project_id}/insights/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/insights/{id}/"},
				{Method: "DELETE", Path: "/api/projects/{project_id}/insights/{id}/"},
				{Method: "POST", Path: "/api/projects/{project_id}/cohorts/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/cohorts/{id}/"},
				{Method: "DELETE", Path: "/api/projects/{project_id}/cohorts/{id}/"},
				{Method: "POST", Path: "/api/projects/{project_id}/experiments/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/experiments/{id}/"},
				{Method: "DELETE", Path: "/api/projects/{project_id}/experiments/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/feature_flags/"},
				{Method: "GET", Path: "/api/projects/{project_id}/feature_flags/{id}/"},
				{Method: "POST", Path: "/api/projects/{project_id}/feature_flags/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/feature_flags/{id}/"},
				{Method: "DELETE", Path: "/api/projects/{project_id}/feature_flags/{id}/"},
				{Method: "GET", Path: "/api/projects/{project_id}/annotations/"},
				{Method: "POST", Path: "/api/projects/{project_id}/annotations/"},
				{Method: "PATCH", Path: "/api/projects/{project_id}/annotations/{id}/"},
			},
		},
	}
}

// hubspotCRMObjectSelectors returns the ten core lifecycle operations for one
// standard CRM object type. The five object types below make HubSpot's curated
// catalog cover contacts, companies, deals, tickets, and products evenly.
func hubspotCRMObjectSelectors(objectType, idParam string) []OperationSelector {
	basePath := "/crm/v3/objects/" + objectType
	return []OperationSelector{
		{Method: "GET", Path: basePath},
		{Method: "POST", Path: basePath},
		{Method: "GET", Path: basePath + "/{" + idParam + "}"},
		{Method: "PATCH", Path: basePath + "/{" + idParam + "}"},
		{Method: "DELETE", Path: basePath + "/{" + idParam + "}"},
		{Method: "POST", Path: basePath + "/search"},
		{Method: "POST", Path: basePath + "/batch/archive"},
		{Method: "POST", Path: basePath + "/batch/create"},
		{Method: "POST", Path: basePath + "/batch/read"},
		{Method: "POST", Path: basePath + "/batch/update"},
	}
}
