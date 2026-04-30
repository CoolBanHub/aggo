package langfuse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetTextPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RequestURI(); got != "/api/public/v2/prompts/folder%2Fprompt%20name?resolve=false&version=3" {
			t.Fatalf("request uri = %s", got)
		}
		if got := r.URL.Query().Get("version"); got != "3" {
			t.Fatalf("version = %s", got)
		}
		if got := r.URL.Query().Get("resolve"); got != "false" {
			t.Fatalf("resolve = %s", got)
		}
		if got := r.Header.Get("authorization"); got != "Basic cGs6c2s=" {
			t.Fatalf("authorization = %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        "prompt-1",
			"name":      "folder/prompt name",
			"version":   3,
			"type":      PromptTypeText,
			"prompt":    "hello {{name}}",
			"config":    map[string]any{"temperature": 0.1},
			"labels":    []string{"production"},
			"tags":      []string{"agent"},
			"createdBy": "user-1",
			"projectId": "project-1",
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		Host:       server.URL,
		PublicKey:  "pk",
		SecretKey:  "sk",
		FlushAt:    100,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	text, err := client.GetTextPrompt(context.Background(), "folder/prompt name", WithPromptVersion(3), WithPromptResolve(false))
	if err != nil {
		t.Fatalf("GetTextPrompt: %v", err)
	}
	if text != "hello {{name}}" {
		t.Fatalf("text = %q", text)
	}
}

func TestClientGetChatPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("label"); got != "staging" {
			t.Fatalf("label = %s", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    "chat",
			"version": 2,
			"type":    PromptTypeChat,
			"prompt": []map[string]any{
				{"role": "system", "content": "You are concise."},
				{"name": "history", "type": "placeholder"},
			},
			"config": map[string]any{},
			"labels": []string{"staging"},
			"tags":   []string{},
		})
	}))
	defer server.Close()

	client, err := NewClient(ClientConfig{
		Host:       server.URL,
		PublicKey:  "pk",
		SecretKey:  "sk",
		FlushAt:    100,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	prompt, err := client.GetPrompt(context.Background(), "chat", WithPromptLabel("staging"))
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if prompt.Type != PromptTypeChat {
		t.Fatalf("type = %s", prompt.Type)
	}
	if len(prompt.Messages) != 2 {
		t.Fatalf("messages len = %d", len(prompt.Messages))
	}
	if prompt.Messages[0].Role != "system" || prompt.Messages[1].Name != "history" {
		t.Fatalf("messages = %#v", prompt.Messages)
	}
}

func TestClientGetPromptRejectsLabelAndVersion(t *testing.T) {
	client, err := NewClient(ClientConfig{
		Host:      "http://langfuse.test",
		PublicKey: "pk",
		SecretKey: "sk",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if _, err := client.GetPrompt(context.Background(), "prompt", WithPromptLabel("prod"), WithPromptVersion(1)); err == nil {
		t.Fatal("expected error")
	}
}
