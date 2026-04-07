package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
	tea "charm.land/bubbletea/v2"
)

type appView int

const (
	appViewFirstRun appView = iota
	appViewMail
	appViewSetup
	appViewSettings
	appViewProps
	appViewAddon
	appViewDoctor
	appViewTutorial
	appViewNirvana
	appViewSkills
	appViewProjects
)

// App is the root Bubble Tea model. Routes between views via slash commands.
type App struct {
	currentView appView
	mail        MailModel
	setup       SetupModel
	settings    SettingsModel
	props       PropsModel
	skills      SkillsModel
	projects    ProjectsModel
	firstRun    FirstRunModel
	addon       AddonModel
	doctor      DoctorModel
	tutorial    TutorialConfirmModel
	nirvana     NirvanaModel

	globalDir     string
	projectDir    string // .lingtai/ directory
	orchDir       string // full path to orchestrator dir
	orchName      string
	lingtaiCmd    string
	width         int
	height        int
	tuiConfig   config.TUIConfig
}

func humanAddr(projectDir string) string {
	return "human"
}

// NewApp creates the root app model.
func NewApp(globalDir, projectDir string, needsFirstRun bool, orchestrators []string, tuiCfg config.TUIConfig) App {
	// Apply persisted theme (or default).
	SetThemeByName(tuiCfg.Theme)

	lingtaiCmd := config.LingtaiCmd(globalDir)

	app := App{
		globalDir:  globalDir,
		projectDir: projectDir,
		lingtaiCmd: lingtaiCmd,
		tuiConfig:  tuiCfg,
	}

	if needsFirstRun {
		app.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		app.firstRun = NewFirstRunModel(projectDir, globalDir, hasPresets)
	} else {
		// Determine orchestrator
		localSettings := LoadSettings(projectDir)
		if len(orchestrators) == 1 {
			app.orchName = orchestrators[0]
			app.orchDir = filepath.Join(projectDir, orchestrators[0])
		} else if len(orchestrators) > 1 {
			// Check saved setting
			if localSettings.Orchestrator != "" {
				// Verify it still exists
				found := false
				for _, o := range orchestrators {
					if o == localSettings.Orchestrator {
						found = true
						break
					}
				}
				if found {
					app.orchName = localSettings.Orchestrator
					app.orchDir = filepath.Join(projectDir, localSettings.Orchestrator)
				}
			}
			// If no saved or stale, use first (app could prompt, but keep simple for now)
			if app.orchName == "" {
				app.orchName = orchestrators[0]
				app.orchDir = filepath.Join(projectDir, orchestrators[0])
				localSettings.Orchestrator = orchestrators[0]
				SaveSettings(projectDir, localSettings)
			}
		}

		app.currentView = appViewMail
		humanDir := filepath.Join(projectDir, "human")
		addr := humanAddr(projectDir)
		app.mail = NewMailModel(humanDir, addr, projectDir, app.orchDir, app.orchName, tuiCfg.MailPageSize, tuiCfg.Greeting, globalDir, tuiCfg.Language, tuiCfg.Insights)

	}

	return app
}

