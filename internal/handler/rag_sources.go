package handler

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	"github.com/usehivy/hivy/internal/resources"
	"github.com/usehivy/hivy/internal/webcrawl"
)

type RAGSourceHandler struct {
	db        *gorm.DB
	enq       enqueue.TaskEnqueuer
	credits   *billing.CreditsService
	discovery *resources.Discovery
	catalog   *catalog.Catalog
	web       webcrawl.Provider
}

func NewRAGSourceHandler(db *gorm.DB, enq enqueue.TaskEnqueuer, credits *billing.CreditsService, discovery *resources.Discovery, cat *catalog.Catalog, web webcrawl.Provider) *RAGSourceHandler {
	return &RAGSourceHandler{db: db, enq: enq, credits: credits, discovery: discovery, catalog: cat, web: web}
}

type ragSourceResponse struct {
	ID                      string                  `json:"id"`
	OrgID                   string                  `json:"org_id"`
	Kind                    string                  `json:"kind"`
	Name                    string                  `json:"name"`
	Status                  string                  `json:"status"`
	Enabled                 bool                    `json:"enabled"`
	ConnectionID            *string                 `json:"connection_id,omitempty"`
	Config                  json.RawMessage         `json:"config"`
	IndexingStart           *time.Time              `json:"indexing_start,omitempty"`
	LastSuccessfulIndexTime *time.Time              `json:"last_successful_index_time,omitempty"`
	LastPruned              *time.Time              `json:"last_pruned,omitempty"`
	RefreshFreqSeconds      *int                    `json:"refresh_freq_seconds,omitempty"`
	PruneFreqSeconds        *int                    `json:"prune_freq_seconds,omitempty"`
	TotalDocsIndexed        int                     `json:"total_docs_indexed"`
	InRepeatedErrorState    bool                    `json:"in_repeated_error_state"`
	LatestAttempt           *ragLatestAttemptStatus `json:"latest_attempt,omitempty"`
	CreatedAt               time.Time               `json:"created_at"`
	UpdatedAt               time.Time               `json:"updated_at"`
}

// ragLatestAttemptStatus is the per-source progress slice rendered on
// dashboards: counts + status + timestamps without the full attempt
// payload.
type ragLatestAttemptStatus struct {
	ID               string      `json:"id"`
	Status           string      `json:"status"`
	NewDocsIndexed   *int        `json:"new_docs_indexed,omitempty"`
	TotalDocsIndexed *int        `json:"total_docs_indexed,omitempty"`
	DocsEstimated    *int        `json:"docs_estimated,omitempty"`
	TotalBatches     *int        `json:"total_batches,omitempty"`
	CompletedBatches int         `json:"completed_batches"`
	Progress         ragProgress `json:"progress"`
	ErrorMsg         *string     `json:"error_msg,omitempty"`
	TimeStarted      *time.Time  `json:"time_started,omitempty"`
	TimeUpdated      time.Time   `json:"time_updated"`
}

// ragProgress is an honest sync-progress signal. Percent is set when a
// denominator exists: docs_estimated (only some connectors estimate) or, failing
// that, the batch counts once docfetching has enumerated the work. When neither
// is available Percent is nil and Indeterminate is true — callers should show
// the running DocsIndexed count instead of a bar. Basis records which
// denominator was used ("docs_estimated", "batches", or "" when indeterminate).
type ragProgress struct {
	Percent       *int   `json:"percent,omitempty"`
	Indeterminate bool   `json:"indeterminate"`
	Basis         string `json:"basis,omitempty"`
	DocsIndexed   int    `json:"docs_indexed"`
}

type ragSourceDetailResponse struct {
	ragSourceResponse
	RecentAttempts []ragIndexAttemptResponse `json:"recent_attempts"`
}

type ragIndexAttemptResponse struct {
	ID                   string      `json:"id"`
	Status               string      `json:"status"`
	FromBeginning        bool        `json:"from_beginning"`
	NewDocsIndexed       *int        `json:"new_docs_indexed,omitempty"`
	TotalDocsIndexed     *int        `json:"total_docs_indexed,omitempty"`
	DocsRemovedFromIndex *int        `json:"docs_removed_from_index,omitempty"`
	DocsEstimated        *int        `json:"docs_estimated,omitempty"`
	TotalBatches         *int        `json:"total_batches,omitempty"`
	CompletedBatches     int         `json:"completed_batches"`
	Progress             ragProgress `json:"progress"`
	ErrorMsg             *string     `json:"error_msg,omitempty"`
	PollRangeStart       *time.Time  `json:"poll_range_start,omitempty"`
	PollRangeEnd         *time.Time  `json:"poll_range_end,omitempty"`
	TimeStarted          *time.Time  `json:"time_started,omitempty"`
	TimeCreated          time.Time   `json:"time_created"`
	TimeUpdated          time.Time   `json:"time_updated"`
}

type ragAttemptDetailResponse struct {
	ragIndexAttemptResponse
	FullExceptionTrace *string                  `json:"full_exception_trace,omitempty"`
	Errors             []ragAttemptErrorPayload `json:"errors"`
	ErrorCount         int                      `json:"error_count"`
}

