package agents

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

func isDuplicateKeyError(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		(err != nil && strings.Contains(err.Error(), "duplicate key"))
}
