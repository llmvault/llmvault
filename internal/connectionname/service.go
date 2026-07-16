// Package connectionname assigns stable, org-scoped names to connections.
package connectionname

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	generatedLength = 6
	charset         = "0123456789abcdefghijklmnopqrstuvwxyz"
	maxNameLength   = 80
)

var (
	ErrInvalidName = errors.New("invalid connection name")
	ErrNameTaken   = errors.New("connection name already exists")
)

// Identity is the stored display name and its canonical URL/runtime slug.
type Identity struct {
	Name      string
	Slug      string
	NeedsName bool
}

// ForConnection returns the default identity for a Nango connection. The
// provider name is used only when this is the first active connection of that
// provider in the org; later connections receive a six-character name.
func ForConnection(ctx context.Context, db *gorm.DB, orgID uuid.UUID, provider, displayName string) (Identity, error) {
	var count int64
	if err := db.WithContext(ctx).
		Table("connections").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND integrations.provider = ?", orgID, provider).
		Count(&count).Error; err != nil {
		return Identity{}, fmt.Errorf("count provider connections: %w", err)
	}
	if count == 0 {
		name := strings.TrimSpace(displayName)
		if name == "" {
			name = provider
		}
		preferred := Identity{Name: name, Slug: slugify(provider)}
		available, err := slugAvailable(ctx, db, "connections", orgID, preferred.Slug)
		if err != nil {
			return Identity{}, err
		}
		if available {
			return preferred, nil
		}
	}
	return generatedAvailable(ctx, db, "connections", orgID)
}

// ForDatabase returns the default identity for a database connection.
func ForDatabase(ctx context.Context, db *gorm.DB, orgID uuid.UUID, provider string) (Identity, error) {
	var count int64
	if err := db.WithContext(ctx).Table("database_connections").
		Where("org_id = ? AND provider = ? AND revoked_at IS NULL", orgID, provider).
		Count(&count).Error; err != nil {
		return Identity{}, fmt.Errorf("count database connections: %w", err)
	}
	if count == 0 {
		preferred := Identity{Name: provider, Slug: slugify(provider)}
		available, err := slugAvailable(ctx, db, "database_connections", orgID, preferred.Slug)
		if err != nil {
			return Identity{}, err
		}
		if available {
			return preferred, nil
		}
	}
	return generatedAvailable(ctx, db, "database_connections", orgID)
}

// Normalize validates a user-supplied name and derives its canonical slug.
func Normalize(raw string) (Identity, error) {
	name := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if name == "" || len([]rune(name)) > maxNameLength {
		return Identity{}, ErrInvalidName
	}
	slug := slugify(name)
	if slug == "" {
		return Identity{}, ErrInvalidName
	}
	return Identity{Name: name, Slug: slug, NeedsName: false}, nil
}

func generatedIdentity() (Identity, error) {
	value, err := randomString(generatedLength)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Name: value, Slug: value, NeedsName: true}, nil
}

func generatedAvailable(ctx context.Context, db *gorm.DB, table string, orgID uuid.UUID) (Identity, error) {
	for range 8 {
		identity, err := generatedIdentity()
		if err != nil {
			return Identity{}, err
		}
		available, err := slugAvailable(ctx, db, table, orgID, identity.Slug)
		if err != nil {
			return Identity{}, err
		}
		if available {
			return identity, nil
		}
	}
	return Identity{}, fmt.Errorf("generate unique connection name: %w", ErrNameTaken)
}

func slugAvailable(ctx context.Context, db *gorm.DB, table string, orgID uuid.UUID, slug string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Table(table).
		Where("org_id = ? AND slug = ? AND revoked_at IS NULL", orgID, slug).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check connection name availability: %w", err)
	}
	return count == 0, nil
}

func randomString(length int) (string, error) {
	max := big.NewInt(int64(len(charset)))
	value := make([]byte, length)
	for i := range value {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate connection name: %w", err)
		}
		value[i] = charset[n.Int64()]
	}
	return string(value), nil
}

func slugify(value string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			dash = false
		} else if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}