type ragAttemptErrorPayload struct {
	ID                   string     `json:"id"`
	DocumentID           *string    `json:"document_id,omitempty"`
	DocumentLink         *string    `json:"document_link,omitempty"`
	EntityID             *string    `json:"entity_id,omitempty"`
	FailedTimeRangeStart *time.Time `json:"failed_time_range_start,omitempty"`
	FailedTimeRangeEnd   *time.Time `json:"failed_time_range_end,omitempty"`
	FailureMessage       string     `json:"failure_message"`
	IsResolved           bool       `json:"is_resolved"`
	ErrorType            *string    `json:"error_type,omitempty"`
	TimeCreated          time.Time  `json:"time_created"`
}

type ragIntegrationResponse struct {
	ID          string `json:"id"`
	UniqueKey   string `json:"unique_key"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
}

type ragListResponse struct {
	Data  []ragSourceResponse `json:"data"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"page_size"`
}

type ragAttemptsListResponse struct {
	Data  []ragIndexAttemptResponse `json:"data"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Size  int                       `json:"page_size"`
}

type ragIntegrationsListResponse struct {
	Data []ragIntegrationResponse `json:"data"`
}

func toRAGSourceResponse(s *ragmodel.RAGSource) ragSourceResponse {
	resp := ragSourceResponse{
		ID:                      s.ID.String(),
		OrgID:                   s.OrgIDValue.String(),
		Kind:                    string(s.KindValue),
		Name:                    s.Name,
		Status:                  string(s.Status),
		Enabled:                 s.Enabled,
		Config:                  s.Config(),
		IndexingStart:           s.IndexingStart,
		LastSuccessfulIndexTime: s.LastSuccessfulIndexTime,
		LastPruned:              s.LastPruned,
		RefreshFreqSeconds:      s.RefreshFreqSeconds,
		PruneFreqSeconds:        s.PruneFreqSeconds,
		TotalDocsIndexed:        s.TotalDocsIndexed,
		InRepeatedErrorState:    s.InRepeatedErrorState,
		CreatedAt:               s.CreatedAt,
		UpdatedAt:               s.UpdatedAt,
	}
	if s.ConnectionID != nil {
		v := s.ConnectionID.String()
		resp.ConnectionID = &v
	}
	return resp
}

func toRAGAttemptResponse(a *ragmodel.RAGIndexAttempt) ragIndexAttemptResponse {
	return ragIndexAttemptResponse{
		ID:                   a.ID.String(),
		Status:               string(a.Status),
		FromBeginning:        a.FromBeginning,
		NewDocsIndexed:       a.NewDocsIndexed,
		TotalDocsIndexed:     a.TotalDocsIndexed,
		DocsRemovedFromIndex: a.DocsRemovedFromIndex,
		DocsEstimated:        a.DocsEstimated,
		TotalBatches:         a.TotalBatches,
		CompletedBatches:     a.CompletedBatches,
		Progress:             computeRAGProgress(a),
		ErrorMsg:             a.ErrorMsg,
		PollRangeStart:       a.PollRangeStart,
		PollRangeEnd:         a.PollRangeEnd,
		TimeStarted:          a.TimeStarted,
		TimeCreated:          a.TimeCreated,
		TimeUpdated:          a.TimeUpdated,
	}
}

func toRAGLatestAttemptStatus(a *ragmodel.RAGIndexAttempt) *ragLatestAttemptStatus {
	if a == nil {
		return nil
	}
	return &ragLatestAttemptStatus{
		ID:               a.ID.String(),
		Status:           string(a.Status),
		NewDocsIndexed:   a.NewDocsIndexed,
		TotalDocsIndexed: a.TotalDocsIndexed,
		DocsEstimated:    a.DocsEstimated,
		TotalBatches:     a.TotalBatches,
		CompletedBatches: a.CompletedBatches,
		Progress:         computeRAGProgress(a),
		ErrorMsg:         a.ErrorMsg,
		TimeStarted:      a.TimeStarted,
		TimeUpdated:      a.TimeUpdated,
	}
}

// computeRAGProgress turns raw attempt counters into an honest progress signal.
// A terminal-successful attempt is 100%. Otherwise it prefers a docs_estimated
// denominator, then batch counts, and only reports indeterminate when neither is
// available. Percent is clamped to [0, 100].
func computeRAGProgress(a *ragmodel.RAGIndexAttempt) ragProgress {
	docsIndexed := 0
	if a.TotalDocsIndexed != nil {
		docsIndexed = *a.TotalDocsIndexed
	}
	p := ragProgress{DocsIndexed: docsIndexed}

	if a.Status.IsSuccessful() {
		full := 100
		p.Percent = &full
		p.Basis = "complete"
		return p
	}

	if a.DocsEstimated != nil && *a.DocsEstimated > 0 && a.TotalDocsIndexed != nil {
		p.Percent = clampPercent(*a.TotalDocsIndexed, *a.DocsEstimated)
		p.Basis = "docs_estimated"
		return p
	}
	if a.TotalBatches != nil && *a.TotalBatches > 0 {
		p.Percent = clampPercent(a.CompletedBatches, *a.TotalBatches)
		p.Basis = "batches"
		return p
	}

	p.Indeterminate = true
	return p
}

func clampPercent(done, total int) *int {
	if total <= 0 {
		return nil
	}
	pct := min(max(done*100/total, 0), 100)
	return &pct
}

func toRAGIntegrationResponse(i *model.Integration) ragIntegrationResponse {
	return ragIntegrationResponse{
		ID:          i.ID.String(),
		UniqueKey:   i.UniqueKey,
		Provider:    i.Provider,
		DisplayName: i.DisplayName,
	}
}

func parseSourceID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
