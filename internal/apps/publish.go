package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

var sha256HexRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

// PublishParams carries a builder agent's publish request: two drive object
// keys (uploaded via the existing streaming drive PUT) plus their sha256s.
type PublishParams struct {
	OrgID uuid.UUID
	AppID uuid.UUID

	SourceKey    string
	BundleKey    string
	SourceSHA256 string
	BundleSHA256 string

	Notes           string
	TemplateVersion string

	ActorAgentID    *uuid.UUID
	SourceSessionID *uuid.UUID
}

// Publish admits the drive keys (agent-owned in this org, sheets admission
// rule), stream-copies both objects into the immutable
// pub/o/{orgID}/apps/{slug}/{bundleSHA}/{source|bundle}.zip layout while
// verifying their sha256s, records the app_versions row, and deploys it.
// The bundle sha identifies the version directory: new content = new key,
// version rows are the history and objects are never overwritten.
func (s *Service) Publish(ctx context.Context, params PublishParams) (*model.AppVersion, error) {
	if s.store == nil || s.keys == nil {
		return nil, fmt.Errorf("apps: object storage is not configured")
	}
	app, err := s.GetApp(ctx, params.OrgID, params.AppID)
	if err != nil {
		return nil, err
	}

	sourceSHA := strings.ToLower(strings.TrimSpace(params.SourceSHA256))
	bundleSHA := strings.ToLower(strings.TrimSpace(params.BundleSHA256))
	if !sha256HexRe.MatchString(sourceSHA) || !sha256HexRe.MatchString(bundleSHA) {
		return nil, validationErrorf("source_sha256 and bundle_sha256 must be 64 hex characters")
	}
	sourceKey := strings.TrimSpace(params.SourceKey)
	bundleKey := strings.TrimSpace(params.BundleKey)
	if sourceKey == "" || bundleKey == "" {
		return nil, validationErrorf("source_key and bundle_key are required")
	}

	// The drive keys must belong to this org: org-owned keys pass, agent
	// drive keys only when the agent is the org's (sheets admission rule).
	if err := s.keys.AuthorizeObjectKeys(ctx, params.OrgID, []string{sourceKey, bundleKey}); err != nil {
		return nil, validationErrorf("%s", err.Error())
	}

	sourceDst := appVersionObjectKey(params.OrgID, app.Slug, bundleSHA, "source.zip")
	bundleDst := appVersionObjectKey(params.OrgID, app.Slug, bundleSHA, "bundle.zip")

	sourceBytes, err := s.copyVerified(ctx, sourceKey, sourceDst, sourceSHA)
	if err != nil {
		return nil, err
	}
	bundleBytes, err := s.copyVerified(ctx, bundleKey, bundleDst, bundleSHA)
	if err != nil {
		return nil, err
	}

	return s.finalizeVersion(ctx, app, publishMeta{
		Notes:           params.Notes,
		TemplateVersion: params.TemplateVersion,
		ActorAgentID:    params.ActorAgentID,
		SourceSessionID: params.SourceSessionID,
	}, versionArtifacts{
		sourceDst:   sourceDst,
		bundleDst:   bundleDst,
		sourceSHA:   sourceSHA,
		bundleSHA:   bundleSHA,
		sourceBytes: sourceBytes,
		bundleBytes: bundleBytes,
	})
}

// PublishStreamParams carries a REST publish request: the two zips as readers
// (streamed straight from the multipart body) plus their server-computed
// sha256s. Unlike PublishParams there are no drive keys and no client-supplied
// hashes — the caller hashed each file while spooling it and the copy re-hashes
// to guarantee the content-addressed object matches its key.
type PublishStreamParams struct {
	OrgID uuid.UUID
	AppID uuid.UUID

	Source io.Reader
	Bundle io.Reader

	SourceSHA256 string
	BundleSHA256 string

	Notes           string
	TemplateVersion string

	ActorAgentID    *uuid.UUID
	SourceSessionID *uuid.UUID
}

