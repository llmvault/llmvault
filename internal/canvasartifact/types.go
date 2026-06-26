package canvasartifact

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/storage"
)

var ErrStorageNotConfigured = errors.New("canvas artifact storage is not configured")

type FileStore interface {
	Stream(ctx context.Context, key, contentType string, body io.Reader) (*storage.StoredAsset, error)
	Delete(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string) (string, error)
}

type ProjectResponse struct {
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Slug          string    `json:"slug"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	ArtifactCount int64     `json:"artifact_count,omitempty"`
}

type ProjectListResponse struct {
	Projects []ProjectResponse `json:"projects"`
}

type ProjectCreateRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

type ArtifactResponse struct {
	ID        uuid.UUID              `json:"id"`
	Slug      string                 `json:"slug"`
	Type      string                 `json:"type"`
	Name      string                 `json:"name"`
	ProjectID uuid.UUID              `json:"project_id"`
	Manifest  model.RawJSON          `json:"manifest"`
	Files     []ArtifactFileResponse `json:"files,omitempty"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
}

type ArtifactFileResponse struct {
	Path        string `json:"path"`
	Role        string `json:"role,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ObjectKey   string `json:"object_key,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256,omitempty"`
}

type ArtifactListResponse struct {
	Artifacts []ArtifactResponse `json:"artifacts"`
}

type ArtifactFilter struct {
	ProjectID   *uuid.UUID
	ProjectSlug string
	SessionID   *uuid.UUID
}

type SyncRequest struct {
	SessionID    *uuid.UUID        `json:"session_id,omitempty"`
	Project      SyncProjectInput  `json:"project"`
	Artifact     SyncArtifactInput `json:"artifact"`
	Files        []SyncFileInput   `json:"files"`
	DeletedPaths []string          `json:"deleted_paths,omitempty"`
}

type SyncProjectInput struct {
	Ref  string `json:"ref,omitempty"`
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type SyncArtifactInput struct {
	ID       string        `json:"id,omitempty"`
	Slug     string        `json:"slug"`
	Type     string        `json:"type"`
	Name     string        `json:"name"`
	Manifest model.RawJSON `json:"manifest"`
}

type SyncFileInput struct {
	Path        string `json:"path"`
	Role        string `json:"role,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Content     string `json:"content"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type SyncResponse struct {
	Project  ProjectResponse  `json:"project"`
	Artifact ArtifactResponse `json:"artifact"`
}

type SnapshotResponse struct {
	Projects []SnapshotProject `json:"projects"`
}

type SnapshotProject struct {
	ID        uuid.UUID          `json:"id"`
	Slug      string             `json:"slug"`
	Name      string             `json:"name"`
	Artifacts []SnapshotArtifact `json:"artifacts"`
}

type SnapshotArtifact struct {
	ID       uuid.UUID              `json:"id"`
	Slug     string                 `json:"slug"`
	Type     string                 `json:"type"`
	Name     string                 `json:"name"`
	Manifest model.RawJSON          `json:"manifest"`
	Files    []ArtifactFileResponse `json:"files"`
}
