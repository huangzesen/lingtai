package tui

import (
	"image/color"
	"sort"

	"charm.land/lipgloss/v2"
)

// Theme defines the full color palette and derived styles for the TUI.
type Theme struct {
	// 核心色
	BG        color.Color // background
	Surface   color.Color // panels
	Border    color.Color // separators
	Text      color.Color // primary text
	TextDim   color.Color // secondary text
	TextFaint color.Color // faintest text

	// 角色色
	Agent  color.Color
	Human  color.Color
	System color.Color

	// 状态色
	Active    color.Color
	Idle      color.Color
	Stuck     color.Color
	Asleep    color.Color
	Suspended color.Color

	// 事件色
	Thinking color.Color
	Tool     color.Color
	Input    color.Color

	// 装饰色
	Accent color.Color
	Cursor color.Color

	// PulseShades is the breathing animation color cycle for the thinking indicator.
	// Should form a triangle wave (ramp up, then back down).
	PulseShades []string

	// Glamour markdown style name. Valid: "dark", "light", "notty", "ascii", "dracula", "auto".
	GlamourStyle string

	// Whether to force a painted background
	PaintBG bool
}

// ThemeInkDark is the default theme — 金漆墨韵.
// Gold lacquer accents on ink-dark ground.
func ThemeInkDark() Theme {
	return Theme{
		BG:        lipgloss.Color("#161718"), // 墨色（背景）
		Surface:   lipgloss.Color("#1c1d1e"), // 玄色（面板）
		Border:    lipgloss.Color("#2a2a30"), // 墨线（分割线）
		Text:      lipgloss.Color("#e8e4df"), // 宣纸白（主文字）
		TextDim:   lipgloss.Color("#8a8680"), // 旧墨灰（次要文字）
		TextFaint: lipgloss.Color("#4a4845"), // 淡墨（极淡文字）

		Agent:  lipgloss.Color("#7dab8f"), // 竹青（器灵）
		Human:  lipgloss.Color("#c49a6c"), // 琥珀（人）
		System: lipgloss.Color("#8ab4c4"), // 藤紫（系统）

		Active:    lipgloss.Color("#7dab8f"), // 竹青
		Idle:      lipgloss.Color("#6b8fa8"), // 苍蓝
		Stuck:     lipgloss.Color("#c4956a"), // 赭石
		Asleep:    lipgloss.Color("#9b8fa0"), // 藕荷
		Suspended: lipgloss.Color("#b85c5c"), // 朱砂

		Thinking: lipgloss.Color("#6b8fa8"), // 苍蓝（心思）
		Tool:     lipgloss.Color("#4a4845"), // 墨灰（工具）
		Input:    lipgloss.Color("#3a3835"), // 浓墨（输入）

		Accent: lipgloss.Color("#c49a6c"), // 琥珀（光）
		Cursor: lipgloss.Color("#c49a6c"), // 琥珀（光标）

		PulseShades: []string{
			"#2a4a5a", "#334f62", "#3a5a6a", "#425f72", "#4a6a7a",
			"#527082", "#5a7a8a", "#628092", "#6a8a9a", "#7290a2",
			"#7a9aaa", "#82a0b2", "#8aaaba", "#82a0b2", "#7a9aaa",
			"#7290a2", "#6a8a9a", "#628092", "#5a7a8a", "#527082",
			"#4a6a7a", "#425f72", "#3a5a6a", "#334f62",
		},
		GlamourStyle: "dark",
		PaintBG:      true,
	}
}

// ThemeXuanPaper is the light theme — 水墨宣纸.
// Ink wash on warm xuan paper, matching portal/web lightTheme.
func ThemeXuanPaper() Theme {
	return Theme{
		BG:        lipgloss.Color("#f5f0e8"), // 宣纸色（背景）
		Surface:   lipgloss.Color("#ebe6dc"), // 熟宣（面板）
		Border:    lipgloss.Color("#c5bfb5"), // 淡墨线（分割线）
		Text:      lipgloss.Color("#2a2520"), // 浓墨（主文字）
		TextDim:   lipgloss.Color("#5a504a"), // 暗墨灰（次要文字）
		TextFaint: lipgloss.Color("#8a8078"), // 旧墨（极淡文字）

		Agent:  lipgloss.Color("#3d7a54"), // 深竹青（器灵）
		Human:  lipgloss.Color("#9a7040"), // 深琥珀（人）
		System: lipgloss.Color("#3a6b85"), // 深苍蓝（系统）

		Active:    lipgloss.Color("#3d7a54"), // 深竹青
		Idle:      lipgloss.Color("#3a6b85"), // 深苍蓝
		Stuck:     lipgloss.Color("#a06930"), // 深赭石
		Asleep:    lipgloss.Color("#7a6480"), // 深藕荷
		Suspended: lipgloss.Color("#9b3a3a"), // 深朱砂

		Thinking: lipgloss.Color("#3a6b85"), // 深苍蓝（心思）
		Tool:     lipgloss.Color("#8a8078"), // 旧墨（工具）
		Input:    lipgloss.Color("#ebe6dc"), // 熟宣（输入）

		Accent: lipgloss.Color("#9a7040"), // 深琥珀（光）
		Cursor: lipgloss.Color("#9a7040"), // 深琥珀（光标）

		PulseShades: []string{
			"#8aafbf", "#7fa5b5", "#749bab", "#6991a1", "#5e8797",
			"#537d8d", "#487383", "#3d6979", "#3a6b85", "#3d6979",
			"#487383", "#537d8d", "#5e8797", "#6991a1", "#749bab",
			"#7fa5b5", "#8aafbf", "#95b9c9", "#a0c3d3", "#95b9c9",
			"#8aafbf", "#7fa5b5", "#749bab", "#6991a1",
		},
		GlamourStyle: "light",
		PaintBG:      true,
	}
}

