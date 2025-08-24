Output demo.gif

# Terminal appearance and environment
Set FontSize 24
Set FontFamily "MesloLGS NF"
Set Theme "Catppuccin Mocha"
Set Width 1800
Set Height 900
Set Padding 10
Set Margin 30
Set MarginFill "#674EFF"
Set BorderRadius 12
Set Shell zsh

# Build the app and prepare a temporary Git repo with sample branches
Hide
Type "go build -o worktree-tui ./cmd/worktree-tui"
Enter
Type `PROJ="$PWD"`
Enter
Type `TMP="$(mktemp -d -t worktree-tui-demo)"`
Enter
Type `cd "$TMP"`
Enter
Type "git init -q && git config user.name Demo && git config user.email demo@example.com"
Enter
Type "git branch -m main && printf 'one\n' > file.txt && git add file.txt && git commit -q -m 'feat: initial commit'"
Enter
Type "git branch feature/alpha && git branch feature/beta"
Enter
Type `alias worktree-tui="$PROJ/worktree-tui" && clear`
Enter
Show

# Launch the TUI in the demo repo
Type "worktree-tui"
Enter
Sleep 2200ms

# Navigate the worktree list
Down
Sleep 600ms
Down
Sleep 600ms
Up
Sleep 600ms

# Add a worktree from an existing branch (feature/alpha)
Type "a"
Sleep 700ms
Down
Sleep 500ms
Enter
Sleep 1700ms

# Back in the list, move to the newly created worktree
Down
Sleep 600ms
Down
Sleep 600ms

# Delete for real (Enter = Yes)
Type "d"
Sleep 900ms
Enter
Sleep 1700ms

# Create a brand-new branch + worktree via inline input
Type "a"
Sleep 700ms
Type "n"
Sleep 500ms
Type "demo-branch"
Sleep 500ms
Enter
Sleep 1700ms

# Remove the demo branch worktree to keep things tidy
Down
Sleep 500ms
Down
Sleep 500ms
Type "d"
Sleep 900ms
Enter
Sleep 1700ms

# Quit
Type "q"
Sleep 900ms

# Done
Sleep 600ms
