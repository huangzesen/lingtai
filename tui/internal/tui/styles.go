package tui

import (
	"image/color"
	"os"
	"sort"
	"strings"

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

// inTmux is true when the process is running inside a tmux session.
// tmux does not propagate terminal-level background colors set via
// tea.View.BackgroundColor, so we need to paint explicit ANSI
// background codes on every line of the viewport content.
var inTmux = os.Getenv("TMUX") != ""

// PaintViewportBG replaces the default terminal background with our
// theme BG on every character cell. Only active inside tmux where
// terminal-level BG doesn't propagate.
//
// Walks each line through a minimal ANSI SGR state machine. Wherever
// the background is "default" (after a reset or never explicitly set),
// our theme BG is injected. Content that sets its own explicit
// background (e.g. code blocks) is left untouched. Each line is
// padded to the full terminal width with BG-colored spaces.
func PaintViewportBG(content string, width int) string {
	if !inTmux || !activeTheme.PaintBG {
		return content
	}

	// Extract our BG escape by probing lipgloss.
	bgStyle := lipgloss.NewStyle().Background(ColorBG)
	probe := bgStyle.Render(" ")
	// probe = "<BG-on> <reset>". Strip trailing " \033[0m".
	const ansiReset = "\033[0m"
	bgEsc := strings.TrimSuffix(probe, " "+ansiReset)
	if bgEsc == "" || bgEsc == probe {
		bgEsc = strings.TrimSuffix(strings.TrimSuffix(probe, ansiReset), " ")
	}
	if bgEsc == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = paintLineBG(line, width, bgEsc)
	}
	return strings.Join(lines, "\n")
}

// paintLineBG processes one line: injects bgEsc wherever BG is
// "default", leaves explicit BG untouched, pads to full width.
func paintLineBG(line string, width int, bgEsc string) string {
	var out strings.Builder
	out.Grow(len(line) + len(bgEsc)*10 + width)

	bgExplicit := false // true when content has set its own BG
	visibleW := 0
	b := []byte(line)
	n := len(b)

	// Initial state: BG is default → inject ours
	out.WriteString(bgEsc)

	for j := 0; j < n; {
		// Detect ESC sequence
		if b[j] == 0x1b && j+1 < n && b[j+1] == '[' {
			// Find end of CSI sequence
			k := j + 2
			for k < n && !((b[k] >= 0x40 && b[k] <= 0x7E)) {
				k++
			}
			if k < n {
				k++ // include terminator
			}
			seq := b[j:k]

			if seq[len(seq)-1] == 'm' {
				// SGR sequence — track BG state
				wasExplicit := bgExplicit
				bgExplicit = sgrSetsBG(string(seq), bgExplicit)

				out.Write(seq)

				// Transition: explicit → default — re-inject our BG
				if wasExplicit && !bgExplicit {
					out.WriteString(bgEsc)
				}
			} else {
				// Non-SGR CSI — pass through
				out.Write(seq)
			}
			j = k
			continue
		}

		// Regular byte — possibly multi-byte UTF-8
		if b[j] >= 0x80 {
			size := 1
			ch := rune(b[j])
			switch {
			case b[j]&0xE0 == 0xC0 && j+1 < n:
				ch = rune(b[j]&0x1F)<<6 | rune(b[j+1]&0x3F)
				size = 2
			case b[j]&0xF0 == 0xE0 && j+2 < n:
				ch = rune(b[j]&0x0F)<<12 | rune(b[j+1]&0x3F)<<6 | rune(b[j+2]&0x3F)
				size = 3
			case b[j]&0xF8 == 0xF0 && j+3 < n:
				ch = rune(b[j]&0x07)<<18 | rune(b[j+1]&0x3F)<<12 | rune(b[j+2]&0x3F)<<6 | rune(b[j+3]&0x3F)
				size = 4
			}
			out.Write(b[j : j+size])
			visibleW += cjkWidth(ch)
			j += size
		} else {
			out.WriteByte(b[j])
			visibleW++
			j++
		}
	}

	// Pad to full width with our BG
	if visibleW < width {
		if bgExplicit {
			// Content's BG is active — reset it, apply ours
			out.WriteString("\033[49m")
			out.WriteString(bgEsc)
		}
		for p := 0; p < width-visibleW; p++ {
			out.WriteByte(' ')
		}
	}

	out.WriteString("\033[0m")
	return out.String()
}

// sgrSetsBG parses SGR params and returns whether an explicit
// (non-default) background is active after this sequence.
func sgrSetsBG(seq string, wasExplicit bool) bool {
	if len(seq) < 4 {
		return wasExplicit
	}
	inner := seq[2 : len(seq)-1] // between \033[ and m
	if inner == "" || inner == "0" {
		return false // full reset
	}
	explicit := wasExplicit
	params := strings.Split(inner, ";")
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == "0":
			explicit = false
		case p == "49":
			explicit = false
		case p == "48":
			explicit = true
			// Skip sub-params (48;5;N or 48;2;R;G;B)
			if i+1 < len(params) {
				if params[i+1] == "5" {
					i += 2
				} else if params[i+1] == "2" {
					i += 4
				}
			}
		default:
			// 40-47 or 100-107
			if len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7' {
				explicit = true
			}
			if len(p) == 3 && p[0] == '1' && p[1] == '0' && p[2] >= '0' && p[2] <= '7' {
				explicit = true
			}
		}
	}
	return explicit
}

// cjkWidth returns 2 for CJK/fullwidth characters, 1 otherwise.
func cjkWidth(r rune) int {
	if (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE6F) ||
		(r >= 0xFF01 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x20000 && r <= 0x2FA1F) {
		return 2
	}
	return 1
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
	case "REFRESHING":
		return ColorIdle
	default:
		return ColorTextDim
	}
}


