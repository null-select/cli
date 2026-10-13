# null.select CLI

`nullselect` connects MCP clients to the null.select managed gateway without
putting gateway credentials in client configuration files.

## Commands

```text
nullselect version [--json]
nullselect mcp install --client <client> [options]
nullselect mcp uninstall --client <client> [options]
nullselect mcp doctor [options]
nullselect mcp proxy [options]
```

Implemented install targets are Codex, Claude Code, Cursor, VS Code, Claude
Desktop, GitHub Copilot, and the portable `.mcp.json` format. The exact scope,
transport, configuration location, and verification level for each target are
recorded in [CLIENT-SUPPORT.md](CLIENT-SUPPORT.md).

The installer uses the native client command when the client exposes a suitable
configuration interface. Codex and Claude Code connect to the managed HTTPS
endpoint directly. Other clients receive a stdio entry that launches the local
proxy. Existing entries are never replaced unless `--force` is supplied.
JSON configuration changes are written atomically with mode `0600`; an existing
file is backed up before replacement.

## Credential handling

Set the gateway credential in an environment variable before using the client:

```sh
export NULL_SELECT_CREDENTIAL='the credential issued by null.select'
```

The CLI accepts the environment variable's name, never the credential itself,
as a command-line argument. The installer writes only that variable name to
configuration. `mcp doctor` and `mcp proxy` read its value at runtime. This
keeps the credential out of process arguments and client configuration files.

## Install a client connection

```sh
nullselect mcp install --client codex
nullselect mcp install --client claude-code --scope user
nullselect mcp install --client cursor --scope project
nullselect mcp install --client vscode --scope project
nullselect mcp install --client claude-desktop
nullselect mcp install --client copilot --scope user
nullselect mcp install --client portable --scope project
```

VS Code uses workspace `.mcp.json` for project scope and
`~/.copilot/mcp-config.json` for user scope. Claude Desktop configuration is
accepted only on macOS and Windows. Portable configuration is workspace-scoped.

Use `--dry-run --json` to validate a target and inspect the resolved destination
without changing it. Use `--force` only when replacing the same named entry is
intentional.

Verify authentication, protocol negotiation, and the available tool list:

```sh
nullselect mcp doctor
```

Remove only the named client entry and retain a backup of changed JSON
configuration:

```sh
nullselect mcp uninstall --client cursor --scope project
```

## Build and verify

Go 1.26.9 or newer is required.

```sh
make verify
make build
make cross-build
```

`make cross-build` creates stripped, reproducible binaries and a SHA-256
checksum file under `dist/` for Darwin, Linux, and Windows on AMD64 and ARM64.
The release workflow creates an attested draft release candidate. It does not
make unsigned macOS or Windows binaries public.

## Security properties

- Gateway endpoints must use HTTPS and cannot contain user information, query
  parameters, or fragments.
- HTTP connections require TLS 1.2 or newer and use bounded connection and
  response-header timeouts.
- MCP events and stdio frames are capped at 8 MiB.
- The proxy refuses unnamed tools, duplicate names, and tool sets above 500.
- Probe command output is discarded so existing client configuration is not
  copied into logs.
- Errors from client installers do not echo command arguments or credential
  values.
