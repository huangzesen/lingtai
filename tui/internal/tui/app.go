package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/config"
	"github.com/anthropics/lingtai-tui/internal/fs"
	"github.com/anthropics/lingtai-tui/internal/preset"
	"github.com/anthropics/lingtai-tui/internal/process"
)

type appView int

const (
	appViewFirstRun appView = iota
	appViewMail
	appViewSettings
	appViewProps
	appViewAddon
	appViewDoctor
	appViewUpdate
	appViewUpdateTUI
	appViewNirvana
	appViewLibrary
	appViewProjects
	appViewLogin
	appViewKnowledge
	appViewMailbox
	appViewSystem
	appViewPresets
	appViewDaemons
	appViewNotification
	appViewHelp
	appViewTaskCard
)

const doubleEscReturnWindow = 600 * time.Millisecond

var appNow = time.Now

// App is the root Bubble Tea model. Routes between views via slash commands.
type App struct {
	currentView   appView
	mail          MailModel
	settings      SettingsModel
	props         PropsModel
	library       LibraryModel
	projects      ProjectsModel
	knowledge     KnowledgeModel
	system        SystemModel
	mailbox       MailboxModel
	daemons       DaemonsModel
	notification  NotificationModel
	presetLibrary PresetLibraryModel
	help          HelpModel
	taskcard      TaskCardModel
	firstRun      FirstRunModel
	addon         AddonModel
	doctor        DoctorModel
	update        UpdateModel
	updateTUI     UpdateTUIModel
	nirvana       NirvanaModel
	login         LoginModel

	globalDir    string
	projectDir   string // .lingtai/ directory
	orchDir      string // full path to orchestrator dir
	orchName     string
	lingtaiCmd   string
	width        int
	height       int
	tuiConfig    config.TUIConfig
	recoveryMode bool // global config lost, agents intact — setup then propagate
	// degradedConfig is true when ~/.lingtai-tui/config.json is missing but API
	// keys were derived from .env (the agents' source of truth). The app launches
	// normally with a persistent banner instead of a hard recovery gate;
	// key-dependent features are gated and a self-heal is offered.
	degradedConfig bool
	startupBanner  string // non-empty warning shown on first render
	// autoRefreshArmed is true while exactly one auto-refresh ticker is in
	// flight. It guards against starting a second concurrent ticker when the
	// feature is re-enabled or a view is re-entered. The autoRefreshTickMsg
	// handler keeps it true while it re-arms; turning the feature off lets the
	// loop lapse and flips this back to false.
	autoRefreshArmed bool
	// selectMode is the global ctrl+y "select text" mode for every view EXCEPT
	// mail. When on, View() drops mouse capture (so the terminal can drag-select
	// transcript text) and renders a top-chrome indicator. The mail view owns its
	// own copyMode (see mail.go) and never sets this flag; entering mail resets
	// it so the two badges can't both show.
	selectMode bool

	mailGeneration       uint64
	projectsActivationID uint64

	nirvanaCleanupPending bool
	nirvanaCleanupStarted bool

	visiting                bool
	visitOriginalProjectDir string
	visitOriginalOrchDir    string
	visitOriginalOrchName   string
	visitOriginalMail       MailModel
	visitOriginalProjects   ProjectsModel
	visitOriginalView       appView
	visitTargetProjectDir   string
	visitTargetAgentDir     string
	visitTargetAgentName    string
	doubleEscArmed          bool
	doubleEscFirstAt        time.Time
}

func humanAddr(projectDir string) string {
	return "human"
}

func (a *App) installMailModel(m MailModel) {
	explicitlyCollapsed := a.mail.agentRail.explicitlyCollapsed
	m.agentRail.explicitlyCollapsed = explicitlyCollapsed
	m = m.blurAgentRail()
	a.mailGeneration++
	m.generation = a.mailGeneration
	m.advancePollEpoch()
	// Detached activity work is activation-local. A first deferred rebuild may
	// briefly coexist with the first periodic refresh, but the periodic lane
	// itself remains single-flight thereafter.
	m.mailRefreshInFlight = false
	m.mailRefreshInFlightSerial = 0
	m.mailRefreshPending = false
	m.networkActivityInFlight = false
	// Durable direct-unread operation results are activation-local. A preserved
	// Mail model can return after prior lane results were routed through a
	// visited context, so a new activation must not retain unfinishable
	// in-flight/coalesced markers or accept a previous activation's operation.
	m.directUnreadOpInFlight = false
	m.directUnreadSyncPending = false
	m.directUnreadOpSerial = nextAcceptedSnapshotSerial(m.directUnreadOpSerial)
	// The Home telemetry row's display expression is an app-owned TUI preference,
	// not a property of the mail context being installed. Validating it here — the
	// one boundary every real construction and restore path goes through — keeps
	// it off NewMailModel's signature and out of every unrelated caller.
	m.homeTelemetryDisplay = homeTelemetryDisplayFromConfig(a.tuiConfig.HomeTelemetryDisplay)
	a.mail = m
}

// beginNirvanaCleanup synchronously retires queued Mail work before any
// destructive command can run. The one direct-unread durable operation already
// in flight keeps its serial so its exact terminal result can clear the lane;
// only its queued continuation is cancelled.
func (a App) beginNirvanaCleanup() (App, tea.Cmd) {
	if a.currentView != appViewNirvana || !a.nirvana.cleaning || a.nirvanaCleanupPending {
		return a, nil
	}

	a.nirvanaCleanupPending = true
	a.nirvanaCleanupStarted = false
	a.mailGeneration++
	a.mail.generation = a.mailGeneration
	a.mail.directUnreadSyncPending = false
	return a.maybeStartNirvanaCleanup()
}

func (a App) maybeStartNirvanaCleanup() (App, tea.Cmd) {
	if !a.nirvanaCleanupPending || a.nirvanaCleanupStarted || a.mail.directUnreadOpInFlight {
		return a, nil
	}
	a.nirvanaCleanupStarted = true
	return a, a.nirvana.doClean()
}

// issueMailRefresh issues the one refresh request serial on the Update loop
// for the installed Mail model, then detaches the real prepared-refresh command.
func (a *App) issueMailRefresh() tea.Cmd {
	var cmd tea.Cmd
	a.mail, cmd = a.mail.issueRefreshRequest()
	return cmd
}

// issueMailInitialRebuild re-dispatches the bounded initial rebuild under a
// freshly issued request serial so its completion competes honestly with any
// periodic refresh issued afterwards.
func (a *App) issueMailInitialRebuild() tea.Cmd {
	a.mail.refreshRequestSerial = nextAcceptedSnapshotSerial(a.mail.refreshRequestSerial)
	return a.mail.initialRebuild
}

func (a *App) newMailForCurrentContext() MailModel {
	humanDir := filepath.Join(a.projectDir, "human")
	addr := humanAddr(a.projectDir)
	return NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate)
}

func (a App) projectsContext() ProjectsContext {
	ctx := ProjectsContext{
		FocusedAgentDir:  a.orchDir,
		CurrentAgentName: a.orchName,
		Visiting:         a.visiting,
	}
	if a.visiting {
		ctx.OriginalProjectDir = a.visitOriginalProjectDir
		ctx.OriginalAgentDir = a.visitOriginalOrchDir
	}
	return ctx
}

func (a App) openProjectsView() (App, tea.Cmd) {
	a.currentView = appViewProjects
	a.projectsActivationID++
	if a.projectsActivationID == 0 {
		a.projectsActivationID = 1
	}
	a.projects = NewProjectsModelWithActivation(a.globalDir, a.projectDir, a.projectsContext(), a.projectsActivationID)
	return a, tea.Batch(a.projects.Init(), a.sendSize())
}

// NewApp creates the root app model.
// NewApp constructs the top-level TUI app.
//
// rehydrateOrchDir and rehydrateOrchName, when both non-empty, signal that
// the network is a cloned agora network awaiting rehydration. The app
// enters first-run view with a FirstRunModel constructed via
// NewRehydrateModel, which prefills the orchestrator's name/dir and adds
// a final stepPropagate page to copy the new init.json to every worker.
func NewApp(globalDir, projectDir string, needsFirstRun, needsRecovery, degradedConfig bool, orchestrators []string, tuiCfg config.TUIConfig, rehydrateOrchDir, rehydrateOrchName string) App {
	// Apply persisted theme (or default).
	SetThemeByName(tuiCfg.Theme)

	lingtaiCmd := config.LingtaiCmd(globalDir)

	app := App{
		globalDir:        globalDir,
		projectDir:       projectDir,
		lingtaiCmd:       lingtaiCmd,
		tuiConfig:        tuiCfg,
		autoRefreshArmed: tuiCfg.AutoRefreshEnabled(),
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
			app.firstRun = NewFirstRunModel(projectDir, globalDir, hasPresets)
		}
	} else {
		app.degradedConfig = degradedConfig
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
		app.installMailModel(NewMailModel(humanDir, addr, projectDir, app.orchDir, app.orchName, tuiCfg.MailPageSize, globalDir, tuiCfg.Language, tuiCfg.Insights, tuiCfg.ToolCallTruncate))

		// Validate codex-auth.json if any agent uses a codex preset.
		if warn := validateCodexAuthForAgents(globalDir, projectDir); warn != "" {
			app.startupBanner = warn
		}

	}

	return app
}

func (a App) Init() tea.Cmd {
	// The app-level auto-refresh tick runs alongside whatever the initial view
	// needs. It is a single ticker for all reloadable views (see
	// auto_refresh.go); each tick asks the current view to reload if it opts in
	// via autoReloadable. Started here when enabled, and re-armed on each tick.
	var cmds []tea.Cmd
	switch a.currentView {
	case appViewFirstRun:
		cmds = append(cmds, a.firstRun.Init())
	case appViewMail:
		cmds = append(cmds, a.mail.Init())
	}
	if a.tuiConfig.AutoRefreshEnabled() {
		// Init runs once on a value copy; the autoRefreshTickMsg handler owns
		// the armed flag from here on. Arming unconditionally here is safe
		// because no ticker exists yet at startup.
		cmds = append(cmds, autoRefreshTick())
	}
	return tea.Batch(cmds...)
}

