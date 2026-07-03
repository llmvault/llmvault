package e2e

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/sheets"
	"github.com/usehivy/hivy/internal/tasks"
)

// TestAgentSessionsSheetsE2E is the Sheets Phase 7 end-to-end journey against
// the live compose stack:
//
//	plugin install (org → agent, normal API path) → MCP tool gating +
//	skills_list visibility → agent creates a sheet and writes/queries rows
//	with the exact SKILL.md payloads → user reads and cell-edits via /v1 →
//	agent reverts the user's edit via sheet_operations → CSV imports from
//	both the MCP tool (CSV uploaded through the sandbox drive endpoint, so
//	the import runs from a pub/e/{agentID}/… key — the real agent flow) and
//	the REST endpoint (org-owned pub/o key) run through the real enqueuer
//	and worker to completion.
//
// The MCP layer is built through the production mcpserver.BuildServer path
// (same builder the API's ServerCache uses) over the compose Postgres, with
// the sheets service wired to a spy enqueuer that delegates to a real Asynq
// client on the compose Redis — so the 354a4dec0 regression (MCP-created
// import jobs must reach the enqueuer) is asserted directly, and the job is
// still processed by the live worker.
func TestAgentSessionsSheetsE2E(t *testing.T) {
	loadEnv(t)
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	ownerAuth := agentSessionsRegister(t, ctx, apiBase, "sheets-e2e-"+runID+"@example.com", "sheets-e2e-password", "Sheets E2E "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken
	orgUUID := uuid.MustParse(orgID)

	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	agentUUID := uuid.MustParse(defaultAgent.ID)
	t.Logf("org=%s agent=%s", orgID, defaultAgent.ID)

	db := agentSessionsOpenDB(t)
	enqClient := enqueue.NewClient(asynq.RedisClientOpt{Addr: sheetsE2ERedisAddr()})
	t.Cleanup(func() { _ = enqClient.Close() })
	spy := &sheetsE2ESpyEnqueuer{inner: tasks.NewSheetImportEnqueuer(enqClient)}
	svc := sheets.NewService(db).WithImportEnqueuer(spy)

	// --- Gating: before the plugin is installed for the agent, the sheets
	// tool group must NOT register (skills tools still do).
	before := sheetsE2EConnectMCP(t, ctx, db, svc, orgUUID, agentUUID)
	beforeTools := sheetsE2EListTools(t, ctx, before)
	for _, name := range sheetsE2EToolNames {
		if _, ok := beforeTools[name]; ok {
			t.Fatalf("tool %s registered before the sheets plugin was installed", name)
		}
	}
	if _, ok := beforeTools["skills_list"]; !ok {
		t.Fatalf("skills_list missing from pre-install server; tools=%v", sheetsE2EToolNameList(beforeTools))
	}

	// --- Install via the normal path: org install, then per-agent enable.
	agentSessionsInstallPlugin(t, ctx, apiBase, ownerToken, orgID, "sheets")
	agentSessionsJSON(t, ctx, http.MethodPost, apiBase+"/v1/agents/"+defaultAgent.ID+"/plugins/sheets", ownerToken, orgID, nil, http.StatusOK, nil)
	t.Log("sheets plugin installed for org and enabled for the default agent")

	// --- After install: all 8 tools register and the skill is discoverable.
	client := sheetsE2EConnectMCP(t, ctx, db, svc, orgUUID, agentUUID)
	tools := sheetsE2EListTools(t, ctx, client)
	for _, name := range sheetsE2EToolNames {
		if _, ok := tools[name]; !ok {
			t.Fatalf("tool %s not registered after plugin install; tools=%v", name, sheetsE2EToolNameList(tools))
		}
	}
	skillsOut := sheetsE2ECallTool(t, ctx, client, "skills_list", map[string]any{})
	if !sheetsE2ESkillListed(skillsOut, "sheets") {
		t.Fatalf("sheets skill not visible in skills_list: %#v", skillsOut)
	}
	t.Log("sheets skill visible via skills_list and all 8 MCP tools registered")

	// --- Agent journey with the documented SKILL.md payloads.
	sheetsE2ECallToolJSON(t, ctx, client, "sheet_list", `{}`)
	created := sheetsE2ECallToolJSON(t, ctx, client, "sheet_create", sheetsE2ESkillSheetCreate)
	sheetID := created["sheet"].(map[string]any)["id"].(string)
	page := created["pages"].([]any)[0].(map[string]any)
	pageID := page["id"].(string)
	fieldByName := map[string]string{}
	for _, raw := range page["fields"].([]any) {
		field := raw.(map[string]any)
		fieldByName[field["name"].(string)] = field["id"].(string)
	}
	replace := strings.NewReplacer(
		"9a2d4e7f-…", pageID,
		"fld_8k2mx1q9", fieldByName["Company"],
		"fld_3n7pw2rt", fieldByName["Website"],
		"fld_6q1zv8mk", fieldByName["Contact Email"],
		"fld_2j9xc4hb", fieldByName["Tier"],
		"fld_7t5ry1ns", fieldByName["Employees"],
	).Replace
	tierID, companyID := fieldByName["Tier"], fieldByName["Company"]

	inserted := sheetsE2ECallToolJSON(t, ctx, client, "rows_write", replace(sheetsE2ESkillRowsInsert))
	if inserted["inserted"] != float64(2) {
		t.Fatalf("rows_write insert = %#v", inserted)
	}
	queried := sheetsE2ECallToolJSON(t, ctx, client, "rows_query", replace(sheetsE2ESkillRowsQuery))
	queryRows := queried["rows"].([]any)
	if len(queryRows) != 1 {
		t.Fatalf("documented filter query returned %d rows, want 1 (Acme): %#v", len(queryRows), queried)
	}
	acme := queryRows[0].(map[string]any)
	acmeID := acme["id"].(string)
	if acme["data"].(map[string]any)[companyID] != "Acme Corp" {
		t.Fatalf("filter query returned wrong row: %#v", acme)
	}
	t.Logf("agent created sheet=%s page=%s and inserted/queried rows", sheetID, pageID)

	// --- User journey over /v1: bootstrap read, rows query, cell edit.
	var structure struct {
		Sheet struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"sheet"`
		Pages []struct {
			ID       string `json:"id"`
			RowCount int64  `json:"row_count"`
		} `json:"pages"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets/"+sheetID, ownerToken, orgID, nil, http.StatusOK, &structure)
	if structure.Sheet.Name != "SaaS Competitor Research" || len(structure.Pages) != 1 || structure.Pages[0].RowCount != 2 {
		t.Fatalf("user bootstrap read mismatch: %+v", structure)
	}

	var listPage struct {
		Sheets []struct {
			ID string `json:"id"`
		} `json:"sheets"`
		NextCursor string `json:"next_cursor"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets?limit=50", ownerToken, orgID, nil, http.StatusOK, &listPage)
	foundListed := false
	for _, s := range listPage.Sheets {
		if s.ID == sheetID {
			foundListed = true
		}
	}
	if !foundListed {
		t.Fatalf("created sheet missing from GET /v1/sheets: %+v", listPage)
	}

	pagePath := apiBase + "/v1/sheets/" + sheetID + "/pages/" + pageID
	var restQuery struct {
		Rows []struct {
			ID   string         `json:"id"`
			Data map[string]any `json:"data"`
		} `json:"rows"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, pagePath+"/rows/query", ownerToken, orgID, map[string]any{
		"filter": map[string]any{"and": []map[string]any{{"field": tierID, "op": "eq", "value": "direct"}}},
	}, http.StatusOK, &restQuery)
	if len(restQuery.Rows) != 1 || restQuery.Rows[0].ID != acmeID {
		t.Fatalf("REST rows/query mismatch: %+v", restQuery)
	}

	// A single cell edit is a batch of one row with one key.
	agentSessionsJSON(t, ctx, http.MethodPatch, pagePath+"/rows", ownerToken, orgID, map[string]any{
		"rows": []map[string]any{{"id": acmeID, "data": map[string]any{tierID: "adjacent"}}},
	}, http.StatusOK, nil)
	t.Log("user cell-edited Tier via PATCH rows")

	// --- Agent reverts the user's edit via sheet_operations.
	ops := sheetsE2ECallToolJSON(t, ctx, client, "sheet_operations", replace(sheetsE2ESkillOpsList))
	newest := ops["operations"].([]any)[0].(map[string]any)
	if newest["type"] != "rows_update" || newest["actor_user_id"] == nil {
		t.Fatalf("newest operation is not the user's cell edit: %#v", newest)
	}
	reverted := sheetsE2ECallToolJSON(t, ctx, client, "sheet_operations",
		strings.ReplaceAll(sheetsE2ESkillOpsRevert, "b82fd4c1-…", newest["id"].(string)))
	if reverted["reverted"] != true {
		t.Fatalf("revert failed: %#v", reverted)
	}
	agentSessionsJSON(t, ctx, http.MethodPost, pagePath+"/rows/query", ownerToken, orgID, map[string]any{
		"filter": map[string]any{"and": []map[string]any{{"field": tierID, "op": "eq", "value": "direct"}}},
	}, http.StatusOK, &restQuery)
	if len(restQuery.Rows) != 1 || restQuery.Rows[0].ID != acmeID {
		t.Fatalf("agent revert did not restore the user-edited cell: %+v", restQuery)
	}
	t.Log("agent reverted the user's cell edit via sheet_operations")

	// --- CSV import, MCP path — the REAL agent flow from SKILL.md: the CSV
	// is uploaded through the sandbox drive endpoint (runtime-secret auth,
	// key lands under pub/e/{agentID}/…) and imported from that drive key.
	// Also pins the 354a4dec0 regression: the tool-created job must reach
	// the enqueuer, then complete via the live worker.
	csvOne := "Company,Website,Employees\n" +
		"Import One,https://one.example.com,10\n" +
		"Import Two,https://two.example.com,20\n" +
		"Import Three,https://three.example.com,30\n"
	runtimeSecret := "sheets-e2e-" + runID
	sandbox := agentSessionsCreateCanvasSandboxRow(t, ctx, orgID, defaultAgent.ID, runtimeSecret, "sheets-"+runID)
	keyOne := sheetsE2EUploadDriveCSV(t, ctx, apiBase, defaultAgent.ID, sandbox.ID.String(), runtimeSecret, "imports/mcp-leads-"+runID+".csv", csvOne)
	if !strings.HasPrefix(keyOne, "pub/e/"+defaultAgent.ID+"/") {
		t.Fatalf("drive upload key %q is not under pub/e/%s/", keyOne, defaultAgent.ID)
	}
	importPayload := strings.ReplaceAll(replace(sheetsE2ESkillImportStart), "pub/e/…/imports/leads.csv", keyOne)
	started := sheetsE2ECallToolJSON(t, ctx, client, "sheet_import_csv", importPayload)
	mcpJobID := started["job"].(map[string]any)["job_id"].(string)
	if !spy.saw(uuid.MustParse(mcpJobID)) {
		t.Fatalf("354a4dec0 regression: MCP-created import job %s never reached the enqueuer (spy saw %v)", mcpJobID, spy.jobIDs())
	}
	t.Logf("MCP import job %s reached the enqueuer", mcpJobID)
	mcpJob := sheetsE2EWaitForImport(t, ctx, apiBase, ownerToken, orgID, mcpJobID)
	if mcpJob.ProcessedRows != 3 {
		t.Fatalf("MCP import processed %d rows, want 3", mcpJob.ProcessedRows)
	}

	// --- CSV import, REST path: proves the API's own wired enqueuer
	// end-to-end (job creation → asynq → worker → completed).
	csvTwo := "Company,Website,Employees\n" +
		"Rest One,https://r1.example.com,11\n" +
		"Rest Two,https://r2.example.com,22\n"
	keyTwo := sheetsE2EUploadCSV(t, ctx, apiBase, ownerToken, orgID, "rest-leads-"+runID+".csv", csvTwo)
	var restJob struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, pagePath+"/imports", ownerToken, orgID, map[string]any{
		"object_key": keyTwo,
		"options": map[string]any{
			"has_header": true,
			"field_mapping": map[string]any{
				"Company":   fieldByName["Company"],
				"Website":   fieldByName["Website"],
				"Employees": fieldByName["Employees"],
			},
		},
	}, http.StatusCreated, &restJob)
	restDone := sheetsE2EWaitForImport(t, ctx, apiBase, ownerToken, orgID, restJob.ID)
	if restDone.ProcessedRows != 2 {
		t.Fatalf("REST import processed %d rows, want 2", restDone.ProcessedRows)
	}

	// Final structure: 2 inserted + 3 MCP-imported + 2 REST-imported.
	agentSessionsJSON(t, ctx, http.MethodGet, apiBase+"/v1/sheets/"+sheetID, ownerToken, orgID, nil, http.StatusOK, &structure)
	if structure.Pages[0].RowCount != 7 {
		t.Fatalf("final row count = %d, want 7", structure.Pages[0].RowCount)
	}
	t.Log("both imports completed through the live worker; final row count verified")
}
