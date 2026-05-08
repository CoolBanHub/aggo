package langfuse

import (
	"context"
	"testing"
)

func TestCurrentTraceReturnsConfiguredTrace(t *testing.T) {
	input := map[string]any{"question": "hello"}
	output := map[string]any{"answer": "world"}
	ctx := SetTrace(context.Background(),
		WithID(" trace-1 "),
		WithName(" test-trace "),
		WithUserID(" user-1 "),
		WithSessionID(" session-1 "),
		WithRelease(" release-1 "),
		WithVersion(" version-1 "),
		WithEnvironment(" staging "),
		WithMetadata(map[string]string{"group_id": "group-1"}),
		WithTags(" example ", "", "trace"),
		WithPublic(true),
		WithInput(input),
		WithOutput(output),
	)

	trace := CurrentTrace(ctx)
	if trace.ID != "trace-1" {
		t.Fatalf("ID = %q", trace.ID)
	}
	if trace.Name != "test-trace" {
		t.Fatalf("Name = %q", trace.Name)
	}
	if trace.UserID != "user-1" {
		t.Fatalf("UserID = %q", trace.UserID)
	}
	if trace.SessionID != "session-1" {
		t.Fatalf("SessionID = %q", trace.SessionID)
	}
	if trace.Release != "release-1" {
		t.Fatalf("Release = %q", trace.Release)
	}
	if trace.Version != "version-1" {
		t.Fatalf("Version = %q", trace.Version)
	}
	if trace.Environment != "staging" {
		t.Fatalf("Environment = %q", trace.Environment)
	}
	if trace.Metadata["group_id"] != "group-1" {
		t.Fatalf("Metadata[group_id] = %q", trace.Metadata["group_id"])
	}
	if len(trace.Tags) != 2 || trace.Tags[0] != "example" || trace.Tags[1] != "trace" {
		t.Fatalf("Tags = %#v", trace.Tags)
	}
	if trace.Public == nil || !*trace.Public {
		t.Fatalf("Public = %#v", trace.Public)
	}
	gotInput, ok := trace.Input.(map[string]any)
	if !ok || gotInput["question"] != "hello" {
		t.Fatalf("Input = %#v", trace.Input)
	}
	gotOutput, ok := trace.Output.(map[string]any)
	if !ok || gotOutput["answer"] != "world" {
		t.Fatalf("Output = %#v", trace.Output)
	}
	input["question"] = "changed"
	if gotInput["question"] != "changed" {
		t.Fatalf("Input was unexpectedly copied: %#v", trace.Input)
	}
	output["answer"] = "changed"
	if gotOutput["answer"] != "changed" {
		t.Fatalf("Output was unexpectedly copied: %#v", trace.Output)
	}
}

func TestCurrentTraceReturnsCopies(t *testing.T) {
	ctx := SetTrace(context.Background(),
		WithMetadata(map[string]string{"group_id": "group-1"}),
		WithTags("example"),
		WithPublic(true),
	)

	trace := CurrentTrace(ctx)
	trace.Metadata["group_id"] = "changed"
	trace.Tags[0] = "changed"
	*trace.Public = false

	trace = CurrentTrace(ctx)
	if trace.Metadata["group_id"] != "group-1" {
		t.Fatalf("Metadata[group_id] = %q", trace.Metadata["group_id"])
	}
	if len(trace.Tags) != 1 || trace.Tags[0] != "example" {
		t.Fatalf("Tags = %#v", trace.Tags)
	}
	if trace.Public == nil || !*trace.Public {
		t.Fatalf("Public = %#v", trace.Public)
	}
}

func TestCurrentTraceEmptyWhenUnset(t *testing.T) {
	trace := CurrentTrace(context.Background())
	if trace.ID != "" ||
		trace.Name != "" ||
		trace.UserID != "" ||
		trace.SessionID != "" ||
		trace.Release != "" ||
		trace.Version != "" ||
		trace.Environment != "" ||
		trace.Metadata != nil ||
		trace.Tags != nil ||
		trace.Public != nil ||
		trace.Input != nil ||
		trace.Output != nil {
		t.Fatalf("trace = %#v", trace)
	}
}
