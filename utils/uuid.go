package utils

import (
	"strings"

	"github.com/google/uuid"
)

func GetUUIDNoDash() string {
	id, _ := uuid.NewV7()
	return strings.Replace(id.String(), "-", "", -1)
}