// PublishStream is the REST entry point: it streams the two zips into the same
// immutable pub/o/{orgID}/apps/{slug}/{bundleSHA}/ layout the MCP path uses,
// re-verifying each sha during the copy, then shares the version+deploy core
// (finalizeVersion) with Publish. No drive-key admission runs — the bytes come
// from the org member's own multipart upload, not an agent's drive.
func (s *Service) PublishStream(ctx context.Context, params PublishStreamParams) (*model.AppVersion, error) {
	if s.store == nil {
		return nil, fmt.Errorf("apps: object storage is not configured")
	}
	app, err := s.GetApp(ctx, params.OrgID, params.AppID)
	if err != nil {
		return nil, err
	}

	sourceSHA := strings.ToLower(strings.TrimSpace(params.SourceSHA256))
	bundleSHA := strings.ToLower(strings.TrimSpace(params.BundleSHA256))
	if !sha256HexRe.MatchString(sourceSHA) || !sha256HexRe.MatchString(bundleSHA) {
		return nil, validationErrorf("source and bundle must each produce a valid sha256 digest")
	}
	if params.Source == nil || params.Bundle == nil {
		return nil, validationErrorf("source and bundle are required")
	}

	sourceDst := appVersionObjectKey(params.OrgID, app.Slug, bundleSHA, "source.zip")
	bundleDst := appVersionObjectKey(params.OrgID, app.Slug, bundleSHA, "bundle.zip")

	sourceBytes, err := s.streamVerified(ctx, params.Source, sourceDst, sourceSHA)
	if err != nil {
		return nil, err
	}
	bundleBytes, err := s.streamVerified(ctx, params.Bundle, bundleDst, bundleSHA)
	if err != nil {
		return nil, err
	}

	return s.finalizeVersion(ctx, app, publishMeta{
		Notes:           params.Notes,
		TemplateVersion: params.TemplateVersion,
		ActorAgentID:    params.ActorAgentID,
		SourceSessionID: params.SourceSessionID,
	}, versionArtifacts{
		sourceDst:   sourceDst,
		bundleDst:   bundleDst,
		sourceSHA:   sourceSHA,
		bundleSHA:   bundleSHA,
		sourceBytes: sourceBytes,
		bundleBytes: bundleBytes,
	})
}

// publishMeta is the non-artifact side of a version: changelog + provenance,
// shared by both publish entry points.
type publishMeta struct {
	Notes           string
	TemplateVersion string
	ActorAgentID    *uuid.UUID
	SourceSessionID *uuid.UUID
}

// versionArtifacts is the outcome of placing both zips in the immutable layout:
// their final object keys, verified shas, and stored sizes.
type versionArtifacts struct {
	sourceDst, bundleDst string
	sourceSHA, bundleSHA string
	sourceBytes          int64
	bundleBytes          int64
}

// finalizeVersion records the app_versions row and deploys it. Shared by the
// MCP (drive-key) and REST (reader-streaming) publish paths so version history
// and the synchronous deploy behave identically regardless of ingestion.
func (s *Service) finalizeVersion(ctx context.Context, app *model.App, meta publishMeta, art versionArtifacts) (*model.AppVersion, error) {
	version := model.AppVersion{
		AppID:            app.ID,
		OrgID:            app.OrgID,
		SourceObjectKey:  art.sourceDst,
		BundleObjectKey:  art.bundleDst,
		SourceSHA256:     art.sourceSHA,
		BundleSHA256:     art.bundleSHA,
		SourceBytes:      art.sourceBytes,
		BundleBytes:      art.bundleBytes,
		Notes:            strings.TrimSpace(meta.Notes),
		TemplateVersion:  strings.TrimSpace(meta.TemplateVersion),
		CreatedByAgentID: meta.ActorAgentID,
		SourceSessionID:  meta.SourceSessionID,
	}
	if err := s.db.WithContext(ctx).Create(&version).Error; err != nil {
		return nil, fmt.Errorf("create app version: %w", err)
	}

	if err := s.Deploy(ctx, app, &version); err != nil {
		return &version, err
	}
	return &version, nil
}

// appVersionObjectKey builds the immutable per-version object key (plan §0).
func appVersionObjectKey(orgID uuid.UUID, slug, sha, leaf string) string {
	return fmt.Sprintf("pub/o/%s/apps/%s/%s/%s", orgID, slug, sha, leaf)
}

// copyVerified stream-copies an existing store object src → dst while hashing,
// and rejects a sha256 mismatch (MCP drive-key path).
func (s *Service) copyVerified(ctx context.Context, srcKey, dstKey, wantSHA string) (int64, error) {
	src, err := s.store.Open(ctx, srcKey)
	if err != nil {
		return 0, validationErrorf("open object %q: %s", srcKey, err.Error())
	}
	defer src.Close()
	return s.streamVerified(ctx, src, dstKey, wantSHA)
}

// streamVerified streams src → dstKey while hashing, and rejects a sha256
// mismatch. The object is written before the hash is known (streaming), so on
// mismatch the copy is deleted — a content-addressed key never holds bytes that
// don't match its sha. Shared by the MCP copy and the REST reader-streaming
// paths.
func (s *Service) streamVerified(ctx context.Context, src io.Reader, dstKey, wantSHA string) (int64, error) {
	hasher := sha256.New()
	stored, err := s.store.Stream(ctx, dstKey, "application/zip", io.TeeReader(src, hasher))
	if err != nil {
		return 0, fmt.Errorf("copy object to %q: %w", dstKey, err)
	}
	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if gotSHA != wantSHA {
		if delErr := s.store.Delete(ctx, dstKey); delErr != nil {
			return 0, fmt.Errorf("sha256 mismatch for %q (got %s want %s); cleanup failed: %w", dstKey, gotSHA, wantSHA, delErr)
		}
		return 0, validationErrorf("sha256 mismatch for %q: got %s want %s", dstKey, gotSHA, wantSHA)
	}
	return stored.Bytes, nil
}
