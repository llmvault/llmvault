package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

var (
	errRAGSourceNotPaused        = errors.New("rag source is not paused")
	errRAGLatestAttemptNotFailed = errors.New("latest rag attempt did not fail")
	errRAGIngestAlreadyRunning   = errors.New("rag ingest is already running")
)

type ingestAction string

const (
	ingestActionResume ingestAction = "resume"
	ingestActionRetry  ingestAction = "retry"
)

// @Summary Resume ingestion for a knowledge source
// @Description Activates a paused source and immediately enqueues an ingest attempt. If an attempt is already queued or running, the source is activated without dispatching a duplicate.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/resume [post]
func (h *RAGSourceHandler) ResumeIngestion(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	if !h.requireRAGIngestManager(w, r, org.ID) {
		return
	}
	src, status := h.loadSourceForTrigger(w, r)
	if status != 0 {
		return
	}

	reserved, err := h.reserveIngest(r.Context(), org.ID, src.ID, ingestActionResume)
	if err != nil {
		writeRAGIngestActionError(r.Context(), w, src.ID, err)
		return
	}
	if reserved.Existing != nil {
		writeJSON(w, http.StatusAccepted, triggerResponse{
			TaskType:     ragtasks.TypeRagIngest,
			SourceID:     src.ID.String(),
			AttemptID:    reserved.Existing.ID.String(),
			Deduplicated: true,
		})
		return
	}

	if err := h.enqueueReservedIngest(r.Context(), reserved.Attempt); err != nil {
		h.rollbackReservedIngest(r.Context(), org.ID, reserved)
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "resume rag ingest enqueue failed", "source_id", src.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to enqueue ingest task"})
		return
	}

	writeJSON(w, http.StatusAccepted, triggerResponse{
		TaskType:  ragtasks.TypeRagIngest,
		SourceID:  src.ID.String(),
		AttemptID: reserved.Attempt.ID.String(),
	})
}

// @Summary Retry a failed knowledge-source ingestion
// @Description Requires the latest ingest attempt to be failed. Deletes the archived failed queue job, clears the source's error state, and dispatches a new sync attempt while preserving the failed attempt in history.
// @Tags rag
// @Produce json
// @Param id path string true "Source ID"
// @Success 202 {object} triggerResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/retry [post]
func (h *RAGSourceHandler) RetryIngestion(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	if !h.requireRAGIngestManager(w, r, org.ID) {
		return
	}
	src, status := h.loadSourceForTrigger(w, r)
	if status != 0 {
		return
	}

	latest, err := h.latestIngestAttempt(r.Context(), org.ID, src.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeRAGIngestActionError(r.Context(), w, src.ID, errRAGLatestAttemptNotFailed)
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load latest rag attempt for retry", "source_id", src.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load latest ingest attempt"})
		return
	}
	if latest.Status != ragmodel.IndexingStatusFailed {
		writeRAGIngestActionError(r.Context(), w, src.ID, errRAGLatestAttemptNotFailed)
		return
	}
	if h.cleaner == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to clean up failed ingest task"})
		return
	}
	if err := h.cleaner.DeleteTask(ragtasks.QueueRagWork, ragtasks.IngestTaskID(src.ID)); err != nil &&
		!errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete failed rag ingest task", "source_id", src.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to clean up failed ingest task"})
		return
	}

	reserved, err := h.reserveIngest(r.Context(), org.ID, src.ID, ingestActionRetry)
	if err != nil {
		writeRAGIngestActionError(r.Context(), w, src.ID, err)
		return
	}
	if err := h.enqueueReservedIngest(r.Context(), reserved.Attempt); err != nil {
		h.rollbackReservedIngest(r.Context(), org.ID, reserved)
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "retry rag ingest enqueue failed", "source_id", src.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to enqueue ingest task"})
		return
	}

	writeJSON(w, http.StatusAccepted, triggerResponse{
		TaskType:  ragtasks.TypeRagIngest,
		SourceID:  src.ID.String(),
		AttemptID: reserved.Attempt.ID.String(),
	})
}

func (h *RAGSourceHandler) requireRAGIngestManager(w http.ResponseWriter, r *http.Request, orgID uuid.UUID) bool {
	userID := strings.TrimSpace(middleware.UserID(r.Context()))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return false
	}
	actor, err := access.Resolve(r.Context(), h.db, orgID, userID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "resolve rag ingest action actor", "org_id", orgID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return false
	}
	if !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "not permitted"})
		return false
	}
	return true
}
