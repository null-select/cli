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

func TestRemovePortablePreservesOtherConfiguration(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(project, ".mcp.json")
	original := []byte(`{"other":true,"mcpServers":{"null-select":{"command":"old"},"keep":{"command":"safe"}}}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	options := baseOptions(project)
	options.Now = func() time.Time { return time.Date(2026, 10, 13, 11, 12, 13, 456, time.UTC) }
	result, err := Remove(options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Backup == "" {
		t.Fatalf("unexpected result: %#v", result)
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
	if _, exists := servers["null-select"]; exists {
		t.Fatal("removed entry remains in configuration")
	}
	if _, exists := servers["keep"]; !exists || document["other"] != true {
		t.Fatalf("unrelated configuration was not preserved: %#v", document)
	}
}

func TestRemoveMissingEntryDoesNotRewrite(t *testing.T) {
	project := t.TempDir()
	target := filepath.Join(project, ".mcp.json")
	original := []byte(`{"mcpServers":{"keep":{"command":"safe"}}}`)
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Remove(baseOptions(project))
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.Backup != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, original) {
		t.Fatal("missing-entry removal rewrote configuration")
	}
}

func TestRemoveCodexIsIdempotent(t *testing.T) {
	options := baseOptions(t.TempDir())
	options.Client = "codex"
	options.Scope = "user"
	options.CommandExists = func(string) bool { return true }
	options.CommandProbe = func(string, ...string) error { return errors.New("missing") }
	options.CommandRunner = func(string, ...string) error { t.Fatal("remove command should not run"); return nil }
	result, err := Remove(options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatal("missing Codex entry reported a change")
	}
}

func TestRemoveCodexVerifiesAbsence(t *testing.T) {
	probes := 0
	runs := 0
	options := baseOptions(t.TempDir())
	options.Client = "codex"
	options.Scope = "user"
	options.CommandExists = func(string) bool { return true }
	options.CommandProbe = func(string, ...string) error {
		probes++
		if probes == 1 {
			return nil
		}
		return errors.New("missing")
	}
	options.CommandRunner = func(name string, arguments ...string) error {
		runs++
		if !reflect.DeepEqual(append([]string{name}, arguments...), []string{"codex", "mcp", "remove", "null-select"}) {
			t.Fatalf("unexpected command: %s %#v", name, arguments)
		}
		return nil
	}

	result, err := Remove(options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || probes != 2 || runs != 1 {
		t.Fatalf("result=%#v probes=%d runs=%d", result, probes, runs)
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
	if probes != 2 {
		t.Fatalf("probe called %d times, want 2", probes)
	}
	want := [][]string{
		{"codex", "mcp", "remove", "null-select"},
		{"codex", "mcp", "add", "null-select", "--url", "https://console.null.select/mcp", "--bearer-token-env-var", "NULL_SELECT_CREDENTIAL"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestApplyClaudeCodeUsesDirectHTTPWithEnvironmentReference(t *testing.T) {
	var commands [][]string
	probes := 0
	options := baseOptions(t.TempDir())
	options.Client = "claude-code"
	options.Scope = "project"
	options.CommandExists = func(name string) bool { return name == "claude" }
	options.CommandProbe = func(name string, arguments ...string) error {
		probes++
		if probes == 1 {
			return errors.New("missing")
		}
		return nil
	}
	options.CommandRunner = func(name string, arguments ...string) error {
		commands = append(commands, append([]string{name}, arguments...))
		return nil
	}

	if _, err := Apply(options); err != nil {
		t.Fatal(err)
	}
	if probes != 2 {
		t.Fatalf("probe called %d times, want 2", probes)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %#v", commands)
	}
	command := commands[0]
	if len(command) != 7 || !reflect.DeepEqual(command[:6], []string{"claude", "mcp", "add-json", "--scope", "project", "null-select"}) {
		t.Fatalf("unexpected command: %#v", command)
	}
	var configuration map[string]any
	if err := json.Unmarshal([]byte(command[6]), &configuration); err != nil {
		t.Fatal(err)
	}
	if configuration["type"] != "http" || configuration["url"] != "https://console.null.select/mcp" {
		t.Fatalf("unexpected configuration: %#v", configuration)
	}
	headers := configuration["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer ${NULL_SELECT_CREDENTIAL}" {
		t.Fatalf("unexpected authorization reference: %#v", headers)
	}
	if strings.Contains(command[6], options.Executable) {
		t.Fatal("Claude Code configuration unexpectedly uses the local proxy")
	}
}

func TestJSONClientConfigurationMatrix(t *testing.T) {
	tests := []struct {
		name       string
		client     string
		scope      string
		runtimeOS  string
		targetPath func(string, string) string
	}{
		{name: "cursor-user", client: "cursor", scope: "user", targetPath: func(home, project string) string { return filepath.Join(home, ".cursor", "mcp.json") }},
		{name: "cursor-project", client: "cursor", scope: "project", targetPath: func(home, project string) string { return filepath.Join(project, ".cursor", "mcp.json") }},
		{name: "vscode-user", client: "vscode", scope: "user", targetPath: func(home, project string) string { return filepath.Join(home, ".copilot", "mcp-config.json") }},
		{name: "vscode-project", client: "vscode", scope: "project", targetPath: func(home, project string) string { return filepath.Join(project, ".mcp.json") }},
		{name: "copilot-user", client: "copilot", scope: "user", targetPath: func(home, project string) string { return filepath.Join(home, ".copilot", "mcp-config.json") }},
		{name: "copilot-project", client: "copilot", scope: "project", targetPath: func(home, project string) string { return filepath.Join(project, ".mcp.json") }},
		{name: "portable-project", client: "portable", scope: "project", targetPath: func(home, project string) string { return filepath.Join(project, ".mcp.json") }},
		{name: "claude-desktop-macos", client: "claude-desktop", scope: "user", runtimeOS: "darwin", targetPath: func(home, project string) string {
			return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			options := baseOptions(project)
			options.Client = test.client
			options.Scope = test.scope
			options.HomeDirectory = home
			options.RuntimeOS = test.runtimeOS

			result, err := Apply(options)
			if err != nil {
				t.Fatal(err)
			}
			wantTarget := test.targetPath(home, project)
			if result.Target != wantTarget || !result.Changed {
				t.Fatalf("unexpected result: %#v", result)
			}
			raw, err := os.ReadFile(wantTarget)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			servers, ok := document["mcpServers"].(map[string]any)
			if !ok {
				t.Fatalf("missing portable mcpServers object: %#v", document)
			}
			entry := servers["null-select"].(map[string]any)
			if entry["command"] != "/opt/nullselect/bin/nullselect" {
				t.Fatalf("unexpected entry: %#v", entry)
			}
			arguments := entry["args"].([]any)
			wantArguments := []any{"mcp", "proxy", "--endpoint", "https://console.null.select/mcp", "--credential-env", "NULL_SELECT_CREDENTIAL"}
			if !reflect.DeepEqual(arguments, wantArguments) {
				t.Fatalf("arguments = %#v, want %#v", arguments, wantArguments)
			}
		})
	}
}

func TestUnsupportedClientScopeAndOperatingSystemFailClosed(t *testing.T) {
	portable := baseOptions(t.TempDir())
	portable.Scope = "user"
	if _, err := Apply(portable); err == nil || !strings.Contains(err.Error(), "--scope project") {
		t.Fatalf("unexpected portable error: %v", err)
	}

	desktop := baseOptions(t.TempDir())
	desktop.Client = "claude-desktop"
	desktop.Scope = "user"
	desktop.RuntimeOS = "linux"
	if _, err := Apply(desktop); err == nil || !strings.Contains(err.Error(), "macOS and Windows") {
		t.Fatalf("unexpected Claude Desktop error: %v", err)
	}

	desktop.RuntimeOS = "windows"
	desktop.Environment = func(string) string { return "" }
	if _, err := Apply(desktop); err == nil || !strings.Contains(err.Error(), "APPDATA") {
		t.Fatalf("unexpected Windows error: %v", err)
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
