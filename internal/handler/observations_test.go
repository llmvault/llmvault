package handler_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type observationFields struct {
	ID              uuid.UUID
	ChannelID       *uuid.UUID
	Content         string
	Kind            string
	ProofCount      int
	LastMentionedAt time.Time
	SupersededBy    *uuid.UUID
	ArchivedAt      *time.Time
	HumanVerified   bool
	Metadata        model.JSON
}

func seedObservation(t *testing.T, db *gorm.DB, orgID uuid.UUID, channelID *uuid.UUID, content string, proofCount int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	row := db.Raw(
		`INSERT INTO agent_observations (org_id, channel_id, content, kind, entities, proof_count, last_mentioned_at, metadata)
		 VALUES (?, ?, ?, 'decision', '{}', ?, now() - interval '1 hour', '{}') RETURNING id`,
		orgID, channelID, content, proofCount,
	).Row()
	if err := row.Scan(&id); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM memory_suppressions WHERE org_id = ?`, orgID)
		db.Exec(`DELETE FROM agent_observations WHERE org_id = ?`, orgID)
	})
	return id
}

func loadObservation(t *testing.T, db *gorm.DB, id uuid.UUID) observationFields {
	t.Helper()
	var out observationFields
	row := db.Raw(
		`SELECT id, channel_id, content, kind, proof_count, last_mentioned_at, superseded_by, archived_at, human_verified, metadata
		 FROM agent_observations WHERE id = ?`, id,
	).Row()
	var metadata []byte
	if err := row.Scan(&out.ID, &out.ChannelID, &out.Content, &out.Kind, &out.ProofCount, &out.LastMentionedAt, &out.SupersededBy, &out.ArchivedAt, &out.HumanVerified, &metadata); err != nil {
		t.Fatalf("load observation: %v", err)
	}
	if err := json.Unmarshal(metadata, &out.Metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return out
}

func TestIntegration_ObservationsConfirm(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)
	id := seedObservation(t, h.db, fx.org.ID, &channel.ID, "ACME prefers Railway.", 2)

	// Non-admin members are rejected.
	forbidden := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/confirm", fx, fx.member, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("member confirm status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	before := loadObservation(t, h.db, id)
	confirm := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/confirm", fx, fx.owner, nil)
	if confirm.Code != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", confirm.Code, confirm.Body.String())
	}
	after := loadObservation(t, h.db, id)
	if after.ProofCount != 3 {
		t.Fatalf("proof_count=%d, want 3", after.ProofCount)
	}
	if !after.LastMentionedAt.After(before.LastMentionedAt) {
		t.Fatal("last_mentioned_at should be refreshed")
	}
}

func TestIntegration_ObservationsCorrect(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)
	id := seedObservation(t, h.db, fx.org.ID, &channel.ID, "Deploys happen on Fridays.", 4)

	missing := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/correct", fx, fx.owner, map[string]any{"content": " "})
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("empty correction status=%d body=%s", missing.Code, missing.Body.String())
	}

	correct := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/correct", fx, fx.owner, map[string]any{
		"content": "Deploys happen on Tuesdays since July 2026.",
	})
	if correct.Code != http.StatusOK {
		t.Fatalf("correct status=%d body=%s", correct.Code, correct.Body.String())
	}
	var out struct {
		Observation struct {
			ID            string `json:"id"`
			Content       string `json:"content"`
			ProofCount    int    `json:"proof_count"`
			HumanVerified bool   `json:"human_verified"`
		} `json:"observation"`
	}
	if err := json.Unmarshal(correct.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode correct response: %v", err)
	}
	// Human-verified protection: the corrected text lives on a NEW verified row
	// (consolidation appends evidence to it but never rewrites the text).
	if !out.Observation.HumanVerified {
		t.Fatal("corrected observation must be human_verified")
	}
	if out.Observation.ProofCount != 4 {
		t.Fatalf("proof_count=%d, want carried-over 4", out.Observation.ProofCount)
	}
	if out.Observation.Content != "Deploys happen on Tuesdays since July 2026." {
		t.Fatalf("content=%q", out.Observation.Content)
	}

	old := loadObservation(t, h.db, id)
	if old.ArchivedAt == nil {
		t.Fatal("old observation must be archived")
	}
	if old.SupersededBy == nil || old.SupersededBy.String() != out.Observation.ID {
		t.Fatalf("superseded_by=%v, want %s", old.SupersededBy, out.Observation.ID)
	}
	replacement := loadObservation(t, h.db, uuid.MustParse(out.Observation.ID))
	if audit, ok := replacement.Metadata["audit"].([]any); !ok || len(audit) != 1 {
		t.Fatalf("metadata audit=%v, want one correct entry", replacement.Metadata["audit"])
	}

	// Archived rows can no longer be mutated.
	again := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/confirm", fx, fx.owner, nil)
	if again.Code != http.StatusNotFound {
		t.Fatalf("confirm archived status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestIntegration_ObservationsDeleteSuppresses(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)
	content := "The  Sandbox has   3.9 GB Disk."
	id := seedObservation(t, h.db, fx.org.ID, &channel.ID, content, 1)

	del := h.doJSON(t, http.MethodDelete, "/v1/observations/"+id.String(), fx, fx.owner, nil)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	if loadObservation(t, h.db, id).ArchivedAt == nil {
		t.Fatal("deleted observation must be archived")
	}

	sum := sha256.Sum256([]byte(strings.ToLower(strings.Join(strings.Fields(content), " "))))
	wantFingerprint := hex.EncodeToString(sum[:])
	var count int64
	row := h.db.Raw(
		`SELECT count(*) FROM memory_suppressions WHERE org_id = ? AND channel_id = ? AND content_fingerprint = ?`,
		fx.org.ID, channel.ID, wantFingerprint,
	).Row()
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count suppressions: %v", err)
	}
	if count != 1 {
		t.Fatalf("suppression rows=%d, want 1 (fingerprint %s)", count, wantFingerprint)
	}
}

func TestIntegration_ObservationsPinCreatesDirective(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)
	id := seedObservation(t, h.db, fx.org.ID, &channel.ID, "Always deploy from main.", 2)

	pin := h.doJSON(t, http.MethodPost, "/v1/observations/"+id.String()+"/pin", fx, fx.owner, nil)
	if pin.Code != http.StatusCreated {
		t.Fatalf("pin status=%d body=%s", pin.Code, pin.Body.String())
	}
	var out struct {
		Directive directiveOut `json:"directive"`
	}
	if err := json.Unmarshal(pin.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode pin response: %v", err)
	}
	if out.Directive.Source != "extracted-confirmed" || out.Directive.Content != "Always deploy from main." {
		t.Fatalf("pinned directive=%+v", out.Directive)
	}
	if out.Directive.ChannelID == nil || *out.Directive.ChannelID != channel.ID.String() {
		t.Fatalf("pinned directive channel_id=%v, want observation scope %s", out.Directive.ChannelID, channel.ID)
	}
	var directive model.AgentDirective
	if err := h.db.Where("id = ?", out.Directive.ID).First(&directive).Error; err != nil {
		t.Fatalf("load directive: %v", err)
	}
	if pinned, _ := loadObservation(t, h.db, id).Metadata["pinned"].(bool); !pinned {
		t.Fatal("observation metadata.pinned must be true")
	}
}

func TestIntegration_ObservationsListPagination(t *testing.T) {
	h := newMemoryControlHarness(t)
	fx, channel := h.seed(t)
	first := seedObservation(t, h.db, fx.org.ID, &channel.ID, "obs one", 1)
	seedObservation(t, h.db, fx.org.ID, &channel.ID, "obs two", 1)
	seedObservation(t, h.db, fx.org.ID, &channel.ID, "obs three", 1)
	// Archive one row: it must sort after the live ones.
	h.db.Exec(`UPDATE agent_observations SET archived_at = now() WHERE id = ?`, first)

	list := h.doJSON(t, http.MethodGet, "/v1/observations?channel_id="+channel.ID.String()+"&limit=2", fx, fx.owner, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var out struct {
		Data []struct {
			ID         string  `json:"id"`
			ArchivedAt *string `json:"archived_at"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out.Data) != 2 || !out.HasMore {
		t.Fatalf("page=%d has_more=%v, want 2/true", len(out.Data), out.HasMore)
	}
	for _, item := range out.Data {
		if item.ArchivedAt != nil {
			t.Fatal("non-archived observations must sort first")
		}
	}

	page2 := h.doJSON(t, http.MethodGet, "/v1/observations?channel_id="+channel.ID.String()+"&limit=2&offset=2", fx, fx.owner, nil)
	if err := json.Unmarshal(page2.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}
	if len(out.Data) != 1 || out.HasMore {
		t.Fatalf("page2=%d has_more=%v, want 1/false", len(out.Data), out.HasMore)
	}
	if out.Data[0].ID != first.String() || out.Data[0].ArchivedAt == nil {
		t.Fatalf("archived row should be last: %+v", out.Data[0])
	}
}
