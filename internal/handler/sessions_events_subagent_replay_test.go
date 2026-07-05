package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type subagentEventOut struct {
	SessionID      string         `json:"session_id"`
	EventType      string         `json:"event_type"`
	SequenceNumber int64          `json:"sequence_number"`
	RuntimeSeq     *int64         `json:"runtime_seq"`
	Durability     string         `json:"durability"`
	Payload        map[string]any `json:"payload"`
}

// TestListEvents_ReplaysSubagentDetailEventsUnderParentSession proves that durable
// subagent detail events persisted under the parent session are returned by
// ListEvents, keep their scope + subagent payload, and can be reassembled per job
// via the runtime sequence number.
func TestListEvents_ReplaysSubagentDetailEventsUnderParentSession(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "hello")
	sessionID := uuid.MustParse(created.Session.ID)
	childSessionID := uuid.New()
	now := time.Now().UTC()
	jobA := "job-" + uuid.NewString()[:8]
	jobB := "job-" + uuid.NewString()[:8]

	seq := func(v int64) *int64 { return &v }
	subPayload := func(jobID, text string) model.JSON {
		return model.JSON{
			"scope": "subagent",
			"text":  text,
			"subagent": map[string]any{
				"job_id":            jobID,
				"agent_name":        "researcher",
				"parent_session_id": sessionID.String(),
				"child_session_id":  childSessionID.String(),
			},
		}
	}

	events := []model.SessionEvent{
		{
			OrgID: fx.org.ID, SessionID: sessionID, AgentID: fx.agent.ID,
			EventID: "evt-sub-20", EventType: "tool_call_started", Source: "runtime",
			SequenceNumber: 20, RuntimeSeq: seq(20), RuntimeEventID: "evt-sub-20",
			Durability: "durable", Payload: subPayload(jobA, "job A started"),
			EventAt: now, CreatedAt: now,
		},
		{
			OrgID: fx.org.ID, SessionID: sessionID, AgentID: fx.agent.ID,
			EventID: "evt-sub-21", EventType: "token", Source: "runtime",
			SequenceNumber: 21, RuntimeSeq: seq(21), RuntimeEventID: "evt-sub-21",
			Durability: "durable", Payload: subPayload(jobB, "job B started"),
			EventAt: now.Add(time.Second), CreatedAt: now.Add(time.Second),
		},
		{
			OrgID: fx.org.ID, SessionID: sessionID, AgentID: fx.agent.ID,
			EventID: "evt-sub-22", EventType: "tool_call_completed", Source: "runtime",
			SequenceNumber: 22, RuntimeSeq: seq(22), RuntimeEventID: "evt-sub-22",
			Durability: "durable", Payload: subPayload(jobA, "job A done"),
			EventAt: now.Add(2 * time.Second), CreatedAt: now.Add(2 * time.Second),
		},
	}
	if err := h.db.Create(&events).Error; err != nil {
		t.Fatalf("seed subagent detail events: %v", err)
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/sessions/"+created.Session.ID+"/events?limit=100", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []subagentEventOut `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode events response: %v\n%s", err, rr.Body.String())
	}

	// Collect the three subagent detail rows by runtime sequence, verifying each
	// stays parent-scoped with an intact scope/subagent payload.
	bySeq := map[int64]subagentEventOut{}
	for _, ev := range resp.Data {
		if ev.RuntimeSeq == nil {
			continue
		}
		switch *ev.RuntimeSeq {
		case 20, 21, 22:
			bySeq[*ev.RuntimeSeq] = ev
		}
	}
	if len(bySeq) != 3 {
		t.Fatalf("replay returned %d subagent detail events, want 3 (data=%+v)", len(bySeq), resp.Data)
	}

	wantJob := map[int64]string{20: jobA, 21: jobB, 22: jobA}
	for _, s := range []int64{20, 21, 22} {
		ev := bySeq[s]
		if ev.SessionID != sessionID.String() {
			t.Fatalf("seq %d session_id = %s, want parent %s", s, ev.SessionID, sessionID)
		}
		if ev.Durability != "durable" {
			t.Fatalf("seq %d durability = %q, want durable", s, ev.Durability)
		}
		if scope, _ := ev.Payload["scope"].(string); scope != "subagent" {
			t.Fatalf("seq %d payload.scope = %v, want subagent", s, ev.Payload["scope"])
		}
		sub, ok := ev.Payload["subagent"].(map[string]any)
		if !ok {
			t.Fatalf("seq %d payload.subagent missing: %#v", s, ev.Payload["subagent"])
		}
		if jobID, _ := sub["job_id"].(string); jobID != wantJob[s] {
			t.Fatalf("seq %d payload.subagent.job_id = %v, want %s", s, sub["job_id"], wantJob[s])
		}
	}

	// Reassemble job A's stream by runtime sequence and confirm ordering holds.
	var jobASeqs []int64
	for _, s := range []int64{20, 21, 22} {
		sub, _ := bySeq[s].Payload["subagent"].(map[string]any)
		if jobID, _ := sub["job_id"].(string); jobID == jobA {
			jobASeqs = append(jobASeqs, *bySeq[s].RuntimeSeq)
		}
	}
	if len(jobASeqs) != 2 || jobASeqs[0] != 20 || jobASeqs[1] != 22 {
		t.Fatalf("job A reassembled sequence = %v, want [20 22]", jobASeqs)
	}
}
