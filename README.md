# squirrel (a.k.a. SlopDeck)

The best context manager for running AI coding agents in parallel.

Manage wortktrees, run agents, track their status, test worktrees locally with [launch](https://github.com/adamarutyunov/launch), integrate your Linear workspace.

<img width="1752" height="1045" alt="Screenshot 2026-03-31 at 14 35 42" src="https://github.com/user-attachments/assets/18643749-dbae-4f3f-8716-1395c43fcb36" />

Running several agents manually is painful:

- Managing git worktrees and branches by hand;
- Losing track of which agent is doing what;
- Getting a terminal window mess;
- Forgetting which issue a worktree was for;
- Shutting down and restarting all the processes to test a different branch.

Squirrel gives you a single terminal UI to manage contexts, run agents in them, test changes quickly, and link issues from task managers (currently supporting Linear only).

## Features

- **Runs in terminal, powered by tmux**;
- **One-key context creation** — creates a git worktree with a new branch;
- **Agent management** — launch Claude or Codex agents per context, track their status live, attach side-by-side;
- **Task manager integration** — link Linear issues to your worktrees by task name, or create new contexts out of issues;
- **Companion terminal pane** — a real shell runs side-by-side with squirrel; `Ctrl+W` to toggle between them;
- **Config editing** — open user and project config files directly in your editor;
- **Launch integration** — embedded [launch](https://github.com/adamarutyunov/launch) panels for managing project processes; one panel per project, switching contexts within a project restarts processes automatically;
- **Session resume** — sessions are persistent, reopen squirrel and reattach to existing agent sessions.

## Install

```sh
go install squirrel@latest
```

Or use the install script:

```sh
curl -fsSL https://raw.githubusercontent.com/adamarutyunov/squirrel/main/install.sh | sh
```

Or install a specific version:

```sh
VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/adamarutyunov/squirrel/main/install.sh | sh
```

Or build from source:

```sh
git clone https://github.com/adamarutyunov/squirrel
cd squirrel
go build -o ~/bin/sq .
```

## Setup

### 1. Install agent hooks

Squirrel tracks agent status (idle/working/done) via hooks. Run:

```sh
sq --install-hooks
```

This installs hooks for both Claude (`~/.claude/settings.json`) and Codex (`~/.codex/hooks.json`, plus enables the `codex_hooks` feature flag). If you already have hooks configured, it will overwrite the `hooks` key — back up your settings first if needed.

### 2. Set project Linear API key (optional)

To enable Linear integration, add your API key to the project config:

```sh
ctrl+p
```

### 3. Set tmux config (recommended)

Squirrel runs inside tmux. For full keybinding support (e.g. `Alt+Backspace` in fish), add to `~/.tmux.conf`:

```
set-option -g xterm-keys on
```

## Usage

```sh
sq                  # run from a git repo or parent directory
```

Squirrel discovers git repositories in the current directory and shows all worktrees as contexts.

## Config

### User config

Global settings at `~/.config/squirrel/config.yaml`:

```yaml
agent_command: claude --dangerously-skip-permissions
```

| Field | Default | Description |
|---|---|---|
| `agent_command` | `claude` (or `codex` if found) | Command to run as the AI agent |

### Project config

Per-project settings at `~/.config/squirrel/projects/<name>-<hash>/config.yaml`:

```yaml
setup_command: pnpm install
symlinks:
  - node_modules
  - .env
linear_api_key: lin_api_...
```

| Field | Default | Description |
|---|---|---|
| `setup_command` | — | Command to run after creating a new context (e.g. install dependencies) |
| `symlinks` | — | Files/directories to symlink from the main worktree into new contexts |
| `linear_api_key` | — | Linear API key used only for this project |

Project configs are stored outside the repo so they stay local to your machine.

## Keybindings

| Key | Action |
|---|---|
| `j` / `k` or `Up` / `Down` | Navigate |
| `Enter` | Select context (cd in terminal pane) |
| `n` | New context (with Linear issue picker if API key set) |
| `d` | Delete context (double-press for dirty worktrees) |
| `c` | Copy context path to clipboard |
| `a` | Launch agent in companion pane |
| `l` | Open launch for context (no-op if already open; switches if different context in same project) |
| `L` | Kill launch for current project |
| `s` | Cycle sort mode (Agent / Alpha / Linear / Updated) |
| `Tab` | Cycle focus: context list → launch panels → context list |
| `Ctrl+T` | Toggle between squirrel and terminal pane |
| `Ctrl+U` | Open user config in `$VISUAL` / `$EDITOR` |
| `Ctrl+P` | Open project config for the selected repo in `$VISUAL` / `$EDITOR` |
| `q` | Quit |

Bug reports and pull requests are welcome at [github.com/adamarutyunov/squirrel](https://github.com/adamarutyunov/squirrel/issues).

---

Made by [Adam](https://adam.ci) · [@_adamci](https://twitter.com/_adamci)
