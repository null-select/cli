package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/null-select/cli/internal/bridge"
	"github.com/null-select/cli/internal/doctor"
	"github.com/null-select/cli/internal/install"
)

const defaultEndpoint = "https://console.null.select/mcp"

type Build struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

func Run(ctx context.Context, arguments []string, stdout, stderr io.Writer, build Build) error {
	if len(arguments) == 0 {
		writeUsage(stdout)
		return nil
	}
	switch arguments[0] {
	case "help", "-h", "--help":
		writeUsage(stdout)
		return nil
	case "version":
		return runVersion(arguments[1:], stdout, stderr, build)
	case "mcp":
		return runMCP(ctx, arguments[1:], stdout, stderr, build)
	default:
		return fmt.Errorf("unknown command %q; run nullselect help", arguments[0])
	}
}

func runVersion(arguments []string, stdout, stderr io.Writer, build Build) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(arguments); err != nil {
		return flagError(err)
	}
	if flags.NArg() != 0 {
		return errors.New("version does not accept positional arguments")
	}
	if *jsonOutput {
		return writeJSON(stdout, build)
	}
	_, err := fmt.Fprintf(stdout, "nullselect %s (%s, built %s)\n", build.Version, build.Commit, build.BuiltAt)
	return err
}

func runMCP(ctx context.Context, arguments []string, stdout, stderr io.Writer, build Build) error {
	if len(arguments) == 0 {
		writeMCPUsage(stdout)
		return nil
	}
	switch arguments[0] {
	case "help", "-h", "--help":
		writeMCPUsage(stdout)
		return nil
	case "install":
		return runInstall(arguments[1:], stdout, stderr)
	case "uninstall":
		return runUninstall(arguments[1:], stdout, stderr)
	case "doctor":
		return runDoctor(ctx, arguments[1:], stdout, stderr, build)
	case "proxy":
		return runProxy(ctx, arguments[1:], stderr, build)
	default:
		return fmt.Errorf("unknown mcp command %q; run nullselect mcp help", arguments[0])
	}
}

