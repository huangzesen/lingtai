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
		"setup_general":   "General Settings",
		"setup_review":    "Review",
		"setup_done":      "Setup Complete",
		"setup_lang_hint": "↑/↓ to select, Enter to confirm",
		"setup_saved":     "Configuration saved successfully!",
		"setup_files":     "Files written:",
		"setup_multimodal": "Multimodal",
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
		"setup_general":   "基本设置",
		"setup_review":    "确认",
		"setup_done":      "设置完成",
		"setup_lang_hint": "↑/↓ 选择，Enter 确认",
		"setup_saved":     "配置保存成功！",
		"setup_files":     "已写入文件：",
		"setup_multimodal": "多模态",
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
		"setup_general":   "通则",
		"setup_review":    "审定",
		"setup_done":      "初设已毕",
		"setup_lang_hint": "↑/↓ 择之，Enter 定之",
		"setup_saved":     "设定已录！",
		"setup_files":     "所录之档：",
		"setup_multimodal": "诸能",
	},
}

func detectLang() string {
	lang := os.Getenv("LANG")
	if len(lang) >= 2 && (lang[:2] == "zh") {
		return "zh"
	}
	return "en"
}
