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
		if strings.HasPrefix(line, "bare") {
			// not usually relevant here
			continue
		}
		if strings.HasPrefix(line, "detached") {
			continue
		}
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
	return filepath.Join(".worktrees", branch)
}
