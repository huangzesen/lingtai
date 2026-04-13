package tui

import (
	"fmt"
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
	"github.com/anthropics/lingtai-tui/internal/secretary"
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
	appViewNirvana
	appViewSkills
	appViewProjects
	appViewBriefs
	appViewAgora
	appViewLogin
	appViewLibrary
)

// App is the root Bubble Tea model. Routes between views via slash commands.
type App struct {
	currentView appView
	mail        MailModel
	setup       SetupModel
	settings    SettingsModel
	props       PropsModel
	skills          MarkdownViewerModel
	projects        ProjectsModel
	agora           AgoraModel
	secretaryMail   MailModel
	briefs          MarkdownViewerModel
	library         MarkdownViewerModel
	firstRun        FirstRunModel
	addon           AddonModel
	doctor          DoctorModel
	nirvana         NirvanaModel
	login           LoginModel

	inSecretaryView bool      // true when viewing secretary mail (within appViewMail)
	lastEscTime     time.Time // for double-esc detection

	globalDir       string
	projectDir      string // .lingtai/ directory
	orchDir         string // full path to orchestrator dir
	orchName        string
	lingtaiCmd      string
	width           int
	height          int
	tuiConfig       config.TUIConfig
	pendingRecipe   string
	pendingCustomDir string
	recoveryMode    bool // global config lost, agents intact — setup then propagate
}

func humanAddr(projectDir string) string {
	return "human"
}


