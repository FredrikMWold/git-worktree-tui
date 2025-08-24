package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"git-worktree-tui/internal/git"
)

type item struct {
	title string
	desc  string
	wt    git.Worktree
	isAdd bool
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

type refreshMsg struct{}

type loadedWorktreesMsg struct {
	wts []git.Worktree
	err error
}

type loadedBranchesMsg struct {
	branches []string
	err      error
}

func initialModel() model {
	// Main worktree list with default delegate
	li := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	li.Title = "Git Worktrees"
	li.SetShowStatusBar(true)
	li.SetShowPagination(true)
	li.SetFilteringEnabled(false)
	li.SetShowFilter(false)
	li.SetShowHelp(true)
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

	m := model{state: stateList, list: li, input: in}

	// Branch picker with custom delegate supporting inline editing for the add item
	del := &branchDelegate{input: &m.input}
	br := list.New([]list.Item{}, del, 0, 0)
	br.Title = "Pick branch"
	br.SetShowStatusBar(true)
	br.SetShowPagination(true)
	br.SetShowHelp(true)
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

// branchDelegate renders the branches list and swaps the add item into an inline text input when editing.
type branchDelegate struct {
	input   *textinput.Model
	editing bool
}

func (d *branchDelegate) Height() int  { return 2 }
func (d *branchDelegate) Spacing() int { return 0 }
func (d *branchDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d *branchDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	it, ok := listItem.(item)
	if !ok {
		fmt.Fprintln(w, "?")
		fmt.Fprintln(w)
		return
	}
	// Inline editor for the synthetic add item when editing
	if it.isAdd && d.editing {
		fmt.Fprintln(w, d.input.View())
		fmt.Fprintln(w)
		return
	}
	// Basic two-line rendering: title and description
	fmt.Fprintln(w, it.Title())
	if it.desc != "" {
		fmt.Fprintln(w, it.Description())
	} else {
		fmt.Fprintln(w)
	}
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
	brs, err := git.ListBranches()
	return loadedBranchesMsg{branches: brs, err: err}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// The lists render their own titles; give them the full height
		m.list.SetSize(msg.Width, msg.Height)
		m.branches.SetSize(msg.Width, msg.Height)
		return m, nil
	case loadedWorktreesMsg:
		if msg.err != nil {
			return m, m.list.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
		}
		items := make([]list.Item, 0, len(msg.wts)+1)
		// Prepend an inline action to add a new worktree
		items = append(items, item{title: "[+] Add new worktree", desc: "Create from existing or new branch", isAdd: true})
		for _, wt := range msg.wts {
			branch := wt.Branch
			if branch == "" {
				branch = wt.HEAD
			}
			t := wt.Path
			d := branch
			if wt.IsMain {
				d += " (main)"
			}
			items = append(items, item{title: t, desc: d, wt: wt})
		}
		m.list.SetItems(items)
		return m, nil
	case loadedBranchesMsg:
		if msg.err != nil {
			return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", msg.err))
		}
		items := make([]list.Item, 0, len(msg.branches)+1)
		// Prepend synthetic option to create a new branch
		items = append(items, item{title: "[+] Create new branch", desc: "Type a new branch name", isAdd: true})
		for _, b := range msg.branches {
			items = append(items, item{title: b, desc: ""})
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
					m.state = stateList
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree at %s for new branch %s", path, branch)))
				}
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
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
				}
				return m, nil
			case "enter":
				if it, ok := m.branches.SelectedItem().(item); ok {
					if it.isAdd {
						if m.branchDel != nil {
							m.branchDel.editing = true
							m.input.SetValue("")
							m.input.Focus()
						}
						return m, nil
					}
					branch := it.title
					path := git.DefaultWorktreeDir(branch)
					if err := git.CreateWorktree(branch, path, false); err != nil {
						return m, m.branches.NewStatusMessage(fmt.Sprintf("Error: %v", err))
					}
					m.state = stateList
					return m, tea.Batch(loadWorktrees, m.list.NewStatusMessage(fmt.Sprintf("Created worktree at %s for %s", path, branch)))
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
