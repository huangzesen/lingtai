package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/charmbracelet/x/ansi"
)

func TestSlashPaletteFitsFrameAndScrollsSelection(t *testing.T) {
	originalLang := i18n.Lang()
	t.Cleanup(func() { _ = i18n.SetLang(originalLang) })
	if err := i18n.SetLang("en"); err != nil {
		t.Fatal(err)
	}

	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 80, height: 24},
		{width: 60, height: 20},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			app := mailLayoutApp(t)
			app.mail.initialLoading = false

			model, _ := app.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			app = model.(App)
			model, _ = app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			app = model.(App)

			content := app.View().Content
			if got := lipgloss.Height(content); got > size.height {
				t.Errorf("slash palette frame height = %d, want <= terminal height %d", got, size.height)
			}
			maxWidth := 0
			for _, line := range strings.Split(content, "\n") {
				if width := lipgloss.Width(line); width > maxWidth {
					maxWidth = width
				}
			}
			if maxWidth > size.width {
				t.Errorf("slash palette maximum line width = %d, want <= terminal width %d", maxWidth, size.width)
			}

			for range 20 {
				model, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				app = model.(App)
			}
			if marked := ansi.Strip(app.View().Content); !strings.Contains(marked, "> /skills") {
				t.Errorf("selected command is not visibly marked after scrolling:\n%s", marked)
			}

			model, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			app = model.(App)
			if cmd == nil {
				t.Fatal("Enter did not emit a palette selection command")
			}
			selected, ok := cmd().(PaletteSelectMsg)
			if !ok {
				t.Fatalf("Enter emitted %T, want PaletteSelectMsg", cmd())
			}
			if selected.Command != "skills" || selected.Args != "" {
				t.Fatalf("Enter selected %#v, want command skills with no args", selected)
			}
		})
	}
}

func TestMailNoMatchPaletteDoesNotAddPhantomRow(t *testing.T) {
	originalLang := i18n.Lang()
	t.Cleanup(func() { _ = i18n.SetLang(originalLang) })
	if err := i18n.SetLang("en"); err != nil {
		t.Fatal(err)
	}

	app := mailLayoutApp(t)
	app.mail.initialLoading = false
	model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app = model.(App)
	for _, r := range "/zz" {
		model, _ = app.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		app = model.(App)
	}

	if !app.mail.input.IsPaletteActive() {
		t.Fatal("slash input did not leave the palette active")
	}
	if got := app.mail.palette.LineCount(); got != 0 {
		t.Fatalf("no-match palette LineCount = %d, want 0", got)
	}
	if got := app.mail.palette.View(); got != "" {
		t.Fatalf("no-match palette View = %q, want empty", got)
	}
	if got := lipgloss.Height(app.View().Content); got > 24 {
		t.Fatalf("no-match slash frame height = %d, want <= 24", got)
	}
}

func TestSlashPaletteFitsLocalizedMailChildWidths(t *testing.T) {
	originalLang := i18n.Lang()
	t.Cleanup(func() { _ = i18n.SetLang(originalLang) })

	sizes := []struct {
		terminalWidth int
		childWidth    int
	}{
		{terminalWidth: 60, childWidth: 60},
		{terminalWidth: 80, childWidth: 80},
		{terminalWidth: 85, childWidth: 61},
		{terminalWidth: 100, childWidth: 76},
	}
	for _, lang := range []string{"en", "zh", "wen"} {
		if err := i18n.SetLang(lang); err != nil {
			t.Fatal(err)
		}
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%d", lang, size.terminalWidth), func(t *testing.T) {
				app := mailLayoutApp(t)
				app.mail.initialLoading = false

				model, _ := app.Update(tea.WindowSizeMsg{Width: size.terminalWidth, Height: 24})
				app = model.(App)
				model, _ = app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
				app = model.(App)

				if got := app.mail.width; got != size.childWidth {
					t.Fatalf("Mail child width = %d, want %d", got, size.childWidth)
				}
				assertPaletteGeometry(t, app.mail.palette, size.childWidth)
			})
		}
	}
}

