package tui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/secretary"
)

// SecretaryModel is a three-mode view:
//   - mode 0 (briefs): markdown viewer showing profile.md + journal.md files
//   - mode 1 (projects): registered project list + network info (absorbed /projects)
//   - mode 2 (kanban): PropsModel showing the secretary agent's status
//
// ctrl+t toggles between briefs and projects.
// ctrl+k toggles the secretary kanban.
// Esc returns to mail.
type SecretaryModel struct {
	mode     int // 0=briefs, 1=projects, 2=kanban
	briefs   MarkdownViewerModel
	projects ProjectsModel
	kanban   PropsModel

	globalDir   string
	projectDir  string
	lingtaiCmd  string
	width       int
	height      int
}

// SecretaryCloseMsg is sent when the user exits the secretary view.
type SecretaryCloseMsg struct{}

func NewSecretaryModel(globalDir, projectDir, lingtaiCmd string) SecretaryModel {
	return NewSecretaryModelAt(globalDir, projectDir, lingtaiCmd, 0)
}

// NewSecretaryModelAt creates a SecretaryModel starting at the given mode.
func NewSecretaryModelAt(globalDir, projectDir, lingtaiCmd string, startMode int) SecretaryModel {
	briefs := buildSecretaryBriefs(globalDir, projectDir)
	secLingtaiDir := secretary.LingtaiDir(globalDir)
	secAgentDir := secretary.AgentDir(globalDir)
	kanban := NewPropsModel(secLingtaiDir, secAgentDir)
	projects := NewProjectsModel(globalDir, projectDir)

	return SecretaryModel{
		mode:       startMode,
		briefs:     NewMarkdownViewer(briefs, "Secretary Briefs"),
		projects:   projects,
		kanban:     kanban,
		globalDir:  globalDir,
		projectDir: projectDir,
		lingtaiCmd: lingtaiCmd,
	}
}

func (m SecretaryModel) Init() tea.Cmd {
	return tea.Batch(m.kanban.Init(), m.projects.Init())
}

func (m SecretaryModel) Update(msg tea.Msg) (SecretaryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+t":
			// Toggle between briefs (0) and projects (1)
			if m.mode == 0 {
				m.mode = 1
			} else {
				m.mode = 0
			}
			return m, nil
		case "ctrl+k":
			// Toggle kanban overlay
			if m.mode == 2 {
				m.mode = 0 // back to briefs
			} else {
				m.mode = 2
			}
			return m, nil
		case "ctrl+r":
			// Refresh (suspend + relaunch) the secretary agent
			if m.lingtaiCmd == "" {
				return m, nil
			}
			secAgentDir := secretary.AgentDir(m.globalDir)
			return m, func() tea.Msg {
				hardRefreshDir(m.lingtaiCmd, secAgentDir)
				return m.kanban.loadData()
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Forward to all children so viewports initialize correctly
		m.briefs, _ = m.briefs.Update(msg)
		m.projects, _ = m.projects.Update(msg)
		m.kanban, _ = m.kanban.Update(msg)
		return m, nil
	case MarkdownViewerCloseMsg:
		// Intercept the close from briefs viewer — emit our own close
		return m, func() tea.Msg { return SecretaryCloseMsg{} }
	case ViewChangeMsg:
		// Intercept "go to mail" from kanban or projects — emit our own close
		if msg.View == "mail" {
			return m, func() tea.Msg { return SecretaryCloseMsg{} }
		}
	case projectsLoadMsg:
		// Always route to projects child regardless of active mode
		m.projects, _ = m.projects.Update(msg)
		return m, nil
	case propsLoadMsg:
		// Always route to kanban child regardless of active mode
		m.kanban, _ = m.kanban.Update(msg)
		return m, nil
	}

	var cmd tea.Cmd
	switch m.mode {
	case 0:
		m.briefs, cmd = m.briefs.Update(msg)
	case 1:
		m.projects, cmd = m.projects.Update(msg)
	case 2:
		m.kanban, cmd = m.kanban.Update(msg)
	}
	return m, cmd
}

func (m SecretaryModel) View() string {
	switch m.mode {
	case 1:
		return m.projects.View()
	case 2:
		return m.kanban.View()
	default:
		return m.briefs.View()
	}
}

// buildSecretaryBriefs constructs the markdown entry list for the briefs viewer.
// Group 1: This project — profile.md + journal.md
// Group 2: Other projects — journal.md for each other project hash
func buildSecretaryBriefs(globalDir, projectDir string) []MarkdownEntry {
	briefBase := filepath.Join(globalDir, "brief")
	projectPath := filepath.Dir(projectDir) // projectDir is .lingtai/, parent is the project
	thisHash := fs.ProjectHash(projectPath)

	var entries []MarkdownEntry

	// Profile (universal)
	profilePath := filepath.Join(briefBase, "profile.md")
	if _, err := os.Stat(profilePath); err == nil {
		entries = append(entries, MarkdownEntry{
			Label: "profile.md",
			Group: "This Project",
			Path:  profilePath,
		})
	} else {
		entries = append(entries, MarkdownEntry{
			Label:   "profile.md",
			Group:   "This Project",
			Content: "*No profile yet — the secretary has not run a briefing cycle.*",
		})
	}

	// This project's journal
	thisJournal := filepath.Join(briefBase, "projects", thisHash, "journal.md")
	if _, err := os.Stat(thisJournal); err == nil {
		entries = append(entries, MarkdownEntry{
			Label: "journal.md",
			Group: "This Project",
			Path:  thisJournal,
		})
	} else {
		entries = append(entries, MarkdownEntry{
			Label:   "journal.md",
			Group:   "This Project",
			Content: "*No journal yet — the secretary has not run a briefing cycle.*",
		})
	}

	// Other projects' journals
	projectsDir := filepath.Join(briefBase, "projects")
	dirEntries, err := os.ReadDir(projectsDir)
	if err == nil {
		for _, d := range dirEntries {
			if !d.IsDir() || d.Name() == thisHash {
				continue
			}
			journalPath := filepath.Join(projectsDir, d.Name(), "journal.md")
			if _, err := os.Stat(journalPath); err == nil {
				// Try to show a friendlier label — use first line of journal if possible
				label := d.Name() + "/journal.md"
				if data, err := os.ReadFile(journalPath); err == nil {
					if first := firstNonEmptyLine(string(data)); first != "" {
						label = strings.TrimPrefix(first, "# ")
						if len(label) > 40 {
							label = label[:37] + "..."
						}
					}
				}
				entries = append(entries, MarkdownEntry{
					Label: label,
					Group: "Other Projects",
					Path:  journalPath,
				})
			}
		}
	}

	return entries
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
