package command

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	want := Build{Version: "1.2.3", Commit: "abc123", BuiltAt: "2026-10-13T10:00:00Z"}
	if err := Run(context.Background(), []string{"version", "--json"}, &stdout, &stderr, want); err != nil {
		t.Fatal(err)
	}
	var got Build
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestInstallDryRunDoesNotRequireCredential(t *testing.T) {
	var stdout bytes.Buffer
	err := Run(context.Background(), []string{
		"mcp", "install", "--client", "portable", "--scope", "project", "--dry-run", "--json",
	}, &stdout, &bytes.Buffer{}, Build{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"dry_run":true`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestDoctorRequiresCredentialEnvironment(t *testing.T) {
	t.Setenv("NULL_SELECT_TEST_CREDENTIAL", "")
	err := Run(context.Background(), []string{
		"mcp", "doctor", "--credential-env", "NULL_SELECT_TEST_CREDENTIAL",
	}, &bytes.Buffer{}, &bytes.Buffer{}, Build{Version: "test"})
	if err == nil || !strings.Contains(err.Error(), "NULL_SELECT_TEST_CREDENTIAL") {
		t.Fatalf("unexpected error: %v", err)
	}
}
