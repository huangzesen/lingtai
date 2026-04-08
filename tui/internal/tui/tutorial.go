package tui

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
)

// bodhiLeaf is braille art of a Bodhi leaf (菩提叶), shown on the preparing screen.
// Source image stored in lingtai-web repo. All lines padded to equal width.
const bodhiLeaf = "" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡴⠖⠚⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣰⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣸⠻⣆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⡴⠞⠁⠀⠈⠳⢦⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⣀⡴⠋⠑⣆⠀⠀⣶⠀⠀⣰⠊⠙⢦⣀⠀⠀⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⣠⠞⠙⣆⠀⠀⠈⢳⡀⣿⢀⡞⠁⠀⠀⣰⠋⠳⣄⠀⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⢀⡞⢧⡀⠀⠈⠳⣄⠀⠀⠙⣿⠋⠀⠀⣠⠞⠁⠀⢀⡼⢳⡀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⣰⠋⠀⠀⠙⢦⣀⠀⠈⠙⢦⡀⣿⢀⡴⠋⠁⠀⣀⡴⠋⠀⠀⠙⣆⠀⠀⠀\n" +
	"⠀⠀⡼⠁⠙⢦⣀⠀⠀⠈⠳⢤⡀⠀⠙⣿⠋⠀⢀⡤⠞⠁⠀⠀⣀⡴⠋⠈⢧⠀⠀\n" +
	"⠀⣼⠡⣄⠀⠀⠈⠓⢦⣀⠀⠀⠉⠳⢄⣿⡠⠞⠉⠀⠀⣀⡴⠚⠁⠀⠀⣠⠌⣧⠀\n" +
	"⢰⠇⠀⠈⠓⢦⣀⠀⠀⠈⠙⠲⣄⡀⠀⣿⠀⢀⣠⠖⠋⠁⠀⠀⣀⡴⠚⠁⠀⠸⡆\n" +
	"⣼⠰⣄⠀⠀⠀⠈⠙⠲⢤⣀⠀⠀⠉⠳⣿⠞⠉⠀⠀⣀⡤⠖⠋⠁⠀⠀⠀⣠⠆⣧\n" +
	"⣿⠀⠈⠙⠦⣄⡀⠀⠀⠀⠈⠙⠲⢤⡀⣿⢀⡤⠖⠋⠁⠀⠀⠀⢀⣠⠴⠋⠁⠀⣿\n" +
	"⢻⢦⡀⠀⠀⠀⠉⠓⠦⣄⣀⠀⠀⠀⠈⣿⠁⠀⠀⠀⣀⣠⠴⠚⠉⠀⠀⠀⢀⡴⡟\n" +
	"⠘⡆⠉⠳⢤⣀⠀⠀⠀⠀⠉⠙⠲⢤⣀⣿⣀⡤⠖⠋⠉⠀⠀⠀⠀⣀⡤⠞⠉⢰⠃\n" +
	"⠀⠹⣄⠀⠀⠈⠙⠒⠦⢤⣄⣀⡀⠀⠈⣿⠁⠀⢀⣀⣠⡤⠴⠒⠋⠁⠀⠀⣠⠏⠀\n" +
	"⠀⠀⠈⠳⢤⣀⡀⠀⠀⠀⠀⠈⣉⣙⣷⣿⣾⣋⣉⠁⠀⠀⠀⠀⢀⣀⡤⠞⠁⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠈⠉⠉⠉⠉⠉⠉⠉⠀⠀⣿⠀⠀⠉⠉⠉⠉⠉⠉⠉⠁⠀⠀⠀⠀⠀\n" +
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀"

// TutorialConfirmDoneMsg is emitted when the user confirms the tutorial.
type TutorialConfirmDoneMsg struct {
	OrchDir  string
	OrchName string
}

// TutorialConfirmModel is the full-screen confirmation view for /tutorial.
// Cursor defaults to Cancel (index 1) so the user must deliberately move up.
type TutorialConfirmModel struct {
	lingtaiDir string // .lingtai/ path
	globalDir  string
	lingtaiCmd string
	lang       string
	cursor     int  // 0 = Start Tutorial, 1 = Cancel
	preparing  bool // true while setup runs
	done       bool // true when setup complete, waiting for Enter
	orchDir    string
	orchName   string
	message    string
	width      int
	height     int
}

func NewTutorialConfirmModel(lingtaiDir, globalDir, lingtaiCmd, lang string) TutorialConfirmModel {
	return TutorialConfirmModel{
		lingtaiDir: lingtaiDir,
		globalDir:  globalDir,
		lingtaiCmd: lingtaiCmd,
		lang:       lang,
		cursor:     1, // default to Cancel
	}
}

func (m TutorialConfirmModel) Init() tea.Cmd { return nil }