func (a App) Init() tea.Cmd {
	switch a.currentView {
	case appViewFirstRun:
		return a.firstRun.Init()
	case appViewMail:
		return a.mail.Init()
	}
	return nil
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Forward to current view so it can resize
		var cmd tea.Cmd
		switch a.currentView {
		case appViewMail:
			a.mail, cmd = a.mail.Update(msg)
		case appViewSetup:
			a.setup, cmd = a.setup.Update(msg)
		case appViewSettings:
			a.settings, cmd = a.settings.Update(msg)
		case appViewProps:
			a.props, cmd = a.props.Update(msg)
		case appViewAddon:
			a.addon, cmd = a.addon.Update(msg)
		case appViewDoctor:
			a.doctor, cmd = a.doctor.Update(msg)
		case appViewTutorial:
			a.tutorial, cmd = a.tutorial.Update(msg)
		case appViewNirvana:
			a.nirvana, cmd = a.nirvana.Update(msg)
		case appViewSkills:
			a.skills, cmd = a.skills.Update(msg)
		case appViewProjects:
			a.projects, cmd = a.projects.Update(msg)
		case appViewFirstRun:
			a.firstRun, cmd = a.firstRun.Update(msg)
		}
		return a, cmd

	// === Cross-view messages ===

	case ViewChangeMsg:
		return a.switchToView(msg.View)

	case doctorResultMsg:
		if a.currentView == appViewDoctor {
			a.doctor, _ = a.doctor.Update(msg)
		}
		return a, nil

	case refreshDoneMsg:
		a.mail.AddSystemMessage(i18n.T("mail.refreshed"))
		return a, a.mail.refreshMail

	case refreshAllDoneMsg:
		a.mail.AddSystemMessage(i18n.TF("mail.refresh_all", msg.count))
		return a, a.mail.refreshMail

	case PaletteSelectMsg:
		return a.handlePaletteCommand(msg.Command, msg.Args)

	case FirstRunDoneMsg:
		// First-run complete: launch agent and switch to mail.
		// Ensure human folder exists before launching — InitProject is
		// idempotent and prevents the race where the agent tries to
		// send mail before the human mailbox is ready.
		if err := process.InitProject(a.projectDir); err != nil {
			a.currentView = appViewMail
			humanDir := filepath.Join(a.projectDir, "human")
			addr := humanAddr(a.projectDir)
			a.mail = NewMailModel(humanDir, addr, a.projectDir, "", "", a.tuiConfig.MailPageSize, false, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)
			a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", err))
			return a, tea.Batch(a.mail.Init(), a.sendSize())
		}
		a.orchDir = msg.OrchDir
		a.orchName = msg.OrchName
		// Launch the agent
		var launchErr string
		if a.lingtaiCmd != "" {
			if _, err := process.LaunchAgent(a.lingtaiCmd, a.orchDir); err != nil {
				launchErr = i18n.TF("mail.launch_failed", err)
			}
		}
		// Initialize mail view
		a.currentView = appViewMail
		humanDir := filepath.Join(a.projectDir, "human")
		addr := humanAddr(a.projectDir)
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.tuiConfig.Greeting, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)

		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	case TutorialConfirmDoneMsg:
		// Tutorial confirmed: .lingtai/ has been wiped and rebuilt
		a.orchDir = msg.OrchDir
		a.orchName = msg.OrchName
		a.currentView = appViewMail
		humanDir := filepath.Join(a.projectDir, "human")
		addr := humanAddr(a.projectDir)
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, false, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)

		return a, tea.Batch(a.mail.Init(), a.sendSize())

	case NirvanaDoneMsg:
		// Nirvana complete: .lingtai/ wiped, go to first-run.
		// Re-init project to recreate the human folder so agents can
		// deliver mail once the new orchestrator starts.
		process.InitProject(a.projectDir)
		a.orchDir = ""
		a.orchName = ""
		a.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, hasPresets)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())

	case AddonSavedMsg:
		a.mail.AddSystemMessage(i18n.T("addon.saved"))
		return a.switchToView("mail")

	case SetupSavedMsg:
		a.mail.AddSystemMessage(i18n.T("setup.saved_refresh"))
		return a.switchToView("mail")

	case SetupDoneMsg:
		// During first-run, forward to firstrun model (needs to create default preset)
		if a.currentView == appViewFirstRun {
			updated, cmd := a.firstRun.Update(msg)
			a.firstRun = updated
			return a, cmd
		}
		return a.switchToView("mail")

	case UsePresetMsg:
		// Create agent from preset
		process.InitProject(a.projectDir)
		p, err := preset.Load(msg.Name)
		if err != nil {
			return a, nil
		}
		agentName := p.Name
		if err := preset.GenerateInitJSON(p, agentName, agentName, a.projectDir, a.globalDir); err != nil {
			return a, nil
		}
		orchDir := filepath.Join(a.projectDir, agentName)
		var launchErr string
		if a.lingtaiCmd != "" {
			if _, err := process.LaunchAgent(a.lingtaiCmd, orchDir); err != nil {
				launchErr = i18n.TF("mail.launch_failed", err)
			}
		}
		a.orchDir = orchDir
		a.orchName = agentName
		a.currentView = appViewMail
		humanDir := filepath.Join(a.projectDir, "human")
		addr := humanAddr(a.projectDir)
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.tuiConfig.Greeting, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)

		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	// === Global keys ===

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "q":
			// Only quit if not in a text input context
			if a.currentView != appViewSetup && a.currentView != appViewFirstRun && a.currentView != appViewMail && a.currentView != appViewProps && a.currentView != appViewAddon && a.currentView != appViewNirvana && a.currentView != appViewSkills && a.currentView != appViewProjects {
				return a, tea.Quit
			}
		}
	}

	// === Forward to current view ===
	switch a.currentView {
	case appViewFirstRun:
		updated, cmd := a.firstRun.Update(msg)
		a.firstRun = updated
		return a, cmd
	case appViewMail:
		updated, cmd := a.mail.Update(msg)
		a.mail = updated
		return a, cmd
	case appViewSetup:
		var cmd tea.Cmd
		a.setup, cmd = a.setup.Update(msg)
		return a, cmd
	case appViewSettings:
		updated, cmd := a.settings.Update(msg)
		a.settings = updated
		return a, cmd
	case appViewProps:
		updated, cmd := a.props.Update(msg)
		a.props = updated
		return a, cmd
	case appViewAddon:
		updated, cmd := a.addon.Update(msg)
		a.addon = updated
		return a, cmd
	case appViewDoctor:
		updated, cmd := a.doctor.Update(msg)
		a.doctor = updated
		return a, cmd
	case appViewTutorial:
		updated, cmd := a.tutorial.Update(msg)
		a.tutorial = updated
		return a, cmd
	case appViewNirvana:
		updated, cmd := a.nirvana.Update(msg)
		a.nirvana = updated
		return a, cmd
	case appViewSkills:
		updated, cmd := a.skills.Update(msg)
		a.skills = updated
		return a, cmd
	case appViewProjects:
		updated, cmd := a.projects.Update(msg)
		a.projects = updated
		return a, cmd
	}

	return a, nil
}

