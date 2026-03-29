package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
	tea "github.com/charmbracelet/bubbletea"
)

type appView int

const (
	appViewFirstRun appView = iota
	appViewMail
	appViewManage
	appViewSetup
	appViewSettings
	appViewPresets
	appViewProps
)

// App is the root Bubble Tea model. Routes between views via slash commands.
type App struct {
	currentView appView
	mail        MailModel
	manage      ManageModel
	setup       SetupModel
	settings    SettingsModel
	presets     PresetsModel
	props       PropsModel
	firstRun    FirstRunModel

	globalDir     string
	projectDir    string // .lingtai/ directory
	vizURL        string
	orchDir       string // full path to orchestrator dir
	orchName      string
	lingtaiCmd    string
	width         int
	height        int
	appSettings   Settings
	pendingRename bool
	pendingLang   bool
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

// NewApp creates the root app model.
func NewApp(globalDir, projectDir, vizURL string, needsFirstRun bool, orchestrators []string, settings Settings) App {
	lingtaiCmd := config.LingtaiCmd(globalDir)

	app := App{
		globalDir:   globalDir,
		projectDir:  projectDir,
		vizURL:      vizURL,
		lingtaiCmd:  lingtaiCmd,
		appSettings: settings,
	}

	if needsFirstRun {
		app.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		app.firstRun = NewFirstRunModel(projectDir, globalDir, hasPresets)
	} else {
		// Determine orchestrator
		if len(orchestrators) == 1 {
			app.orchName = orchestrators[0]
			app.orchDir = filepath.Join(projectDir, orchestrators[0])
		} else if len(orchestrators) > 1 {
			// Check saved setting
			if settings.Orchestrator != "" {
				// Verify it still exists
				found := false
				for _, o := range orchestrators {
					if o == settings.Orchestrator {
						found = true
						break
					}
				}
				if found {
					app.orchName = settings.Orchestrator
					app.orchDir = filepath.Join(projectDir, settings.Orchestrator)
				}
			}
			// If no saved or stale, use first (app could prompt, but keep simple for now)
			if app.orchName == "" {
				app.orchName = orchestrators[0]
				app.orchDir = filepath.Join(projectDir, orchestrators[0])
				settings.Orchestrator = orchestrators[0]
				SaveSettings(projectDir, settings)
			}
		}

		app.currentView = appViewMail
		humanDir := filepath.Join(projectDir, "human")
		addr := humanAddr(projectDir)
		app.mail = NewMailModel(humanDir, addr, projectDir, app.orchDir, app.orchName, settings.PollRate)
		app.manage = NewManageModel(projectDir, lingtaiCmd)
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
		case appViewManage:
			a.manage, cmd = a.manage.Update(msg)
		case appViewSetup:
			a.setup, cmd = a.setup.Update(msg)
		case appViewSettings:
			a.settings, cmd = a.settings.Update(msg)
		case appViewPresets:
			a.presets, cmd = a.presets.Update(msg)
		case appViewProps:
			a.props, cmd = a.props.Update(msg)
		case appViewFirstRun:
			a.firstRun, cmd = a.firstRun.Update(msg)
		}
		return a, cmd

	// === Cross-view messages ===

	case ViewChangeMsg:
		return a.switchToView(msg.View)

	case refreshDoneMsg:
		a.mail.AddSystemMessage(i18n.T("mail.refreshed"))
		return a, a.mail.refreshMail

	case PaletteSelectMsg:
		return a.handlePaletteCommand(msg.Command, msg.Args)

	case FirstRunDoneMsg:
		// First-run complete: launch agent and switch to mail
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
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.appSettings.PollRate)
		a.manage = NewManageModel(a.projectDir, a.lingtaiCmd)
		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

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
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.appSettings.PollRate)
		a.manage = NewManageModel(a.projectDir, a.lingtaiCmd)
		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	// === Global keys ===

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "q":
			// Only quit if not in a text input context
			if a.currentView != appViewSetup && a.currentView != appViewFirstRun && a.currentView != appViewMail && a.currentView != appViewProps {
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
		// Intercept SendMsg for pending rename or lang
		if _, ok := msg.(SendMsg); ok && (a.pendingRename || a.pendingLang) {
			text := strings.TrimSpace(a.mail.input.Value())
			a.mail.input.Reset()
			if a.pendingRename {
				a.pendingRename = false
				if text != "" {
					a.doRename(text)
				}
			} else if a.pendingLang {
				a.pendingLang = false
				a.doLang(text)
				return a, func() tea.Msg {
					a.hardRefresh()
					return refreshDoneMsg{}
				}
			}
			return a, a.mail.refreshMail
		}
		updated, cmd := a.mail.Update(msg)
		a.mail = updated
		return a, cmd
	case appViewManage:
		updated, cmd := a.manage.Update(msg)
		a.manage = updated
		return a, cmd
	case appViewSetup:
		var cmd tea.Cmd
		a.setup, cmd = a.setup.Update(msg)
		return a, cmd
	case appViewSettings:
		updated, cmd := a.settings.Update(msg)
		a.settings = updated
		return a, cmd
	case appViewPresets:
		updated, cmd := a.presets.Update(msg)
		a.presets = updated
		return a, cmd
	case appViewProps:
		updated, cmd := a.props.Update(msg)
		a.props = updated
		return a, cmd
	}

	return a, nil
}

