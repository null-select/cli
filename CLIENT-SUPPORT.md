# MCP client contract matrix

Contract review date: 2026-10-13

This matrix separates implemented configuration targets from customer release
support. A target is customer-supported only after its client and operating
system row has a recorded install, doctor, tool-discovery, revoke, and
uninstall run against a signed `nullselect` release artifact.

| Target | Scope | Connection | Configuration contract | Verification state |
|---|---|---|---|---|
| Codex | user | native Streamable HTTP | `codex mcp add --url` with `--bearer-token-env-var` | Native command contract exercised with Codex CLI 0.162.1; managed doctor/revoke lifecycle proven separately |
| Claude Code | user, project | native HTTP | `claude mcp add-json`; bearer header expands `${NULL_SELECT_CREDENTIAL}` at runtime | Native add/get/remove contract exercised with Claude Code 2.1.296 |
| Cursor | user, project | local stdio bridge | `~/.cursor/mcp.json` or `.cursor/mcp.json`, top-level `mcpServers` | Atomic configuration fixture coverage; live client run required for release support |
| VS Code | user, project | local stdio bridge | `~/.copilot/mcp-config.json` or workspace `.mcp.json`, top-level `mcpServers` | Atomic configuration fixture coverage; live client run required for release support |
| GitHub Copilot | user, project | local stdio bridge | `~/.copilot/mcp-config.json` or workspace `.mcp.json`, top-level `mcpServers` | Atomic configuration fixture coverage; live client run required for release support |
| Claude Desktop | user | local stdio bridge | macOS or Windows `claude_desktop_config.json`, top-level `mcpServers` | Atomic configuration fixture coverage; live client run required for release support |
| Portable | project | local stdio bridge | workspace `.mcp.json`, top-level `mcpServers` | Atomic configuration fixture coverage |

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