// startAutoRefresh returns the App with an auto-refresh ticker armed, plus the
// command to run, but only if the feature is enabled and no ticker is already
// in flight. When a ticker already exists (or the feature is off) it returns
// the App unchanged and a nil command, so callers can invoke it freely (on view
// switch or settings change) without ever spawning a second concurrent ticker.
func (a App) startAutoRefresh() (App, tea.Cmd) {
	if !a.tuiConfig.AutoRefreshEnabled() || a.autoRefreshArmed {
		return a, nil
	}
	a.autoRefreshArmed = true
	return a, autoRefreshTick()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case childWindowSizeMsg:
		return a.updateChildWindowSize(msg.WindowSizeMsg)

	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		// Derive both axes from the root layout budget, then forward the
		// child content rectangle — never the raw terminal dimensions. See
		// layout.go (LayoutBudget) for the contract.
		budget := a.layoutBudget()
		if a.currentView == appViewMail &&
			!budget.RailVisible &&
			a.mail.agentRail.focused {
			a.mail = a.mail.blurAgentRail()
		}
		return a.updateChildWindowSize(budget.ChildWindowSize())

	case tea.MouseClickMsg:
		if a.currentView == appViewMail {
			return a.updateMailMouseClick(msg)
		}

	case tea.MouseWheelMsg:
		if a.currentView == appViewMail {
			return a.updateMailMouseWheel(msg)
		}

	case tea.PasteMsg:
		if a.currentView == appViewMail && a.layoutBudget().RailVisible && a.mail.agentRail.focused {
			return a, nil
		}

	case tea.FocusMsg:
		ApplyTerminalBG()
		return a, nil

	// === Cross-view messages ===

	case nirvanaCleanStartMsg:
		return a.beginNirvanaCleanup()

	case directUnreadResultMsg:
		// The exact terminal result must always pass through Mail so it can clear
		// the durable lane. During Nirvana retirement, discard every follow-up
		// command and start cleanup only after that lane is observably idle.
		var cmd tea.Cmd
		a.mail, cmd = a.mail.Update(msg)
		if a.nirvanaCleanupPending {
			if a.mail.directUnreadOpInFlight {
				return a, nil
			}
			return a.maybeStartNirvanaCleanup()
		}
		return a, cmd

	case mailRefreshMsg, mailPersistMsg, networkActivityMsg, homeTelemetryMsg, homeAsyncStatsMsg:
		// Mail content/count rebuilds, older pages, post-frame persistence, and
		// telemetry can outlive the view that launched them. Route all at the root so Projects/Help
		// cannot drop Mail's state machine; MailModel owns generation acceptance.
		var cmd tea.Cmd
		a.mail, cmd = a.mail.Update(msg)
		return a, cmd

	case ViewChangeMsg:
		return a.switchToView(msg.View)

	case MarkdownViewerCloseMsg:
		a.currentView = appViewMail
		// Fresh-on-entry: copy mode resets whenever we re-enter the preserved
		// mail model. This is the confirmed "reset when leaving chat/mail"
		// behavior — equivalent because copy mode only has any effect while the
		// mail view is current (see App.View).
		a.mail.copyMode = false
		// Likewise clear any global select mode left on by the view we came from
		// (mail owns its own copyMode; the two must never both be active).
		a.selectMode = false
		tickCmd, pulseCmd := a.mail.restartPollLoop()
		return a, tea.Batch(a.issueMailRefresh(), tickCmd, pulseCmd, a.sendSize())

	case doctorResultMsg:
		if a.currentView == appViewDoctor {
			a.doctor, _ = a.doctor.Update(msg)
		}
		return a, nil

	case doctorReportSavedMsg:
		if a.currentView == appViewDoctor {
			a.doctor, _ = a.doctor.Update(msg)
		}
		return a, nil

	case updateCheckedMsg:
		if a.currentView == appViewUpdate {
			var cmd tea.Cmd
			a.update, cmd = a.update.Update(msg)
			return a, cmd
		}
		return a, nil

	case updateDoneMsg:
		if a.currentView == appViewUpdate {
			var cmd tea.Cmd
			a.update, cmd = a.update.Update(msg)
			return a, cmd
		}
		return a, nil

	case updateTUICheckedMsg:
		if a.currentView == appViewUpdateTUI {
			var cmd tea.Cmd
			a.updateTUI, cmd = a.updateTUI.Update(msg)
			return a, cmd
		}
		return a, nil

	case updateTUIDoneMsg:
		if a.currentView == appViewUpdateTUI {
			var cmd tea.Cmd
			a.updateTUI, cmd = a.updateTUI.Update(msg)
			return a, cmd
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
		if msg.generation != 0 && msg.generation != a.mail.generation {
			return a, nil
		}
		if msg.err != nil {
			a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", firstLine(msg.err)))
		} else {
			a.mail.AddSystemMessage(i18n.T("mail.refreshed"))
		}
		cmds := []tea.Cmd{a.issueMailRefresh()}
		if a.currentView == appViewKnowledge {
			var kcmd tea.Cmd
			a.knowledge, kcmd = a.knowledge.reloadVisible()
			cmds = append(cmds, kcmd)
		}
		return a, tea.Batch(cmds...)

	case clearDoneMsg:
		if msg.generation != 0 && msg.generation != a.mail.generation {
			return a, nil
		}
		if msg.err != nil {
			a.mail.AddSystemMessage(i18n.TF("mail.clear_failed", firstLine(msg.err)))
		} else if msg.completed {
			a.mail.AddSystemMessage(i18n.T("mail.cleared"))
		} else {
			a.mail.AddSystemMessage(i18n.T("mail.clear_requested"))
		}
		return a, a.issueMailRefresh()

	case refreshAllDoneMsg:
		if msg.generation != 0 && msg.generation != a.mail.generation {
			return a, nil
		}
		if len(msg.failures) > 0 {
			a.mail.AddSystemMessage(i18n.TF("mail.refresh_all_with_failures", msg.count-len(msg.failures), len(msg.failures), strings.Join(msg.failures, ", ")))
		} else {
			a.mail.AddSystemMessage(i18n.TF("mail.refresh_all", msg.count))
		}
		return a, a.issueMailRefresh()

	case PaletteSelectMsg:
		return a.handlePaletteCommand(msg.Command, msg.Args)

	case ProjectsAgentSelectedMsg:
		if a.currentView != appViewProjects || msg.ActivationID != a.projects.activationID || msg.RequestSeq != a.projects.requestSeq {
			return a, nil
		}
		return a.enterVisitedAgent(msg)

	case FirstRunDoneMsg:
		// First-run complete: launch agent and switch to mail.
		// Reload tuiConfig from disk so any settings the wizard saved
		// (theme, mail page size, insights) are reflected downstream.
		// a.tuiConfig was captured at NewApp time and is otherwise stale
		// after the wizard's SaveTUIConfig calls.
		a.tuiConfig = config.LoadTUIConfig(a.globalDir)
		// Persist config.json so main.go's first-run heuristic does
		// not re-trigger the recovery wizard for OAuth / no-key presets
		// (codex etc.) whose wizard skipped the SaveConfig path. For
		// API-key flows this is a no-op rewrite. See issue #181.
		config.EnsureConfigPersisted(a.globalDir)
		// Ensure human folder exists before launching — InitProject is
		// idempotent and prevents the race where the agent tries to
		// send mail before the human mailbox is ready.
		if err := process.InitProject(a.projectDir); err != nil {
			a.currentView = appViewMail
			humanDir := filepath.Join(a.projectDir, "human")
			addr := humanAddr(a.projectDir)
			a.installMailModel(NewMailModel(humanDir, addr, a.projectDir, "", "", a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate))
			a.mail.AddSystemMessage(i18n.TF("mail.launch_failed", err))
			return a, tea.Batch(a.mail.Init(), a.sendSize())
		}
		a.orchDir = msg.OrchDir
		a.orchName = msg.OrchName
		// Propagate LLM config to all agents in the network
		PropagateOrchestratorConfig(a.projectDir, a.orchDir)

		// Recipe application: when the project carries a .recipe/ bundle
		// (set by the first-run wizard or imported from a bundle), make
		// sure every agent's .prompt + skills.paths + .tui-asset/.recipe/
		// snapshot are in sync before the agent process boots. This
		// catches the rehydration case: RehydrateNetwork just generated
		// init.json for each imported agent, but .prompt and library
		// registration haven't run yet for this launch. The startup
		// reconciliation in main.go covers subsequent launches, but the
		// very first launch after rehydration needs this hook too.
		//
		// a.projectDir is the .lingtai/ dir (main.go passes lingtaiDir to
		// NewApp); the recipe resolvers want the parent project root that
		// contains both .recipe/ and .lingtai/, so derive it here.
		projectRoot := filepath.Dir(a.projectDir)
		if preset.RecipeNeedsApply(projectRoot) {
			humanDir := filepath.Join(a.projectDir, "human")
			haddr := "human"
			if humanNode, err := fs.ReadAgent(humanDir); err == nil && humanNode.Address != "" {
				haddr = humanNode.Address
			}
			lang := a.tuiConfig.Language
			if lang == "" {
				lang = "en"
			}
			subst := func(tmpl string) string {
				return SubstituteGreetPlaceholders(tmpl, haddr, humanDir, lang, "120")
			}
			applied, err := preset.ApplyRecipe(projectRoot, lang, subst)
			if err != nil {
				// Recipe materialization failed (.prompt writes,
				// init.json/skills.paths parse/write, snapshot copy, …).
				// Launching now would boot the agent with stale/partial
				// prompt + skill state, which is worse than not launching.
				// Block the launch and surface a persistent, localized mail
				// warning so the failure is visible, not silent.
				fmt.Fprintf(os.Stderr, "warning: recipe re-apply failed before first-run launch (applied %d): %v\n", applied, err)
				a.currentView = appViewMail
				humanDir := filepath.Join(a.projectDir, "human")
				addr := humanAddr(a.projectDir)
				a.installMailModel(NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate))
				a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: i18n.TF("mail.recipe_reapply_failed", err), Type: "mail"})
				return a, tea.Batch(a.mail.Init(), a.sendSize())
			}
			fmt.Fprintf(os.Stderr, "recipe re-applied before first-run launch (%d agent(s))\n", applied)
		}

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
		a.installMailModel(NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate))

		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	case NirvanaDoneMsg:
		// Nirvana complete: .lingtai/ wiped, go to first-run.
		// Re-init project to recreate the human folder so agents can
		// deliver mail once the new orchestrator starts.
		a.nirvanaCleanupPending = false
		a.nirvanaCleanupStarted = false
		process.InitProject(a.projectDir)
		a.orchDir = ""
		a.orchName = ""
		a.currentView = appViewFirstRun
		hasPresets := preset.HasAny()
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, hasPresets)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())

	case AddonSavedMsg:
		a.mail.AddSystemMessage(i18n.T("mcp.saved"))
		return a.switchToView("mail")

	case SetupSavedMsg:
		if a.recoveryMode {
			// Recovery: global config was lost but agents are intact.
			// Propagate the new LLM + capabilities to all agents, init
			// the mail view, and launch the orchestrator.
			a.recoveryMode = false
			a.tuiConfig = config.LoadTUIConfig(a.globalDir)
			// Persist config.json so the recovery wizard does not
			// re-trigger on next launch for OAuth / no-key presets
			// (codex etc.). Without this, recovery would loop forever
			// because config.json was never created. See issue #181.
			config.EnsureConfigPersisted(a.globalDir)
			PropagateOrchestratorConfig(a.projectDir, a.orchDir)
			a.currentView = appViewMail
			humanDir := filepath.Join(a.projectDir, "human")
			addr := humanAddr(a.projectDir)
			a.installMailModel(NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate))
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
		a.installMailModel(NewMailModel(humanDir, addr, a.projectDir, a.orchDir, a.orchName, a.tuiConfig.MailPageSize, a.globalDir, a.tuiConfig.Language, a.tuiConfig.Insights, a.tuiConfig.ToolCallTruncate))

		if launchErr != "" {
			a.mail.messages = append(a.mail.messages, ChatMessage{From: i18n.T("mail.system_sender"), Body: launchErr, Type: "mail"})
		}
		return a, tea.Batch(a.mail.Init(), a.sendSize())

	case autoRefreshTickMsg:
		// Single app-level auto-refresh tick. If disabled, let the loop lapse —
		// mark it unarmed and do not re-arm, so it stays stopped until a
		// settings change re-enables it (via switchToView -> startAutoRefresh).
		// If enabled, ask the current view to reload (no-op when it doesn't opt
		// in or returns nil), then schedule the next tick.
		if !a.tuiConfig.AutoRefreshEnabled() {
			a.autoRefreshArmed = false
			return a, nil
		}
		a.autoRefreshArmed = true
		a, reloadCmd := a.autoRefreshActiveView()
		return a, tea.Batch(reloadCmd, autoRefreshTick())

	// === Global keys ===

	case tea.KeyPressMsg:
		if updated, cmd, handled := a.maybeHandleVisitEsc(msg); handled {
			return updated, cmd
		} else {
			a = updated
		}
		// Global select-text mode (ctrl+y). The mail view keeps owning its own
		// copyMode via mail.go's handler, so only intercept ctrl+y here for every
		// OTHER view; in mail we fall through and let the mail model toggle. esc
		// exits select mode when it is on (non-mail), handled before forwarding so
		// it reliably leaves the mode rather than being consumed by the child view.
		if a.currentView != appViewMail {
			switch msg.String() {
			case copyModeToggleKey:
				a.selectMode = !a.selectMode
				return a, nil
			case "esc":
				if a.selectMode {
					a.selectMode = false
					return a, nil
				}
			}
		}
		switch msg.String() {
		case "ctrl+c":
			return a, tea.Quit
		case "q":
			// Only quit if not in a text input context
			if a.currentView != appViewFirstRun && a.currentView != appViewMail && a.currentView != appViewProps && a.currentView != appViewAddon && a.currentView != appViewNirvana && a.currentView != appViewLibrary && a.currentView != appViewProjects && a.currentView != appViewLogin && a.currentView != appViewKnowledge && a.currentView != appViewMailbox && a.currentView != appViewSystem && a.currentView != appViewPresets && a.currentView != appViewDaemons && a.currentView != appViewNotification && a.currentView != appViewHelp && a.currentView != appViewTaskCard {
				return a, tea.Quit
			}
		}
		if msg.Code == tea.KeyF2 || msg.Code == 'g' && msg.Mod == tea.ModCtrl {
			if a.agentRailToggleEligible() {
				return a.toggleAgentRail()
			}
			return a, nil
		}
		if a.currentView == appViewMail {
			if updated, cmd, handled := a.updateAgentRailKey(msg); handled {
				return updated, cmd
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
	case appViewUpdate:
		updated, cmd := a.update.Update(msg)
		a.update = updated
		return a, cmd
	case appViewUpdateTUI:
		updated, cmd := a.updateTUI.Update(msg)
		a.updateTUI = updated
		return a, cmd
	case appViewNirvana:
		updated, cmd := a.nirvana.Update(msg)
		a.nirvana = updated
		return a, cmd
	case appViewLibrary:
		updated, cmd := a.library.Update(msg)
		a.library = updated
		return a, cmd
	case appViewProjects:
		updated, cmd := a.projects.Update(msg)
		a.projects = updated
		return a, cmd
	case appViewLogin:
		var cmd tea.Cmd
		a.login, cmd = a.login.Update(msg)
		return a, cmd
	case appViewKnowledge:
		updated, cmd := a.knowledge.Update(msg)
		a.knowledge = updated
		return a, cmd
	case appViewMailbox:
		updated, cmd := a.mailbox.Update(msg)
		a.mailbox = updated
		return a, cmd
	case appViewSystem:
		updated, cmd := a.system.Update(msg)
		a.system = updated
		return a, cmd
	case appViewPresets:
		updated, cmd := a.presetLibrary.Update(msg)
		a.presetLibrary = updated
		return a, cmd
	case appViewDaemons:
		updated, cmd := a.daemons.Update(msg)
		a.daemons = updated
		return a, cmd
	case appViewNotification:
		updated, cmd := a.notification.Update(msg)
		a.notification = updated
		return a, cmd
	case appViewTaskCard:
		updated, cmd := a.taskcard.Update(msg)
		a.taskcard = updated
		return a, cmd
	case appViewHelp:
		updated, cmd := a.help.Update(msg)
		a.help = updated
		return a, cmd
	}

	return a, nil
}

