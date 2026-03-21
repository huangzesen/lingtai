package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lingtai-daemon/internal/agent"
	"lingtai-daemon/internal/config"
	"lingtai-daemon/internal/i18n"
	"lingtai-daemon/internal/setup"
)

// View identifies the active sub-view.
type View int

const (
	ViewStatus View = iota
	ViewWizard
	ViewChat
	ViewStarting // loading screen while agent boots
)

// agentStartedMsg is sent when the async agent startup completes.
type agentStartedMsg struct {
	config *config.Config
	proc   *agent.Process
	err    error
}

// RootModel is the top-level bubbletea model that routes between views.
type RootModel struct {
	view   View
	status StatusModel
	wizard setup.WizardModel
	chat   ChatModel

	config     *config.Config
	proc       *agent.Process
	lingtaiDir string
	configPath string

	// Window dimensions — stored globally so new sub-models can be sized.
	width  int
	height int
}

// RootOpts configures the RootModel.
type RootOpts struct {
	LingtaiDir  string
	ConfigPath  string
	Config      *config.Config
	Proc        *agent.Process
	InitialView View
}

func NewRoot(opts RootOpts) RootModel {
	m := RootModel{
		view:       opts.InitialView,
		lingtaiDir: opts.LingtaiDir,
		configPath: opts.ConfigPath,
		config:     opts.Config,
		proc:       opts.Proc,
	}
	m.status = NewStatus(opts.LingtaiDir)
	if opts.InitialView == ViewWizard {
		m.wizard = setup.NewWizardModel(opts.LingtaiDir)
	}
	if opts.Config != nil && opts.Proc != nil {
		m.chat = NewChat(opts.Config, opts.Proc)
	}
	return m
}

func (m RootModel) Init() tea.Cmd {
	switch m.view {
	case ViewWizard:
		return m.wizard.Init()
	case ViewChat:
		return m.chat.Init()
	default:
		return m.status.Init()
	}
}

// windowSizeCmd returns a tea.Cmd that injects a synthetic WindowSizeMsg.
func (m RootModel) windowSizeCmd() tea.Cmd {
	if m.width == 0 && m.height == 0 {
		return nil
	}
	w, h := m.width, m.height
	return func() tea.Msg {
		return tea.WindowSizeMsg{Width: w, Height: h}
	}
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Capture window size globally before routing to sub-views.
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
		// Fall through to sub-view routing below.
	}

	switch msg := msg.(type) {
	case StatusTransitionMsg:
		switch msg.Target {
		case ViewWizard:
			os.MkdirAll(m.lingtaiDir, 0755)
			m.wizard = setup.NewWizardModel(m.lingtaiDir)
			m.view = ViewWizard
			return m, tea.Batch(m.wizard.Init(), m.windowSizeCmd())
		case ViewChat:
			if m.proc == nil {
				// Show loading screen, start agent asynchronously
				m.view = ViewStarting
				configPath := m.configPath
				return m, func() tea.Msg {
					cfg, err := config.Load(configPath)
					if err != nil {
						return agentStartedMsg{err: err}
					}
					proc, err := agent.Start(agent.StartOptions{
						ConfigPath: configPath,
						AgentPort:  cfg.AgentPort,
						WorkingDir: cfg.WorkingDir(),
						Headless:   true,
					})
					return agentStartedMsg{config: cfg, proc: proc, err: err}
				}
			}
			m.view = ViewChat
			return m, tea.Batch(m.chat.Init(), m.windowSizeCmd())
		}

	case agentStartedMsg:
		if msg.err != nil {
			// Failed — show error on status page
			m.status.errMsg = fmt.Sprintf("Agent failed to start: %v", msg.err)
			m.view = ViewStatus
			return m, nil
		}
		m.config = msg.config
		m.proc = msg.proc
		m.chat = NewChat(msg.config, msg.proc)
		m.view = ViewChat
		// Inject WindowSizeMsg so ChatModel can initialize its viewport.
		return m, tea.Batch(m.chat.Init(), m.windowSizeCmd())

	case ChatExitMsg:
		// Stop mail listener before leaving chat
		m.chat.stopListener()
		m.status.scan()
		m.view = ViewStatus
		return m, nil
	}

	// Route to active sub-view
	switch m.view {
	case ViewWizard:
		updated, cmd := m.wizard.Update(msg)
		m.wizard = updated.(setup.WizardModel)
		// Check if wizard is done
		if m.wizard.Done() {
			if m.wizard.Err() == nil {
				// Wizard completed successfully — reload config and go to status
				cfg, err := config.Load(m.configPath)
				if err == nil {
					m.config = cfg
				}
			}
			m.status.scan()
			m.view = ViewStatus
			return m, nil
		}
		return m, cmd

	case ViewChat:
		updated, cmd := m.chat.Update(msg)
		m.chat = updated.(ChatModel)
		return m, cmd

	case ViewStatus:
		var cmd tea.Cmd
		m.status, cmd = m.status.Update(msg)
		return m, cmd
	}

	return m, nil
}

var startingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))

func (m RootModel) View() string {
	switch m.view {
	case ViewWizard:
		return m.wizard.View()
	case ViewChat:
		return m.chat.View()
	case ViewStarting:
		return fmt.Sprintf("\n  %s  %s\n", startingStyle.Render(i18n.S("title")), i18n.S("starting"))
	default:
		return m.status.View()
	}
}

// RunTUI starts the unified TUI.
func RunTUI(opts RootOpts) {
	m := NewRoot(opts)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		os.Exit(1)
	}
	// Use the final model from p.Run() — not the stale original m.
	if final, ok := finalModel.(RootModel); ok && final.proc != nil {
		final.proc.Stop()
	}
}
