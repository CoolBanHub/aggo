package utils

import "github.com/oklog/ulid/v2"

func GetULID() string {
	id := ulid.Make()
	return id.String()
}