func (a App) handlePaletteCommand(command, args string) (tea.Model, tea.Cmd) {
	switch command {
	case "sleep":
		if args == "all" {
			agents, _ := fs.DiscoverAgents(a.projectDir)
			count := 0
			for _, agent := range agents {
				if agent.IsHuman {
					continue
				}
				if fs.IsAlive(agent.WorkingDir, 3.0) {
					os.WriteFile(filepath.Join(agent.WorkingDir, ".sleep"), []byte(""), 0o644)
					count++
				}
			}
			a.mail.AddSystemMessage(i18n.TF("mail.sleep_all", count))
		} else if a.orchDir != "" {
			os.WriteFile(filepath.Join(a.orchDir, ".sleep"), []byte(""), 0o644)
			a.mail.AddSystemMessage(i18n.T("mail.sleep_sent"))
		}
		return a, nil
	case "suspend":
		if args == "all" {
			agents, _ := fs.DiscoverAgents(a.projectDir)
			count := 0
			for _, agent := range agents {
				if agent.IsHuman {
					continue
				}
				if fs.IsAlive(agent.WorkingDir, 3.0) {
					os.WriteFile(filepath.Join(agent.WorkingDir, ".suspend"), []byte(""), 0o644)
					count++
				}
			}
			a.mail.AddSystemMessage(i18n.TF("mail.suspended_all", count))
		} else if a.orchDir != "" {
			os.WriteFile(filepath.Join(a.orchDir, ".suspend"), []byte(""), 0o644)
			a.mail.AddSystemMessage(i18n.TF("mail.suspended", a.orchName))
		}
		return a, nil
	case "cpr":
		if args == "all" {
			agents, _ := fs.DiscoverAgents(a.projectDir)
			count := 0
			for _, agent := range agents {
				if agent.IsHuman {
					continue
				}
				if !fs.IsAlive(agent.WorkingDir, 3.0) && a.lingtaiCmd != "" {
					process.LaunchAgent(a.lingtaiCmd, agent.WorkingDir)
					count++
				}
			}
			a.mail.AddSystemMessage(i18n.TF("mail.cpr_all", count))
		} else if a.orchDir != "" && a.lingtaiCmd != "" {
			if !fs.IsAlive(a.orchDir, 3.0) {
				process.LaunchAgent(a.lingtaiCmd, a.orchDir)
				a.mail.AddSystemMessage(i18n.TF("mail.cpr", a.orchName))
			} else {
				a.mail.AddSystemMessage(i18n.T("mail.cpr_alive"))
			}
		}
		return a, nil
	case "lang":
		// Redirect to /settings — agent language is now configured there
		a.mail.AddSystemMessage(i18n.T("mail.lang_moved"))
		return a, nil
	case "clear":
		if a.orchDir != "" && a.lingtaiCmd != "" {
			a.mail.AddSystemMessage(i18n.T("mail.clearing"))
			return a, func() tea.Msg {
				// Suspend and wait for process to die
				suspendFile := filepath.Join(a.orchDir, ".suspend")
				os.WriteFile(suspendFile, []byte(""), 0o644)
				lockFile := filepath.Join(a.orchDir, ".agent.lock")
				for i := 0; i < 40; i++ {
					if tryLock(lockFile) {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
				os.Remove(suspendFile)
				// Wipe conversation history (token ledger is preserved)
				os.Remove(filepath.Join(a.orchDir, "history", "chat_history.jsonl"))
				// Relaunch with clean context
				process.LaunchAgent(a.lingtaiCmd, a.orchDir)
				return refreshDoneMsg{}
			}
		}
		return a, nil
	case "refresh":
		if args == "all" && a.lingtaiCmd != "" {
			a.mail.AddSystemMessage(i18n.T("mail.refreshing_all"))
			lingtaiCmd := a.lingtaiCmd
			projectDir := a.projectDir
			return a, func() tea.Msg {
				agents, _ := fs.DiscoverAgents(projectDir)
				count := 0
				for _, agent := range agents {
					if agent.IsHuman {
						continue
					}
					hardRefreshDir(lingtaiCmd, agent.WorkingDir)
					count++
				}
				return refreshAllDoneMsg{count: count}
			}
		} else if a.orchDir != "" && a.lingtaiCmd != "" {
			a.mail.AddSystemMessage(i18n.T("mail.refreshing"))
			return a, func() tea.Msg {
				a.hardRefresh()
				return refreshDoneMsg{}
			}
		}
		return a, nil
	case "doctor":
		if a.orchDir != "" {
			a.currentView = appViewDoctor
			a.doctor = NewDoctorModel(a.orchDir)
			return a, tea.Batch(a.doctor.Init(), a.sendSize())
		}
		return a, nil
	case "viz":
		url := a.portalURL()
		if url != "" {
			openBrowser(url)
		} else {
			a.mail.AddSystemMessage("lingtai-portal not found on PATH. Run: brew link --overwrite lingtai-tui")
		}
		return a, nil
	case "addon":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.orchDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "tutorial":
		a.currentView = appViewTutorial
		a.tutorial = NewTutorialConfirmModel(a.projectDir, a.globalDir, a.lingtaiCmd, a.tuiConfig.Language)
		return a, tea.Batch(a.tutorial.Init(), a.sendSize())
	case "setup":
		a.currentView = appViewFirstRun
		a.firstRun = NewSetupModeModel(a.projectDir, a.globalDir, a.orchDir, a.orchName)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	case "settings":
		a.currentView = appViewSettings
		tuiCfg := config.LoadTUIConfig(a.globalDir)
		a.settings = NewSettingsModel(a.globalDir, a.projectDir, a.orchDir, tuiCfg)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "nirvana":
		a.currentView = appViewNirvana
		a.nirvana = NewNirvanaModel(a.projectDir)
		return a, tea.Batch(a.nirvana.Init(), a.sendSize())
	case "kanban":
		a.currentView = appViewProps
		a.props = NewPropsModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "skills":
		a.currentView = appViewSkills
		a.skills = NewSkillsModel(a.projectDir)
		return a, tea.Batch(a.skills.Init(), a.sendSize())
	case "projects":
		a.currentView = appViewProjects
		a.projects = NewProjectsModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.projects.Init(), a.sendSize())
	case "insights":
		if a.orchDir != "" {
			if !fs.IsAlive(a.orchDir, 3.0) {
				a.mail.AddSystemMessage(i18n.T("mail.btw_suspended"))
				return a, nil
			}
			question := i18n.T("insight.auto_question")
			fs.WriteInquiry(a.orchDir, "insight", question)
			a.mail.AddSystemMessage(i18n.T("mail.insight_sent"))
		}
		return a, nil
	case "btw":
		if a.orchDir != "" && args != "" {
			if !fs.IsAlive(a.orchDir, 3.0) {
				a.mail.AddSystemMessage(i18n.T("mail.btw_suspended"))
				return a, nil
			}
			fs.WriteInquiry(a.orchDir, "human", args)
			a.mail.AddSystemMessage(i18n.TF("mail.btw_sent", args))
		} else if args == "" {
			a.mail.AddSystemMessage(i18n.T("mail.btw_usage"))
		}
		return a, nil
	case "quit":
		return a, tea.Quit
	}
	return a, nil
}

// hardRefresh suspends the orchestrator and relaunches it.
// Used by /rename and /refresh to force a full reload from init.json.
func (a *App) hardRefresh() {
	if a.orchDir == "" || a.lingtaiCmd == "" {
		return
	}
	hardRefreshDir(a.lingtaiCmd, a.orchDir)
}

// hardRefreshDir suspends the agent in the given directory and relaunches it.
func hardRefreshDir(lingtaiCmd, dir string) {
	suspendFile := filepath.Join(dir, ".suspend")
	os.WriteFile(suspendFile, []byte(""), 0o644)
	lockFile := filepath.Join(dir, ".agent.lock")
	for i := 0; i < 40; i++ { // 40 × 250ms = 10s max
		if tryLock(lockFile) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	os.Remove(suspendFile)
	process.LaunchAgent(lingtaiCmd, dir)
}

// tryLock is defined in lock_unix.go / lock_windows.go

// sendSize returns a tea.Cmd that sends the current terminal dimensions to the
// newly created view so it doesn't render with zero width/height.
func (a App) sendSize() tea.Cmd {
	w, h := a.width, a.height
	return func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }
}

type refreshDoneMsg struct{}
type refreshAllDoneMsg struct{ count int }

func (a App) switchToView(viewName string) (tea.Model, tea.Cmd) {
	switch viewName {
	case "mail":
		a.currentView = appViewMail
		// Reload config in case settings changed it
		a.tuiConfig = config.LoadTUIConfig(a.globalDir)
		ps := a.tuiConfig.MailPageSize
		if ps <= 0 {
			ps = unlimitedPageSize
		}
		a.mail.pageSize = ps
		a.mail.insightsEnabled = a.tuiConfig.Insights
		// Re-apply theme to textarea (settings may have changed it)
		a.mail.input.ApplyTheme()
		// Restart mail tick + refresh + pulse (ticks die when another view is active)
		return a, tea.Batch(a.mail.refreshMail, tickEvery(a.mail.pollRate), pulseTick(), a.sendSize())
	case "setup":
		a.currentView = appViewFirstRun
		a.firstRun = NewSetupModeModel(a.projectDir, a.globalDir, a.orchDir, a.orchName)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	case "settings":
		a.currentView = appViewSettings
		tuiCfg := config.LoadTUIConfig(a.globalDir)
		a.settings = NewSettingsModel(a.globalDir, a.projectDir, a.orchDir, tuiCfg)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "props", "kanban":
		a.currentView = appViewProps
		a.props = NewPropsModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "skills":
		a.currentView = appViewSkills
		a.skills = NewSkillsModel(a.projectDir)
		return a, tea.Batch(a.skills.Init(), a.sendSize())
	case "projects":
		a.currentView = appViewProjects
		a.projects = NewProjectsModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.projects.Init(), a.sendSize())
	case "addon":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.orchDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "welcome":
		a.currentView = appViewFirstRun
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, true)
		a.firstRun.welcomeOnly = true
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	}
	return a, nil
}

