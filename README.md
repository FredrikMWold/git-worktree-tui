# git-worktree-tui

A simple TUI to list, create, and delete Git worktrees using Bubble Tea.

- Lists existing worktrees
- Add a new worktree by selecting an existing branch or typing a new branch name
- Delete a selected worktree with confirmation

## Requirements

- Go 1.22+
- git installed and available on PATH

## Build

```sh
# from repo root
go build -o worktree-tui ./cmd/worktree-tui
```

## Run

```sh
./worktree-tui
```

## Key bindings

- Up/Down or j/k: navigate
- a: add new worktree
- d: delete selected worktree
- r: refresh
- enter: select / confirm
- esc: cancel dialog
- q or ctrl+c: quit

## Notes

- The app shells out to `git worktree` and `git branch` and parses their output. It assumes the current working directory is inside a Git repository (the main repo where worktrees are managed).
- New worktrees are created under `.worktrees/<branch>` by default (configurable later).
