package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestAgentSessionsQAAgentE2E exercises the PRODUCTION QA agent shipped in the
// catalog (global/agents/anna-qa-engineer/) against the live compose stack. Nothing
// is hand-built: the org installs the sheets + qa plugins, installs the catalog
// agent (which carries its own instructions, skills, and sub-agents), and asks
// it to author and run browser test cases against the real login page, then
// verifies the QA registry sheet the agent authored on its own.
//
// The QA turn is long-running (browser automation across multiple test cases),
// so the access token — which expires around 15 minutes — is refreshed both
// proactively (every 10 minutes) and reactively (on any 401 from the events
// API) inside a LOCAL polling loop. A prior version of this test died at
// exactly 15:00 from JWT expiry; the refresh logic below prevents that.
func TestAgentSessionsQAAgentE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 32*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-e2e-password"
	ownerEmail := "qa-agent-owner-" + runID + "@example.com"
	doneMarker := "QA_E2E_DONE_" + runID

	// --- Register a fresh owner. Keep email + password for re-login (the QA
	// turn outlives a single access token).
	t.Logf("registering owner=%s", ownerEmail)
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "QA Agent Owner "+runID)
	if len(ownerAuth.Orgs) != 1 || ownerAuth.Orgs[0].ID == "" {
		t.Fatalf("owner register did not return one org: %+v", ownerAuth.Orgs)
	}
	orgID := ownerAuth.Orgs[0].ID
	token := ownerAuth.AccessToken

	// --- Org-install the plugins the catalog agent requires. The runtime
	// plugin is auto-installed, so it is NOT installed here.
	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "sheets")
	qaAgentInstallPlugin(t, ctx, apiBase, token, orgID, "qa")
	t.Log("installed sheets + qa org plugins")

	// --- Install the catalog agent. A 409 here means the required plugins are
	// not installed; agentSessionsInstallCatalogAgent fatals with the body
	// dumped when the status is not 201.
	agent := agentSessionsInstallCatalogAgent(t, ctx, apiBase, token, orgID, "anna-playwright-qa-engineer")
	t.Logf("installed catalog agent id=%s name=%s model=%s sandbox_image=%s", agent.ID, agent.Name, agent.Model, agent.SandboxImage)
	if agent.Model != "glm-5.2" {
		t.Fatalf("installed QA agent model=%q want glm-5.2", agent.Model)
	}

	// default_reasoning_effort is exposed on the full agent response
	// (agents_response.go) but not on the catalog install list item; fetch the
	// agent to assert it.
	var agentDetail struct {
		ID                     string `json:"id"`
		Model                  string `json:"model"`
		DefaultReasoningEffort string `json:"default_reasoning_effort"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/agents/"+agent.ID, token, orgID, nil, http.StatusOK, &agentDetail)
	if agentDetail.DefaultReasoningEffort != "low" {
		t.Fatalf("installed QA agent default_reasoning_effort=%q want low", agentDetail.DefaultReasoningEffort)
	}
	t.Logf("QA agent model=%q default_reasoning_effort=%q", agentDetail.Model, agentDetail.DefaultReasoningEffort)

	// --- Credential-free channel with the installed agent as its default. No
	// channel env vars: this scenario runs against the public login page.
	channel := agentSessionsCreateChannel(t, ctx, apiBase, token, orgID, "qa-"+runID, agent.ID)
	if channel.DefaultAgentID != agent.ID {
		t.Fatalf("channel default_agent_id=%s want %s", channel.DefaultAgentID, agent.ID)
	}
	t.Logf("created channel id=%s default_agent=%s", channel.ID, channel.DefaultAgentID)

	// --- Kick off the QA session. The agent authors the test cases itself; we
	// only describe the target and the completion marker.
	message := "Test our login page at http://usehivy.com/auth/login: confirm that the Google sign-in button redirects to Google and the GitHub sign-in button redirects to GitHub. No test cases exist yet — create them, run them, and record everything in the registry. When everything is recorded, end your final reply with " + doneMarker + " and nothing after it."
	session := agentSessionsCreateSession(t, ctx, apiBase, token, orgID, channel.ID, message)
	if session.Session.ID == "" {
		t.Fatalf("session was not created correctly: %+v", session)
	}
	t.Logf("created QA session id=%s queued=%t", session.Session.ID, session.Queued)

	// --- Wait for the completion marker in a final event. Refreshes the access
	// token proactively and on 401 so the long QA turn survives JWT expiry. The
	// refreshed token is used for every subsequent assertion below.
	final := qaAgentWaitForFinalMarker(t, ctx, apiBase, &token, orgID, session.Session.ID, doneMarker, ownerEmail, password)
	t.Logf("observed QA completion marker in final event id=%s", final.ID)

	// --- Assertion (a): the agent authored a QA Test Registry sheet.
	var listResp struct {
		Sheets []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"sheets"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets?channel_id="+channel.ID+"&limit=50", token, orgID, nil, http.StatusOK, &listResp)
	var registrySheetID, registrySheetName string
	for _, s := range listResp.Sheets {
		lower := strings.ToLower(s.Name)
		if strings.Contains(lower, "qa test registry") {
			registrySheetID, registrySheetName = s.ID, s.Name
			break
		}
	}
	if registrySheetID == "" {
		// Case-insensitive fallback: any sheet mentioning "registry".
		for _, s := range listResp.Sheets {
			if strings.Contains(strings.ToLower(s.Name), "registry") {
				registrySheetID, registrySheetName = s.ID, s.Name
				break
			}
		}
	}
	if registrySheetID == "" {
		t.Fatalf("no QA registry sheet found in channel %s; sheets=%+v", channel.ID, listResp.Sheets)
	}
	t.Logf("found QA registry sheet id=%s name=%q", registrySheetID, registrySheetName)

	// --- Assertion (b): the registry has populated Test Cases and Test
	// Results pages.
	var structure struct {
		Sheet struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"sheet"`
		Pages []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			RowCount int64  `json:"row_count"`
		} `json:"pages"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets/"+registrySheetID, token, orgID, nil, http.StatusOK, &structure)
	var testCasesPage, testResultsPage struct {
		ID       string
		Name     string
		RowCount int64
	}
	for _, p := range structure.Pages {
		t.Logf("registry page name=%q row_count=%d", p.Name, p.RowCount)
		lower := strings.ToLower(p.Name)
		if testCasesPage.ID == "" && strings.Contains(lower, "test case") {
			testCasesPage.ID, testCasesPage.Name, testCasesPage.RowCount = p.ID, p.Name, p.RowCount
		}
		if testResultsPage.ID == "" && strings.Contains(lower, "test result") {
			testResultsPage.ID, testResultsPage.Name, testResultsPage.RowCount = p.ID, p.Name, p.RowCount
		}
	}
	if testCasesPage.ID == "" {
		t.Fatalf("registry sheet %s has no Test Cases page; pages=%+v", registrySheetID, structure.Pages)
	}
	if testCasesPage.RowCount < 1 {
		t.Fatalf("Test Cases page %q row_count=%d, want >= 1", testCasesPage.Name, testCasesPage.RowCount)
	}
	if testResultsPage.ID == "" {
		t.Fatalf("registry sheet %s has no Test Results page; pages=%+v", registrySheetID, structure.Pages)
	}
	if testResultsPage.RowCount < 1 {
		t.Fatalf("Test Results page %q row_count=%d, want >= 1", testResultsPage.Name, testResultsPage.RowCount)
	}
	t.Logf("Test Cases page=%q rows=%d, Test Results page=%q rows=%d", testCasesPage.Name, testCasesPage.RowCount, testResultsPage.Name, testResultsPage.RowCount)

	// --- Assertion (c): at least one Test Results row records a pass.
	var resultsQuery struct {
		Rows []struct {
			ID   string         `json:"id"`
			Data map[string]any `json:"data"`
		} `json:"rows"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost,
		apiBase+"/v1/sheets/"+registrySheetID+"/pages/"+testResultsPage.ID+"/rows/query",
		token, orgID, map[string]any{}, http.StatusOK, &resultsQuery)
	passedRows := 0
	for _, row := range resultsQuery.Rows {
		rowPassed := false
		for _, v := range row.Data {
			if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), "passed") {
				rowPassed = true
				break
			}
		}
		if rowPassed {
			passedRows++
		}
		raw, _ := json.Marshal(row.Data)
		t.Logf("test result row id=%s passed=%t data=%s", row.ID, rowPassed, raw)
	}
	if passedRows < 1 {
		t.Fatalf("no Test Results row recorded a pass; rows=%d", len(resultsQuery.Rows))
	}
	// Two buttons were requested; the agent may model this as one case with two
	// assertions, so fewer than two rows is logged non-fatally.
	if len(resultsQuery.Rows) < 2 {
		t.Errorf("only %d Test Results row(s) recorded; expected up to 2 (Google + GitHub) — the agent may have modeled both as one case", len(resultsQuery.Rows))
	}
	t.Logf("Test Results: %d row(s), %d passed", len(resultsQuery.Rows), passedRows)

	// --- Assertion (d): log the full final event payload for human review.
	finalRaw, _ := json.Marshal(final.Payload)
	t.Logf("final event payload: %s", finalRaw)
}