func (a App) openSetupCredentials() (App, tea.Cmd) {
	a.currentView = appViewLogin
	a.login = NewSetupCredentialsModel(a.orchDir, a.globalDir)
	return a, tea.Batch(a.login.Init(), a.sendSize())
}

func (a App) handlePaletteCommand(command, args string) (tea.Model, tea.Cmd) {
	if a.currentView == appViewMail && a.mail.agentRail.focused {
		a.mail = a.mail.blurAgentRail()
	}
	addMsg := func(text string) {
		a.mail.AddSystemMessage(text)
	}
	targetDir := a.orchDir
	targetName := a.orchName
	if target, ok := a.mail.currentDirectTarget(); ok {
		targetDir = target.Directory
		targetName = target.Address
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
				if fs.IsAlive(agent.WorkingDir, fs.AgentAliveThresholdSec()) {
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
				if fs.IsAlive(agent.WorkingDir, fs.AgentAliveThresholdSec()) {
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
				if !fs.IsAlive(agent.WorkingDir, fs.AgentAliveThresholdSec()) {
					count++
					if err := reviveAgentDir(a.lingtaiCmd, agent.WorkingDir); err != nil {
						failures = append(failures, fmt.Sprintf("%s (%s)", filepath.Base(agent.WorkingDir), firstLine(err)))
					}
				}
			}
			if len(failures) > 0 {
				addMsg(i18n.TF("mail.cpr_all_with_failures", count-len(failures), len(failures), strings.Join(failures, ", ")))
			} else {
				addMsg(i18n.TF("mail.cpr_all", count))
			}
		} else if targetDir != "" {
			if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
				if err := reviveAgentDir(a.lingtaiCmd, targetDir); err != nil {
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
			generation := a.mail.generation
			return a, func() tea.Msg {
				completed, err := requestClearContext(lingtaiCmd, dir)
				return clearDoneMsg{generation: generation, completed: completed, err: err}
			}
		}
		return a, nil
	case "refresh":
		if args == "all" && a.lingtaiCmd != "" {
			addMsg(i18n.T("mail.refreshing_all"))
			lingtaiCmd := a.lingtaiCmd
			projectDir := a.projectDir
			generation := a.mail.generation
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
				return refreshAllDoneMsg{generation: generation, count: count, failures: failures}
			}
		} else if args != "" && targetDir != "" && a.lingtaiCmd != "" {
			// `/refresh <preset>` — switch to a named preset and
			// relaunch. Resolve the name against the agent's
			// manifest.preset.allowed list before doing any
			// destructive work; surface a clear error message in
			// the status bar if it doesn't match.
			resolved, err := resolvePresetInAllowed(targetDir, args)
			if err != nil {
				addMsg(firstLine(err))
				return a, nil
			}
			addMsg(fmt.Sprintf(i18n.T("mail.refreshing_to_preset"),
				strings.TrimSuffix(filepath.Base(resolved), ".json")))
			lingtaiCmd := a.lingtaiCmd
			dir := targetDir
			generation := a.mail.generation
			return a, func() tea.Msg {
				return refreshDoneMsg{generation: generation, err: hardRefreshDirWithPreset(lingtaiCmd, dir, resolved)}
			}
		} else if targetDir != "" && a.lingtaiCmd != "" {
			addMsg(i18n.T("mail.refreshing"))
			lingtaiCmd := a.lingtaiCmd
			dir := targetDir
			generation := a.mail.generation
			return a, func() tea.Msg {
				return refreshDoneMsg{generation: generation, err: hardRefreshDir(lingtaiCmd, dir)}
			}
		}
		return a, nil
	case "doctor":
		if targetDir != "" {
			a.currentView = appViewDoctor
			a.doctor = NewDoctorModel(targetDir, a.globalDir)
			return a, tea.Batch(a.doctor.Init(), a.sendSize())
		}
		return a, nil
	case "update":
		if targetDir != "" {
			a.currentView = appViewUpdate
			a.update = NewUpdateModel(targetDir, a.globalDir)
			return a, tea.Batch(a.update.Init(), a.sendSize())
		}
		return a, nil
	case "update-tui":
		if a.globalDir != "" {
			a.currentView = appViewUpdateTUI
			a.updateTUI = NewUpdateTUIModel(a.globalDir)
			return a, tea.Batch(a.updateTUI.Init(), a.sendSize())
		}
		return a, nil
	case "viz":
		url, err := a.portalURL()
		switch {
		case err == nil:
			openBrowser(url)
		case errors.Is(err, errPortalNotFound):
			addMsg("lingtai-portal not found on PATH. Run: brew link --overwrite lingtai-tui")
		default:
			// Start failure or readiness timeout: the error carries the log path.
			addMsg(err.Error())
		}
		return a, nil
	case "mcp":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.projectDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "login":
		return a.openSetupCredentials()
	case "setup":
		trimmedArgs := strings.TrimSpace(args)
		if strings.EqualFold(trimmedArgs, "credentials") || strings.EqualFold(trimmedArgs, "login") {
			return a.openSetupCredentials()
		}
		a.currentView = appViewFirstRun
		a.firstRun = NewSetupModeModel(a.projectDir, a.globalDir, a.orchDir, a.orchName)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	case "settings":
		a.currentView = appViewSettings
		tuiCfg := config.LoadTUIConfig(a.globalDir)
		a.settings = NewSettingsModel(a.globalDir, a.projectDir, targetDir, tuiCfg)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "nirvana":
		a.nirvanaCleanupPending = false
		a.nirvanaCleanupStarted = false
		a.currentView = appViewNirvana
		a.nirvana = NewNirvanaModel(a.projectDir)
		return a, tea.Batch(a.nirvana.Init(), a.sendSize())
	case "kanban":
		a.currentView = appViewProps
		a.props = NewPropsModel(a.projectDir, targetDir, a.globalDir)
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "daemons":
		a.currentView = appViewDaemons
		a.daemons = NewDaemonsModel(a.projectDir, targetDir)
		return a, tea.Batch(a.daemons.Init(), a.sendSize())
	case "notification":
		a.currentView = appViewNotification
		a.notification = NewNotificationModel(targetDir)
		return a, tea.Batch(a.notification.Init(), a.sendSize())
	case "taskcard":
		a.currentView = appViewTaskCard
		a.taskcard = NewTaskCardModel(targetDir)
		return a, tea.Batch(a.taskcard.Init(), a.sendSize())
	case "goal":
		if targetDir == "" {
			addMsg(i18n.T("mail.goal_no_agent"))
			return a, nil
		}
		if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
			addMsg(i18n.T("mail.btw_suspended"))
			return a, nil
		}
		eventID, err := writeGoalRequestNotification(targetDir, args, time.Now())
		if err != nil {
			addMsg(i18n.TF("mail.goal_failed", firstLine(err)))
			return a, nil
		}
		addMsg(i18n.TF("mail.goal_sent", eventID))
		return a, nil
	case "skills":
		a.currentView = appViewLibrary
		// Agent-scoped: mirror what the skills capability would inject for
		// this agent. Scans <agent>/.library/ plus every Tier-1 path declared
		// in init.json (manifest.capabilities.skills.paths).
		a.library = NewLibraryModel(a.projectDir, targetDir, a.tuiConfig.Language)
		return a, tea.Batch(a.library.Init(), a.sendSize())
	case "agents":
		// The /agents selector is a Mail-owned overlay over the canonical
		// conversation rows; it works at every terminal width. A delayed
		// palette result cannot displace an editor warning that already owns
		// the Mail surface.
		if a.currentView == appViewMail && !a.mail.showEditorWarn {
			a.mail = a.mail.openAgentSelector()
		}
		return a, nil
	case "projects":
		return a.openProjectsView()
	case "knowledge", "library", "codex":
		a.currentView = appViewKnowledge
		a.knowledge = NewKnowledgeModel(a.projectDir, targetDir)
		return a, tea.Batch(a.knowledge.Init(), a.sendSize())
	case "system":
		a.currentView = appViewSystem
		a.system = NewSystemModel(a.projectDir, targetDir)
		return a, tea.Batch(a.system.Init(), a.sendSize())
	case "mailbox":
		a.currentView = appViewMailbox
		a.mailbox = NewMailboxModel(a.projectDir)
		return a, tea.Batch(a.mailbox.Init(), a.sendSize())
	case "presets":
		a.currentView = appViewPresets
		// Agent-scoped: shows only the presets in this agent's
		// manifest.preset.allowed list — these are exactly the ones
		// `/refresh <name>` can switch to. The currently-active preset
		// is highlighted in the view. Falls back to the full global
		// registry only when no orchestrator agent is current (e.g.
		// before /setup completes), since there's no allow-list to
		// scope by yet.
		if targetDir != "" {
			allowed := readAllowedPresets(targetDir)
			active := readActivePreset(targetDir)
			a.presetLibrary = NewPresetLibraryModelForAgent(
				a.tuiConfig.Language, a.globalDir, allowed, active,
			)
		} else {
			a.presetLibrary = NewPresetLibraryModel(a.tuiConfig.Language, a.globalDir)
		}
		return a, tea.Batch(a.presetLibrary.Init(), a.sendSize())
	case "export":
		if args != "" && args != "recipe" {
			addMsg(i18n.T("export.help"))
			return a, nil
		}
		if targetDir == "" {
			addMsg(i18n.T("export.no_agent"))
			return a, nil
		}
		if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
			addMsg(i18n.T("mail.btw_suspended"))
			return a, nil
		}
		fs.WritePrompt(targetDir, i18n.T("export.recipe_prompt"))
		addMsg(i18n.T("export.recipe_sent"))
		return a, nil
	case "molt":
		if targetDir == "" {
			return a, nil
		}
		if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
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
			if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
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
			if !fs.IsAlive(targetDir, fs.AgentAliveThresholdSec()) {
				addMsg(i18n.T("mail.btw_suspended"))
				return a, nil
			}
			fs.WriteInquiry(targetDir, "human", args)
			addMsg(i18n.TF("mail.btw_sent", args))
		} else if args == "" {
			addMsg(i18n.T("mail.btw_usage"))
		}
		return a, nil
	case "help":
		a.currentView = appViewHelp
		a.help = NewHelpModel()
		return a, tea.Batch(a.help.Init(), a.sendSize())
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

