package tui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/fs"
)

// bodhiLeaf is braille art of a Bodhi leaf (菩提叶), shown on preparing screens.
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

// NirvanaDoneMsg is emitted after .lingtai/ has been wiped.
// The app should transition to first-run.
type NirvanaDoneMsg struct{}

// nirvanaCleanDoneMsg is internal — signals that cleanup finished.
type nirvanaCleanDoneMsg struct{}

// NirvanaModel is a full-screen confirmation view for /nirvana (clean & start fresh).
// Cursor defaults to Cancel (index 1) so the user must deliberately move up.
type NirvanaModel struct {
	lingtaiDir string // .lingtai/ path
	cursor     int    // 0 = Confirm, 1 = Cancel
	cleaning   bool   // true while cleanup runs
	done       bool   // true when cleanup complete, waiting for Enter
	width      int
	height     int
}

func NewNirvanaModel(lingtaiDir string) NirvanaModel {
	return NirvanaModel{
		lingtaiDir: lingtaiDir,
		cursor:     1, // default to Cancel
	}
}

func (m NirvanaModel) Init() tea.Cmd { return nil }

func (m NirvanaModel) Update(msg tea.Msg) (NirvanaModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case nirvanaCleanDoneMsg:
		m.cleaning = false
		m.done = true
		return m, nil

	case tea.KeyPressMsg:
		if m.cleaning {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.done {
			switch msg.String() {
			case "enter":
				return m, func() tea.Msg { return NirvanaDoneMsg{} }
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
			case 0: // Confirm
				m.cleaning = true
				return m, m.doClean()
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

func (m NirvanaModel) doClean() tea.Cmd {
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

		return nirvanaCleanDoneMsg{}
	}
}

func (m NirvanaModel) View() string {
	if m.cleaning || m.done {
		return m.viewProgress()
	}
	return m.viewConfirm()
}

func (m NirvanaModel) viewProgress() string {
	leafStyle := lipgloss.NewStyle().Foreground(ColorAgent)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)

	leaf := leafStyle.Render(bodhiLeaf)

	var statusText string
	if m.done {
		statusText = titleStyle.Render(i18n.T("nirvana.done"))
	} else {
		statusText = titleStyle.Render(i18n.T("nirvana.cleaning"))
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

func (m NirvanaModel) viewConfirm() string {
	var b string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAgent)
	warnStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSuspended)

	b += "\n  " + titleStyle.Render(i18n.T("nirvana.title")) + "\n\n"
	b += "  " + warnStyle.Render(i18n.T("nirvana.warning")) + "\n\n"
	b += "  " + i18n.TF("nirvana.path", m.lingtaiDir) + "\n\n"
	b += "  " + i18n.T("nirvana.detail") + "\n\n"

	opts := []string{
		i18n.T("nirvana.confirm"),
		i18n.T("nirvana.cancel"),
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

	b += "\n" + StyleFaint.Render("  ↑↓ "+i18n.T("welcome.select_lang")+
		"  [Enter] "+i18n.T("welcome.confirm")+
		"  [Esc] "+i18n.T("nirvana.cancel")) + "\n"

	return b
}
