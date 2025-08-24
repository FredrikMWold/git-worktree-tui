package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git-worktree-tui/internal/git"
	"git-worktree-tui/internal/theme"
)

type item struct {
	title string
	desc  string
	wt    git.Worktree
	isAdd bool
	br    git.Branch
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// states

type state int

const (
	stateList state = iota
	stateAddPick
	stateAddNewInput
	stateConfirmDelete
)

type model struct {
	state      state
	list       list.Model
	branches   list.Model
	input      textinput.Model
	confirmMsg string
	selected   git.Worktree
	branchDel  *branchDelegate
}

// refreshMsg was previously used; keep reserved if needed in future

type loadedWorktreesMsg struct {
	wts []git.Worktree
	err error
}

type loadedBranchesMsg struct {
	branches []git.Branch
	err      error
}

type editorDoneMsg struct{ err error }

func initialModel() model {
	// Main worktree list with default delegate (built-in indicator)
	mainDel := list.NewDefaultDelegate()
	applyDelegateTheme(&mainDel)
	li := list.New([]list.Item{}, mainDel, 0, 0)
	li.Title = "Git Worktrees"
	li.SetShowStatusBar(true)
	li.SetShowPagination(true)
	li.SetFilteringEnabled(false)
	li.SetShowFilter(false)
	li.SetShowHelp(true)
	li.SetShowTitle(true)
	applyListTheme(&li)
	li.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		}
	}
	li.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		}
	}

	// Create the model and input before wiring delegate so the delegate can point to m.input
	in := textinput.New()
	in.Placeholder = "new-branch-name"
	in.CharLimit = 64
	in.Prompt = ""
	in.TextStyle = lipgloss.NewStyle().Foreground(theme.Text)
	in.PlaceholderStyle = lipgloss.NewStyle().Foreground(theme.Surface2)
	in.Cursor.Style = lipgloss.NewStyle().Foreground(theme.Mauve)

	m := model{state: stateList, list: li, input: in}

	// Branch picker with custom delegate supporting inline editing for the add item
	brBase := list.NewDefaultDelegate()
	applyDelegateTheme(&brBase)
	del := &branchDelegate{base: brBase, input: &m.input}
	br := list.New([]list.Item{}, del, 0, 0)
	br.Title = "Pick branch"
	br.SetShowStatusBar(true)
	br.SetShowPagination(true)
	br.SetShowHelp(true)
	br.SetFilteringEnabled(false)
	br.SetShowTitle(true)
	applyListTheme(&br)
	br.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new branch")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select/create")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back/cancel")),
		}
	}
	br.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new branch")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select/create")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back/cancel")),
		}
	}

	m.branches = br
	m.branchDel = del
	return m
}

// applyListTheme applies app-wide colors to list chrome using the theme palette.
func applyListTheme(l *list.Model) {
	s := l.Styles
	// Title with Lavender background and dark text; leave everything else as defaults
	s.Title = s.Title.Background(theme.Lavender).Foreground(theme.Crust).Bold(true)
	l.Styles = s
}

// applyDelegateTheme styles the default list item delegate for normal and selected states.
func applyDelegateTheme(d *list.DefaultDelegate) {
	st := d.Styles
	// Normal item titles use theme Text color
	st.NormalTitle = st.NormalTitle.Foreground(theme.Text)
	st.NormalDesc = st.NormalDesc.Foreground(theme.Surface1)
	st.SelectedDesc = st.SelectedDesc.Foreground(theme.Surface1)
	// Selected item: color only the left indicator (border) Mauve
	st.SelectedTitle = st.SelectedTitle.BorderLeftForeground(theme.Mauve).Foreground(theme.Mauve)
	// Color only the selected description's indicator (border) Mauve; leave text color default
	st.SelectedDesc = st.SelectedDesc.Foreground(theme.Surface1).BorderLeftForeground(theme.Mauve)
	d.Styles = st
}

// branchDelegate renders the branches list and swaps the add item into an inline text input when editing.
type branchDelegate struct {
	base    list.DefaultDelegate
	input   *textinput.Model
	editing bool
}

func (d *branchDelegate) Height() int                               { return d.base.Height() }
func (d *branchDelegate) Spacing() int                              { return d.base.Spacing() }
func (d *branchDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return d.base.Update(msg, m) }
func (d *branchDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	// Use built-in delegate rendering (indicator on title line only)
	d.base.Render(w, m, index, listItem)
}

func NewProgram() *tea.Program {
	m := initialModel()
	return tea.NewProgram(m)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadWorktrees, tea.EnterAltScreen)
}