func (a App) handlePaletteCommand(command, args string) (tea.Model, tea.Cmd) {
	switch command {
	case "sleep":
		if a.orchDir != "" {
			sleepFile := filepath.Join(a.orchDir, ".sleep")
			os.WriteFile(sleepFile, []byte(""), 0o644)
			a.mail.AddSystemMessage(i18n.T("mail.sleep_sent"))
		}
		return a, nil
	case "sleep-all":
		agents, _ := fs.DiscoverAgents(a.projectDir)
		count := 0
		for _, agent := range agents {
			if agent.IsHuman {
				continue
			}
			if fs.IsAlive(agent.WorkingDir, 3.0) {
				sleepFile := filepath.Join(agent.WorkingDir, ".sleep")
				os.WriteFile(sleepFile, []byte(""), 0o644)
				count++
			}
		}
		a.mail.AddSystemMessage(i18n.TF("mail.sleep_all", count))
		return a, nil
	case "suspend":
		if a.orchDir != "" {
			suspendFile := filepath.Join(a.orchDir, ".suspend")
			os.WriteFile(suspendFile, []byte(""), 0o644)
			a.mail.AddSystemMessage(i18n.TF("mail.suspended", a.orchName))
		}
		return a, nil
	case "suspend-all":
		agents, _ := fs.DiscoverAgents(a.projectDir)
		count := 0
		for _, agent := range agents {
			if agent.IsHuman {
				continue
			}
			if fs.IsAlive(agent.WorkingDir, 3.0) {
				// Cancel all schedules
				schedulesDir := filepath.Join(agent.WorkingDir, "mailbox", "schedules")
				os.MkdirAll(schedulesDir, 0o755)
				os.WriteFile(filepath.Join(schedulesDir, ".cancel"), []byte(""), 0o644)
				// Suspend
				suspendFile := filepath.Join(agent.WorkingDir, ".suspend")
				os.WriteFile(suspendFile, []byte(""), 0o644)
				count++
			}
		}
		a.mail.AddSystemMessage(i18n.TF("mail.suspended_all", count))
		return a, nil
	case "cpr":
		if a.orchDir != "" && a.lingtaiCmd != "" {
			if !fs.IsAlive(a.orchDir, 3.0) {
				process.LaunchAgent(a.lingtaiCmd, a.orchDir)
				a.mail.AddSystemMessage(i18n.TF("mail.cpr", a.orchName))
			} else {
				a.mail.AddSystemMessage(i18n.T("mail.cpr_alive"))
			}
		}
		return a, nil
	case "nickname":
		if args != "" {
			humanPath := filepath.Join(a.projectDir, "human", ".agent.json")
			if data, err := os.ReadFile(humanPath); err == nil {
				var manifest map[string]interface{}
				if err := json.Unmarshal(data, &manifest); err == nil {
					manifest["nickname"] = args
					if out, err := json.MarshalIndent(manifest, "", "  "); err == nil {
						os.WriteFile(humanPath, out, 0o644)
					}
				}
			}
			a.mail.AddSystemMessage(i18n.TF("mail.nick_set", args))
		} else {
			a.mail.AddSystemMessage(i18n.T("mail.nick_prompt"))
		}
		return a, nil
	case "rename":
		if a.orchDir != "" {
			if args != "" {
				a.doRename(args)
				return a, func() tea.Msg {
					a.hardRefresh()
					return refreshDoneMsg{}
				}
			}
			a.pendingRename = true
			a.mail.AddSystemMessage(i18n.T("mail.rename_prompt"))
		}
		return a, nil
	case "lang":
		if a.orchDir != "" {
			if args != "" {
				a.doLang(args)
				return a, func() tea.Msg {
					a.hardRefresh()
					return refreshDoneMsg{}
				}
			} else {
				a.mail.AddSystemMessage(i18n.T("mail.lang_prompt"))
			}
		}
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
				// Wipe conversation history
				os.Remove(filepath.Join(a.orchDir, "history", "chat_history.jsonl"))
				os.Remove(filepath.Join(a.orchDir, "history", "status.json"))
				// Relaunch with clean context
				process.LaunchAgent(a.lingtaiCmd, a.orchDir)
				return refreshDoneMsg{}
			}
		}
		return a, nil
	case "refresh":
		if a.orchDir != "" && a.lingtaiCmd != "" {
			a.mail.AddSystemMessage(i18n.T("mail.refreshing"))
			return a, func() tea.Msg {
				a.hardRefresh()
				return refreshDoneMsg{}
			}
		}
		return a, nil
	case "manage":
		a.currentView = appViewManage
		a.manage = NewManageModel(a.projectDir, a.lingtaiCmd)
		return a, tea.Batch(a.manage.Init(), a.sendSize())
	case "viz":
		// Open browser, stay on mail
		openBrowser(a.vizURL)
		return a, nil
	case "setup":
		a.currentView = appViewSetup
		a.setup = NewSetupModel(a.globalDir)
		return a, tea.Batch(a.setup.Init(), a.sendSize())
	case "settings":
		a.currentView = appViewSettings
		settings := LoadSettings(a.projectDir)
		a.settings = NewSettingsModel(a.projectDir, a.globalDir, settings)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "presets":
		a.currentView = appViewPresets
		a.presets = NewPresetsModel()
		return a, tea.Batch(a.presets.Init(), a.sendSize())
	case "help":
		// Render help inline as a system message in the chat stream
		helpText := i18n.T("help.title") + "\n" +
			i18n.T("help.sleep") + "\n" +
			i18n.T("help.sleep_all") + "\n" +
			i18n.T("help.suspend") + "\n" +
			i18n.T("help.suspend_all") + "\n" +
			i18n.T("help.cpr") + "\n" +
			i18n.T("help.nickname") + "\n" +
			i18n.T("help.rename") + "\n" +
			i18n.T("help.lang") + "\n" +
			i18n.T("help.clear") + "\n" +
			i18n.T("help.refresh") + "\n" +
			i18n.T("help.manage") + "\n" +
			i18n.T("help.viz") + "\n" +
			i18n.T("help.setup") + "\n" +
			i18n.T("help.settings") + "\n" +
			i18n.T("help.presets") + "\n" +
			i18n.T("help.help") + "\n" +
			i18n.T("help.verbose")
		a.mail.messages = append(a.mail.messages, ChatMessage{
			From: i18n.T("mail.system_sender"),
			Body: helpText,
			Type: "mail",
		})
		if a.mail.ready {
			a.mail.viewport.SetContent(a.mail.renderMessages())
			a.mail.viewport.GotoBottom()
		}
		return a, nil
	case "quit":
		return a, tea.Quit
	}
	return a, nil
}

