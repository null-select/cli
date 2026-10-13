package install

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplyPortableMergesAndBacksUpConfiguration(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	target := filepath.Join(project, ".mcp.json")
	original := []byte("{\n  \"other\": true,\n  \"mcpServers\": {\"existing\": {\"command\": \"safe\"}}\n}\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 10, 11, 12, 123, time.UTC)

	result, err := Apply(Options{
		Client:           "portable",
		Scope:            "project",
		Name:             "company-tools",
		Endpoint:         "https://console.null.select/mcp",
		CredentialEnv:    "NULL_SELECT_CREDENTIAL",
		Executable:       "/opt/nullselect/bin/nullselect",
		HomeDirectory:    home,
		ProjectDirectory: project,
		Now:              func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	wantBackup := target + ".bak.20261013T101112.000000123Z"
	if result.Target != target || result.Backup != wantBackup {
		t.Fatalf("unexpected result: %#v", result)
	}
	backup, err := os.ReadFile(wantBackup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backup, original) {
		t.Fatal("backup does not match the original configuration")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	servers := document["mcpServers"].(map[string]any)
	entry := servers["company-tools"].(map[string]any)
	if entry["command"] != "/opt/nullselect/bin/nullselect" {
		t.Fatalf("unexpected command: %v", entry["command"])
	}
	if strings.Contains(string(raw), "secret-value") {
		t.Fatal("configuration contains credential material")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration mode is %o, want 600", info.Mode().Perm())
	}
}

func TestApplyRefusesDuplicateWithoutForce(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(project, ".mcp.json")
	original := []byte(`{"mcpServers":{"null-select":{"command":"existing"}}}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Apply(baseOptions(project))
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatal("configuration changed after duplicate refusal")
	}
}

func TestApplyAcceptsEmptyConfiguration(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(baseOptions(project)); err != nil {
		t.Fatal(err)
	}
}

func TestApplyCodexUsesSilentProbeAndEnvironmentReference(t *testing.T) {
	var commands [][]string
	probes := 0
	options := baseOptions(t.TempDir())
	options.Client = "codex"
	options.Scope = "user"
	options.Force = true
	options.CommandExists = func(name string) bool { return name == "codex" }
	options.CommandProbe = func(name string, arguments ...string) error {
		probes++
		return nil
	}
	options.CommandRunner = func(name string, arguments ...string) error {
		commands = append(commands, append([]string{name}, arguments...))
		return nil
	}

	if _, err := Apply(options); err != nil {
		t.Fatal(err)
	}
	if probes != 1 {
		t.Fatalf("probe called %d times, want 1", probes)
	}
	want := [][]string{
		{"codex", "mcp", "remove", "null-select"},
		{"codex", "mcp", "add", "null-select", "--url", "https://console.null.select/mcp", "--bearer-token-env-var", "NULL_SELECT_CREDENTIAL"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestApplyRejectsUnsafeEndpoint(t *testing.T) {
	options := baseOptions(t.TempDir())
	options.Endpoint = "https://console.null.select/mcp?token=secret-value"
	_, err := Apply(options)
	if err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyReportsCommandFailureWithoutArguments(t *testing.T) {
	options := baseOptions(t.TempDir())
	options.Client = "codex"
	options.Scope = "user"
	options.CommandExists = func(string) bool { return true }
	options.CommandProbe = func(string, ...string) error { return errors.New("not found") }
	options.CommandRunner = func(string, ...string) error { return errors.New("secret-value") }
	_, err := Apply(options)
	if err == nil || err.Error() != "add Codex MCP server" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func baseOptions(project string) Options {
	return Options{
		Client:           "portable",
		Scope:            "project",
		Name:             "null-select",
		Endpoint:         "https://console.null.select/mcp",
		CredentialEnv:    "NULL_SELECT_CREDENTIAL",
		Executable:       "/opt/nullselect/bin/nullselect",
		HomeDirectory:    filepath.Dir(project),
		ProjectDirectory: project,
	}
}