// ThemeRegistry maps theme names to constructors.
// Add new themes here.
var ThemeRegistry = map[string]func() Theme{
	"ink-dark":   ThemeInkDark,
	"xuan-paper": ThemeXuanPaper,
}

// DefaultThemeName is the fallback when no theme is configured.
const DefaultThemeName = "ink-dark"

// ThemeByName returns the theme for a given name, falling back to default.
func ThemeByName(name string) Theme {
	if name == "" {
		name = DefaultThemeName
	}
	if fn, ok := ThemeRegistry[name]; ok {
		return fn()
	}
	return ThemeInkDark()
}

// ThemeNames returns all registered theme names in sorted order.
func ThemeNames() []string {
	names := make([]string, 0, len(ThemeRegistry))
	for name := range ThemeRegistry {
		names = append(names, name)
	}
	// Sort for stable UI ordering (default first since "ink-dark" < "xuan-paper")
	sort.Strings(names)
	return names
}

// activeTheme is the current theme. Set via SetTheme().
var activeTheme = ThemeInkDark()

// SetTheme switches the active theme and rebuilds all derived values.
// Does NOT write OSC sequences — call ApplyTerminalBG() or use
// ApplyTerminalBGCmd() separately.
func SetTheme(t Theme) {
	activeTheme = t
	rebuildStyles()
}

// SetThemeByName looks up a theme by name and applies it.
func SetThemeByName(name string) {
	SetTheme(ThemeByName(name))
}


// ActiveTheme returns the current theme (read-only copy).
func ActiveTheme() Theme { return activeTheme }

// ─── Package-level color aliases (used throughout the TUI) ─────────────
// These vars are rebuilt from activeTheme via rebuildStyles(), keeping all
// existing call sites (ColorText, StyleTitle, etc.) unchanged.

var (
	ColorBG        color.Color
	ColorSurface   color.Color
	ColorBorder    color.Color
	ColorText      color.Color
	ColorTextDim   color.Color
	ColorSubtle    color.Color // alias for TextDim
	ColorTextFaint color.Color

	ColorAgent  color.Color
	ColorHuman  color.Color
	ColorSystem color.Color
	ColorMail   color.Color // alias for System

	ColorActive    color.Color
	ColorIdle      color.Color
	ColorStuck     color.Color
	ColorAsleep    color.Color
	ColorSuspended color.Color

	ColorThinking color.Color
	ColorTool     color.Color
	ColorInput    color.Color

	ColorAccent color.Color
	ColorCursor color.Color

	// Lipgloss 样式
	StyleTitle  lipgloss.Style
	StyleSubtle lipgloss.Style
	StyleFaint  lipgloss.Style
	StyleAccent lipgloss.Style

	// 边框字符
	RuneBullet = "·"
)

func init() {
	rebuildStyles()
}

// rebuildStyles syncs all package-level vars from activeTheme.
func rebuildStyles() {
	t := activeTheme

	ColorBG = t.BG
	ColorSurface = t.Surface
	ColorBorder = t.Border
	ColorText = t.Text
	ColorTextDim = t.TextDim
	ColorSubtle = t.TextDim
	ColorTextFaint = t.TextFaint

	ColorAgent = t.Agent
	ColorHuman = t.Human
	ColorSystem = t.System
	ColorMail = t.System

	ColorActive = t.Active
	ColorIdle = t.Idle
	ColorStuck = t.Stuck
	ColorAsleep = t.Asleep
	ColorSuspended = t.Suspended

	ColorThinking = t.Thinking
	ColorTool = t.Tool
	ColorInput = t.Input

	ColorAccent = t.Accent
	ColorCursor = t.Cursor

	StyleTitle = lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	StyleSubtle = lipgloss.NewStyle().Foreground(ColorTextDim)
	StyleFaint = lipgloss.NewStyle().Foreground(ColorTextFaint)
	StyleAccent = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
}

// StateColor returns the color for a given agent state string.
func StateColor(state string) color.Color {
	switch state {
	case "ACTIVE":
		return ColorActive
	case "IDLE":
		return ColorIdle
	case "STUCK":
		return ColorStuck
	case "ASLEEP":
		return ColorAsleep
	case "SUSPENDED":
		return ColorSuspended
	default:
		return ColorTextDim
	}
}