// hardRefreshDir force-restarts the agent in the given directory. It is the
// escape hatch behind `/refresh`: rather than refusing when an interpreter is
// still alive, it escalates through suspend → lock-clear poll → SIGTERM/SIGKILL
// → stale-state cleanup → ForceLaunchAgent. Returns the launch error if the
// final relaunch fails; the kill/cleanup steps are best-effort and swallowed.
//
// Sequence:
//  1. Touch `.suspend` so a cooperative agent exits cleanly.
//  2. Wait for `.agent.lock` to free (up to 60s, then forced).
//  3. If `ps` still shows `lingtai run <dir>`, SIGTERM (then SIGKILL) those
//     PIDs — this is what makes /refresh actually forceful rather than a
//     polite request.
//  4. Sweep stale handshake files (.agent.lock, .refresh, .refresh.taken,
//     .suspend) so the fresh interpreter doesn't immediately re-suspend or
//     stall on a leftover lock.
//  5. Reset manifest.preset.active to manifest.preset.default — documented
//     escape hatch when the active preset is misbehaving (rate-limited,
//     broken adapter, etc.).
//  6. ForceLaunchAgent (bypassing the duplicate-protection gate; we've
//     already verified the agent dir is clear above).
func hardRefreshDir(lingtaiCmd, dir string) error {
	suspendFile := filepath.Join(dir, ".suspend")
	os.WriteFile(suspendFile, []byte(""), 0o644)
	waitForLockClear(dir)
	// Escalation: if the agent ignored .suspend (deadlocked, slow shutdown,
	// detached child), kill the lingering interpreter so LaunchAgent's
	// duplicate-protection gate doesn't refuse the relaunch.
	if process.IsAgentRunning(dir) {
		_ = process.TerminateAgentProcesses(dir)
	}
	// Clear lingering handshake files. .agent.lock is removed only after taking
	// ownership of the current lock inode; the others are one-shot markers.
	removeAgentLockIfOwned(filepath.Join(dir, ".agent.lock"))
	os.Remove(filepath.Join(dir, ".refresh"))
	os.Remove(filepath.Join(dir, ".refresh.taken"))
	os.Remove(suspendFile)
	resetActivePresetToDefault(dir)
	cmd, err := process.ForceLaunchAgent(lingtaiCmd, dir)
	// Defensive: ForceLaunchAgent → launchAgentUnsafe calls fs.CleanSignals
	// internally, but a fresh .suspend written by another path between our
	// remove() above and the relaunch would put the new process to sleep.
	// Removing again here is cheap and idempotent.
	os.Remove(suspendFile)
	if err != nil {
		return err
	}
	return waitForLaunchHeartbeat(cmd, dir, 10*time.Second)
}

