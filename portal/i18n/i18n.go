package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed en.json zh.json wen.json
var localeFS embed.FS

var (
	mu      sync.RWMutex
	lang    = "en"
	strings map[string]string
)

func init() { load("en") }

func SetLang(l string) {
	mu.Lock()
	defer mu.Unlock()
	lang = l
	load(l)
}

func load(l string) {
	data, err := localeFS.ReadFile(l + ".json")
	if err != nil {
		return
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	strings = m
}

func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if s, ok := strings[key]; ok {
		return s
	}
	return key
}

func TF(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}
