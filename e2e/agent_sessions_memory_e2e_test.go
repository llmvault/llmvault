package e2e

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSessionsMemoryE2E(t *testing.T) {
	if os.Getenv("HIVY_AGENT_SESSIONS_E2E") != "1" {
		t.Skip("set HIVY_AGENT_SESSIONS_E2E=1 to run against the live compose stack")
	}
	loadEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	apiBase := agentSessionsBaseURL("HIVY_API_BASE_URL", "HIVY_COMPOSE_API_PORT", "8080")
	workerBase := agentSessionsBaseURL("HIVY_WORKER_BASE_URL", "HIVY_COMPOSE_WORKER_HEALTH_PORT", "8090")
	requireAgentSessionsHealthy(t, ctx, apiBase, "api")
	requireAgentSessionsHealthy(t, ctx, workerBase, "worker")
	agentSessionsEnsureSystemOpenRouterCredential(t)

	runID := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	password := "agent-sessions-memory-e2e-password"
	ownerEmail := "agent-sessions-memory-" + runID + "@example.com"
	finalMarker := "MEMORY_E2E_PASS_" + runID
	orgMarker := "ORG_MEMORY_" + runID
	userMarker := "USER_MEMORY_" + runID

	ownerAuth := agentSessionsRegister(t, ctx, apiBase, ownerEmail, password, "Agent Memory Owner "+runID)
	orgID := ownerAuth.Orgs[0].ID
	ownerToken := ownerAuth.AccessToken
	agents := agentSessionsListAgents(t, ctx, apiBase, ownerToken, orgID)
	defaultAgent := findDefaultAgent(t, agents)
	channels := agentSessionsListChannels(t, ctx, apiBase, ownerToken, orgID)
	general := findDefaultGeneralChannel(t, channels, defaultAgent.ID)

	orgMemory := agentSessionsCreateMemory(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"scope":    "org",
		"agent_id": defaultAgent.ID,
		"content":  "Organization memory marker: " + orgMarker + ". The launch codename is Helio.",
		"tags":     []string{"e2e", "memory", "launch"},
	})
	userMemory := agentSessionsCreateMemory(t, ctx, apiBase, ownerToken, orgID, map[string]any{
		"scope":    "user",
		"agent_id": defaultAgent.ID,
		"content":  "User memory marker: " + userMarker + ". The preferred escalation word is Prism.",
		"tags":     []string{"e2e", "memory", "escalation"},
	})
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		agentSessionsArchiveMemory(t, cleanupCtx, apiBase, ownerToken, orgID, orgMemory.ID)
		agentSessionsArchiveMemory(t, cleanupCtx, apiBase, ownerToken, orgID, userMemory.ID)
	})
	waitForAgentSessionsMemoryReady(t, ctx, apiBase, ownerToken, orgID, orgMemory.ID)
	waitForAgentSessionsMemoryReady(t, ctx, apiBase, ownerToken, orgID, userMemory.ID)

	query := url.Values{"q": []string{"launch codename escalation word"}, "limit": []string{"10"}}
	searchHits := agentSessionsListMemories(t, ctx, apiBase, ownerToken, orgID, query)
	assertAgentSessionsMemorySearchHits(t, searchHits, orgMarker, userMarker)

	session := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the memory E2E.",
		"Use preloaded memory context only.",
		"Reply exactly " + finalMarker + " followed by the organization launch codename and the user escalation word, separated by spaces, and no other text.",
	}, "\n"))
	assertAgentSessionsBackendOwnedMutationEvent(t, session.Event)
	stream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	stream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return strings.Contains(event.RawData, finalMarker) &&
			strings.Contains(event.RawData, "Helio") &&
			strings.Contains(event.RawData, "Prism")
	})
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, session.Session.ID, finalMarker)
	events := agentSessionsListAllEvents(t, ctx, apiBase, ownerToken, orgID, session.Session.ID)
	assertAgentSessionsMemoryDynamicContext(t, events, orgMarker, userMarker)

	toolMarker := "TOOL_MEMORY_" + runID
	toolFinalMarker := "MEMORY_TOOL_E2E_PASS_" + runID
	toolSession := agentSessionsCreateSession(t, ctx, apiBase, ownerToken, orgID, general.ID, strings.Join([]string{
		"This is the live MCP memory tool E2E.",
		"Before replying, call hivy_retain_memory exactly once.",
		"Use content exactly: Tool-retained memory marker: " + toolMarker + ". The remembered onboarding word is Nova.",
		"Use target owner user and visibility this_agent.",
		"Use tags exactly e2e-memory and tool-retain.",
		"After the tool result, reply exactly " + toolFinalMarker + " and no other text.",
	}, "\n"))
	assertAgentSessionsBackendOwnedMutationEvent(t, toolSession.Event)
	toolStream := agentSessionsStartSandboxStream(t, ctx, apiBase, ownerToken, orgID, toolSession.Session.ID)
	toolCall := toolStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_call" &&
			eventString(event.Payload, "tool") == "hivy_retain_memory" &&
			strings.Contains(event.RawData, toolMarker)
	})
	if strings.Contains(toolCall.RawData, "_hivy_session_id") {
		t.Fatalf("memory tool call exposed hidden session id to the model: %s", toolCall.RawData)
	}
	toolStream.waitForEvent(t, ctx, 3*time.Minute, func(event runtimeSSEEvent) bool {
		return event.Name == "tool_result" && strings.Contains(event.RawData, toolMarker)
	})
	createdByTool := waitForAgentSessionsMemoryRow(t, ctx, toolMarker, "pending", "ready")
	assertAgentSessionsToolMemoryRow(t, createdByTool, orgID, defaultAgent.ID, ownerAuth.User.ID, toolSession.Session.ID, toolMarker)
	waitForAgentSessionsResponse(t, ctx, apiBase, ownerToken, orgID, toolSession.Session.ID, toolFinalMarker)
	readyToolMemory := waitForAgentSessionsMemoryRow(t, ctx, toolMarker, "ready")
	assertAgentSessionsToolMemoryEmbedding(t, readyToolMemory)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Minute)
		defer cleanupCancel()
		agentSessionsArchiveMemory(t, cleanupCtx, apiBase, ownerToken, orgID, readyToolMemory.ID.String())
	})
}