func runInstall(arguments []string, stdout, stderr io.Writer) error {
	executable, home, project, err := localPaths()
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("mcp install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := flags.String("client", "", "target MCP client")
	scope := flags.String("scope", "user", "configuration scope: user or project")
	name := flags.String("name", "null-select", "MCP server name")
	endpoint := flags.String("endpoint", defaultEndpoint, "managed gateway HTTPS endpoint")
	credentialEnv := flags.String("credential-env", "NULL_SELECT_CREDENTIAL", "environment variable containing the credential")
	projectDirectory := flags.String("project-dir", project, "project directory for project-scoped configuration")
	force := flags.Bool("force", false, "replace an existing entry with the same name")
	dryRun := flags.Bool("dry-run", false, "validate and report the target without changing it")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(arguments); err != nil {
		return flagError(err)
	}
	if flags.NArg() != 0 {
		return errors.New("mcp install does not accept positional arguments")
	}
	if *client == "" {
		return fmt.Errorf("--client is required; supported clients: %s", install.SupportedClients())
	}
	result, err := install.Apply(install.Options{
		Client:           strings.ToLower(*client),
		Scope:            strings.ToLower(*scope),
		Name:             *name,
		Endpoint:         *endpoint,
		CredentialEnv:    *credentialEnv,
		Executable:       executable,
		HomeDirectory:    home,
		ProjectDirectory: *projectDirectory,
		Force:            *force,
		DryRun:           *dryRun,
		Stdout:           stdout,
		Stderr:           stderr,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	action := "Installed"
	if result.DryRun {
		action = "Validated"
	}
	if _, err := fmt.Fprintf(stdout, "%s %s for %s scope at %s\n", action, result.Name, result.Scope, result.Target); err != nil {
		return err
	}
	if result.Backup != "" {
		_, err = fmt.Fprintf(stdout, "Backup: %s\n", result.Backup)
	}
	return err
}

func runUninstall(arguments []string, stdout, stderr io.Writer) error {
	executable, home, project, err := localPaths()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("mcp uninstall", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := flags.String("client", "", "target MCP client")
	scope := flags.String("scope", "user", "configuration scope: user or project")
	name := flags.String("name", "null-select", "MCP server name")
	projectDirectory := flags.String("project-dir", project, "project directory for project-scoped configuration")
	dryRun := flags.Bool("dry-run", false, "validate and report the target without changing it")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(arguments); err != nil {
		return flagError(err)
	}
	if flags.NArg() != 0 {
		return errors.New("mcp uninstall does not accept positional arguments")
	}
	if *client == "" {
		return fmt.Errorf("--client is required; supported clients: %s", install.SupportedClients())
	}
	result, err := install.Remove(install.Options{
		Client:           strings.ToLower(*client),
		Scope:            strings.ToLower(*scope),
		Name:             *name,
		Executable:       executable,
		HomeDirectory:    home,
		ProjectDirectory: *projectDirectory,
		DryRun:           *dryRun,
		Stdout:           stdout,
		Stderr:           stderr,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, result)
	}
	if result.DryRun {
		_, err = fmt.Fprintf(stdout, "Validated %s for %s scope at %s\n", result.Name, result.Scope, result.Target)
		return err
	}
	if !result.Changed {
		_, err = fmt.Fprintf(stdout, "No %s entry found for %s scope at %s\n", result.Name, result.Scope, result.Target)
		return err
	}
	if _, err = fmt.Fprintf(stdout, "Removed %s for %s scope at %s\n", result.Name, result.Scope, result.Target); err != nil {
		return err
	}
	if result.Backup != "" {
		_, err = fmt.Fprintf(stdout, "Backup: %s\n", result.Backup)
	}
	return err
}

func runDoctor(ctx context.Context, arguments []string, stdout, stderr io.Writer, build Build) error {
	flags := flag.NewFlagSet("mcp doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", defaultEndpoint, "managed gateway HTTPS endpoint")
	credentialEnv := flags.String("credential-env", "NULL_SELECT_CREDENTIAL", "environment variable containing the credential")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(arguments); err != nil {
		return flagError(err)
	}
	if flags.NArg() != 0 {
		return errors.New("mcp doctor does not accept positional arguments")
	}
	credential, err := credentialFromEnvironment(*credentialEnv)
	if err != nil {
		return err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	report, err := doctor.Check(checkCtx, *endpoint, credential, build.Version, time.Now())
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(stdout, report)
	}
	_, err = fmt.Fprintf(
		stdout,
		"Connected to %s\nServer: %s %s\nProtocol: %s\nTools: %d\n",
		report.Endpoint,
		report.ServerName,
		report.ServerVersion,
		report.ProtocolVersion,
		report.ToolCount,
	)
	return err
}

func runProxy(ctx context.Context, arguments []string, stderr io.Writer, build Build) error {
	flags := flag.NewFlagSet("mcp proxy", flag.ContinueOnError)
	flags.SetOutput(stderr)
	endpoint := flags.String("endpoint", defaultEndpoint, "managed gateway HTTPS endpoint")
	credentialEnv := flags.String("credential-env", "NULL_SELECT_CREDENTIAL", "environment variable containing the credential")
	if err := flags.Parse(arguments); err != nil {
		return flagError(err)
	}
	if flags.NArg() != 0 {
		return errors.New("mcp proxy does not accept positional arguments")
	}
	credential, err := credentialFromEnvironment(*credentialEnv)
	if err != nil {
		return err
	}
	proxy, err := bridge.New(ctx, *endpoint, credential, build.Version)
	if err != nil {
		return err
	}
	defer proxy.Close()
	return proxy.Serve(ctx, &mcp.StdioTransport{MaxLineLength: 8 << 20})
}

func credentialFromEnvironment(name string) (string, error) {
	if name == "" {
		return "", errors.New("credential environment variable name is empty")
	}
	credential, exists := os.LookupEnv(name)
	if !exists || strings.TrimSpace(credential) == "" {
		return "", fmt.Errorf("credential environment variable %s is not set", name)
	}
	return credential, nil
}

func filepathAbs(path string) (string, error) {
	return filepath.Abs(path)
}

func localPaths() (string, string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", "", errors.New("locate the nullselect executable")
	}
	executable, err = filepathAbs(executable)
	if err != nil {
		return "", "", "", errors.New("resolve the nullselect executable path")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", errors.New("locate the user home directory")
	}
	project, err := os.Getwd()
	if err != nil {
		return "", "", "", errors.New("locate the project directory")
	}
	return executable, home, project, nil
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func flagError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func writeUsage(writer io.Writer) {
	fmt.Fprint(writer, `nullselect manages access to the null.select MCP gateway.

Usage:
  nullselect version [--json]
  nullselect mcp <command>

Run "nullselect mcp help" for MCP commands.
`)
}

func writeMCPUsage(writer io.Writer) {
	fmt.Fprint(writer, `Usage:
  nullselect mcp install --client <client> [options]
  nullselect mcp uninstall --client <client> [options]
  nullselect mcp doctor [options]
  nullselect mcp proxy [options]

Clients:
  codex, claude-code, cursor, vscode, claude-desktop, copilot, portable

Credentials are read from NULL_SELECT_CREDENTIAL by default and are never
written to MCP client configuration files.
`)
}