func loadWorktrees() tea.Msg {
	wts, err := git.ListWorktrees()
	return loadedWorktreesMsg{wts: wts, err: err}
}

func loadBranches() tea.Msg {
	brs, err := git.ListBranchesDetailed()
	return loadedBranchesMsg{branches: brs, err: err}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The lists render their own titles; give them the full height
		m.list.SetSize(msg.Width, msg.Height)
		m.branches.SetSize(msg.Width, msg.Height)
		// Size the inline editor to fit the list content width with a small margin
		w := msg.Width - 6
		if w < 10 {
			w = 10
		}
		m.input.Width = w
		return m, nil
	case editorDoneMsg:
		// Exit the app after the editor process completes
		return m, tea.Quit
	case loadedWorktreesMsg:
		if msg.err != nil {
			return m, m.list.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
		}
		items := make([]list.Item, 0, len(msg.wts)+1)
		// Prepend an inline action to add a new worktree
		items = append(items, item{title: "[+] Add new worktree", desc: "Create from existing or new branch", isAdd: true})
		// Use varied accents for labels to add visual distinction
		labelBranch := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Sky).Render(s) }
		labelPath := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Green).Render(s) }
		value := func(s string) string { return s }
		for _, wt := range msg.wts {
			branch := wt.Branch
			if branch == "" {
				branch = wt.HEAD
			}
			// Show just the branch name (strip common refs prefixes)
			if strings.HasPrefix(branch, "refs/heads/") {
				branch = strings.TrimPrefix(branch, "refs/heads/")
			} else if strings.HasPrefix(branch, "heads/") {
				branch = strings.TrimPrefix(branch, "heads/")
			} else if strings.HasPrefix(branch, "refs/") {
				branch = strings.TrimPrefix(branch, "refs/")
			}
			// Title: just the name of the worktree (folder name)
			t := filepath.Base(wt.Path)
			// Desc: labeled info segments
			var segs []string
			if branch != "" {
				segs = append(segs, labelBranch("Branch:")+" "+value(branch))
			}
			segs = append(segs, labelPath("Path:")+" "+value(wt.Path))
			d := strings.Join(segs, "  ")
			items = append(items, item{title: t, desc: d, wt: wt})
		}
		m.list.SetItems(items)
		return m, nil
	case loadedBranchesMsg:
		if msg.err != nil {
			return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
		}
		items := make([]list.Item, 0, len(msg.branches)+1)
		labelType := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Peach).Render(s) }
		labelRemote := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Sky).Render(s) }
		value := func(s string) string { return s }
		// Prepend synthetic option to create a new branch
		items = append(items, item{title: "[+] Create new branch", desc: "Type a new branch name", isAdd: true})
		for _, b := range msg.branches {
			// Title: branch name; Desc: labeled status
			var segs []string
			if b.IsRemote {
				segs = append(segs, labelType("Type:")+" "+value("remote"))
				if b.Remote != "" {
					segs = append(segs, labelRemote("Remote:")+" "+value(b.Remote))
				}
			} else {
				segs = append(segs, labelType("Type:")+" "+value("local"))
			}
			desc := strings.Join(segs, "  ")
			items = append(items, item{title: b.Name, desc: desc, br: b})
		}
		m.branches.SetItems(items)
		return m, nil
	case tea.KeyMsg:
		k := msg.String()
		// Global: ctrl+c should always quit
		if k == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.state {
		case stateList:
			switch k {
			case "q", "ctrl+c":
				return m, tea.Quit
			case "r":
				return m, loadWorktrees
			case "a":
				m.state = stateAddPick
				return m, loadBranches
			case "enter":
				if it, ok := m.list.SelectedItem().(item); ok {
					if it.isAdd {
						m.state = stateAddPick
						return m, loadBranches
					}
					if it.wt.Path != "" {
						cmd, err := buildEditorCmd(it.wt.Path)
						if err != nil {
							return m, m.list.NewStatusMessage(fmt.Sprintf("No editor: %v", err))
						}
						return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} })
					}
				}
				return m, nil
			case "d":
				if it, ok := m.list.SelectedItem().(item); ok {
					if it.isAdd {
						return m, nil
					}
					if it.wt.IsMain {
						return m, m.list.NewStatusMessage("Cannot delete main worktree")
					}
					m.selected = it.wt
					m.confirmMsg = fmt.Sprintf("Remove worktree %s? enter=Yes esc=No", it.wt.Path)
					m.state = stateConfirmDelete
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		case stateAddPick:
			// Inline editing mode for the "Create new branch" synthetic item
			if m.branchDel != nil && m.branchDel.editing {
				switch k {
				case "esc":
					m.branchDel.editing = false
					m.input.Blur()
					// reset the add item title
					m.resetAddItemTitle()
					return m, nil
				case "enter":
					branch := strings.TrimSpace(m.input.Value())
					if branch == "" {
						return m, nil
					}
					path := git.DefaultWorktreeDir(branch)
					if err := git.CreateWorktree(branch, path, true); err != nil {
						return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", err))
					}
					m.branchDel.editing = false
					m.input.Blur()
					m.resetAddItemTitle()
					m.state = stateList
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree at %s for new branch %s", path, branch)))
				}
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				// update visible text in the first item
				m.updateAddItemTitle(m.input.Value())
				return m, cmd
			}
			switch k {
			case "esc":
				m.state = stateList
				return m, nil
			case "n":
				if m.branchDel != nil {
					m.branchDel.editing = true
					m.input.SetValue("")
					m.input.Focus()
					// Ensure selection stays on the add item (index 0)
					m.branches.Select(0)
					m.updateAddItemTitle("")
				}
				return m, nil
			case "enter":
				if it, ok := m.branches.SelectedItem().(item); ok {
					if it.isAdd {
						if m.branchDel != nil {
							m.branchDel.editing = true
							m.input.SetValue("")
							m.input.Focus()
							m.updateAddItemTitle("")
						}
						return m, nil
					}
					b := it.br
					branchName := b.Name
					path := git.DefaultWorktreeDir(branchName)
					var err error
					if b.IsRemote {
						err = git.CreateWorktreeFromRef(branchName, path, b.RemoteRef)
					} else {
						err = git.CreateWorktree(branchName, path, false)
					}
					if err != nil {
						return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", err))
					}
					m.state = stateList
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree at %s for %s", path, branchName)))
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.branches, cmd = m.branches.Update(msg)
			return m, cmd
		case stateAddNewInput:
			switch k {
			case "esc":
				m.state = stateList
				return m, nil
			case "enter":
				branch := strings.TrimSpace(m.input.Value())
				if branch == "" {
					return m, nil
				}
				path := git.DefaultWorktreeDir(branch)
				if err := git.CreateWorktree(branch, path, true); err != nil {
					// Return to list and show error
					m.state = stateList
					return m, m.list.NewStatusMessage(fmt.Sprintf("Error: %v", err))
				}
				m.state = stateList
				return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree at %s for new branch %s", path, branch)))
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case stateConfirmDelete:
			switch k {
			case "esc":
				m.state = stateList
				return m, nil
			case "enter":
				if err := git.RemoveWorktree(m.selected.Path, true); err != nil {
					m.state = stateList
					return m, m.list.NewStatusMessage(fmt.Sprintf("Error: %v", err))
				}
				m.state = stateList
				return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Removed worktree %s", m.selected.Path)))
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.state {
	case stateList:
		return m.list.View()
	case stateAddPick:
		return m.branches.View()
	case stateAddNewInput:
		return m.input.View() + "\n"
	case stateConfirmDelete:
		return m.confirmMsg + "\n"
	}
	return ""
}

// buildEditorCmd constructs an *exec.Cmd to open the given path in the user's editor.
// It uses $VISUAL, then $EDITOR; if neither is set, returns an error.
func buildEditorCmd(path string) (*exec.Cmd, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return nil, fmt.Errorf("$EDITOR not set")
	}
	// Open the directory in the editor; common CLIs support a path argument
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// stripANSI removes ANSI escape sequences from s.
// (removed) stripANSI helper no longer needed; we update the list item directly.

// updateAddItemTitle updates the title of the synthetic add-new-branch item (index 0)
func (m *model) updateAddItemTitle(val string) {
	if m.branches.Items() == nil || len(m.branches.Items()) == 0 {
		return
	}
	it0, ok := m.branches.Items()[0].(item)
	if !ok || !it0.isAdd {
		return
	}
	title := val
	// Only substitute the default label when not actively editing
	if strings.TrimSpace(title) == "" && !(m.branchDel != nil && m.branchDel.editing) {
		title = "[+] Create new branch"
	}
	it0.title = title
	items := m.branches.Items()
	items[0] = it0
	m.branches.SetItems(items)
}

// resetAddItemTitle resets the synthetic add item title back to its default label
func (m *model) resetAddItemTitle() { m.updateAddItemTitle("") }