// NewApp creates the root app model.
// NewApp constructs the top-level TUI app.
//
// rehydrateOrchDir and rehydrateOrchName, when both non-empty, signal that
// the network is a cloned agora network awaiting rehydration. The app
// enters first-run view with a FirstRunModel constructed via
// NewRehydrateModel, which prefills the orchestrator's name/dir and adds
// a final stepPropagate page to copy the new init.json to every worker.
func NewApp(globalDir, projectDir string, needsFirstRun, needsRecovery bool, orchestrators []string, tuiCfg config.TUIConfig, rehydrateOrchDir, rehydrateOrchName string) App {
	// Apply persisted theme (or default).
	SetThemeByName(tuiCfg.Theme)

	lingtaiCmd := config.LingtaiCmd(globalDir)

	app := App{
		globalDir:  globalDir,
		projectDir: projectDir,
		lingtaiCmd: lingtaiCmd,
		tuiConfig:  tuiCfg,
	}

	if needsRecovery && len(orchestrators) > 0 {
		// Global config lost but agents intact — show setup for API keys,
		// then propagate LLM config to all agents and go to mail view.
		orchName := orchestrators[0]
		orchDir := filepath.Join(projectDir, orchName)
		// Check per-project settings for saved orchestrator
		localSettings := LoadSettings(projectDir)
		if localSettings.Orchestrator != "" {
			for _, o := range orchestrators {
				if o == localSettings.Orchestrator {
					orchName = o
					orchDir = filepath.Join(projectDir, o)
					break
				}
			}
		}
		app.orchName = orchName
		app.orchDir = orchDir
		app.recoveryMode = true
		app.currentView = appViewFirstRun
		app.firstRun = NewSetupModeModel(projectDir, globalDir, orchDir, orchName)
	} else if needsFirstRun {
		app.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		if rehydrateOrchDir != "" && rehydrateOrchName != "" {
			app.firstRun = NewRehydrateModel(projectDir, globalDir, rehydrateOrchDir, rehydrateOrchName, hasPresets)
		} else {
			app.firstRun = NewFirstRunModel(projectDir, globalDir, hasPresets, "")
		}
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
		app.mail = NewMailModel(humanDir, addr, projectDir, app.orchDir, app.orchName, tuiCfg.MailPageSize, globalDir, tuiCfg.Language, tuiCfg.Insights)

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
			if a.inSecretaryView {
				a.secretaryMail, cmd = a.secretaryMail.Update(msg)
			} else {
				a.mail, cmd = a.mail.Update(msg)
			}
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
		case appViewNirvana:
			a.nirvana, cmd = a.nirvana.Update(msg)
		case appViewSkills:
			a.skills, cmd = a.skills.Update(msg)
		case appViewBriefs:
			a.briefs, cmd = a.briefs.Update(msg)
		case appViewProjects:
			a.projects, cmd = a.projects.Update(msg)
		case appViewAgora:
			a.agora, cmd = a.agora.Update(msg)
		case appViewFirstRun:
			a.firstRun, cmd = a.firstRun.Update(msg)
		case appViewLogin:
			a.login, cmd = a.login.Update(msg)
		case appViewLibrary:
			a.library, cmd = a.library.Update(msg)
		}
		return a, cmd

	// === Cross-view messages ===

	case ViewChangeMsg:
		return a.switchToView(msg.View)

	case MarkdownViewerCloseMsg:
		// If agora is active, forward to AgoraModel (detail → list, not mail)
		if a.currentView == appViewAgora {
			updated, cmd := a.agora.Update(msg)
			a.agora = updated
			return a, cmd
		}
		a.currentView = appViewMail
		if a.inSecretaryView {
			return a, tea.Batch(a.secretaryMail.refreshMail, tickEvery(a.secretaryMail.pollRate), pulseTick(), a.sendSize())
		}
		return a, tea.Batch(a.mail.refreshMail, tickEvery(a.mail.pollRate), pulseTick(), a.sendSize())

	case doctorResultMsg:
		if a.currentView == appViewDoctor {
			a.doctor, _ = a.doctor.Update(msg)
		}
		return a, nil

	case loginHealthMsg:
		if a.currentView == appViewLogin {
			a.login, _ = a.login.Update(msg)
		}
		return a, nil

	case CodexOAuthDoneMsg:
		if a.currentView == appViewLogin {
			a.login, _ = a.login.Update(msg)
		} else if a.currentView == appViewFirstRun {
			a.firstRun, _ = a.firstRun.Update(msg)
		}
		return a, nil

	case refreshDoneMsg:
		if a.inSecretaryView {
			if msg.err != nil {
				a.secretaryMail.AddSystemMessage(i18n.TF("mail.launch_failed", firstLine(msg.err)))
			} else {
				a.secretaryMail.AddSystemMessage(i18n.T("mail.refreshed"))
			}
			return a, a.secretaryMail.refreshMail
		}
		if msg.err != nil {
			a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", firstLine(msg.err)))
		} else {
			a.mail.AddSystemMessage(i18n.T("mail.refreshed"))
		}
		return a, a.mail.refreshMail

	case refreshAllDoneMsg:
		if a.inSecretaryView {
			if len(msg.failures) > 0 {
				a.secretaryMail.AddSystemMessage(i18n.TF("mail.refresh_all_with_failures", msg.count-len(msg.failures), len(msg.failures), strings.Join(msg.failures, ", ")))
			} else {
				a.secretaryMail.AddSystemMessage(i18n.TF("mail.refresh_all", msg.count))
			}
			return a, a.secretaryMail.refreshMail
		}
		if len(msg.failures) > 0 {
			a.mail.AddSystemMessage(i18n.TF("mail.refresh_all_with_failures", msg.count-len(msg.failures), len(msg.failures), strings.Join(msg.failures, ", ")))
		} else {
			a.mail.AddSystemMessage(i18n.TF("mail.refresh_all", msg.count))
		}
		return a, a.mail.refreshMail

	case PaletteSelectMsg:
		return a.handlePaletteCommand(msg.Command, msg.Args)

	case FirstRunDoneMsg:
		// First-run complete: launch agent and switch to mail.
		// Reload tuiConfig from disk so any settings the wizard saved
		// (theme, mail page size, insights) are reflected downstream.
		// a.tuiConfig was captured at NewApp time and is otherwise stale
		// after the wizard's SaveTUIConfig calls.
		a.tuiConfig = config.LoadTUIConfig(a.globalDir)
		// Ensure human folder exists before launching — InitProject is
		// idempotent and prevents the race where the agent tries to
		// send mail before the human mailbox is ready.
		if err := process.InitProject(a.projectDir, a.globalDir); err != nil {
			a.currentView = appViewMail
			humanDir := filepath.Join(a.projectDir, "human")
			addr := humanAddr(a.projectDir)
			a.mail = NewMailModel(humanDir, addr, a.projectDir, "", "", a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)
			a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", err))
			return a, tea.Batch(a.mail.Init(), a.sendSize())
		}
		a.orchDir = msg.OrchDir
		a.orchName = msg.OrchName
		// Propagate LLM config to all agents in the network
		PropagateOrchestratorConfig(a.projectDir, a.orchDir)
		// Launch the agent
		var launchErr string
		if a.lingtaiCmd != "" {
			if _, err := process.LaunchAgent(a.lingtaiCmd, a.orchDir); err != nil {
				launchErr = i18n.TF("mail.launch_failed", err)
			}
		}
		// Launch secretary if confirmed
		if msg.LaunchSecretary && a.lingtaiCmd != "" {
			secDir := secretary.AgentDir(a.globalDir)
			if _, err := process.LaunchAgent(a.lingtaiCmd, secDir); err != nil {
				launchErr += "\nSecretary launch failed: " + err.Error()
			}
		}
		// Initialize mail view
		a.currentView = appViewMail
		humanDir := filepath.Join(a.projectDir, "human")
		addr := humanAddr(a.projectDir)
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)

		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	case RecipeFreshStartMsg:
		a.pendingRecipe = msg.Recipe
		a.pendingCustomDir = msg.CustomDir
		a.currentView = appViewNirvana
		a.nirvana = NewNirvanaModel(a.projectDir)
		return a, tea.Batch(a.nirvana.Init(), a.sendSize())

	case NirvanaDoneMsg:
		// Nirvana complete: .lingtai/ wiped, go to first-run.
		// Re-init project to recreate the human folder so agents can
		// deliver mail once the new orchestrator starts.
		process.InitProject(a.projectDir, a.globalDir)
		a.orchDir = ""
		a.orchName = ""
		a.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		preselected := a.pendingRecipe
		a.pendingRecipe = ""
		pendingCustom := a.pendingCustomDir
		a.pendingCustomDir = ""
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, hasPresets, preselected)
		if preselected == preset.RecipeCustom && pendingCustom != "" {
			a.firstRun.recipeCustomInput.SetValue(pendingCustom)
		}
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())

	case AddonSavedMsg:
		a.mail.AddSystemMessage(i18n.T("addon.saved"))
		return a.switchToView("mail")

	case SetupSavedMsg:
		if a.recoveryMode {
			// Recovery: global config was lost but agents are intact.
			// Propagate the new LLM + capabilities to all agents, init
			// the mail view, and launch the orchestrator.
			a.recoveryMode = false
			a.tuiConfig = config.LoadTUIConfig(a.globalDir)
			PropagateOrchestratorConfig(a.projectDir, a.orchDir)
			a.currentView = appViewMail
			humanDir := filepath.Join(a.projectDir, "human")
			addr := humanAddr(a.projectDir)
			a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)
			if a.lingtaiCmd != "" {
				if _, err := process.LaunchAgent(a.lingtaiCmd, a.orchDir); err != nil {
					a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", err))
				}
			}
			return a, tea.Batch(a.mail.Init(), a.sendSize())
		}
		PropagateOrchestratorConfig(a.projectDir, a.orchDir)
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
		process.InitProject(a.projectDir, a.globalDir)
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
		a.mail = NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)

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
			if a.currentView != appViewSetup && a.currentView != appViewFirstRun && a.currentView != appViewMail && a.currentView != appViewProps && a.currentView != appViewAddon && a.currentView != appViewNirvana && a.currentView != appViewSkills && a.currentView != appViewBriefs && a.currentView != appViewProjects && a.currentView != appViewAgora && a.currentView != appViewLogin && a.currentView != appViewLibrary {
				return a, tea.Quit
			}
		}
	}

	// === Double-esc: toggle back from secretary view ===
	if a.inSecretaryView && a.currentView == appViewMail {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "esc" {
			now := time.Now()
			if now.Sub(a.lastEscTime) < 500*time.Millisecond {
				// Double-esc detected — toggle back to main mail
				a.inSecretaryView = false
				a.lastEscTime = time.Time{}
				return a, tea.Batch(a.mail.refreshMail, tickEvery(a.mail.pollRate), pulseTick(), a.sendSize())
			}
			a.lastEscTime = now
			// Fall through to let the first esc do its normal thing (dismiss insights)
		}
	}

	// === Forward to current view ===
	switch a.currentView {
	case appViewFirstRun:
		updated, cmd := a.firstRun.Update(msg)
		a.firstRun = updated
		return a, cmd
	case appViewMail:
		if a.inSecretaryView {
			updated, cmd := a.secretaryMail.Update(msg)
			a.secretaryMail = updated
			return a, cmd
		}
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
	case appViewNirvana:
		updated, cmd := a.nirvana.Update(msg)
		a.nirvana = updated
		return a, cmd
	case appViewSkills:
		updated, cmd := a.skills.Update(msg)
		a.skills = updated
		return a, cmd
	case appViewBriefs:
		updated, cmd := a.briefs.Update(msg)
		a.briefs = updated
		return a, cmd
	case appViewProjects:
		updated, cmd := a.projects.Update(msg)
		a.projects = updated
		return a, cmd
	case appViewAgora:
		updated, cmd := a.agora.Update(msg)
		a.agora = updated
		return a, cmd
	case appViewLogin:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd
	case appViewLibrary:
		updated, cmd := a.library.Update(msg)
		a.library = updated
		return a, cmd
	}

	return a, nil
}

