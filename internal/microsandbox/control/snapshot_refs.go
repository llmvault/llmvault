package control

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/model"
)

var snapshotAliasPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func normalizeSnapshotAlias(alias string) string {
	return strings.TrimSpace(alias)
}

func validateSnapshotAlias(alias string) error {
	if alias == "" {
		return nil
	}
	if len(alias) > 128 {
		return fmt.Errorf("snapshot alias must be 128 characters or fewer")
	}
	if !snapshotAliasPattern.MatchString(alias) {
		return fmt.Errorf("snapshot alias must contain only lowercase letters, numbers, and single dashes")
	}
	return nil
}

func (s *Server) loadSnapshotByRef(ctx context.Context, ref string) (model.Snapshot, error) {
	var snapshot model.Snapshot
	if err := s.db.WithContext(ctx).First(&snapshot, "id = ?", ref).Error; err == nil {
		return snapshot, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Snapshot{}, err
	}
	if err := s.db.WithContext(ctx).First(&snapshot, "alias = ?", ref).Error; err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func snapshotUsableByOrg(snapshot model.Snapshot, orgID string) bool {
	return snapshot.Global || snapshot.OrgID == orgID
}
