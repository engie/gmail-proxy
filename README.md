# gmail-proxy

MCP server that gives AI agents restricted access to a Gmail inbox. Runs as a remote service over streamable HTTP so the agent never sees Gmail credentials.

## What it allows

- **Read emails** with a specific label only (label filtered at API level + verified server-side)
- **Create drafts** (including threaded replies) — no sending, no deleting

Everything else is blocked — there are no tools for sending, deleting, modifying, or accessing emails outside the allowed label.

## Security model

- Gmail credentials live on the server only, passed via `GMAIL_TOKEN_JSON` env var
- Agents connect over HTTP and authenticate with a static bearer token
- The agent can only call the 5 defined MCP tools — no access to credentials, no way to escalate
- Run on a separate machine from the agent to prevent credential access via filesystem/process inspection

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `GMAIL_TOKEN_JSON` | Yes | Contents of `token.json` (OAuth refresh token, client ID/secret) |
| `ALLOWED_LABEL` | Yes | Gmail label name to restrict reads to |
| `MCP_AUTH_TOKEN` | Yes | Bearer token agents must present to authenticate |
| `PORT` | No | Listen port (default: 8080) |

## Setup

### 1. Get Google OAuth credentials

Create OAuth credentials in the [Google Cloud Console](https://console.cloud.google.com/apis/credentials) with Gmail API enabled. Download as `client_secret.json`.

### 2. Authorize

```bash
go run ./cmd/reauth
# Opens browser for Google OAuth consent
# Writes token.json with gmail.readonly + gmail.compose scopes
```

### 3. Run the server

```bash
export GMAIL_TOKEN_JSON="$(cat token.json)"
export ALLOWED_LABEL="HOUSE"
export MCP_AUTH_TOKEN="your-secret-token"
go run .
```

### 4. Connect from Claude Code

Add to your MCP config (e.g. `~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "gmail": {
      "type": "http",
      "url": "https://your-server:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-secret-token"
      }
    }
  }
}
```

## MCP tools

| Tool | Description |
|---|---|
| `list_messages` | List emails with the allowed label. Supports pagination and Gmail search queries. |
| `get_message` | Get a single email by ID. Rejects messages without the allowed label. |
| `get_attachment` | Get an attachment. Parent message must have the allowed label. |
| `create_draft` | Create a draft email. Supports replies via `inReplyTo`, `references`, and `threadId`. |