func (a *App) doRename(newName string) {
	// Update init.json agent_name
	initPath := filepath.Join(a.orchDir, "init.json")
	if data, err := os.ReadFile(initPath); err == nil {
		var init map[string]interface{}
		if err := json.Unmarshal(data, &init); err == nil {
			if m, ok := init["manifest"].(map[string]interface{}); ok {
				m["agent_name"] = newName
			}
			if out, err := json.MarshalIndent(init, "", "  "); err == nil {
				os.WriteFile(initPath, out, 0o644)
			}
		}
	}
	a.orchName = newName
	a.mail.orchName = newName
	a.mail.AddSystemMessage(i18n.TF("mail.renamed", newName))
}

func (a *App) doLang(lang string) {
	valid := map[string]bool{"en": true, "zh": true, "wen": true}
	if !valid[lang] {
		a.mail.AddSystemMessage(i18n.TF("mail.lang_invalid", lang))
		return
	}
	initPath := filepath.Join(a.orchDir, "init.json")
	if data, err := os.ReadFile(initPath); err == nil {
		var initData map[string]interface{}
		if err := json.Unmarshal(data, &initData); err == nil {
			if m, ok := initData["manifest"].(map[string]interface{}); ok {
				m["language"] = lang
			}
			initData["covenant_file"] = preset.CovenantPath(a.globalDir, lang)
			initData["principle_file"] = preset.PrinciplePath(a.globalDir, lang)
			delete(initData, "covenant")  // use file, not inline
			delete(initData, "principle") // use file, not inline
			if out, err := json.MarshalIndent(initData, "", "  "); err == nil {
				os.WriteFile(initPath, out, 0o644)
			}
		}
	}
	a.mail.AddSystemMessage(i18n.TF("mail.lang_changed", lang))
}

