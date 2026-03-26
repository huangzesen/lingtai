package tui

import (
	"path/filepath"

	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	tea "github.com/charmbracelet/bubbletea"
)

type view int

const (
	viewSetup view = iota
	viewMail
	viewAgents
)

type App struct {
	currentView view
	setup       SetupModel
	agents      AgentsModel
	mail        MailModel
	globalDir   string
	projectDir  string
	vizURL      string
	width       int
	height      int
}

func humanAddr(projectDir string) string {
	humanDir := filepath.Join(projectDir, "human")
	node, err := fs.ReadAgent(humanDir)
	if err != nil {
		return humanDir
	}
	if node.Address != "" {
		return node.Address
	}
	return humanDir
}

func NewApp(globalDir, projectDir, vizURL string, needsSetup bool) App {
	app := App{globalDir: globalDir, projectDir: projectDir, vizURL: vizURL}
	if needsSetup {
		app.currentView = viewSetup
		app.setup = NewSetupModel(globalDir)
	} else {
		app.currentView = viewMail
		lingtaiCmd := config.LingtaiCmd(globalDir)
		app.agents = NewAgentsModel(projectDir, lingtaiCmd)
		humanDir := filepath.Join(projectDir, "human")
		addr := humanAddr(projectDir)
		app.mail = NewMailModel(humanDir, addr, projectDir)
	}
	return app
}

func (a App) Init() tea.Cmd {
	switch a.currentView {
	case viewSetup:
		return a.setup.Init()
	case viewMail:
		return a.mail.Init()
	case viewAgents:
		return a.agents.Init()
	}
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case setupDoneMsg:
		a.currentView = viewMail
		lingtaiCmd := config.LingtaiCmd(a.globalDir)
		a.agents = NewAgentsModel(a.projectDir, lingtaiCmd)
		humanDir := filepath.Join(a.projectDir, "human")
		addr := humanAddr(a.projectDir)
		a.mail = NewMailModel(humanDir, addr, a.projectDir)
		return a, a.mail.Init()
	case agentRefreshMsg:
		if a.currentView == viewAgents {
			updated, cmd := a.agents.Update(msg)
			a.agents = updated
			return a, cmd
		}
		return a, nil
	case mailRefreshMsg:
		updated, cmd := a.mail.Update(msg)
		a.mail = updated
		return a, cmd
	case tickMsg:
		updated, cmd := a.mail.Update(msg)
		a.mail = updated
		return a, cmd
	case tea.KeyMsg:
		if a.currentView != viewSetup {
			switch msg.String() {
			case "tab":
				if a.currentView == viewMail {
					a.currentView = viewAgents
					return a, a.agents.Init()
				} else {
					a.currentView = viewMail
				}
				return a, nil
			case "q", "ctrl+c":
				return a, tea.Quit
			}
		}
	}
	switch a.currentView {
	case viewSetup:
		updated, cmd := a.setup.Update(msg)
		a.setup = updated.(SetupModel)
		return a, cmd
	case viewMail:
		updated, cmd := a.mail.Update(msg)
		a.mail = updated
		return a, cmd
	case viewAgents:
		updated, cmd := a.agents.Update(msg)
		a.agents = updated
		return a, cmd
	}
	return a, nil
}

func (a App) View() string {
	switch a.currentView {
	case viewSetup:
		return a.setup.View()
	case viewMail:
		return a.mail.View()
	case viewAgents:
		return a.agents.View()
	}
	return ""
}
