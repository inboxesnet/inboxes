# inboxes

Connect coding agents (Claude Code, Codex, opencode) to your
[Inboxes](https://github.com/inboxesnet/inboxes) email server over MCP.

```
npx inboxes setup
```

The command opens your browser, you sign in and approve, and it writes the
MCP configuration for every harness it finds on your machine. Agents can then
read your mail and prepare drafts for you to review. Sending stays off until
an org admin enables it in Settings → Agents.

Options:

| Flag | Meaning |
|---|---|
| `--url <server>` | Self-hosted server URL (default: https://app.inboxes.net) |
| `--key <key>` | Use an existing API key, skip the browser flow |
| `--name <name>` | Name for the created key (shown in Settings → Agents) |
| `--yes` | Accept defaults, no prompts |
| `--no-browser` | Print the approval URL instead of opening a browser |

Keys are revocable anytime in Settings → Agents. See `docs/mcp.md` in the
main repository for the full MCP server documentation.
