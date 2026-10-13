# MCP client contract matrix

Contract review date: 2026-10-13

This matrix separates implemented configuration targets from customer release
support. A target is customer-supported only after its client and operating
system row has a recorded install, doctor, tool-discovery, revoke, and
uninstall run against a signed `nullselect` release artifact.

The published binary matrix is Linux AMD64 and Linux ARM64. Darwin and Windows
cross-builds are verification targets only and are not published as customer
downloads.

| Target | Scope | Connection | Configuration contract | Verification state |
|---|---|---|---|---|
| Codex | user | native Streamable HTTP | `codex mcp add --url` with `--bearer-token-env-var` | Linux release artifact installed, verified, and removed the native configuration with Codex CLI 0.162.1; the same release completed managed doctor/revoke checks on AMD64 and ARM64 |
| Claude Code | user, project | native HTTP | `claude mcp add-json`; bearer header expands `${NULL_SELECT_CREDENTIAL}` at runtime | Linux ARM64 release artifact installed a live connected configuration and removed it with Claude Code 2.1.296; managed doctor/revoke also passed on both published architectures |
| Cursor | user, project | local stdio bridge | `~/.cursor/mcp.json` or `.cursor/mcp.json`, top-level `mcpServers` | Linux release artifact install/remove and atomic fixture coverage pass; live Cursor application run required for client support |
| VS Code | user, project | local stdio bridge | `~/.copilot/mcp-config.json` or workspace `.mcp.json`, top-level `mcpServers` | Linux release artifact install/remove and atomic fixture coverage pass; live VS Code application run required for client support |
| GitHub Copilot | user, project | local stdio bridge | `~/.copilot/mcp-config.json` or workspace `.mcp.json`, top-level `mcpServers` | Linux release artifact install/remove and atomic fixture coverage pass; live Copilot application run required for client support |
| Claude Desktop | user | local stdio bridge | macOS or Windows `claude_desktop_config.json`, top-level `mcpServers` | Atomic configuration fixture coverage; live client run required for release support |
| Portable | project | local stdio bridge | workspace `.mcp.json`, top-level `mcpServers` | Linux release artifact install/remove and atomic configuration fixture coverage pass |

Claude Desktop is rejected on Linux because Anthropic distributes and
documents the desktop application for macOS and Windows. The installer does
not invent a Linux configuration path.

VS Code writes the portable locations recommended by its current
documentation. It does not create the older `.vscode/mcp.json` form. Cursor
continues to use its documented Cursor-specific locations.

## Credential boundary

- Codex stores the credential environment-variable name through its native
  bearer-token option.
- Claude Code stores `Bearer ${NULL_SELECT_CREDENTIAL}` and expands it at
  runtime.
- Stdio targets store the absolute `nullselect` executable path and the
  credential environment-variable name. The proxy reads the value from its
  inherited environment.
- No installer accepts a credential value as an argument or writes one to a
  client configuration file.

## Contract sources

- OpenAI Codex configuration reference:
  <https://developers.openai.com/codex/config-reference>
- Anthropic Claude Code MCP guide:
  <https://docs.anthropic.com/en/docs/claude-code/mcp>
- Anthropic Claude Desktop local MCP guide:
  <https://support.anthropic.com/en/articles/10949351-getting-started-with-local-mcp-servers-on-claude-desktop>
- Cursor MCP guide:
  <https://docs.cursor.com/context/model-context-protocol>
- VS Code MCP server guide:
  <https://code.visualstudio.com/docs/agent-customization/mcp-servers>
- VS Code MCP configuration reference:
  <https://code.visualstudio.com/docs/agents/reference/mcp-configuration>

The matrix must be rechecked when a client changes its MCP configuration
contract or before adding a client/version pair to customer documentation.
