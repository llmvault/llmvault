package e2e

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type agentSessionsMemory struct {
	ID                string   `json:"id"`
	Scope             string   `json:"scope"`
	OrgID             string   `json:"org_id"`
	AgentID           *string  `json:"agent_id,omitempty"`
	UserID            *string  `json:"user_id,omitempty"`
	Content           string   `json:"content"`
	Tags              []string `json:"tags"`
	EmbeddingStatus   string   `json:"embedding_status"`
	EmbeddingRevision int      `json:"embedding_revision"`
	EmbeddingError    string   `json:"embedding_error,omitempty"`
	EmbeddedAt        *string  `json:"embedded_at,omitempty"`
}

func agentSessionsCreateMemory(t *testing.T, ctx context.Context, baseURL, token, orgID string, payload map[string]any) agentSessionsMemory {
	t.Helper()
	var out struct {
		Memory agentSessionsMemory `json:"memory"`
	}
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/memories", token, orgID, payload, http.StatusCreated, &out)
	if out.Memory.ID == "" {
		t.Fatalf("memory create returned empty memory: %+v", out)
	}
	return out.Memory
}

func agentSessionsListMemories(t *testing.T, ctx context.Context, baseURL, token, orgID string, query url.Values) []agentSessionsMemory {
	t.Helper()
	endpoint := baseURL + "/v1/memories"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var out struct {
		Data []agentSessionsMemory `json:"data"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, endpoint, token, orgID, nil, http.StatusOK, &out)
	return out.Data
}

func waitForAgentSessionsMemoryReady(t *testing.T, ctx context.Context, baseURL, token, orgID, memoryID string) agentSessionsMemory {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	query := url.Values{"limit": []string{"100"}}
	var last []agentSessionsMemory
	for time.Now().Before(deadline) {
		last = agentSessionsListMemories(t, ctx, baseURL, token, orgID, query)
		for _, memory := range last {
			if memory.ID != memoryID {
				continue
			}
			switch memory.EmbeddingStatus {
			case "ready":
				return memory
			case "failed":
				t.Fatalf("memory embedding failed id=%s error=%s", memory.ID, memory.EmbeddingError)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for memory embedding: %v", ctx.Err())
		case <-time.After(3 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for memory id=%s readiness; last=%+v", memoryID, last)
	return agentSessionsMemory{}
}

func agentSessionsArchiveMemory(t *testing.T, ctx context.Context, baseURL, token, orgID, memoryID string) {
	t.Helper()
	agentSessionsJSON(t, ctx, http.MethodDelete, baseURL+"/v1/memories/"+memoryID, token, orgID, nil, http.StatusOK, nil)
}
