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
	"github.com/fredrikmwold/git-worktree-tui/internal/git"
	"github.com/fredrikmwold/git-worktree-tui/internal/theme"
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
	// Inline delete confirmation state for main list
	confirmIndex int // -1 when not confirming; otherwise index in m.list
	confirmPrev  item
	// App frame style (rounded mauve border around the entire app)
	frame lipgloss.Style
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

	m := model{state: stateList, list: li, input: in, confirmIndex: -1}

	// Create a rounded mauve border frame for the whole app
	m.frame = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Mauve)

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
		// Account for outer frame border (1 char each side)
		// Ensure the frame spans (terminal width - 2) to avoid right-edge overflow
		fw := msg.Width - 2
		if fw < 0 {
			fw = 0
		}
		m.frame = m.frame.Width(fw)
		// Inner width accounts for left+right border (2) plus the extra 2-col adjustment
		innerW := msg.Width - 4
		innerH := msg.Height - 2
		if innerW < 0 {
			innerW = 0
		}
		if innerH < 0 {
			innerH = 0
		}
		m.list.SetSize(innerW, innerH)
		m.branches.SetSize(innerW, innerH)
		// Size the inline editor to fit the list content width with a small margin
		w := innerW - 6
		if w < 10 {
			w = 10
		}
		m.input.Width = w

		// Help line wrapping control: always show help; use short vs full based on width and constrain width
		m.list.SetShowHelp(true)
		m.branches.SetShowHelp(true)
		// Constrain help style width to inner content width to avoid wrapping
		ls := m.list.Styles
		ls.HelpStyle = ls.HelpStyle.Foreground(theme.Surface2).MaxWidth(innerW)
		m.list.Styles = ls
		bs := m.branches.Styles
		bs.HelpStyle = bs.HelpStyle.Foreground(theme.Surface2).MaxWidth(innerW)
		m.branches.Styles = bs
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
		// Clear any pending inline delete confirmation
		m.confirmIndex = -1
		return m, nil
	case loadedBranchesMsg:
		if msg.err != nil {
			return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
		}
		items := make([]list.Item, 0, len(msg.branches)+1)
		labelTrack := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Blue).Render(s) }
		labelMuted := func(s string) string { return lipgloss.NewStyle().Foreground(theme.Surface1).Render(s) }
		value := func(s string) string { return s }
		// Prepend synthetic option to create a new branch
		items = append(items, item{title: "[+] Create new branch", desc: "Type a new branch name", isAdd: true})
		for _, b := range msg.branches {
			// Title: branch name; Desc: show tracking info for locals; gray 'no remote' if none
			desc := ""
			if !b.IsRemote {
				up := strings.TrimSpace(b.Upstream)
				if up == "" {
					desc = labelMuted("No remote")
				} else {
					desc = labelTrack("Tracking:") + " " + value(up)
				}
			}
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
			case "esc":
				// Cancel inline delete confirmation if active
				if m.confirmIndex != -1 {
					// restore previous item content
					items := m.list.Items()
					if idx := m.confirmIndex; idx >= 0 && idx < len(items) {
						items[idx] = m.confirmPrev
						m.list.SetItems(items)
					}
					m.confirmIndex = -1
					return m, nil
				}
				return m, nil
			case "r":
				m.confirmIndex = -1
				return m, loadWorktrees
			case "a":
				m.state = stateAddPick
				return m, loadBranches
			case "enter":
				// If confirming delete inline, Enter = Yes
				if m.confirmIndex != -1 && m.list.Index() == m.confirmIndex {
					if m.selected.Path != "" {
						if err := git.RemoveWorktree(m.selected.Path, true); err != nil {
							// restore and show error
							items := m.list.Items()
							if idx := m.confirmIndex; idx >= 0 && idx < len(items) {
								items[idx] = m.confirmPrev
								m.list.SetItems(items)
							}
							m.confirmIndex = -1
							return m, m.list.NewStatusMessage(fmt.Sprintf("Error: %v", err))
						}
						// cleared by loadWorktrees
						m.confirmIndex = -1
						name := filepath.Base(m.selected.Path)
						return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Removed worktree %s", name)))
					}
					return m, nil
				}
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
					// If another confirmation is active, restore it first
					if m.confirmIndex != -1 {
						items := m.list.Items()
						if idx := m.confirmIndex; idx >= 0 && idx < len(items) {
							items[idx] = m.confirmPrev
							m.list.SetItems(items)
						}
						m.confirmIndex = -1
					}
					m.selected = it.wt
					// Mutate the selected list item to show inline confirmation
					idx := m.list.Index()
					m.confirmIndex = idx
					m.confirmPrev = it
					items := m.list.Items()
					// Build confirmation text on title; keep description for Yes/No
					confirmItem := it
					confirmItem.title = fmt.Sprintf("Are you sure you want to delete: %s", it.title)
					confirmItem.desc = "Yes: Enter    No: Esc"
					items[idx] = confirmItem
					m.list.SetItems(items)
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
					name := filepath.Base(path)
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree %s", name)))
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
					if err := git.CreateWorktree(branchName, path, false); err != nil {
						return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", err))
					}
					m.state = stateList
					name := filepath.Base(path)
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree %s", name)))
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
				name := filepath.Base(path)
				return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree %s", name)))
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
				name := filepath.Base(m.selected.Path)
				return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Removed worktree %s", name)))
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.state {
	case stateList:
		return m.frame.Render(m.list.View())
	case stateAddPick:
		return m.frame.Render(m.branches.View())
	case stateAddNewInput:
		return m.frame.Render(m.input.View())
	case stateConfirmDelete:
		return m.frame.Render(m.confirmMsg)
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