func assertAgentSessionsMemorySearchHits(t *testing.T, hits []agentSessionsMemory, markers ...string) {
	t.Helper()
	raw, _ := json.Marshal(hits)
	for _, marker := range markers {
		if !strings.Contains(string(raw), marker) {
			t.Fatalf("memory search did not include marker=%s hits=%s", marker, raw)
		}
	}
}

func assertAgentSessionsMemoryDynamicContext(t *testing.T, events []agentSessionsEvent, markers ...string) {
	t.Helper()
	for _, event := range events {
		if event.EventType != "user.message.received" || strings.EqualFold(event.Source, "runtime") {
			continue
		}
		raw, _ := json.Marshal(event.Payload["dynamic_context"])
		matches := 0
		for _, marker := range markers {
			if strings.Contains(string(raw), marker) {
				matches++
			}
		}
		if matches == len(markers) {
			return
		}
	}
	all, _ := json.Marshal(events)
	t.Fatalf("backend user event dynamic_context missing memory markers=%v events=%s", markers, all)
}

type agentSessionsMemoryRow struct {
	ID                uuid.UUID  `gorm:"column:id"`
	OrgID             uuid.UUID  `gorm:"column:org_id"`
	AgentID           *uuid.UUID `gorm:"column:agent_id"`
	UserID            *uuid.UUID `gorm:"column:user_id"`
	Scope             string     `gorm:"column:scope"`
	Content           string     `gorm:"column:content"`
	TagsCSV           string     `gorm:"column:tags_csv"`
	Metadata          model.JSON `gorm:"column:metadata"`
	EmbeddingModel    string     `gorm:"column:embedding_model"`
	EmbeddingStatus   string     `gorm:"column:embedding_status"`
	EmbeddingRevision int        `gorm:"column:embedding_revision"`
	EmbeddingError    string     `gorm:"column:embedding_error"`
	EmbeddedAt        *time.Time `gorm:"column:embedded_at"`
	SourceSessionID   *uuid.UUID `gorm:"column:source_session_id"`
	CreatedByUserID   *uuid.UUID `gorm:"column:created_by_user_id"`
	HasEmbedding      bool       `gorm:"column:has_embedding"`
	EmbeddingDims     *int       `gorm:"column:embedding_dims"`
}