// waitForLockClear polls for .agent.lock to free. If the deadline expires, it
// only removes a lock file whose current inode can be locked by this process.
// Used by hardRefreshDir between suspend and relaunch so we don't stomp a
// still-running agent's init.json.
func waitForLockClear(dir string) {
	lockFile := filepath.Join(dir, ".agent.lock")
	for i := 0; i < lockWaitAttempts; i++ {
		if tryLock(lockFile) {
			return
		}
		time.Sleep(lockWaitInterval)
	}
	removeAgentLockIfOwned(lockFile)
}

var (
	lockWaitAttempts = 120
	lockWaitInterval = 500 * time.Millisecond
)

// resetActivePresetToDefault rewrites manifest.preset.active to match
// manifest.preset.default in the agent's init.json. Best-effort: any error
// (missing file, malformed JSON, missing preset block) is silently ignored
// so a /refresh still relaunches even if the preset block is in a weird
// state. Both `default` and `active` are guaranteed by validate_init to be
// in `allowed`, so writing active = default is always authorized.
func resetActivePresetToDefault(dir string) {
	initPath := filepath.Join(dir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	manifest, ok := raw["manifest"].(map[string]interface{})
	if !ok {
		return
	}
	pre, ok := manifest["preset"].(map[string]interface{})
	if !ok {
		return
	}
	def, ok := pre["default"].(string)
	if !ok || def == "" {
		return
	}
	if cur, ok := pre["active"].(string); ok && cur == def {
		return // already on default, nothing to write
	}
	pre["active"] = def
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return
	}
	fs.WriteFileAtomic(initPath, out, 0o644)
}

