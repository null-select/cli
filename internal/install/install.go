package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const maximumConfigurationBytes = 1 << 20

var (
	serverNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	environmentPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)
)

type Options struct {
	Client           string
	Scope            string
	Name             string
	Endpoint         string
	CredentialEnv    string
	Executable       string
	HomeDirectory    string
	ProjectDirectory string
	Force            bool
	DryRun           bool
	Stdout           io.Writer
	Stderr           io.Writer
	CommandRunner    func(string, ...string) error
	CommandProbe     func(string, ...string) error
	CommandExists    func(string) bool
	Now              func() time.Time
}

type Result struct {
	Client        string `json:"client"`
	Scope         string `json:"scope"`
	Name          string `json:"name"`
	Endpoint      string `json:"endpoint"`
	CredentialEnv string `json:"credential_env"`
	Target        string `json:"target"`
	Backup        string `json:"backup,omitempty"`
	DryRun        bool   `json:"dry_run"`
}

func Apply(options Options) (Result, error) {
	options = defaults(options)
	if err := validate(options); err != nil {
		return Result{}, err
	}
	switch options.Client {
	case "codex":
		return installCodex(options)
	case "claude-code":
		return installClaudeCode(options)
	case "cursor", "vscode", "claude-desktop", "copilot", "portable":
		return installJSON(options)
	default:
		return Result{}, fmt.Errorf("unsupported client %q", options.Client)
	}
}

func defaults(options Options) Options {
	if options.Scope == "" {
		options.Scope = "user"
	}
	if options.Name == "" {
		options.Name = "null-select"
	}
	if options.Endpoint == "" {
		options.Endpoint = "https://console.null.select/mcp"
	}
	if options.CredentialEnv == "" {
		options.CredentialEnv = "NULL_SELECT_CREDENTIAL"
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	if options.CommandRunner == nil {
		options.CommandRunner = runCommand(options.Stdout, options.Stderr)
	}
	if options.CommandProbe == nil {
		options.CommandProbe = runCommand(io.Discard, io.Discard)
	}
	if options.CommandExists == nil {
		options.CommandExists = func(name string) bool {
			_, err := exec.LookPath(name)
			return err == nil
		}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return options
}

func validate(options Options) error {
	if !serverNamePattern.MatchString(options.Name) {
		return errors.New("server name must contain only letters, numbers, dots, underscores, and hyphens")
	}
	if !environmentPattern.MatchString(options.CredentialEnv) {
		return errors.New("credential environment variable is invalid")
	}
	parsed, err := url.Parse(options.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("endpoint must be an absolute HTTPS URL without user information, query parameters, or a fragment")
	}
	if options.Executable == "" || !filepath.IsAbs(options.Executable) {
		return errors.New("the nullselect executable path must be absolute")
	}
	if options.HomeDirectory == "" || !filepath.IsAbs(options.HomeDirectory) {
		return errors.New("home directory must be absolute")
	}
	if options.ProjectDirectory == "" || !filepath.IsAbs(options.ProjectDirectory) {
		return errors.New("project directory must be absolute")
	}
	if options.Scope != "user" && options.Scope != "project" {
		return errors.New("scope must be user or project")
	}
	return nil
}

func installCodex(options Options) (Result, error) {
	if options.Scope != "user" {
		return Result{}, errors.New("Codex stores MCP configuration at user scope; use --scope user")
	}
	if !options.CommandExists("codex") {
		return Result{}, errors.New("Codex CLI was not found on PATH")
	}
	result := baseResult(options, "codex mcp configuration")
	if options.DryRun {
		return result, nil
	}
	if err := options.CommandProbe("codex", "mcp", "get", options.Name, "--json"); err == nil {
		if !options.Force {
			return Result{}, fmt.Errorf("MCP server %q already exists in Codex; use --force to replace it", options.Name)
		}
		if err := options.CommandRunner("codex", "mcp", "remove", options.Name); err != nil {
			return Result{}, errors.New("remove existing Codex MCP server")
		}
	}
	if err := options.CommandRunner(
		"codex", "mcp", "add", options.Name, "--url", options.Endpoint,
		"--bearer-token-env-var", options.CredentialEnv,
	); err != nil {
		return Result{}, errors.New("add Codex MCP server")
	}
	return result, nil
}

func installClaudeCode(options Options) (Result, error) {
	if !options.CommandExists("claude") {
		return Result{}, errors.New("Claude Code CLI was not found on PATH")
	}
	result := baseResult(options, "claude mcp configuration")
	if options.DryRun {
		return result, nil
	}
	if err := options.CommandProbe("claude", "mcp", "get", options.Name); err == nil {
		if !options.Force {
			return Result{}, fmt.Errorf("MCP server %q already exists in Claude Code; use --force to replace it", options.Name)
		}
		if err := options.CommandRunner("claude", "mcp", "remove", "--scope", claudeScope(options.Scope), options.Name); err != nil {
			return Result{}, errors.New("remove existing Claude Code MCP server")
		}
	}
	arguments := []string{
		"mcp", "add", "--scope", claudeScope(options.Scope), options.Name, "--",
		options.Executable, "mcp", "proxy", "--endpoint", options.Endpoint,
		"--credential-env", options.CredentialEnv,
	}
	if err := options.CommandRunner("claude", arguments...); err != nil {
		return Result{}, errors.New("add Claude Code MCP server")
	}
	return result, nil
}

func claudeScope(scope string) string {
	if scope == "project" {
		return "project"
	}
	return "user"
}

func installJSON(options Options) (Result, error) {
	target, topLevel, err := configurationTarget(options)
	if err != nil {
		return Result{}, err
	}
	entry := map[string]any{
		"command": options.Executable,
		"args": []string{
			"mcp", "proxy", "--endpoint", options.Endpoint,
			"--credential-env", options.CredentialEnv,
		},
	}
	if options.Client == "vscode" {
		entry["type"] = "stdio"
	}
	result := baseResult(options, target)
	if options.DryRun {
		return result, nil
	}
	backup, err := mergeJSONConfiguration(target, topLevel, options.Name, entry, options.Force, options.Now())
	if err != nil {
		return Result{}, err
	}
	result.Backup = backup
	return result, nil
}

func configurationTarget(options Options) (string, string, error) {
	switch options.Client {
	case "cursor":
		if options.Scope == "project" {
			return filepath.Join(options.ProjectDirectory, ".cursor", "mcp.json"), "mcpServers", nil
		}
		return filepath.Join(options.HomeDirectory, ".cursor", "mcp.json"), "mcpServers", nil
	case "vscode":
		if options.Scope != "project" {
			return "", "", errors.New("VS Code installation requires --scope project; use --client copilot for user scope")
		}
		return filepath.Join(options.ProjectDirectory, ".vscode", "mcp.json"), "servers", nil
	case "claude-desktop":
		if options.Scope != "user" {
			return "", "", errors.New("Claude Desktop installation requires --scope user")
		}
		target, err := claudeDesktopPath(options.HomeDirectory)
		return target, "mcpServers", err
	case "copilot":
		if options.Scope == "project" {
			return filepath.Join(options.ProjectDirectory, ".mcp.json"), "mcpServers", nil
		}
		return filepath.Join(options.HomeDirectory, ".copilot", "mcp-config.json"), "mcpServers", nil
	case "portable":
		if options.Scope == "project" {
			return filepath.Join(options.ProjectDirectory, ".mcp.json"), "mcpServers", nil
		}
		return filepath.Join(options.HomeDirectory, ".mcp.json"), "mcpServers", nil
	default:
		return "", "", fmt.Errorf("unsupported JSON client %q", options.Client)
	}
}

func claudeDesktopPath(home string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", errors.New("APPDATA is required for Claude Desktop on Windows")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
	default:
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "Claude", "claude_desktop_config.json"), nil
	}
}

