package postgres

import "testing"

func TestValidatePostgresIdentifier(t *testing.T) {
	valid := []string{"aggo_vectors", "public.aggo_vectors", "_tenant1.table_2"}
	for _, name := range valid {
		if err := validatePostgresIdentifier(name); err != nil {
			t.Fatalf("validatePostgresIdentifier(%q) returned error: %v", name, err)
		}
	}

	invalid := []string{"bad-name", "public.bad;drop", "a.b.c", "1table", `"quoted"`}
	for _, name := range invalid {
		if err := validatePostgresIdentifier(name); err == nil {
			t.Fatalf("validatePostgresIdentifier(%q) succeeded, want error", name)
		}
	}
}

func TestQuotePostgresIdentifier(t *testing.T) {
	got := quotePostgresIdentifier("public.aggo_vectors")
	want := `"public"."aggo_vectors"`
	if got != want {
		t.Fatalf("quotePostgresIdentifier = %q, want %q", got, want)
	}
}