// readAllowedPresets returns the contents of manifest.preset.allowed from
// the agent's init.json — the per-agent allow-list that the kernel
// enforces on runtime preset swaps. Returns nil on any failure (missing
// file, malformed JSON, missing/empty block); callers should treat nil
// as "no allow-list available" and fall back to the global preset
// library rather than fail.
func readAllowedPresets(dir string) []string {
	initPath := filepath.Join(dir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return nil
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	manifest, ok := raw["manifest"].(map[string]interface{})
	if !ok {
		return nil
	}
	pre, ok := manifest["preset"].(map[string]interface{})
	if !ok {
		return nil
	}
	allowed, ok := pre["allowed"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(allowed))
	for _, v := range allowed {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// readActivePreset returns manifest.preset.active from the agent's
// init.json — the preset currently in force. Returns "" on any failure
// or when the field is missing. Used by /presets to highlight the
// active entry in the agent-scoped view.
func readActivePreset(dir string) string {
	initPath := filepath.Join(dir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return ""
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ""
	}
	manifest, ok := raw["manifest"].(map[string]interface{})
	if !ok {
		return ""
	}
	pre, ok := manifest["preset"].(map[string]interface{})
	if !ok {
		return ""
	}
	active, _ := pre["active"].(string)
	return active
}

// resolvePresetInAllowed matches a user-provided query (`/refresh <query>`)
// against the agent's manifest.preset.allowed list. The query may be:
//   - a bare preset name / basename stem ("mimo", "glm-5.1-pro")
//   - a full home-shortened ref ("~/.lingtai-tui/presets/templates/mimo.json")
//   - any path string that exactly matches an entry in allowed (less
//     common, but supports recipe-style paths).
//
// Returns the canonical allowed[] entry on a unique match. Returns an
// error string if no match, multiple matches, or the agent has no
// allowed list. The returned path is what should be written to
// manifest.preset.active; the kernel's _refresh allowed-gate will
// validate it again with `_preset_ref_in` so home-shortened and
// absolute forms compare equal.
func resolvePresetInAllowed(dir, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("preset name is empty")
	}
	allowed := readAllowedPresets(dir)
	if len(allowed) == 0 {
		return "", fmt.Errorf("agent has no manifest.preset.allowed list — cannot switch")
	}
	// Exact-path match first.
	for _, ref := range allowed {
		if ref == query {
			return ref, nil
		}
	}
	// Basename-stem match (drop directory prefix and .json suffix).
	var matches []string
	for _, ref := range allowed {
		stem := strings.TrimSuffix(filepath.Base(ref), ".json")
		if stem == query {
			matches = append(matches, ref)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		// Two presets in the allow-list with the same basename (e.g.
		// a template "mimo.json" and a saved "mimo.json"). Disambiguate.
		return "", fmt.Errorf("preset %q is ambiguous (matches %d entries) — pass the full path",
			query, len(matches))
	}
	// No match. Build a helpful error listing what's actually allowed
	// (basenames only — full paths are noisy in the status bar).
	stems := make([]string, 0, len(allowed))
	for _, ref := range allowed {
		stems = append(stems, strings.TrimSuffix(filepath.Base(ref), ".json"))
	}
	return "", fmt.Errorf("preset %q is not in this agent's allowed list (available: %s)",
		query, strings.Join(stems, ", "))
}

// setActivePreset rewrites manifest.preset.active to the given path.
// Caller is responsible for ensuring the path is in manifest.preset.allowed
// (use resolvePresetInAllowed) — this function is the dumb writer.
// Returns the error from json or filesystem failures; the kernel will
// reject a non-allowed path on relaunch with its own validation error.
func setActivePreset(dir, presetPath string) error {
	initPath := filepath.Join(dir, "init.json")
	data, err := os.ReadFile(initPath)
	if err != nil {
		return err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	manifest, ok := raw["manifest"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("init.json missing 'manifest' object")
	}
	pre, ok := manifest["preset"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("init.json missing 'manifest.preset' object")
	}
	pre["active"] = presetPath
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteFileAtomic(initPath, out, 0o644)
}

type childWindowSizeMsg struct {
	tea.WindowSizeMsg
}

func (a App) agentRailToggleEligible() bool {
	return a.currentView == appViewMail &&
		a.width >= agentRailMinTerminalWidth &&
		!a.mail.copyMode &&
		!a.mail.directVisibilityObscured()
}

func (a App) collapsedAgentRailControlVisible() bool {
	return a.currentView == appViewMail &&
		a.width >= agentRailMinTerminalWidth &&
		a.mail.agentRail.explicitlyCollapsed &&
		a.mail.ordinaryHeaderVisible()
}

func (a App) toggleAgentRail() (App, tea.Cmd) {
	a.mail.agentRail.explicitlyCollapsed = !a.mail.agentRail.explicitlyCollapsed
	if a.mail.agentRail.explicitlyCollapsed && a.mail.agentRail.focused {
		a.mail = a.mail.blurAgentRail()
	}
	a, cmd := a.updateChildWindowSize(a.layoutBudget().ChildWindowSize())
	if a.mail.directChat.mainComposeStored {
		a.mail.directChat.mainInput.SetWidth(a.mail.width)
	}
	return a, cmd
}

func (a App) updateAgentRailKey(msg tea.KeyPressMsg) (App, tea.Cmd, bool) {
	if !a.layoutBudget().RailVisible {
		return a, nil, false
	}
	if !a.mail.agentRail.focused {
		if msg.Code != tea.KeyTab ||
			!a.mail.input.Focused() ||
			a.mail.showEditorWarn ||
			a.mail.agentSelector.selectorOpen ||
			a.mail.input.IsPaletteActive() {
			return a, nil, false
		}
		a.mail = a.mail.focusAgentRail()
		return a, nil, true
	}

	// Mail owns copy mode. Its toggle and first Esc keep their established
	// precedence even while the rail has presentation focus.
	if msg.String() == copyModeToggleKey || a.mail.copyMode && msg.String() == "esc" {
		return a, nil, false
	}
	if msg.String() == "ctrl+r" {
		var cmd tea.Cmd
		a.mail, cmd = a.mail.Update(msg)
		return a, cmd, true
	}

	switch msg.String() {
	case "tab", "esc":
		a.mail = a.mail.blurAgentRail()
	case "up", "k":
		a.mail = a.mail.moveSelectorCursor(-1).keepAgentRailCursorVisible()
	case "down", "j":
		a.mail = a.mail.moveSelectorCursor(1).keepAgentRailCursorVisible()
	case "home":
		a.mail = a.mail.setSelectorCursor(0).keepAgentRailCursorVisible()
	case "end":
		a.mail = a.mail.setSelectorCursor(len(a.mail.agentSelector.rows) - 1).keepAgentRailCursorVisible()
	case "enter":
		var cmd tea.Cmd
		a.mail, cmd = a.mail.activateConversationRow(a.mail.agentSelector.cursor)
		a.mail = a.mail.keepAgentRailCursorVisible()
		return a, cmd, true
	default:
		if msg.Code == ' ' {
			var cmd tea.Cmd
			a.mail, cmd = a.mail.activateConversationRow(a.mail.agentSelector.cursor)
			a.mail = a.mail.keepAgentRailCursorVisible()
			return a, cmd, true
		}
	}
	return a, nil, true
}

func (a App) mailMouseChildCoordinates(x, y int) (LayoutBudget, int, int, bool) {
	budget := a.layoutBudget()
	if x < 0 || x >= budget.TerminalWidth ||
		y < budget.TopChromeRows ||
		y >= budget.TopChromeRows+budget.ChildHeight {
		return budget, 0, 0, false
	}
	return budget, x - budget.RailWidth, y - budget.TopChromeRows, true
}

func (a App) updateMailMouseClick(msg tea.MouseClickMsg) (App, tea.Cmd) {
	budget, childX, childY, inside := a.mailMouseChildCoordinates(msg.X, msg.Y)
	if !inside {
		return a, nil
	}
	if budget.RailVisible && msg.X < budget.RailWidth {
		if a.mail.copyMode || a.mail.directVisibilityObscured() || msg.Button != tea.MouseLeft {
			return a, nil
		}
		if childY == 0 &&
			msg.X >= agentRailCollapseControlStart &&
			msg.X < agentRailCollapseControlStart+agentRailControlWidth {
			return a.toggleAgentRail()
		}
		row := a.mail.agentRailRowAt(childY)
		if row < 0 {
			return a, nil
		}
		a.mail = a.mail.setSelectorCursor(row).keepAgentRailCursorVisible()
		var cmd tea.Cmd
		a.mail, cmd = a.mail.activateConversationRow(row)
		return a, cmd
	}

	if a.collapsedAgentRailControlVisible() &&
		childY == 0 &&
		childX < agentRailControlWidth {
		if msg.Button != tea.MouseLeft || !a.agentRailToggleEligible() {
			return a, nil
		}
		return a.toggleAgentRail()
	}

	if msg.Button == tea.MouseLeft && a.mail.agentRail.focused {
		a.mail = a.mail.blurAgentRail()
	}
	msg.X = childX
	msg.Y = childY
	var cmd tea.Cmd
	a.mail, cmd = a.mail.Update(msg)
	return a, cmd
}

func (a App) updateMailMouseWheel(msg tea.MouseWheelMsg) (App, tea.Cmd) {
	budget, childX, childY, inside := a.mailMouseChildCoordinates(msg.X, msg.Y)
	if !inside {
		return a, nil
	}
	if budget.RailVisible && msg.X < budget.RailWidth {
		if a.mail.copyMode || a.mail.directVisibilityObscured() {
			return a, nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			a.mail = a.mail.scrollAgentRail(-1)
		case tea.MouseWheelDown:
			a.mail = a.mail.scrollAgentRail(1)
		}
		return a, nil
	}

	msg.X = childX
	msg.Y = childY
	var cmd tea.Cmd
	a.mail, cmd = a.mail.Update(msg)
	return a, cmd
}

func (a App) updateChildWindowSize(msg tea.WindowSizeMsg) (App, tea.Cmd) {
	var cmd tea.Cmd
	switch a.currentView {
	case appViewMail:
		a.mail, cmd = a.mail.Update(msg)
	case appViewSettings:
		a.settings, cmd = a.settings.Update(msg)
	case appViewProps:
		a.props, cmd = a.props.Update(msg)
	case appViewAddon:
		a.addon, cmd = a.addon.Update(msg)
	case appViewDoctor:
		a.doctor, cmd = a.doctor.Update(msg)
	case appViewUpdate:
		a.update, cmd = a.update.Update(msg)
	case appViewUpdateTUI:
		a.updateTUI, cmd = a.updateTUI.Update(msg)
	case appViewNirvana:
		a.nirvana, cmd = a.nirvana.Update(msg)
	case appViewLibrary:
		a.library, cmd = a.library.Update(msg)
	case appViewProjects:
		a.projects, cmd = a.projects.Update(msg)
	case appViewFirstRun:
		a.firstRun, cmd = a.firstRun.Update(msg)
	case appViewLogin:
		a.login, cmd = a.login.Update(msg)
	case appViewKnowledge:
		a.knowledge, cmd = a.knowledge.Update(msg)
	case appViewMailbox:
		a.mailbox, cmd = a.mailbox.Update(msg)
	case appViewSystem:
		a.system, cmd = a.system.Update(msg)
	case appViewPresets:
		a.presetLibrary, cmd = a.presetLibrary.Update(msg)
	case appViewDaemons:
		a.daemons, cmd = a.daemons.Update(msg)
	case appViewNotification:
		a.notification, cmd = a.notification.Update(msg)
	case appViewHelp:
		a.help, cmd = a.help.Update(msg)
	case appViewTaskCard:
		a.taskcard, cmd = a.taskcard.Update(msg)
	}
	return a, cmd
}

// hardRefreshDirWithPreset is the `/refresh <preset>` cousin of
// hardRefreshDir. Sequence is identical (suspend → lock-clear → kill →
// signal sweep → relaunch) except that step 5 writes
// manifest.preset.active = presetPath instead of resetting to default.
// The caller is expected to have already validated presetPath via
// resolvePresetInAllowed.
func hardRefreshDirWithPreset(lingtaiCmd, dir, presetPath string) error {
	suspendFile := filepath.Join(dir, ".suspend")
	os.WriteFile(suspendFile, []byte(""), 0o644)
	waitForLockClear(dir)
	if process.IsAgentRunning(dir) {
		_ = process.TerminateAgentProcesses(dir)
	}
	removeAgentLockIfOwned(filepath.Join(dir, ".agent.lock"))
	os.Remove(filepath.Join(dir, ".refresh"))
	os.Remove(filepath.Join(dir, ".refresh.taken"))
	os.Remove(suspendFile)
	if err := setActivePreset(dir, presetPath); err != nil {
		// Don't refuse the relaunch — the user asked to refresh.
		// Falling back to whatever active currently is.
	}
	cmd, err := process.ForceLaunchAgent(lingtaiCmd, dir)
	os.Remove(suspendFile)
	if err != nil {
		return err
	}
	return waitForLaunchHeartbeat(cmd, dir, 10*time.Second)
}

// reviveDir waits for .agent.lock to free, then relaunches the agent. Timeout
// cleanup only removes a lock inode this process can first lock itself. Used
// by /cpr (dead agent, no prior suspend) and as the tail of hardRefreshDir
// (after writing .suspend).
func reviveDir(lingtaiCmd, dir string) error {
	lockFile := filepath.Join(dir, ".agent.lock")
	locked := true
	for i := 0; i < lockWaitAttempts; i++ {
		if tryLock(lockFile) {
			locked = false
			break
		}
		time.Sleep(lockWaitInterval)
	}
	if locked {
		removeAgentLockIfOwned(lockFile)
	}
	cmd, err := process.LaunchAgent(lingtaiCmd, dir)
	if err != nil {
		return err
	}
	return waitForLaunchHeartbeat(cmd, dir, 10*time.Second)
}

var reviveAgentDir = reviveDir

// Launch heartbeat watchdog tuning. Overridable in tests to keep the poll fast.
var (
	launchHeartbeatPoll      = 200 * time.Millisecond
	launchHeartbeatCap       = 120 * time.Second
	launchHeartbeatIsAlive   = fs.IsAlive
	launchHeartbeatIsRunning = process.IsAgentRunning
)

// waitForLaunchHeartbeat polls for a fresh heartbeat from a freshly launched
// agent. If the launched process exits before the first heartbeat, that is a
// hard failure. Slow startup (e.g. several stdio MCP addons initialized
// serially) can legitimately exceed timeout before the first heartbeat: as
// long as a process is still alive in the agent dir we keep waiting past the
// initial deadline, up to launchHeartbeatCap, so a recovering launch is not
// misreported as failed and the user is not pushed into a duplicate relaunch
// (see #671).
func waitForLaunchHeartbeat(cmd *exec.Cmd, dir string, timeout time.Duration) error {
	started := time.Now()
	deadline := started.Add(timeout)
	for {
		if launchHeartbeatIsAlive(dir, fs.AgentAliveThresholdSec()) {
			return nil
		}
		if cmd != nil && !launchHeartbeatIsRunning(dir) {
			return fmt.Errorf("agent launch exited before writing a fresh heartbeat; see %s", filepath.Join(dir, "logs", "agent.log"))
		}
		if now := time.Now(); now.After(deadline) && now.Sub(started) >= launchHeartbeatCap {
			// The heartbeat was re-checked at the top of this iteration, so
			// this is a true absence-of-heartbeat verdict after the cap.
			return fmt.Errorf("agent launch did not write a fresh heartbeat within %s; see %s", launchHeartbeatCap, filepath.Join(dir, "logs", "agent.log"))
		}
		time.Sleep(launchHeartbeatPoll)
	}
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

// sendSize returns a tea.Cmd that sends the current *child* window size to a
// newly created view so it doesn't render with zero width/height. The size is
// the content rectangle produced by LayoutBudget (see layout.go) — the same
// geometry the incoming-WindowSizeMsg handler forwards, so a freshly-routed
// view and a resized view agree on viewport/composer/header/footer dimensions.
func (a App) sendSize() tea.Cmd {
	cs := a.layoutBudget().ChildWindowSize()
	return func() tea.Msg { return childWindowSizeMsg{WindowSizeMsg: cs} }
}

func (a App) enterVisitedAgent(msg ProjectsAgentSelectedMsg) (App, tea.Cmd) {
	r := msg.Record
	if !r.Enterable {
		a.mail.AddSystemMessage(enterabilityText(r))
		return a, nil
	}
	if !a.visiting {
		a.visiting = true
		a.visitOriginalProjectDir = a.projectDir
		a.visitOriginalOrchDir = a.orchDir
		a.visitOriginalOrchName = a.orchName
		a.visitOriginalMail = a.mail
		a.visitOriginalProjects = a.projects
		a.visitOriginalView = a.currentView
	}
	a.projectDir = filepath.Join(r.Project, ".lingtai")
	a.orchDir = r.AgentDir
	a.orchName = firstNonEmpty(r.AgentName, r.Agent)
	a.visitTargetProjectDir = a.projectDir
	a.visitTargetAgentDir = a.orchDir
	a.visitTargetAgentName = a.orchName
	a.currentView = appViewMail
	a.selectMode = false
	a.doubleEscArmed = false
	a.installMailModel(a.newMailForCurrentContext())
	a.mail.visitExitHint = true
	return a, tea.Batch(a.mail.Init(), a.sendSize())
}

func (a App) returnFromVisit() (App, tea.Cmd) {
	if !a.visiting {
		return a, nil
	}
	restored := a.visitOriginalMail
	restored.copyMode = false
	restored.visitExitHint = false
	a.projectDir = a.visitOriginalProjectDir
	a.orchDir = a.visitOriginalOrchDir
	a.orchName = a.visitOriginalOrchName
	a.currentView = a.visitOriginalView
	a.selectMode = false
	a.visiting = false
	a.visitOriginalProjectDir = ""
	a.visitOriginalOrchDir = ""
	a.visitOriginalOrchName = ""
	a.visitOriginalView = appViewMail
	a.visitTargetProjectDir = ""
	a.visitTargetAgentDir = ""
	a.visitTargetAgentName = ""
	a.doubleEscArmed = false
	resumeCmd := a.resumeMailModel(restored)
	if a.currentView == appViewProjects {
		a.projects = a.visitOriginalProjects
		a.visitOriginalProjects = ProjectsModel{}
		return a, resumeCmd
	}
	a.visitOriginalProjects = ProjectsModel{}
	a.currentView = appViewMail
	return a, resumeCmd
}

func (a *App) resumeMailModel(restored MailModel) tea.Cmd {
	a.installMailModel(restored)
	a.mail.homeTelemetryInFlight = false
	a.mail.homeAsyncStatsInFlight = false
	var refreshCmd tea.Cmd
	if a.mail.initialLoading {
		refreshCmd = a.issueMailInitialRebuild()
	} else {
		refreshCmd = a.issueMailRefresh()
	}
	tickCmd, pulseCmd := a.mail.pollLoopCmds()
	return tea.Batch(refreshCmd, tickCmd, pulseCmd, a.sendSize())
}

func (a App) maybeHandleVisitEsc(msg tea.KeyPressMsg) (App, tea.Cmd, bool) {
	if msg.String() != "esc" {
		a.doubleEscArmed = false
		return a, nil, false
	}
	if !a.visitEscEligible() {
		a.doubleEscArmed = false
		return a, nil, false
	}
	now := appNow()
	if a.doubleEscArmed && now.Sub(a.doubleEscFirstAt) <= doubleEscReturnWindow {
		updated, cmd := a.returnFromVisit()
		return updated, cmd, true
	}
	a.doubleEscArmed = true
	a.doubleEscFirstAt = now
	return a, nil, true
}

func (a App) visitEscEligible() bool {
	if !a.visiting || a.selectMode || a.currentView != appViewMail {
		return false
	}
	return !a.mail.copyMode &&
		!a.mail.agentRail.focused &&
		!a.mail.showEditorWarn &&
		!a.mail.input.IsPaletteActive()
}

type refreshDoneMsg struct {
	generation uint64
	err        error
}
type refreshAllDoneMsg struct {
	generation uint64
	count      int
	failures   []string
}

func (a App) switchToView(viewName string) (tea.Model, tea.Cmd) {
	// Global select mode is scoped to the current view; clear it on any
	// navigation so it never leaks into the destination (and so entering mail
	// hands ctrl+y back to the mail model's own copyMode).
	a.selectMode = false
	if viewName != "mail" && a.mail.agentRail.focused {
		a.mail = a.mail.blurAgentRail()
	}
	switch viewName {
	case "mail":
		a.currentView = appViewMail
		// Fresh-on-entry: copy mode resets on every re-entry to the preserved
		// mail model (the confirmed "reset when leaving chat/mail" behavior).
		// This path is more robust than reset-on-leave because the slash-command
		// handler leaves mail by setting currentView directly, bypassing this.
		a.mail.copyMode = false
		// Reload config in case settings changed it
		a.tuiConfig = config.LoadTUIConfig(a.globalDir)
		ps := config.NormalizeMailPageSize(a.tuiConfig.MailPageSize)
		pageSizeChanged := ps != a.mail.pageSize
		a.mail.pageSize = ps
		a.mail.insightsEnabled = a.tuiConfig.Insights
		a.mail.toolCallTruncate = a.tuiConfig.ToolCallTruncate
		// Re-validate the Home telemetry expression from the reloaded config, the
		// same way the other presentation preferences above are re-applied to the
		// preserved model rather than reconstructing it. A changed expression can
		// change whether the telemetry row occupies a footer line, and the
		// preserved model's reserved height has to follow it now rather than
		// whenever some later event happens to resync — an unreserved row clips
		// the status bar, a reserved-but-unrendered one leaves a blank line.
		telemetryRowWas := a.mail.hasHomeTelemetry()
		a.mail.homeTelemetryDisplay = homeTelemetryDisplayFromConfig(a.tuiConfig.HomeTelemetryDisplay)
		if a.mail.hasHomeTelemetry() != telemetryRowWas {
			a.mail.syncViewportHeight()
		}
		// Re-apply theme to textarea (settings may have changed it)
		a.mail.input.ApplyTheme()
		mailCmd := a.issueMailRefresh()
		var tickCmd, pulseCmd tea.Cmd
		if pageSizeChanged {
			// The page size owns both visible batching and the bounded content
			// snapshot. A preserved cache built with the previous setting cannot be
			// relabelled in place: start a fresh generation and rebuild exactly one
			// new page so old count/older-page completions are rejected.
			a.mail.initialLoading = true
			a.installMailModel(a.mail)
			mailCmd = a.mail.initialRebuild
			tickCmd, pulseCmd = a.mail.pollLoopCmds()
		} else {
			tickCmd, pulseCmd = a.mail.restartPollLoop()
		}
		// Restart Mail refresh + tick/pulse. The new poll epoch invalidates any
		// same-generation chain that was still pending outside Mail.
		// Also (re)start the app-level auto-refresh ticker: this is the path
		// taken when leaving /settings, where auto refresh may have just been
		// toggled back on. startAutoRefresh is a no-op if it is already armed.
		a, arCmd := a.startAutoRefresh()
		return a, tea.Batch(mailCmd, tickCmd, pulseCmd, a.sendSize(), arCmd)
	case "setup":
		a.currentView = appViewFirstRun
		a.firstRun = NewSetupModeModel(a.projectDir, a.globalDir, a.orchDir, a.orchName)
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	case "login":
		return a.openSetupCredentials()
	case "settings":
		a.currentView = appViewSettings
		tuiCfg := config.LoadTUIConfig(a.globalDir)
		a.settings = NewSettingsModel(a.globalDir, a.projectDir, a.orchDir, tuiCfg)
		return a, tea.Batch(a.settings.Init(), a.sendSize())
	case "props", "kanban":
		a.currentView = appViewProps
		a.props = NewPropsModel(a.projectDir, a.orchDir, a.globalDir)
		return a, tea.Batch(a.props.Init(), a.sendSize())
	case "daemons":
		a.currentView = appViewDaemons
		a.daemons = NewDaemonsModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.daemons.Init(), a.sendSize())
	case "notification":
		a.currentView = appViewNotification
		a.notification = NewNotificationModel(a.orchDir)
		return a, tea.Batch(a.notification.Init(), a.sendSize())
	case "taskcard":
		a.currentView = appViewTaskCard
		a.taskcard = NewTaskCardModel(a.orchDir)
		return a, tea.Batch(a.taskcard.Init(), a.sendSize())
	case "skills":
		a.currentView = appViewLibrary
		// Agent-scoped: mirror what the skills capability would inject for
		// this agent. Scans <agent>/.library/ plus every Tier-1 path declared
		// in init.json (manifest.capabilities.skills.paths).
		a.library = NewLibraryModel(a.projectDir, a.orchDir, a.tuiConfig.Language)
		return a, tea.Batch(a.library.Init(), a.sendSize())
	case "knowledge", "library", "codex":
		a.currentView = appViewKnowledge
		a.knowledge = NewKnowledgeModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.knowledge.Init(), a.sendSize())
	case "system":
		a.currentView = appViewSystem
		a.system = NewSystemModel(a.projectDir, a.orchDir)
		return a, tea.Batch(a.system.Init(), a.sendSize())
	case "presets":
		a.currentView = appViewPresets
		// Agent-scoped: same view as `/presets`. Shows only the
		// presets in this agent's manifest.preset.allowed list, with
		// the currently-active one highlighted. Falls back to the
		// global registry when no orchestrator is current.
		if a.orchDir != "" {
			allowed := readAllowedPresets(a.orchDir)
			active := readActivePreset(a.orchDir)
			a.presetLibrary = NewPresetLibraryModelForAgent(
				a.tuiConfig.Language, a.globalDir, allowed, active,
			)
		} else {
			a.presetLibrary = NewPresetLibraryModel(a.tuiConfig.Language, a.globalDir)
		}
		return a, tea.Batch(a.presetLibrary.Init(), a.sendSize())
	case "projects":
		return a.openProjectsView()
	case "mcp":
		if a.orchDir != "" {
			a.currentView = appViewAddon
			a.addon = NewAddonModel(a.projectDir)
			return a, tea.Batch(a.addon.Init(), a.sendSize())
		}
		return a, nil
	case "welcome":
		a.currentView = appViewFirstRun
		a.firstRun = NewFirstRunModel(a.projectDir, a.globalDir, true)
		a.firstRun.welcomeOnly = true
		return a, tea.Batch(a.firstRun.Init(), a.sendSize())
	case "help":
		a.currentView = appViewHelp
		a.help = NewHelpModel()
		return a, tea.Batch(a.help.Init(), a.sendSize())
	}
	return a, nil
}

func (a App) View() tea.View {
	var content string
	switch a.currentView {
	case appViewFirstRun:
		content = a.firstRun.View()
	case appViewMail:
		content = a.mail.view(a.collapsedAgentRailControlVisible())
		budget := a.layoutBudget()
		if budget.RailVisible {
			content = lipgloss.JoinHorizontal(
				lipgloss.Top,
				a.mail.renderAgentRail(budget.RailWidth, budget.ChildHeight),
				content,
			)
		}
	case appViewSettings:
		content = a.settings.View()
	case appViewProps:
		content = a.props.View()
	case appViewAddon:
		content = a.addon.View()
	case appViewDoctor:
		content = a.doctor.View()
	case appViewUpdate:
		content = a.update.View()
	case appViewUpdateTUI:
		content = a.updateTUI.View()
	case appViewNirvana:
		content = a.nirvana.View()
	case appViewLibrary:
		content = a.library.View()
	case appViewProjects:
		content = a.projects.View()
	case appViewLogin:
		content = a.login.View()
	case appViewKnowledge:
		content = a.knowledge.View()
	case appViewMailbox:
		content = a.mailbox.View()
	case appViewSystem:
		content = a.system.View()
	case appViewPresets:
		content = a.presetLibrary.View()
	case appViewDaemons:
		content = a.daemons.View()
	case appViewNotification:
		content = a.notification.View()
	case appViewHelp:
		content = a.help.View()
	case appViewTaskCard:
		content = a.taskcard.View()
	}
	// Compose root-owned chrome (top banner today) around the child content.
	// The child was already sized to the reduced budget, so chrome occupies
	// the rows the child yielded rather than being appended past full height.
	content = a.composeWithChrome(content)
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	// Copy/select mode: drop mouse capture so the terminal can drag-select
	// visible text. The mail view drives this via its own copyMode; every other
	// view uses the global selectMode (ctrl+y), whose indicator is rendered as
	// top chrome by composeWithChrome above. Bubble Tea diffs MouseMode per frame
	// and emits the enable/disable escape sequences on transition.
	if (a.currentView == appViewMail && a.mail.copyMode) || (a.currentView != appViewMail && a.selectMode) {
		v.MouseMode = tea.MouseModeNone
	}
	ApplyThemeToView(&v)
	v.ReportFocus = true
	if a.currentView == appViewMail {
		if cursor := a.mail.Cursor(); cursor != nil {
			projected := *cursor
			budget := a.layoutBudget()
			projected.X += budget.RailWidth
			projected.Y += budget.TopChromeRows
			v.Cursor = &projected
		}
	}
	return v
}

// Portal startup tuning. Overridable in tests to keep the readiness poll fast.
var (
	portalReadyTimeout = 3 * time.Second
	portalReadyPoll    = 200 * time.Millisecond
)

// errPortalNotFound signals lingtai-portal is not on PATH; portalStartError
// wraps an exec.Start failure; portalTimeoutError signals the portal started
// but never became ready. The caller distinguishes these to show an accurate
// message.
var errPortalNotFound = errors.New("lingtai-portal not found on PATH")

type portalStartError struct {
	err     error
	logPath string
}

func (e *portalStartError) Error() string {
	return "failed to start lingtai-portal: " + e.err.Error() + "; see " + e.logPath
}
func (e *portalStartError) Unwrap() error { return e.err }

type portalTimeoutError struct{ logPath string }

func (e *portalTimeoutError) Error() string {
	return "lingtai-portal did not become ready in time; see " + e.logPath
}

// portalURL kills any existing portal and spawns a fresh one, returning its URL
// once the portal writes .portal/port. Ownership of the child is retained until
// readiness succeeds: on timeout or failure the child is killed and reaped so a
// slow portal is never left detached (issue #489). Only after a URL is ready is
// the process released, so a healthy portal survives TUI exit.
func (a *App) portalURL() (string, error) {
	portalRoot := filepath.Join(a.projectDir, ".portal")
	portFile := filepath.Join(portalRoot, "port")
	logPath := filepath.Join(portalRoot, "portal.log")

	// Kill existing portal so we always get a fresh instance with the latest binary
	exec.Command("pkill", "-f", "lingtai-portal.*--dir.*"+filepath.Dir(a.projectDir)).Run()
	os.Remove(portFile)
	time.Sleep(300 * time.Millisecond)

	// Spawn fresh portal
	portalCmd, _ := exec.LookPath("lingtai-portal")
	if portalCmd == "" {
		return "", errPortalNotFound
	}

	// Route portal output to a local log so startup failures are inspectable.
	os.MkdirAll(portalRoot, 0o755)
	logFile, logErr := os.Create(logPath)

	cmd := exec.Command(portalCmd, "--dir", filepath.Dir(a.projectDir))
	if logErr == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return "", &portalStartError{err: err, logPath: logPath}
	}
	// Our copy of the log fd is no longer needed; the child holds its own.
	if logFile != nil {
		logFile.Close()
	}

	// Wait for the port file to appear, holding the process handle so we can
	// reap it on failure instead of leaking a detached portal.
	deadline := time.Now().Add(portalReadyTimeout)
	for time.Now().Before(deadline) {
		time.Sleep(portalReadyPoll)
		if data, err := os.ReadFile(portFile); err == nil {
			// Ready: release so the portal survives TUI exit.
			cmd.Process.Release()
			return "http://localhost:" + strings.TrimSpace(string(data)), nil
		}
	}

	// Timed out: kill and reap the child we started.
	cmd.Process.Kill()
	cmd.Wait()
	return "", &portalTimeoutError{logPath: logPath}
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

// ValidateCodexAuthOnStartup performs a real validity check on the
// stored Codex OAuth tokens at TUI launch. The local file is treated as
// a structural prerequisite (missing → no-op, no banner); when it is
// present we round-trip the refresh token through OpenAI's token
// endpoint to confirm the grant has not been revoked server-side.
//
// Behavior matrix:
//
//   - file missing                                → return "" (user has no codex login, nothing to test)
//   - file malformed / no refresh_token           → file is junk; return banner pointing at re-login
//   - access token still valid (>5 min until exp) → trust local data, no network call
//   - access token expired/expiring               → refresh against auth.openai.com
//   - 200 OK         → atomic write back, return ""
//   - 401/403        → grant revoked, return banner pointing at re-login
//   - transient err  → return "" (do not penalize the user for being offline)
//
// On success the file is updated atomically (.json.tmp → rename) so any
// later code paths in this launch (firstrun's refreshCodexAuth, the
// agent-launch validateCodexAuthForAgents, the kernel's CodexTokenManager
// inside the agent process) all see the freshest tokens.
func ValidateCodexAuthOnStartup(globalDir string) string {
	// Refresh every stored account (legacy + per-account files). A revoked
	// or malformed account yields a banner that names which account; valid
	// or absent accounts are silent. The first problem account wins the
	// returned banner so the launch line stays one short string.
	accounts := listCodexAccounts(globalDir)
	if len(accounts) == 0 {
		return ""
	}
	var banner string
	for _, acct := range accounts {
		if msg := validateOneCodexAuthFile(acct.Path, acct.DisplayName()); msg != "" && banner == "" {
			banner = msg
		}
	}
	return banner
}

// validateOneCodexAuthFile refreshes a single Codex token file in place,
// returning a banner string only on a malformed file or a server-side-revoked
// grant. label identifies the account in the banner without leaking secrets.
// Token material is written 0600 and never logged. The actual expiry check,
// refresh call, and atomic write-back live in ensureFreshCodexTokens
// (oauth.go), shared with the save-time Codex eligibility probe
// (codex_model_probe.go) so both agree on staleness/revocation handling.
func validateOneCodexAuthFile(authPath, label string) string {
	raw, err := os.ReadFile(authPath)
	if err != nil {
		return ""
	}
	var tokens CodexTokens
	if err := json.Unmarshal(raw, &tokens); err != nil || tokens.RefreshToken == "" {
		return fmt.Sprintf("⚠ Codex OAuth (%s): credential malformed — re-login via /setup", label)
	}

	_, err = ensureFreshCodexTokens(authPath, tokens)
	if err == ErrCodexAuthRevoked {
		// Localized banner (#412). The %s slot is a navigation hint
		// (/setup → <credentials section>), so it carries the section
		// label, not the account. Per-account coverage (#415) is provided
		// by validateCodexAuthOnStartup iterating every account file; the
		// account itself is identified via the malformed banner below.
		return i18n.TF("codex.oauth_expired_banner", i18n.T("preset.codex_credential_section"))
	}
	// Both nil (already fresh, or refreshed and persisted) and
	// ErrCodexAuthTransient (network/5xx/timeout/write failure) are silent
	// here, matching pre-extraction behavior: do not penalize the user for
	// being offline, and do not surface anything when nothing is wrong.
	return ""
}

// codexOAuthConfigured reports whether the legacy single-account file
// ~/.lingtai-tui/codex-auth.json parses and carries a non-empty
// refresh_token. It is the fallback signal for a codex preset that declares
// no manifest.llm.codex_auth_path; per-account validity is checked through
// preset.AuthState.CodexAuthDir. It reads no secret to the screen; it only
// returns a bool.
func codexOAuthConfigured(globalDir string) bool {
	return codexAuthPathValid(legacyCodexAuthPath(globalDir))
}

// validateCodexAuthForAgents scans active/default preset manifests for every
// agent under projectDir. Single-account Codex presets validate their bound
// account; Codex pool presets reuse the kernel-mirroring pool eligibility facts.
// Provider ownership comes only from a successfully loaded manifest, never from
// a preset filename or path, so malformed/unreadable refs are skipped. Returns a
// warning naming the first Codex agent without usable credentials, or "" when
// every discovered Codex agent is eligible.
func validateCodexAuthForAgents(globalDir, projectDir string) string {
	entries, _ := os.ReadDir(projectDir)
	poolEligible, poolModels, poolFallback := codexPoolEligibilityFacts(globalDir)
	auth := preset.AuthState{
		CodexOAuthConfigured:      codexOAuthConfigured(globalDir),
		CodexAuthDir:              globalDir,
		CodexPoolEligible:         poolEligible,
		CodexPoolEligibleModels:   poolModels,
		CodexPoolFallbackEligible: poolFallback,
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		initPath := filepath.Join(projectDir, e.Name(), "init.json")
		raw, err := os.ReadFile(initPath)
		if err != nil {
			continue
		}
		var init map[string]interface{}
		if json.Unmarshal(raw, &init) != nil {
			continue
		}
		manifest, _ := init["manifest"].(map[string]interface{})
		presetBlock, _ := manifest["preset"].(map[string]interface{})
		if presetBlock == nil {
			continue
		}
		seen := map[string]bool{}
		for _, key := range []string{"default", "active"} {
			presetRef, _ := presetBlock[key].(string)
			if presetRef == "" || seen[presetRef] {
				continue
			}
			seen[presetRef] = true
			// The family and auth facts come exclusively from the loaded
			// manifest. Unreadable/malformed refs have no family and are
			// intentionally skipped; in particular they never fall back to
			// a legacy Codex credential based on a filename or path.
			rr := preset.ResolveRefsWithAuth([]string{presetRef}, nil, auth)
			if len(rr) != 1 || !rr[0].ManifestValid {
				continue
			}
			if (rr[0].Family == preset.CredentialFamilyCodexSingle || rr[0].Family == preset.CredentialFamilyCodexPool) && !rr[0].HasKey {
				return i18n.TF("codex.oauth_unverified_agent", e.Name())
			}
		}
	}
	return ""
}