func mergeJSONConfiguration(target, topLevel, name string, entry map[string]any, force bool, now time.Time) (string, error) {
	document := make(map[string]any)
	mode := fs.FileMode(0o600)
	raw, err := os.ReadFile(target)
	if err == nil {
		if len(raw) > maximumConfigurationBytes {
			return "", errors.New("existing MCP configuration exceeds 1 MiB")
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &document); err != nil {
				return "", errors.New("existing MCP configuration is not valid JSON")
			}
		}
		if document == nil {
			document = make(map[string]any)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("read existing MCP configuration")
	}
	servers, ok := document[topLevel].(map[string]any)
	if document[topLevel] != nil && !ok {
		return "", fmt.Errorf("existing %s field is not an object", topLevel)
	}
	if servers == nil {
		servers = make(map[string]any)
		document[topLevel] = servers
	}
	if _, exists := servers[name]; exists && !force {
		return "", fmt.Errorf("MCP server %q already exists in %s; use --force to replace it", name, target)
	}
	servers[name] = entry
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", errors.New("encode MCP configuration")
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", errors.New("create MCP configuration directory")
	}
	backup := ""
	if len(raw) > 0 {
		backup = target + ".bak." + now.UTC().Format("20060102T150405.000000000Z")
		if err := writeExclusive(backup, raw, 0o600); err != nil {
			return "", errors.New("back up existing MCP configuration")
		}
	}
	if err := writeAtomic(target, encoded, mode); err != nil {
		return "", err
	}
	return backup, nil
}

func writeExclusive(path string, data []byte, mode fs.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, bytes.NewReader(data)); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func writeAtomic(target string, data []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".nullselect-mcp-*")
	if err != nil {
		return errors.New("create temporary MCP configuration")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return errors.New("secure temporary MCP configuration")
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return errors.New("write temporary MCP configuration")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return errors.New("sync temporary MCP configuration")
	}
	if err := temporary.Close(); err != nil {
		return errors.New("close temporary MCP configuration")
	}
	if err := replaceFile(temporaryPath, target); err != nil {
		return errors.New("replace MCP configuration")
	}
	if err := syncParentDirectory(filepath.Dir(target)); err != nil {
		return errors.New("sync MCP configuration directory")
	}
	return nil
}

func baseResult(options Options, target string) Result {
	return Result{
		Client: options.Client, Scope: options.Scope, Name: options.Name,
		Endpoint: options.Endpoint, CredentialEnv: options.CredentialEnv,
		Target: target, DryRun: options.DryRun,
	}
}

func runCommand(stdout, stderr io.Writer) func(string, ...string) error {
	return func(name string, arguments ...string) error {
		command := exec.Command(name, arguments...)
		command.Stdout = stdout
		command.Stderr = stderr
		return command.Run()
	}
}

func SupportedClients() string {
	return strings.Join([]string{"codex", "claude-code", "cursor", "vscode", "claude-desktop", "copilot", "portable"}, ", ")
}