// hardRefresh suspends the orchestrator and relaunches it.
// Used by /rename and /refresh to force a full reload from init.json.
func (a *App) hardRefresh() {
	if a.orchDir == "" || a.lingtaiCmd == "" {
		return
	}
	// Suspend
	suspendFile := filepath.Join(a.orchDir, ".suspend")
	os.WriteFile(suspendFile, []byte(""), 0o644)
	// Wait for lock file to be released (process fully exited)
	lockFile := filepath.Join(a.orchDir, ".agent.lock")
	for i := 0; i < 40; i++ { // 40 × 250ms = 10s max
		if tryLock(lockFile) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	// Clean signal files before relaunch
	os.Remove(suspendFile)
	// Relaunch
	process.LaunchAgent(a.lingtaiCmd, a.orchDir)
}

// tryLock attempts a non-blocking flock on the lock file. Returns true if lock
// was acquired (meaning no other process holds it), and releases immediately.
func tryLock(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return true // can't open → assume not locked
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return false // locked by another process
	}
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return true
}

// sendSize returns a tea.Cmd that sends the current terminal dimensions to the
// newly created view so it doesn't render with zero width/height.
func (a App) sendSize() tea.Cmd {
	w, h := a.width, a.height
	return func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }
}

type refreshDoneMsg struct{}

func (a App) switchToView(viewName string) (tea.Model, tea.Cmd) {
	switch viewName {
	case "mail":
		a.currentView = appViewMail
		// Restart mail tick + refresh (tick dies when another view is active)
		return a, tea.Batch(a.mail.refreshMail, tickEvery(a.mail.pollRate), a.sendSize())
	case "manage":
		a.currentView = appViewManage
		a.manage = NewManageModel(a.projectDir, a.lingtaiCmd)
		return a, tea.Batch(a.manage.Init(), a.sendSize())
	case "setup":
		a.currentView = appViewSetup
		a.setup = NewSetupModel(a.globalDir)
		return a, tea.Batch(a.setup.Init(), a.sendSize())
	case "settings":
		a.currentView = appViewSettings
		settings := LoadSettings(a.projectDir)
		a.settings = NewSettingsModel(a.projectDir, a.globalDir, settings)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "presets":
		a.currentView = appViewPresets
		a.presets = NewPresetsModel()
		return a, tea.Batch(a.presets.Init(), a.sendSize())
	case "props":
		a.currentView = appViewProps
		a.props = NewPropsModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "welcome":
		a.currentView = appViewFirstRun
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, true)
		a.firstRun.welcomeOnly = true
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	}
	return a, nil
}

func (a App) View() string {
	switch a.currentView {
	case appViewFirstRun:
		return a.firstRun.View()
	case appViewMail:
		return a.mail.View()
	case appViewManage:
		return a.manage.View()
	case appViewSetup:
		return a.setup.View()
	case appViewSettings:
		return a.settings.View()
	case appViewPresets:
		return a.presets.View()
	case appViewProps:
		return a.props.View()
	}
	return ""
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
		cmd = "xdg-open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	}
	if cmd != "" {
		exec.Command(cmd, args...).Start()
	}
}
