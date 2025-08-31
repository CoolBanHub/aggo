package utils

import (
	"strings"

	"github.com/google/uuid"
)

func GetUUIDNoDash() string {
	return strings.Replace(uuid.New().String(), "-", "", -1)
}