func (a App) handlePaletteCommand(command, args string) (tea.Model, tea.Cmd) {
	// addMsg posts a system message to whichever mail view the user is looking at.
	addMsg := func(text string) {
		if a.inSecretaryView {
			a.secretaryMail.AddSystemMessage(text)
		} else {
			a.mail.AddSystemMessage(text)
		}
	}
	// targetDir/targetName: the agent that single-target commands operate on.
	// In secretary view, target the secretary; otherwise the project orchestrator.
	targetDir := a.orchDir
	targetName := a.orchName
	if a.inSecretaryView {
		targetDir = secretary.AgentDir(a.globalDir)
		targetName = "secretary"
	}
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
			addMsg(i18n.TF("mail.sleep_all", count))
		} else if targetDir != "" {
			os.WriteFile(filepath.Join(targetDir, ".sleep"), []byte(""), 0o644)
			addMsg(i18n.T("mail.sleep_sent"))
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
			addMsg(i18n.TF("mail.suspended_all", count))
		} else if targetDir != "" {
			os.WriteFile(filepath.Join(targetDir, ".suspend"), []byte(""), 0o644)
			addMsg(i18n.TF("mail.suspended", targetName))
		}
		return a, nil
	case "cpr":
		if args == "all" {
			agents, _ := fs.DiscoverAgents(a.projectDir)
			count := 0
			var failures []string
			for _, agent := range agents {
				if agent.IsHuman {
					continue
				}
				if !fs.IsAlive(agent.WorkingDir, 3.0) && a.lingtaiCmd != "" {
					count++
					if _, err := process.LaunchAgent(a.lingtaiCmd, agent.WorkingDir); err != nil {
						failures = append(failures, fmt.Sprintf("%s (%s)", filepath.Base(agent.WorkingDir), firstLine(err)))
					}
				}
			}
			if len(failures) > 0 {
				addMsg(i18n.TF("mail.cpr_all_with_failures", count-len(failures), len(failures), strings.Join(failures, ", ")))
			} else {
				addMsg(i18n.TF("mail.cpr_all", count))
			}
		} else if targetDir != "" && a.lingtaiCmd != "" {
			if !fs.IsAlive(targetDir, 3.0) {
				if _, err := process.LaunchAgent(a.lingtaiCmd, targetDir); err != nil {
					addMsg(i18n.TF("mail.launch_failed", firstLine(err)))
				} else {
					addMsg(i18n.TF("mail.cpr", targetName))
				}
			} else {
				addMsg(i18n.T("mail.cpr_alive"))
			}
		}
		return a, nil
	case "lang":
		// Redirect to /settings — agent language is now configured there
		addMsg(i18n.T("mail.lang_moved"))
		return a, nil
	case "clear":
		if targetDir != "" && a.lingtaiCmd != "" {
			addMsg(i18n.T("mail.clearing"))
			lingtaiCmd := a.lingtaiCmd
			dir := targetDir
			return a, func() tea.Msg {
				// Suspend and wait for process to die
				suspendFile := filepath.Join(dir, ".suspend")
				os.WriteFile(suspendFile, []byte(""), 0o644)
				lockFile := filepath.Join(dir, ".agent.lock")
				for i := 0; i < 40; i++ {
					if tryLock(lockFile) {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
				os.Remove(suspendFile)
				// Wipe conversation history (token ledger is preserved)
				os.Remove(filepath.Join(dir, "history", "chat_history.jsonl"))
				// Relaunch with clean context
				_, err := process.LaunchAgent(lingtaiCmd, dir)
				return refreshDoneMsg{err: err}
			}
		}
		return a, nil
	case "refresh":
		if args == "all" && a.lingtaiCmd != "" {
			addMsg(i18n.T("mail.refreshing_all"))
			lingtaiCmd := a.lingtaiCmd
			projectDir := a.projectDir
			return a, func() tea.Msg {
				agents, _ := fs.DiscoverAgents(projectDir)
				count := 0
				var failures []string
				for _, agent := range agents {
					if agent.IsHuman {
						continue
					}
					count++
					if err := hardRefreshDir(lingtaiCmd, agent.WorkingDir); err != nil {
						failures = append(failures, fmt.Sprintf("%s (%s)", filepath.Base(agent.WorkingDir), firstLine(err)))
					}
				}
				return refreshAllDoneMsg{count: count, failures: failures}
			}
		} else if targetDir != "" && a.lingtaiCmd != "" {
			addMsg(i18n.T("mail.refreshing"))
			lingtaiCmd := a.lingtaiCmd
			dir := targetDir
			return a, func() tea.Msg {
				return refreshDoneMsg{err: hardRefreshDir(lingtaiCmd, dir)}
			}
		}
		return a, nil
	case "doctor":
		if targetDir != "" {
			a.currentView = appViewDoctor
			a.doctor = NewDoctorModel(targetDir)
			return a, tea.Batch(a.doctor.Init(), a.sendSize())
		}
		return a, nil
	case "viz":
		url := a.portalURL()
		if url != "" {
			openBrowser(url)
		} else {
			addMsg("lingtai-portal not found on PATH. Run: brew link --overwrite lingtai-tui")
		}
		return a, nil
	case "addon":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.projectDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "login":
		a.currentView = appViewLogin
		a.login = NewLoginModel(a.orchDir, a.globalDir)
		return a, tea.Batch(a.login.Init(), a.sendSize())
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
		if a.inSecretaryView {
			secLingtaiDir := secretary.LingtaiDir(a.globalDir)
			secAgentDir := secretary.AgentDir(a.globalDir)
			a.props = NewPropsModel(secLingtaiDir, secAgentDir)
		} else {
			a.props = NewPropsModel(a.projectDir, a.orchDir)
		}
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "skills":
		a.currentView = appViewSkills
		skillsDir := filepath.Join(a.projectDir, ".skills")
		if a.inSecretaryView {
			skillsDir = filepath.Join(secretary.LingtaiDir(a.globalDir), ".skills")
		}
		sk, prob := scanSkills(skillsDir)
		a.skills = NewMarkdownViewer(buildSkillEntries(skillsDir, a.tuiConfig.Language, sk, prob), i18n.T("skills.title"))
		return a, a.sendSize()
	case "secretary":
		if a.inSecretaryView {
			// Toggle back to main agent mail
			a.inSecretaryView = false
			a.currentView = appViewMail
			return a, tea.Batch(a.mail.refreshMail, tickEvery(a.mail.pollRate), pulseTick(), a.sendSize())
		}
		// Toggle to secretary mail — auto-setup if needed
		// Prune stale projects from registry so the secretary doesn't
		// waste cycles on deleted projects.
		config.LoadAndPrune(a.globalDir)
		secAgentDir := secretary.AgentDir(a.globalDir)
		initPath := filepath.Join(secAgentDir, "init.json")
		if _, err := os.Stat(initPath); err != nil && a.lingtaiCmd != "" {
			orchDirName := filepath.Base(a.orchDir)
			if err := setupSecretary(a.projectDir, a.globalDir, orchDirName); err != nil {
				addMsg(i18n.TF("mail.launch_failed", firstLine(err)))
				return a, nil
			}
		}
		// Auto-revive: if the secretary is dead, CPR + send greet prompt
		if a.lingtaiCmd != "" && !fs.IsAlive(secAgentDir, 3.0) {
			if _, err := process.LaunchAgent(a.lingtaiCmd, secAgentDir); err != nil {
				addMsg(i18n.TF("mail.launch_failed", firstLine(err)))
			} else {
				fs.WritePrompt(secAgentDir, secretary.GreetContent())
			}
		}
		a.inSecretaryView = true
		a.currentView = appViewMail
		if a.secretaryMail.humanDir == "" {
			// First entry — create the MailModel
			secLingtaiDir := secretary.LingtaiDir(a.globalDir)
			secHumanDir := filepath.Join(secLingtaiDir, "human")
			a.secretaryMail = NewMailModel(secHumanDir, "human", secLingtaiDir, secAgentDir, "secretary", a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)
			a.secretaryMail.brandOverride = i18n.T("app.brand_secretary")
			a.secretaryMail.palette.ExcludeCommands("viz", "addon", "setup", "agora", "export", "nirvana")
			return a, tea.Batch(a.secretaryMail.Init(), a.sendSize())
		}
		// Re-entry — reuse existing state, just restart ticks
		return a, tea.Batch(a.secretaryMail.refreshMail, tickEvery(a.secretaryMail.pollRate), pulseTick(), a.sendSize())
	case "projects":
		a.currentView = appViewProjects
		a.projects = NewProjectsModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.projects.Init(), a.sendSize())
	case "brief":
		// Open briefs viewer (standalone markdown viewer with secretary briefing files)
		a.currentView = appViewBriefs
		entries := buildSecretaryBriefs(a.globalDir, a.projectDir)
		a.briefs = NewMarkdownViewer(entries, i18n.T("palette.brief"))
		// Also trigger a briefing cycle if secretary is alive
		secAgentDir := secretary.AgentDir(a.globalDir)
		if a.lingtaiCmd != "" && fs.IsAlive(secAgentDir, 3.0) {
			prompt := "[system] The human has requested an immediate briefing. " +
				"Run the briefing skill now — skip scheduling and process all pending history."
			fs.WritePrompt(secAgentDir, prompt)
		}
		return a, a.sendSize()
	case "library":
		a.currentView = appViewLibrary
		entries := buildLibraryEntries(a.projectDir)
		a.library = NewMarkdownViewer(entries, i18n.T("palette.library"))
		return a, a.sendSize()
	case "agora":
		a.currentView = appViewAgora
		a.agora = NewAgoraModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.agora.Init(), a.sendSize())
	case "export":
		if args != "network" && args != "recipe" {
			addMsg(i18n.T("export.help"))
			return a, nil
		}
		if a.orchDir == "" {
			addMsg(i18n.T("export.no_agent"))
			return a, nil
		}
		if !fs.IsAlive(a.orchDir, 3.0) {
			addMsg(i18n.T("mail.btw_suspended"))
			return a, nil
		}
		if args == "network" {
			fs.WritePrompt(a.orchDir, i18n.T("export.network_prompt"))
			addMsg(i18n.T("export.network_sent"))
		} else {
			fs.WritePrompt(a.orchDir, i18n.T("export.recipe_prompt"))
			addMsg(i18n.T("export.recipe_sent"))
		}
		return a, nil
	case "molt":
		if targetDir == "" {
			return a, nil
		}
		if !fs.IsAlive(targetDir, 3.0) {
			addMsg(i18n.T("mail.btw_suspended"))
			return a, nil
		}
		// Send in agent's language, not TUI language
		lang := "en"
		if manifest, err := fs.ReadInitManifest(targetDir); err == nil {
			if l, ok := manifest["language"].(string); ok && l != "" {
				lang = l
			}
		}
		fs.WritePrompt(targetDir, i18n.TIn(lang, "molt.mandatory_prompt"))
		addMsg(i18n.T("mail.molt_sent"))
		return a, nil
	case "insights":
		if targetDir != "" {
			if !fs.IsAlive(targetDir, 3.0) {
				addMsg(i18n.T("mail.btw_suspended"))
				return a, nil
			}
			question := i18n.T("insight.auto_question")
			fs.WriteInquiry(targetDir, "insight", question)
			addMsg(i18n.T("mail.insight_sent"))
		}
		return a, nil
	case "btw":
		if targetDir != "" && args != "" {
			if !fs.IsAlive(targetDir, 3.0) {
				addMsg(i18n.T("mail.btw_suspended"))
				return a, nil
			}
			fs.WriteInquiry(targetDir, "human", args)
			addMsg(i18n.TF("mail.btw_sent", args))
		} else if args == "" {
			addMsg(i18n.T("mail.btw_usage"))
		}
		return a, nil
	case "quit":
		return a, tea.Quit
	}
	return a, nil
}

