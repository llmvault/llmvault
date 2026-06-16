package handler

import "github.com/google/uuid"

func firstUUIDString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
