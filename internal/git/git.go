package git

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree info we display
// Path is absolute or relative as returned by git
// Branch is the associated branch (if any)
// IsBare indicates it's the main working tree (no .git/worktrees path)
// Detected via parsing `git worktree list --porcelain`.

type Worktree struct {
	Path   string
	Branch string
	HEAD   string
	IsMain bool
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("git %v failed: %v\n%s", args, err, string(ee.Stderr))
		}
		return "", err
	}
	return string(out), nil
}

// ListWorktrees returns worktrees using porcelain format.
func ListWorktrees() ([]Worktree, error) {
	out, err := runGit("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var wts []Worktree
	s := bufio.NewScanner(strings.NewReader(out))
	wt := Worktree{}
	inBlock := false
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "worktree ") {
			if inBlock {
				wts = append(wts, wt)
			}
			inBlock = true
			wt = Worktree{}
			wt.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			// main tree has no 'worktree' subdir under .git/worktrees
			wt.IsMain = true // default; we'll refine below if we see 'worktree <path>' and 'bare' or other markers
			continue
		}
		if strings.HasPrefix(line, "branch ") {
			wt.Branch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			continue
		}
		if strings.HasPrefix(line, "HEAD ") {
			wt.HEAD = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
			continue
		}
		// ignore other lines like 'bare', 'detached', etc.
	}
	if inBlock {
		wts = append(wts, wt)
	}
	// Determine main vs secondary: the first entry from git is usually main.
	if len(wts) > 0 {
		wts[0].IsMain = true
		for i := 1; i < len(wts); i++ {
			wts[i].IsMain = false
		}
	}
	return wts, nil
}

// ListBranches returns local branches without the leading '*'
func ListBranches() ([]string, error) {
	out, err := runGit("branch", "--format", "%(refname:short)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var branches []string
	for _, l := range lines {
		l = strings.TrimSpace(strings.TrimPrefix(l, "* "))
		if l == "" {
			continue
		}
		branches = append(branches, l)
	}
	return branches, nil
}

// Branch represents a git branch, local or remote.
type Branch struct {
	Name      string // short name without remote prefix
	IsRemote  bool
	Remote    string // e.g. "origin" if remote
	RemoteRef string // e.g. "origin/feature"
	// Upstream tracking for local branches
	HasUpstream    bool
	Upstream       string // short, e.g. "origin/feature"
	UpstreamRemote string // e.g. "origin"
}

// ListBranchesDetailed returns local and remote branches, with local first.
func ListBranchesDetailed() ([]Branch, error) {
	var branches []Branch
	seen := map[string]bool{}
	// Local branches with upstream info, sorted by latest commit date
	outLocal, err := runGit("for-each-ref", "--sort=-committerdate", "--format=%(refname:short)|%(upstream:short)", "refs/heads")
	if err == nil {
		for _, l := range strings.Split(strings.TrimSpace(outLocal), "\n") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			name := l
			upstreamShort := ""
			if strings.Contains(l, "|") {
				parts := strings.SplitN(l, "|", 2)
				name = strings.TrimSpace(parts[0])
				upstreamShort = strings.TrimSpace(parts[1])
			}
			br := Branch{Name: name}
			if upstreamShort != "" {
				br.HasUpstream = true
				br.Upstream = upstreamShort
				upParts := strings.SplitN(upstreamShort, "/", 2)
				if len(upParts) >= 1 {
					br.UpstreamRemote = upParts[0]
				}
			}
			branches = append(branches, br)
			seen[name] = true
		}
	}
	// Remote branches (skip HEAD pointers like origin/HEAD), sorted by latest commit date
	outRemote, err := runGit("for-each-ref", "--sort=-committerdate", "--format=%(refname:short)", "refs/remotes")
	if err == nil {
		for _, l := range strings.Split(strings.TrimSpace(outRemote), "\n") {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if strings.HasSuffix(l, "/HEAD") {
				continue
			}
			// Split remote/name
			parts := strings.SplitN(l, "/", 2)
			if len(parts) != 2 {
				continue
			}
			remote := parts[0]
			name := parts[1]
			// If a local branch with the same short name exists, skip the remote to avoid duplicates
			if seen[name] {
				continue
			}
			branches = append(branches, Branch{Name: name, IsRemote: true, Remote: remote, RemoteRef: l})
			seen[name] = true
		}
	}
	return branches, nil
}

// CreateWorktreeFromRef creates a new branch from a given ref and adds a worktree.
// Equivalent to: git worktree add -b <branch> <path> <fromRef>
func CreateWorktreeFromRef(branch, targetDir, fromRef string) error {
	if branch == "" || targetDir == "" || fromRef == "" {
		return fmt.Errorf("branch, targetDir and fromRef required")
	}
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	_, err := runGit("worktree", "add", "-b", branch, targetDir, fromRef)
	return err
}

// CreateWorktree creates a new worktree at targetDir for the given branch.
// If branch doesn't exist and createBranch is true, it will create it from current HEAD.
// targetDir may be relative; we create parent directories as needed.
func CreateWorktree(branch, targetDir string, createBranch bool) error {
	if branch == "" || targetDir == "" {
		return fmt.Errorf("branch and targetDir required")
	}
	// Ensure parent directories exist
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return err
	}
	args := []string{"worktree", "add", targetDir, branch}
	if createBranch {
		args = []string{"worktree", "add", "-b", branch, targetDir}
	}
	_, err := runGit(args...)
	return err
}

// RemoveWorktree removes a worktree by path. If force is true, uses --force.
func RemoveWorktree(path string, force bool) error {
	if path == "" {
		return fmt.Errorf("path required")
	}
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	_, err := runGit(args...)
	return err
}

// DefaultWorktreeDir suggests a directory name for a branch under .worktrees/<branch>
func DefaultWorktreeDir(branch string) string {
	// Place new worktrees as siblings of the current repo directory
	// Use the repo's base directory name and append the branch name
	// e.g., /path/parent/repo-branch
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(".worktrees", branch)
	}
	parent := filepath.Dir(cwd)
	base := filepath.Base(cwd)
	name := base + "-" + branch
	return filepath.Join(parent, name)
}
