# squirrel

The best context manager for running multiple AI agents.

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
- **Agent management** — launch Claude or Codex agents per context, track their status live, attach side-by-side or fullscreen;
- **Task manager integration** — link Linear issues to your worktrees by task name, or create new contexts out of issues;
- **Companion terminal pane** — a real shell runs side-by-side with squirrel; `Ctrl+W` to toggle between them;
- **Launch integration** — embedded [launch](https://github.com/adamarutyunov/launch) panel for managing project processes per context;
- **Session resume** — sessions are persistent, reopen squirrel and reattach to existing agent sessions.

## Install

```sh
go install squirrel@latest
```

Or build from source:

```sh
git clone https://github.com/adamarutyunov/squirrel
cd squirrel
go build -o ~/bin/sq .
```

## Setup

### 1. Install Claude hooks

Squirrel tracks agent status (idle/working/done) via Claude Code hooks. Run:

```sh
sq --install-hooks
```

This writes the required hooks to `~/.claude/settings.json`. If you already have hooks configured, it will overwrite the `hooks` key — back up your settings first if needed.

### 2. Set Linear API key (optional)

To enable Linear integration, set your API key:

```sh
export LINEAR_API_KEY=lin_api_...
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
```

| Field | Default | Description |
|---|---|---|
| `setup_command` | — | Command to run after creating a new context (e.g. install dependencies) |
| `symlinks` | — | Files/directories to symlink from the main worktree into new contexts |

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
| `A` | Launch agent fullscreen (Ctrl+Q to detach) |
| `l` | Toggle launch panel |
| `s` | Cycle sort mode (Agent / Alpha / Linear / Updated) |
| `Ctrl+W` | Toggle between terminal and context panes |
| `q` | Quit |

Bug reports and pull requests are welcome at [github.com/adamarutyunov/squirrel](https://github.com/adamarutyunov/squirrel/issues).

---

Made by [Adam](https://adam.ci) · [@_adamci](https://twitter.com/_adamci)
