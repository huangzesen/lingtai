package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// 墨韵灵台调色板
// 灵感：中国水墨画 · 明镜台 · 古印章
var (
	// 核心色
	ColorBG        = lipgloss.Color("#0d0d0f") // 墨黑（背景）
	ColorSurface   = lipgloss.Color("#151518") // 玄色（面板）
	ColorBorder    = lipgloss.Color("#2a2a30") // 墨线（分割线）
	ColorText      = lipgloss.Color("#e8e4df") // 宣纸白（主文字）
	ColorTextDim   = lipgloss.Color("#8a8680") // 旧墨灰（次要文字）
	ColorSubtle    = ColorTextDim              // 别名，兼容旧代码
	ColorTextFaint = lipgloss.Color("#4a4845") // 淡墨（极淡文字）

	// 角色色
	ColorAgent  = lipgloss.Color("#7dab8f") // 竹青（器灵）
	ColorHuman  = lipgloss.Color("#c49a6c") // 琥珀（人）
	ColorSystem = lipgloss.Color("#8ab4c4") // 藤紫（系统）
	ColorMail   = ColorSystem               // 别名，兼容旧代码

	// 状态色
	ColorActive    = lipgloss.Color("#7dab8f") // 竹青
	ColorIdle      = lipgloss.Color("#6b8fa8") // 苍蓝
	ColorStuck     = lipgloss.Color("#c4956a") // 赭石
	ColorAsleep    = lipgloss.Color("#9b8fa0") // 藕荷
	ColorSuspended = lipgloss.Color("#b85c5c") // 朱砂

	// 事件色
	ColorThinking = lipgloss.Color("#6b8fa8") // 苍蓝（心思）
	ColorTool     = lipgloss.Color("#4a4845") // 墨灰（工具）
	ColorInput    = lipgloss.Color("#3a3835") // 浓墨（输入）

	// 装饰色
	ColorAccent = lipgloss.Color("#c49a6c") // 琥珀（光）
	ColorCursor = lipgloss.Color("#c49a6c") // 琥珀（光标）

	// Lipgloss 样式
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText)

	StyleSubtle = lipgloss.NewStyle().
			Foreground(ColorTextDim)

	StyleFaint = lipgloss.NewStyle().
			Foreground(ColorTextFaint)

	StyleAccent = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	// 边框字符
	RuneBullet = "·"
)

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
