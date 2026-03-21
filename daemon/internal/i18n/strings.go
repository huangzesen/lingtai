package i18n

import "os"

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
		"field_bash_policy": "Bash policy (Enter = use default)",
		"combo_select":      "Select a combo or create new:",
		"combo_create_new":  "Create new",
		"combo_hint":        "↑/↓ navigate  Enter select",
		"combo_save_as":     "Save as combo:",
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
		"field_provider":    "提供商",
		"field_model":       "模型",
		"field_api_key":     "API 密钥",
		"field_endpoint":    "接入点",
		"field_email":       "邮箱地址",
		"field_password":    "密码",
		"field_imap_host":   "IMAP 主机",
		"field_imap_port":   "IMAP 端口",
		"field_smtp_host":   "SMTP 主机",
		"field_smtp_port":   "SMTP 端口",
		"field_bot_token":   "机器人令牌",
		"field_agent_name":  "智能体名称（Enter = 跳过，稍后与智能体商定）",
		"field_agent_port":  "智能体端口",
		"field_bash_policy": "Bash 策略（Enter = 使用默认）",
		"combo_select":      "选择已有组合或新建：",
		"combo_create_new":  "新建",
		"combo_hint":        "↑/↓ 选择  Enter 确认",
		"combo_save_as":     "保存为组合：",
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
		"field_provider":    "供者",
		"field_model":       "模型",
		"field_api_key":     "密钥",
		"field_endpoint":    "入口",
		"field_email":       "邮址",
		"field_password":    "口令",
		"field_imap_host":   "邮驿之主",
		"field_imap_port":   "邮驿之埠",
		"field_smtp_host":   "发驿之主",
		"field_smtp_port":   "发驿之埠",
		"field_bot_token":   "令牌",
		"field_agent_name":  "本我之名（Enter = 略过，入灵台后再议）",
		"field_agent_port":  "通信之埠",
		"field_bash_policy": "令策（Enter = 用默认）",
		"combo_select":      "择旧方或另起炉灶：",
		"combo_create_new":  "另起炉灶",
		"combo_hint":        "↑/↓ 择之  Enter 定之",
		"combo_save_as":     "存为旧方：",
	},
}

func detectLang() string {
	lang := os.Getenv("LANG")
	if len(lang) >= 2 && (lang[:2] == "zh") {
		return "zh"
	}
	return "en"
}
