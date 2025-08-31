package model

import (
	"context"
	"testing"
)

func TestGetEmb(t *testing.T) {
	e, err := GetEmbByText(context.Background(), "hello")
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(e)
}
