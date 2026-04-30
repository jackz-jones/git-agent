# Git Agent Web Mode Guide

[🏠 README](../README.md) | [📖 Usage Guide](USAGE.md) | [🔧 Tuning Guide](TUNING_en.md)

Since v1.0, Git Agent provides a local Web UI in addition to the CLI, ideal for users who prefer graphical interfaces.

## 1. Starting the Web Server

```bash
# One-click build (requires Node.js 18+ installed locally)
make build
./git-agent serve

# Specify port / disable auto-open browser
./git-agent serve --port 9000 --open=false

# Or skip frontend build (only when internal/web/dist already has artifacts)
make build-go-only
./git-agent serve
```

Default access URL: `http://127.0.0.1:8088`.

- Listens on `127.0.0.1` by default — not exposed to the LAN.
- The console prints the actual access URL on startup; `Ctrl+C` for graceful shutdown.
- If the port is occupied, an error is reported — change `--port`.

## 2. Opening a Local Directory

1. On the home page, click the folder button to select a local directory via the **system file picker** (or choose from "Recently Opened").
2. If the directory is not yet a Git repository, a dialog asks "Initialize?"; clicking "Initialize" will:
   - Execute `git init`;
   - Generate a default `.gitignore` (minimal template);
   - If you haven't configured your name/email, it will prompt you to go to the "Settings" page first.
3. After entering the workspace, the left side shows the file tree, the right side shows functional panels (Status / History / Branches), and the 🤖 button in the bottom-right corner opens the AI chat assistant.

System directories (such as `/`, `$HOME`, `/etc`, `C:\Windows`) are automatically rejected to prevent accidental damage.

## 3. Main Features

- **Status Panel**: View which files are modified / added / deleted / untracked. Supports multi-select commit, click file to view diff, one-click "Save Version".
- **History Panel**: Filter commits by author/keyword, view commit diffs, rollback (with confirmation for uncommitted changes).
- **Branch Panel**: Switch branches (with confirmation for uncommitted changes), create new branches, push to remote; shows a hint when push fails and a pull is needed first.
- **AI Chat Assistant**: Right-side drawer panel. Tell the Agent your needs in natural language. The UI **streams** the ReAct process (thinking → tool call → tool result → final reply). Reasoning steps are collapsed by default; click to expand for details.
- **File Preview**: Click a file in the file tree to view its content in a floating modal with syntax highlighting.
- **Multi-Workspace**: The top tab bar supports multiple directories simultaneously; each workspace has its own `Agent`, chat history, and state.

## 4. Settings Page

- **User Info**: Set Git commit author (name / email).
- **LLM Configuration**: Enter API Key / Base URL / Model, with "Test Connection" support.
  - API Key is displayed as masked `sk-****xxxx` on the frontend; leave blank to keep the existing value.
- **HTTP Authentication**: Username + token for pushing to https:// remote repositories.

Configuration is saved to `~/.git-agent/config.json` (permissions `0600`). Priority: CLI flags > environment variables > config file > defaults.

## 5. Data & Logs

| File | Description |
|------|-------------|
| `~/.git-agent/config.json` | User configuration (sanitized storage) |
| `~/.git-agent/recent.json` | Recently opened directory list (max 20 entries) |
| `~/.git-agent/logs/ops.log` | Destructive operation audit (rollback, branch switch, push, init) |

## 6. Coexistence with CLI

Web mode is an additional entry point that does not affect the existing CLI:

```bash
./git-agent          # Continue using CLI as before
./git-agent serve    # Start Web server
./git-agent --help   # View all commands
```

## 7. Security Notes

- Listens on `127.0.0.1` by default — external machines cannot access it.
- To allow LAN access, explicitly add `--host 0.0.0.0 --i-know-what-i-do`, and it is strongly recommended to add access control at a higher level.
- File read APIs include path traversal validation — only files under the workspace root directory are accessible.

## 8. FAQ

- **Blank page after startup?** The frontend resources were not correctly embedded. Confirm that `internal/web/dist/` contains `index.html`; rebuild the frontend with `make build-web`.
- **No streaming effect in the chat panel?** Check the browser DevTools Network panel for an `event-stream` long connection; for reverse proxies, disable buffering (`X-Accel-Buffering: no`).
- **"Please set user first" error when committing?** Go to "Settings → User Info" to fill in your name/email, then try again.
- **AI chat errors?** Confirm that you've configured the LLM API Key in the Settings page and that the connection test succeeded. After modifying backend code, restart the service (or use `make dev-server` with air for auto-restart).

## 9. Development & Debugging

The project supports simultaneous hot-reload for both frontend and backend — no manual restart needed after code changes:

```bash
# Install air (one-time, for Go backend hot-reload)
go install github.com/air-verse/air@latest

# Option A: Two terminals
make dev-server   # Terminal 1: Backend (auto-restart on .go file changes)
make dev-web      # Terminal 2: Frontend (Vite HMR, .vue/.ts changes take effect instantly)

# Option B: One terminal
make dev-all      # Start both frontend and backend with hot-reload
```

Access `http://localhost:5173` — Vite proxies `/api` to the backend at `:8088`.

> ⚠️ **Note**: Frontend changes (`.vue`, `.ts`, `.css`) take effect instantly via Vite HMR; backend changes (`.go`) require air to recompile and restart (approximately 1-2 second delay). If air is not installed, backend changes require manually restarting `make dev-server`.
