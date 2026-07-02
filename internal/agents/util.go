package agents

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		(err != nil && strings.Contains(err.Error(), "duplicate key"))
}

func onConflictDoNothing() clause.OnConflict {
	return clause.OnConflict{DoNothing: true}
}