// hardRefresh suspends the orchestrator and relaunches it.
// Used by /refresh to force a full reload from init.json.
// Returns the error from process.LaunchAgent if the relaunch fails.
func (a *App) hardRefresh() error {
	if a.orchDir == "" || a.lingtaiCmd == "" {
		return nil
	}
	return hardRefreshDir(a.lingtaiCmd, a.orchDir)
}

// hardRefreshDir suspends the agent in the given directory and relaunches it.
// Returns the error from process.LaunchAgent if the relaunch fails. The
// suspend/wait/unsuspend dance always runs to completion regardless of the
// final launch outcome — only the launch error itself is propagated.
func hardRefreshDir(lingtaiCmd, dir string) error {
	suspendFile := filepath.Join(dir, ".suspend")
	os.WriteFile(suspendFile, []byte(""), 0o644)
	lockFile := filepath.Join(dir, ".agent.lock")
	locked := true
	for i := 0; i < 120; i++ { // 120 × 500ms = 60s max
		if tryLock(lockFile) {
			locked = false
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if locked {
		// Process likely died without releasing lock — clean up
		os.Remove(lockFile)
	}
	os.Remove(suspendFile)
	_, err := process.LaunchAgent(lingtaiCmd, dir)
	return err
}

// firstLine returns the first line of err.Error(), trimmed of trailing
// whitespace. Used to sanitize errors before they are rendered in the
// single-line status bar — embedded newlines from wrapped subprocess
// stderr (e.g., Python tracebacks captured by EnsureAddons) would
// otherwise corrupt the layout by pushing the status bar across multiple
// rows.
func firstLine(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, " \t\r")
}

// tryLock is defined in lock_unix.go / lock_windows.go

// sendSize returns a tea.Cmd that sends the current terminal dimensions to the
// newly created view so it doesn't render with zero width/height.
func (a App) sendSize() tea.Cmd {
	w, h := a.width, a.height
	return func() tea.Msg { return tea.WindowSizeMsg{Width: w, Height: h} }
}

// RecipeFreshStartMsg is emitted from stepRecipeSwapConfirm when the user
// chooses "Fresh start (wipe .lingtai/ and reconfigure)". The app routes
// this to NirvanaModel and stores the recipe so post-nirvana first-run
// can pre-select it.
type RecipeFreshStartMsg struct {
	Recipe    string
	CustomDir string
}

type refreshDoneMsg struct{ err error }
type refreshAllDoneMsg struct {
	count    int
	failures []string
}

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
		if a.inSecretaryView {
			// Return to secretary mail (came from kanban/doctor/etc.)
			a.secretaryMail.pageSize = ps
			a.secretaryMail.insightsEnabled = a.tuiConfig.Insights
			a.secretaryMail.input.ApplyTheme()
			return a, tea.Batch(a.secretaryMail.refreshMail, tickEvery(a.secretaryMail.pollRate), pulseTick(), a.sendSize())
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
		if a.inSecretaryView {
			secLingtaiDir := secretary.LingtaiDir(a.globalDir)
			secAgentDir := secretary.AgentDir(a.globalDir)
			a.props = NewPropsModel(secLingtaiDir, secAgentDir)
		} else {
			a.props = NewPropsModel(a.projectDir, a.orchDir)
		}
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "skills":
		a.currentView = appViewSkills
		skillsDir := filepath.Join(a.projectDir, ".skills")
		if a.inSecretaryView {
			skillsDir = filepath.Join(secretary.LingtaiDir(a.globalDir), ".skills")
		}
		sk, prob := scanSkills(skillsDir)
		a.skills = NewMarkdownViewer(buildSkillEntries(skillsDir, a.tuiConfig.Language, sk, prob), i18n.T("skills.title"))
		return a, a.sendSize()
	case "secretary":
		// switchToView always goes to secretary mail (no toggle — toggle is in handlePaletteCommand)
		secAgentDir := secretary.AgentDir(a.globalDir)
		secLingtaiDir := secretary.LingtaiDir(a.globalDir)
		secHumanDir := filepath.Join(secLingtaiDir, "human")
		a.secretaryMail = NewMailModel(secHumanDir, "human", secLingtaiDir, secAgentDir, "secretary", a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights)
		a.secretaryMail.brandOverride = i18n.T("app.brand_secretary")
		a.secretaryMail.palette.ExcludeCommands("viz", "addon", "setup", "agora", "export", "nirvana")
		a.inSecretaryView = true
		a.currentView = appViewMail
		return a, tea.Batch(a.secretaryMail.Init(), a.sendSize())
	case "projects":
		a.currentView = appViewProjects
		a.projects = NewProjectsModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.projects.Init(), a.sendSize())
	case "agora":
		a.currentView = appViewAgora
		a.agora = NewAgoraModel(a.globalDir, a.projectDir)
		return a, tea.Batch(a.agora.Init(), a.sendSize())
	case "addon":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.projectDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "welcome":
		a.currentView = appViewFirstRun
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, true, "")
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
		if a.inSecretaryView {
			content = a.secretaryMail.View()
		} else {
			content = a.mail.View()
		}
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
	case appViewNirvana:
		content = a.nirvana.View()
	case appViewSkills:
		content = a.skills.View()
	case appViewBriefs:
		content = a.briefs.View()
	case appViewProjects:
		content = a.projects.View()
	case appViewAgora:
		content = a.agora.View()
	case appViewLogin:
		content = a.login.View()
	case appViewLibrary:
		content = a.library.View()
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