func waitForAgentSessionsMemoryRow(t *testing.T, ctx context.Context, marker string, statuses ...string) agentSessionsMemoryRow {
	t.Helper()
	want := map[string]bool{}
	for _, status := range statuses {
		want[status] = true
	}
	db := agentSessionsOpenDB(t)
	deadline := time.Now().Add(4 * time.Minute)
	var last agentSessionsMemoryRow
	for time.Now().Before(deadline) {
		row, ok := loadAgentSessionsMemoryRow(t, db, marker)
		if ok {
			last = row
			if want[row.EmbeddingStatus] {
				return row
			}
			if row.EmbeddingStatus == "failed" {
				t.Fatalf("memory tool embedding failed id=%s error=%s", row.ID, row.EmbeddingError)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for memory row marker=%s: %v", marker, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for memory marker=%s statuses=%v last=%+v", marker, statuses, last)
	return agentSessionsMemoryRow{}
}

func loadAgentSessionsMemoryRow(t *testing.T, db interface {
	Raw(string, ...any) *gorm.DB
}, marker string) (agentSessionsMemoryRow, bool) {
	t.Helper()
	var rows []agentSessionsMemoryRow
	if err := db.Raw(`
SELECT id, org_id, agent_id, user_id, scope, content, array_to_string(tags, ',') AS tags_csv, metadata,
       embedding_model, embedding_status, embedding_revision, embedding_error,
       embedded_at, source_session_id, created_by_user_id,
       embedding IS NOT NULL AS has_embedding,
       vector_dims(embedding) AS embedding_dims
FROM agent_memories
WHERE archived_at IS NULL AND content LIKE ?
ORDER BY created_at DESC
LIMIT 1`, "%"+marker+"%").Scan(&rows).Error; err != nil {
		t.Fatalf("load memory row marker=%s: %v", marker, err)
	}
	if len(rows) == 0 {
		return agentSessionsMemoryRow{}, false
	}
	return rows[0], true
}

func assertAgentSessionsToolMemoryRow(t *testing.T, row agentSessionsMemoryRow, orgIDRaw, agentIDRaw, userIDRaw, sessionIDRaw, marker string) {
	t.Helper()
	orgID := uuid.MustParse(orgIDRaw)
	agentID := uuid.MustParse(agentIDRaw)
	userID := uuid.MustParse(userIDRaw)
	sessionID := uuid.MustParse(sessionIDRaw)
	if row.OrgID != orgID || row.Scope != model.AgentMemoryScopeUser {
		t.Fatalf("memory row target mismatch org=%s scope=%s row=%+v", row.OrgID, row.Scope, row)
	}
	if row.AgentID == nil || *row.AgentID != agentID {
		t.Fatalf("memory row agent_id mismatch row=%+v want=%s", row, agentID)
	}
	if row.UserID == nil || *row.UserID != userID {
		t.Fatalf("memory row user_id mismatch row=%+v want=%s", row, userID)
	}
	if row.SourceSessionID == nil || *row.SourceSessionID != sessionID {
		t.Fatalf("memory row source_session_id mismatch row=%+v want=%s", row, sessionID)
	}
	if row.CreatedByUserID == nil || *row.CreatedByUserID != userID {
		t.Fatalf("memory row created_by_user_id mismatch row=%+v want=%s", row, userID)
	}
	if !strings.Contains(row.Content, marker) {
		t.Fatalf("memory row content missing marker=%s row=%+v", marker, row)
	}
	assertAgentSessionsToolMemoryTags(t, row.TagsCSV)
	assertAgentSessionsToolMemoryMetadata(t, row.Metadata, agentID)
}

func assertAgentSessionsToolMemoryTags(t *testing.T, tagsCSV string) {
	t.Helper()
	got := map[string]bool{}
	for _, tag := range strings.Split(tagsCSV, ",") {
		if tag == "" {
			continue
		}
		got[tag] = true
	}
	for _, tag := range []string{"e2e-memory", "tool-retain"} {
		if !got[tag] {
			t.Fatalf("memory row tags missing %q: %s", tag, tagsCSV)
		}
	}
}

func assertAgentSessionsToolMemoryMetadata(t *testing.T, metadata model.JSON, agentID uuid.UUID) {
	t.Helper()
	if got, _ := metadata["source"].(string); got != "mcp_memory_tool" {
		t.Fatalf("memory metadata source=%q want mcp_memory_tool metadata=%v", got, metadata)
	}
	if got, _ := metadata["created_by_agent_id"].(string); got != agentID.String() {
		t.Fatalf("memory metadata created_by_agent_id=%q want %s metadata=%v", got, agentID, metadata)
	}
	target, _ := metadata["target"].(map[string]any)
	if target == nil || target["owner"] != "user" || target["visibility"] != "this_agent" {
		t.Fatalf("memory metadata target mismatch metadata=%v", metadata)
	}
}

func assertAgentSessionsToolMemoryEmbedding(t *testing.T, row agentSessionsMemoryRow) {
	t.Helper()
	if row.EmbeddingStatus != "ready" || row.EmbeddedAt == nil {
		t.Fatalf("memory embedding not ready row=%+v", row)
	}
	if !row.HasEmbedding || row.EmbeddingDims == nil || *row.EmbeddingDims != 1024 {
		t.Fatalf("memory embedding vector not stored correctly row=%+v", row)
	}
	if row.EmbeddingModel == "" || row.EmbeddingRevision != 1 {
		t.Fatalf("memory embedding metadata invalid row=%+v", row)
	}
}
