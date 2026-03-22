package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Lang is the current language code.
var Lang = detectLang()

// S returns the localized string for the given key.
func S(key string) string {
	if m, ok := translations[Lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	// Fall back to English
	if s, ok := translations["en"][key]; ok {
		return s
	}
	return key
}

// Renamed from "strings" to "translations" to avoid shadowing the built-in strings package.
// Languages is the ordered list of supported language codes.
var Languages = []string{"en", "zh", "lzh"}

// LanguageLabels maps language codes to display labels (shown in language selector).
var LanguageLabels = map[string]string{
	"en":  "English",
	"zh":  "中文",
	"lzh": "文言",
}

var translations = map[string]map[string]string{
	"en": {
		"title":           "LingTai",
		"setup_title":     "Setup Wizard",
		"setup_lang":      "Language",
		"manage_title":    "Running Spirits",
		"starting":        "Starting agent...",
		"shutting_down":   "Shutting down...",
		"connected":       "Connected",
		"disconnected":    "Disconnected",
		"press_ctrl_c":    "Press Ctrl+C to shut down",
		"type_message":    "Type a message...",
		"no_spirits":      "No running spirits found.",
		"name":            "Name",
		"pid":             "PID",
		"port":            "Port",
		"uptime":          "Uptime",
		"status":          "Status",
		"running":         "running",
		"dead":            "dead (stale PID)",
		"setup_model":     "LLM Provider",
		"setup_imap":      "IMAP Email",
		"setup_telegram":  "Telegram Bot",
		"setup_messaging": "Messaging",
		"setup_general":   "General Settings",
		"setup_review":    "Review",
		"setup_done":      "Setup Complete",
		"setup_lang_hint": "↑/↓ to select, Enter to confirm",
		"setup_saved":     "Configuration saved successfully!",
		"setup_files":     "Files written:",
		"setup_multimodal": "Multimodal",
		"setup_combo":      "Combo",
		"setup_cycle_hint": "left/right to cycle",
		"field_provider":    "Provider",
		"field_model":       "Model",
		"field_api_key":     "API key",
		"field_endpoint":    "Endpoint",
		"field_email":       "Email address",
		"field_password":    "Password",
		"field_imap_host":   "IMAP host",
		"field_imap_port":   "IMAP port",
		"field_smtp_host":   "SMTP host",
		"field_smtp_port":   "SMTP port",
		"field_bot_token":   "Bot token",
		"field_agent_name":  "Agent name (Enter = skip, discuss with agent later)",
		"field_agent_port":  "Agent port",
		"field_agent_lang":  "Agent language",
		"field_lifetime":    "Lifetime in seconds (default: 86400 = 24h)",
		"field_flow_delay":  "Soul delay in seconds (default: 120)",
		"field_bash_policy": "Bash policy (Enter = use default)",
		"combo_select":      "Select a combo or create new:",
		"combo_create_new":  "Create new",
		"combo_hint":        "↑/↓ navigate  Enter select",
		"combo_save_as":     "Save as combo:",
		"combo":              "Combo",
		"status_help":        "[Enter] Chat   [S] Setup   [L] Language   [K] Kill all   [Q] Quit",
		"mm_quick_setup":     "MiniMax Quick Setup",
		"mm_quick":           "Quick Setup",
		"mm_manual":          "Manual Configuration",
		"mm_skip":            "Skip",
		"mm_endpoint":        "Endpoint",
		"mm_china":           "China",
		"mm_international":   "International",
		"msg_imap":           "IMAP/SMTP Email",
		"msg_telegram":       "Telegram Bot",
		"msg_skip":           "Skip",
		"unknown_daemon":     "Unknown daemon",
		"switched_to":        "Switched to",
		"provider_minimax":   "MiniMax (稀宇)",
		"provider_openai":    "OpenAI",
		"provider_anthropic": "Anthropic",
		"provider_gemini":    "Google (谷歌) Gemini",
		"provider_custom":    "Custom",

		// Banner
		"banner_title": "LingTai AI",
		"banner_line1": "Awakened beneath the Bodhi;",
		"banner_line2": "one mind, thousand avatars.",

		// MM chooser descriptions
		"mm_quick_desc":  "Enter two API keys — fills vision, web search, talk, compose, draw automatically",
		"mm_manual_desc": "Configure each capability individually with any provider",
		"mm_skip_desc":   "Skip multimodal setup for now",

		// MM quick setup
		"mm_key_vision_desc": "(vision, web search)",
		"mm_key_mcp_desc":    "(talk, compose, draw)",
		"not_set":            "(not set)",
		"mm_quick_hint":      "Tab: next field | ←/→: cycle endpoint | Enter: apply & continue | Esc: back",

		// Messaging chooser descriptions
		"msg_imap_desc":     "Connect to an IMAP/SMTP email account",
		"msg_telegram_desc": "Connect a Telegram bot",
		"msg_skip_desc":     "Skip external messaging setup",
		"msg_chooser_hint":  "↑/↓ to select, Enter to configure, Esc to go back",

		// Messaging fields hint
		"msg_fields_hint": "Tab/↓: next field | Enter: save & back | Esc: back | Ctrl+T: test",

		// MM grid
		"mm_col_capability": "Capability",
		"mm_col_provider":   "Provider",
		"mm_col_api_key":    "API Key",
		"mm_col_endpoint":   "Endpoint",
		"no_config_needed":  "no config needed",
		"no_key":            "(no key)",
		"runs_locally":      "runs locally",
		"no_endpoint":       "(no endpoint)",
		"mm_grid_hint":      "↑/↓: move row | Tab: next field | ←/→: cycle provider | Enter: next step | Esc: back",

		// Review
		"review_model":       "Model:",
		"review_provider":    "Provider:",
		"review_api_key":     "API key:",
		"review_endpoint":    "Endpoint:",
		"review_reusing_key": "reusing main key",
		"review_skipped":     "skipped",
		"review_imap":        "IMAP/SMTP:",
		"review_email":       "Email:",
		"review_password":    "Password:",
		"review_imap_server": "IMAP:",
		"review_smtp_server": "SMTP:",
		"review_imap_skipped":     "IMAP/SMTP: skipped",
		"review_telegram":         "Telegram:",
		"review_token":            "Token:",
		"review_telegram_skipped": "Telegram: skipped",
		"review_general":          "General:",
		"review_agent_name":       "Agent Name:",
		"review_port":             "Port:",
		"review_agent_lang":       "Agent Language:",
		"review_lifetime":         "Lifetime:",
		"review_flow_delay":       "Soul Delay:",
		"review_bash_policy":      "Bash Policy:",
		"review_config_path":      "Config →",
		"review_secrets_path":     "Secrets →",
		"review_save_hint":        "Enter → save, Ctrl+C → abort",

		// Field hints
		"hint_tab_next":   "Tab/Down: next field",
		"hint_tab_prev":   "Shift+Tab/Up: prev field",
		"hint_enter_next": "Enter: next step",

		// Chat view
		"verbose_on":    "verbose ●",
		"active_marker": "← active",

		// manage/list
		"stop_hint": "Stop with: kill <PID>",
	},
	"zh": {
		"title":           "灵台",
		"setup_title":     "设置向导",
		"setup_lang":      "语言",
		"manage_title":    "运行中的器灵",
		"starting":        "正在启动...",
		"shutting_down":   "正在关闭...",
		"connected":       "已连接",
		"disconnected":    "未连接",
		"press_ctrl_c":    "按 Ctrl+C 关闭",
		"type_message":    "输入消息...",
		"no_spirits":      "没有运行中的器灵。",
		"name":            "名称",
		"pid":             "进程号",
		"port":            "端口",
		"uptime":          "运行时间",
		"status":          "状态",
		"running":         "运行中",
		"dead":            "已停止（残留PID）",
		"setup_model":     "语言模型配置",
		"setup_imap":      "IMAP 邮箱",
		"setup_telegram":  "Telegram 机器人",
		"setup_messaging": "通信",
		"setup_general":   "基本设置",
		"setup_review":    "确认",
		"setup_done":      "设置完成",
		"setup_lang_hint": "↑/↓ 选择，Enter 确认",
		"setup_saved":     "配置保存成功！",
		"setup_files":     "已写入文件：",
		"setup_multimodal": "多模态",
		"setup_combo":      "组合",
		"setup_cycle_hint": "左/右切换",
		"field_provider":    "Provider (提供商)",
		"field_model":       "Model (模型)",
		"field_api_key":     "API key (密钥)",
		"field_endpoint":    "Endpoint (接入点)",
		"field_email":       "Email (邮箱地址)",
		"field_password":    "Password (密码)",
		"field_imap_host":   "IMAP host (主机)",
		"field_imap_port":   "IMAP port (端口)",
		"field_smtp_host":   "SMTP host (主机)",
		"field_smtp_port":   "SMTP port (端口)",
		"field_bot_token":   "Bot token (令牌)",
		"field_agent_name":  "智能体名称（Enter = 跳过，稍后与智能体商定）",
		"field_agent_port":  "智能体端口",
		"field_agent_lang":  "智能体语言",
		"field_lifetime":    "生命周期（秒，默认：86400 = 24小时）",
		"field_flow_delay":  "内心独白延迟（秒，默认：120）",
		"field_bash_policy": "Bash 策略（Enter = 使用默认）",
		"combo_select":      "选择已有组合或新建：",
		"combo_create_new":  "新建",
		"combo_hint":        "↑/↓ 选择  Enter 确认",
		"combo_save_as":     "保存为组合：",
		"combo":              "组合",
		"status_help":        "[Enter] 对话   [S] 设置   [L] 语言   [K] 终止全部   [Q] 退出",
		"mm_quick_setup":     "稀宇快速配置",
		"mm_quick":           "快速配置",
		"mm_manual":          "手动配置",
		"mm_skip":            "跳过",
		"mm_endpoint":        "Endpoint (接入点)",
		"mm_china":           "中国",
		"mm_international":   "国际",
		"msg_imap":           "IMAP/SMTP 邮箱",
		"msg_telegram":       "Telegram 机器人",
		"msg_skip":           "跳过",
		"unknown_daemon":     "未知灵体",
		"switched_to":        "已切换至",
		"provider_minimax":   "稀宇 MiniMax",
		"provider_openai":    "OpenAI",
		"provider_anthropic": "Anthropic",
		"provider_gemini":    "谷歌 Gemini",
		"provider_custom":    "自定义",

		// Banner
		"banner_title": "灵台AI",
		"banner_line1": "灵台方寸山  斜月三星洞",
		"banner_line2": "闻道菩提下  一心化万相",

		// MM chooser descriptions
		"mm_quick_desc":  "输入两个 API key — 自动配置 vision, web search, talk, compose, draw",
		"mm_manual_desc": "逐个配置每项能力，可选任意 provider",
		"mm_skip_desc":   "暂时跳过多模态配置",

		// MM quick setup
		"mm_key_vision_desc": "(vision, web search)",
		"mm_key_mcp_desc":    "(talk, compose, draw)",
		"not_set":            "（未设置）",
		"mm_quick_hint":      "Tab: 下一项 | ←/→: 切换 endpoint | Enter: 应用并继续 | Esc: 返回",

		// Messaging chooser descriptions
		"msg_imap_desc":     "连接 IMAP/SMTP 邮箱",
		"msg_telegram_desc": "连接 Telegram 机器人",
		"msg_skip_desc":     "跳过外部通信配置",
		"msg_chooser_hint":  "↑/↓ 选择，Enter 配置，Esc 返回",

		// Messaging fields hint
		"msg_fields_hint": "Tab/↓: 下一项 | Enter: 保存并返回 | Esc: 返回 | Ctrl+T: 测试",

		// MM grid
		"mm_col_capability": "Capability (能力)",
		"mm_col_provider":   "Provider (提供商)",
		"mm_col_api_key":    "API Key (密钥)",
		"mm_col_endpoint":   "Endpoint (接入点)",
		"no_config_needed":  "无需配置",
		"no_key":            "（无密钥）",
		"runs_locally":      "本地运行",
		"no_endpoint":       "（无接入点）",
		"mm_grid_hint":      "↑/↓: 移动 | Tab: 下一列 | ←/→: 切换 provider | Enter: 下一步 | Esc: 返回",

		// Review
		"review_model":       "Model:",
		"review_provider":    "Provider:",
		"review_api_key":     "API key:",
		"review_endpoint":    "Endpoint:",
		"review_reusing_key": "复用主密钥",
		"review_skipped":     "已跳过",
		"review_imap":        "IMAP/SMTP:",
		"review_email":       "Email:",
		"review_password":    "Password:",
		"review_imap_server": "IMAP:",
		"review_smtp_server": "SMTP:",
		"review_imap_skipped":     "IMAP/SMTP: 已跳过",
		"review_telegram":         "Telegram:",
		"review_token":            "Token:",
		"review_telegram_skipped": "Telegram: 已跳过",
		"review_general":          "General (通则):",
		"review_agent_name":       "Agent Name (名称):",
		"review_port":             "Port (端口):",
		"review_agent_lang":       "Agent Language (语言):",
		"review_lifetime":         "Lifetime (生命周期):",
		"review_flow_delay":       "Soul Delay (独白延迟):",
		"review_bash_policy":      "Bash Policy (策略):",
		"review_config_path":      "Config →",
		"review_secrets_path":     "Secrets →",
		"review_save_hint":        "Enter → 保存，Ctrl+C → 取消",

		// Field hints
		"hint_tab_next":   "Tab/↓: 下一项",
		"hint_tab_prev":   "Shift+Tab/↑: 上一项",
		"hint_enter_next": "Enter: 下一步",

		// Chat view
		"verbose_on":    "详情 ●",
		"active_marker": "← 当前",

		// manage/list
		"stop_hint": "终止: kill <PID>",
	},
	"lzh": {
		"title":           "灵台",
		"setup_title":     "初设",
		"setup_lang":      "言语",
		"manage_title":    "诸器灵",
		"starting":        "启灵中……",
		"shutting_down":   "收灵中……",
		"connected":       "已通",
		"disconnected":    "未通",
		"press_ctrl_c":    "按 Ctrl+C 止之",
		"type_message":    "书信于此……",
		"no_spirits":      "无器灵运行。",
		"name":            "名",
		"pid":             "号",
		"port":            "埠",
		"uptime":          "历时",
		"status":          "状",
		"running":         "运行",
		"dead":            "已殁（残号）",
		"setup_model":     "模型之设",
		"setup_imap":      "邮驿之设",
		"setup_telegram":  "电报之设",
		"setup_messaging": "通信之设",
		"setup_general":   "通则",
		"setup_review":    "审定",
		"setup_done":      "初设已毕",
		"setup_lang_hint": "↑/↓ 择之，Enter 定之",
		"setup_saved":     "设定已录！",
		"setup_files":     "所录之档：",
		"setup_multimodal": "诸能",
		"setup_combo":      "旧方",
		"setup_cycle_hint": "左右择之",
		"field_provider":    "Provider (供者)",
		"field_model":       "Model (模型)",
		"field_api_key":     "API key (密钥)",
		"field_endpoint":    "Endpoint (入口)",
		"field_email":       "Email (邮址)",
		"field_password":    "Password (口令)",
		"field_imap_host":   "IMAP host (邮驿之主)",
		"field_imap_port":   "IMAP port (邮驿之埠)",
		"field_smtp_host":   "SMTP host (发驿之主)",
		"field_smtp_port":   "SMTP port (发驿之埠)",
		"field_bot_token":   "Bot token (令牌)",
		"field_agent_name":  "本我之名（Enter = 略过，入灵台后再议）",
		"field_agent_port":  "通信之埠",
		"field_agent_lang":  "本我之言",
		"field_lifetime":    "寿数（秒，默认：86400 = 一日）",
		"field_flow_delay":  "内省之延（秒，默认：120）",
		"field_bash_policy": "令策（Enter = 用默认）",
		"combo_select":      "择旧方或另起炉灶：",
		"combo_create_new":  "另起炉灶",
		"combo_hint":        "↑/↓ 择之  Enter 定之",
		"combo_save_as":     "存为旧方：",
		"combo":              "旧方",
		"status_help":        "[Enter] 对谈   [S] 初设   [L] 言   [K] 尽灭   [Q] 退",
		"mm_quick_setup":     "稀宇速设",
		"mm_quick":           "速设",
		"mm_manual":          "手设",
		"mm_skip":            "略过",
		"mm_endpoint":        "Endpoint (入口)",
		"mm_china":           "中华",
		"mm_international":   "海外",
		"msg_imap":           "邮驿",
		"msg_telegram":       "电报",
		"msg_skip":           "略过",
		"unknown_daemon":     "未知灵体",
		"switched_to":        "已切至",
		"provider_minimax":   "稀宇",
		"provider_openai":    "OpenAI",
		"provider_anthropic": "Anthropic",
		"provider_gemini":    "谷歌 Gemini",
		"provider_custom":    "自设",

		// Banner
		"banner_title": "灵台AI",
		"banner_line1": "灵台方寸山  斜月三星洞",
		"banner_line2": "闻道菩提下  一心化万相",

		// MM chooser descriptions
		"mm_quick_desc":  "输二密钥 — 自设 vision, web search, talk, compose, draw",
		"mm_manual_desc": "逐一设之，供者可任择",
		"mm_skip_desc":   "暂略诸能之设",

		// MM quick setup
		"mm_key_vision_desc": "(vision, web search)",
		"mm_key_mcp_desc":    "(talk, compose, draw)",
		"not_set":            "（未设）",
		"mm_quick_hint":      "Tab: 次项 | ←/→: 择入口 | Enter: 用之并续 | Esc: 退",

		// Messaging chooser descriptions
		"msg_imap_desc":     "通邮驿",
		"msg_telegram_desc": "通电报",
		"msg_skip_desc":     "暂略通信之设",
		"msg_chooser_hint":  "↑/↓ 择之，Enter 设之，Esc 退",

		// Messaging fields hint
		"msg_fields_hint": "Tab/↓: 次项 | Enter: 存之并退 | Esc: 退 | Ctrl+T: 试之",

		// MM grid
		"mm_col_capability": "Capability (能)",
		"mm_col_provider":   "Provider (供者)",
		"mm_col_api_key":    "API Key (密钥)",
		"mm_col_endpoint":   "Endpoint (入口)",
		"no_config_needed":  "无需设",
		"no_key":            "（无钥）",
		"runs_locally":      "本地运行",
		"no_endpoint":       "（无入口）",
		"mm_grid_hint":      "↑/↓: 移行 | Tab: 次列 | ←/→: 择供者 | Enter: 续 | Esc: 退",

		// Review
		"review_model":       "Model:",
		"review_provider":    "Provider:",
		"review_api_key":     "API key:",
		"review_endpoint":    "Endpoint:",
		"review_reusing_key": "复用主钥",
		"review_skipped":     "已略",
		"review_imap":        "IMAP/SMTP:",
		"review_email":       "Email:",
		"review_password":    "Password:",
		"review_imap_server": "IMAP:",
		"review_smtp_server": "SMTP:",
		"review_imap_skipped":     "IMAP/SMTP: 已略",
		"review_telegram":         "Telegram:",
		"review_token":            "Token:",
		"review_telegram_skipped": "Telegram: 已略",
		"review_general":          "通则:",
		"review_agent_name":       "本我之名:",
		"review_port":             "埠:",
		"review_agent_lang":       "本我之言:",
		"review_lifetime":         "寿数:",
		"review_flow_delay":       "内省之延:",
		"review_bash_policy":      "令策:",
		"review_config_path":      "Config →",
		"review_secrets_path":     "Secrets →",
		"review_save_hint":        "Enter → 录之，Ctrl+C → 弃之",

		// Field hints
		"hint_tab_next":   "Tab/↓: 次项",
		"hint_tab_prev":   "Shift+Tab/↑: 前项",
		"hint_enter_next": "Enter: 续",

		// Chat view
		"verbose_on":    "详 ●",
		"active_marker": "← 此",

		// manage/list
		"stop_hint": "灭之: kill <PID>",
	},
}

// tuiConfigPath returns ~/.lingtai/tui.json
func tuiConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lingtai", "tui.json")
}

// LoadTUILang reads the TUI language from ~/.lingtai/tui.json.
// Returns "" if not set.
func LoadTUILang() string {
	data, err := os.ReadFile(tuiConfigPath())
	if err != nil {
		return ""
	}
	var cfg struct {
		Language string `json:"language"`
	}
	json.Unmarshal(data, &cfg)
	return cfg.Language
}

// SaveTUILang writes the TUI language to ~/.lingtai/tui.json.
func SaveTUILang(lang string) {
	path := tuiConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.MarshalIndent(map[string]string{"language": lang}, "", "  ")
	os.WriteFile(path, append(data, '\n'), 0644)
}

func detectLang() string {
	// Try persistent TUI config first
	if saved := LoadTUILang(); saved != "" {
		return saved
	}
	// Fall back to system locale
	lang := os.Getenv("LANG")
	if len(lang) >= 2 && (lang[:2] == "zh") {
		return "zh"
	}
	return "en"
}