func TestPaletteNavigationFilterAndExclusionKeepSelectionVisible(t *testing.T) {
	m := NewPaletteModel()
	m.SetSize(60, 5)

	for range len(DefaultCommands()) + 5 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	assertMarkedPaletteSelection(t, m, "quit")
	assertPaletteSelection(t, m, "quit")

	for range len(DefaultCommands()) + 5 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	assertMarkedPaletteSelection(t, m, "btw")
	assertPaletteSelection(t, m, "btw")

	m.ExcludeCommands("btw", "quit")
	m.SetFilter("presets")
	assertMarkedPaletteSelection(t, m, "presets")
	assertPaletteSelection(t, m, "presets")

	m.SetFilter("")
	assertMarkedPaletteSelection(t, m, "sleep")
	assertPaletteSelection(t, m, "sleep")

	m.SetFilter("no-command-matches-this")
	if got := m.LineCount(); got != 0 {
		t.Fatalf("empty filter result LineCount = %d, want 0", got)
	}
	if got := m.View(); got != "" {
		t.Fatalf("empty filter result View = %q, want empty", got)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("empty filter result emitted a selection command")
	}
}

func TestMailPaletteAllowanceReservesVariableRows(t *testing.T) {
	cases := []struct {
		name               string
		input              string
		topBanner          bool
		bottomBanner       bool
		telemetry          bool
		wantPaletteLines   int
		wantInputLineCount int
	}{
		{name: "baseline", input: "/", wantPaletteLines: 17, wantInputLineCount: 1},
		{name: "top banner", input: "/", topBanner: true, wantPaletteLines: 16, wantInputLineCount: 1},
		{name: "telemetry", input: "/", telemetry: true, wantPaletteLines: 16, wantInputLineCount: 1},
		{name: "multiline composer", input: "/\nsecond\nthird", wantPaletteLines: 15, wantInputLineCount: 3},
		{name: "all variable rows", input: "/\nsecond\nthird", topBanner: true, bottomBanner: true, telemetry: true, wantPaletteLines: 12, wantInputLineCount: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := mailLayoutApp(t)
			app.mail.initialLoading = false
			model, _ := app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			app = model.(App)

			app.mail.initialLoading = tc.topBanner
			if tc.bottomBanner {
				app.mail.loadedExtra = 1
			}
			if tc.telemetry {
				app.mail.homeTelemetryLoaded = true
				app.mail.homeTelemetry = homeTelemetry{contextUsage: 0.5}
			}
			app.mail.input.SetValue(tc.input)
			app.mail.palette.SetFilter("")
			app.mail.lastInputLines = -1
			app.mail.syncViewportHeight()

			if got := app.mail.input.LineCount(); got != tc.wantInputLineCount {
				t.Fatalf("input LineCount = %d, want %d", got, tc.wantInputLineCount)
			}
			if got := app.mail.palette.LineCount(); got != tc.wantPaletteLines {
				t.Fatalf("palette LineCount = %d, want %d", got, tc.wantPaletteLines)
			}
			if got := lipgloss.Height(app.mail.View()); got > 24 {
				t.Fatalf("Mail frame height = %d, want <= 24", got)
			}
			assertPaletteGeometry(t, app.mail.palette, 80)
		})
	}
}

func TestSlashPaletteTinySizesDoNotPanic(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 0, height: 0},
		{width: 1, height: 1},
		{width: 3, height: 2},
		{width: 4, height: 3},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			app := mailLayoutApp(t)
			app.mail.initialLoading = false
			model, _ := app.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
			app = model.(App)
			model, _ = app.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
			app = model.(App)
			_ = app.View()
			assertPaletteGeometry(t, app.mail.palette, size.width)
		})
	}
}

func assertPaletteGeometry(t *testing.T, m PaletteModel, width int) {
	t.Helper()
	view := m.View()
	viewHeight := 0
	if view != "" {
		viewHeight = lipgloss.Height(view)
	}
	if got, want := viewHeight, m.LineCount(); got != want {
		t.Fatalf("palette View height = %d, LineCount = %d", got, want)
	}
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("palette line width = %d, want <= %d: %q", got, width, ansi.Strip(line))
		}
	}
}

func assertMarkedPaletteSelection(t *testing.T, m PaletteModel, command string) {
	t.Helper()
	marked := ansi.Strip(m.View())
	if !strings.Contains(marked, "> /"+command) {
		t.Fatalf("selected command /%s is not visibly marked:\n%s", command, marked)
	}
}

func assertPaletteSelection(t *testing.T, m PaletteModel, command string) {
	t.Helper()
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("Enter did not emit selection for /%s", command)
	}
	selected, ok := cmd().(PaletteSelectMsg)
	if !ok {
		t.Fatalf("Enter emitted %T, want PaletteSelectMsg", cmd())
	}
	if selected.Command != command || selected.Args != "" {
		t.Fatalf("Enter selected %#v, want /%s with no args", selected, command)
	}
}