func (m TutorialConfirmModel) Update(msg tea.Msg) (TutorialConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tutorialSetupDoneMsg:
		m.preparing = false
		m.done = true
		m.orchDir = msg.orchDir
		m.orchName = msg.orchName
		return m, nil

	case tea.KeyPressMsg:
		if m.preparing {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.done {
			switch msg.String() {
			case "enter":
				return m, func() tea.Msg {
					return TutorialConfirmDoneMsg{OrchDir: m.orchDir, OrchName: m.orchName}
				}
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < 1 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0: // Start Tutorial
				m.preparing = true
				return m, m.doTutorial()
			case 1: // Cancel
				return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
			}
		case "esc":
			return m, func() tea.Msg { return ViewChangeMsg{View: "mail"} }
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// tutorialSetupDoneMsg is internal — signals that setup finished.
type tutorialSetupDoneMsg struct {
	orchDir  string
	orchName string
}

func (m TutorialConfirmModel) doTutorial() tea.Cmd {
	return func() tea.Msg {
		// 1. Suspend all agents
		agents, _ := fs.DiscoverAgents(m.lingtaiDir)
		for _, agent := range agents {
			if agent.IsHuman {
				continue
			}
			if fs.IsAlive(agent.WorkingDir, 3.0) {
				fs.SuspendAndWait(agent.WorkingDir, 5*time.Second)
			}
		}

		// 2. Remove .lingtai/ entirely
		os.RemoveAll(m.lingtaiDir)

		// 3. Re-init project (create human dir)
		process.InitProject(m.lingtaiDir)

		// 4. Generate tutorial agent
		p := preset.First()
		preset.GenerateTutorialInit(p, m.lingtaiDir, m.globalDir, m.lang)

		// 5. Write initial .prompt
		tutorialDir := filepath.Join(m.lingtaiDir, "tutorial")
		fs.WritePrompt(tutorialDir, "You have just been created as the tutorial guide. A new user is waiting. Send them a welcome email to introduce yourself and begin Lesson 1. The human's email address is: human")

		// 6. Mark tutorial done
		config.MarkTutorialDone(m.globalDir)

		// 7. Launch tutorial agent
		if m.lingtaiCmd != "" {
			process.LaunchAgent(m.lingtaiCmd, tutorialDir, m.globalDir)
		}

		// 8. Update human location in background
		humanDir := filepath.Join(m.lingtaiDir, "human")
		go fs.UpdateHumanLocation(humanDir)

		return tutorialSetupDoneMsg{
			orchDir:  tutorialDir,
			orchName: "guide",
		}
	}
}

func (m TutorialConfirmModel) View() string {
	if m.preparing || m.done {
		return m.viewPreparing()
	}
	return m.viewConfirm()
}

func (m TutorialConfirmModel) viewPreparing() string {
	leafStyle := lipgloss.NewStyle().Foreground(ColorAgent)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)

	leaf := leafStyle.Render(bodhiLeaf)

	var statusText string
	if m.done {
		statusText = titleStyle.Render(i18n.T("tutorial.done"))
	} else {
		statusText = titleStyle.Render(i18n.T("tutorial.preparing"))
	}

	var block string
	if m.done {
		hint := StyleFaint.Render("[Enter]")
		block = lipgloss.JoinVertical(lipgloss.Center, leaf, "", statusText, "", hint)
	} else {
		block = lipgloss.JoinVertical(lipgloss.Center, leaf, "", statusText)
	}

	w := m.width
	h := m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

func (m TutorialConfirmModel) viewConfirm() string {
	var b string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSuspended)

	b += "\n  " + titleStyle.Render(i18n.T("tutorial.title")) + "\n\n"
	b += "  " + warnStyle.Render(i18n.T("tutorial.warning")) + "\n\n"
	b += "  " + i18n.TF("tutorial.path", m.lingtaiDir) + "\n\n"
	b += "  " + i18n.T("tutorial.detail") + "\n"
	b += "  " + i18n.T("tutorial.patience") + "\n\n"

	opts := []string{
		i18n.T("tutorial.start"),
		i18n.T("tutorial.cancel"),
	}

	for i, opt := range opts {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if i == m.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		}
		b += cursor + style.Render(opt) + "\n"
	}

	if m.message != "" {
		errStyle := lipgloss.NewStyle().Foreground(ColorSuspended)
		b += "\n  " + errStyle.Render(m.message) + "\n"
	}

	b += "\n" + StyleFaint.Render("  ↑↓ "+i18n.T("welcome.select_lang")+
		"  [Enter] "+i18n.T("welcome.confirm")+
		"  [Esc] "+i18n.T("tutorial.cancel")) + "\n"

	return b
}