func (a App) View() tea.View {
	var content string
	switch a.currentView {
	case appViewFirstRun:
		content = a.firstRun.View()
	case appViewMail:
		content = a.mail.View()
	case appViewSetup:
		content = a.setup.View()
	case appViewSettings:
		content = a.settings.View()
	case appViewProps:
		content = a.props.View()
	case appViewAddon:
		content = a.addon.View()
	case appViewDoctor:
		content = a.doctor.View()
	case appViewTutorial:
		content = a.tutorial.View()
	case appViewNirvana:
		content = a.nirvana.View()
	case appViewSkills:
		content = a.skills.View()
	case appViewProjects:
		content = a.projects.View()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	t := ActiveTheme()
	if t.PaintBG {
		v.BackgroundColor = t.BG
		v.ForegroundColor = t.Text
	}
	return v
}

// portalURL kills any existing portal and spawns a fresh one.
// Returns the URL or empty string if lingtai-portal is not on PATH.
func (a *App) portalURL() string {
	portFile := filepath.Join(a.projectDir, ".portal", "port")

	// Kill existing portal so we always get a fresh instance with the latest binary
	exec.Command("pkill", "-f", "lingtai-portal.*--dir.*"+filepath.Dir(a.projectDir)).Run()
	os.Remove(portFile)
	time.Sleep(300 * time.Millisecond)

	// Spawn fresh portal
	portalCmd, _ := exec.LookPath("lingtai-portal")
	if portalCmd == "" {
		return ""
	}
	cmd := exec.Command(portalCmd, "--dir", filepath.Dir(a.projectDir))
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return ""
	}
	// Release the process so it survives TUI exit
	cmd.Process.Release()

	// Wait for port file to appear (up to 3 seconds)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if data, err := os.ReadFile(portFile); err == nil {
			return "http://localhost:" + strings.TrimSpace(string(data))
		}
	}
	return ""
}

func isWSL() bool {
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(b))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

func openBrowser(url string) {
	if url == "" {
		return
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		if isWSL() {
			// Prefer wslview (wslu) — handles WSL→Windows browser opening natively.
			// Fallback: powershell.exe Start-Process (more reliable than cmd.exe start
			// with URLs containing colons).
			if path, err := exec.LookPath("wslview"); err == nil {
				cmd = path
				args = []string{url}
			} else {
				cmd = "powershell.exe"
				args = []string{"-NoProfile", "-Command", "Start-Process", "'" + url + "'"}
			}
		} else {
			cmd = "xdg-open"
			args = []string{url}
		}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	}
	if cmd != "" {
		exec.Command(cmd, args...).Start()
	}
}
