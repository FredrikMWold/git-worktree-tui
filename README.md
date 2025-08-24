# git-worktree-tui

[![Go Reference](https://pkg.go.dev/badge/github.com/fredrikmwold/git-worktree-tui.svg)](https://pkg.go.dev/github.com/fredrikmwold/git-worktree-tui)
[![Release](https://img.shields.io/github/v/release/FredrikMWold/git-worktree-tui?sort=semver)](https://github.com/FredrikMWold/git-worktree-tui/releases)

A minimal, keyboard-first TUI for Git worktrees built with [Bubble Tea](https://github.com/charmbracelet/bubbletea). List, create, open in your editor, and delete worktrees — without leaving your terminal.

![Demo](./demo.gif)


<details>
	<summary><strong>Quick keys</strong></summary>

| Context | Key | Action |
|---|---|---|
| Worktree picker | `↑`/`↓` or `j`/`k` | Move selection |
| Worktree picker | `a` | Add new worktree (open branch picker) |
| Worktree picker | `d` | Delete selected worktree (inline confirm) |
| Worktree picker | `Enter` | Open selected worktree in `$VISUAL`/`$EDITOR` or confirm delete |
| Worktree picker | `r` | Refresh worktrees |
| Worktree picker | `Esc` | Cancel delete confirmation |
| Branch picker | `n` | Create new branch (inline input) |
| Branch picker | `Enter` | Select branch / create new branch and worktree |
| Branch picker | `Esc` | Back to list |
| List | `q` or `Ctrl+C` | Quit |
| Anywhere | `Ctrl+C` | Quit |

> Tip: The help footer updates based on what you can do at the moment.

</details>

## Features

- 📂 List existing worktrees with branch and path info
- ➕ Create a worktree from a local or remote branch
- 🌱 Create a brand‑new branch and worktree in one step
- 📝 Open the selected worktree in your `$VISUAL`/`$EDITOR` with Enter

## Install

Install with Go:

```sh
go install github.com/fredrikmwold/git-worktree-tui/cmd/worktree-tui@latest
```

Or download a prebuilt binary from the Releases page and place it on your PATH:

- https://github.com/FredrikMWold/git-worktree-tui/releases
