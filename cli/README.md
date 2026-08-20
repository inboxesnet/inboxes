# inboxes

Connect coding agents (Claude Code, Codex, opencode) to your
[Inboxes](https://github.com/inboxesnet/inboxes) email server over MCP.

```
npx inboxes setup
```

You do not need to create an API key first — setup creates its own. What
the command does, in order:

1. It opens your browser on your Inboxes server.
2. You sign in, if needed, and click Approve. If you do not approve,
   nothing is created.
3. It creates one new API key for this machine, named `cli-<hostname>`.
   Your existing keys do not change.
4. It writes the MCP configuration for every harness it finds on your
   machine.

Agents can then read your mail and prepare drafts for you to review.
Sending stays off until an org admin enables it in Settings → Agents.

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
