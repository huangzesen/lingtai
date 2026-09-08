package tui

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthropics/lingtai-tui/i18n"
	"github.com/anthropics/lingtai-tui/internal/preset"
)

func testPresetEditorPreset() preset.Preset {
	return preset.Preset{
		Name: "scroll-test",
		Description: preset.PresetDescription{
			Summary: "A preset used by preset editor tests",
			Tier:    "3",
			Extra: map[string]interface{}{
				"gains": "good at testing",
				"loses": "not real",
			},
		},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider":    "minimax",
				"model":       "MiniMax-M3",
				"api_compat":  "openai",
				"base_url":    "https://api.minimax.io/v1",
				"api_key_env": "MINIMAX_API_KEY",
			},
			"capabilities": map[string]interface{}{
				"file":       map[string]interface{}{},
				"shell":      map[string]interface{}{"yolo": true},
				"avatar":     map[string]interface{}{},
				"daemon":     map[string]interface{}{},
				"web_search": map[string]interface{}{"provider": "duckduckgo"},
				"vision":     map[string]interface{}{"provider": "inherit"},
			},
		},
	}
}

func TestPresetEditorValidationErrorUsesCurrentLocale(t *testing.T) {
	t.Cleanup(func() { _ = i18n.SetLang("en") })
	if err := i18n.SetLang("zh"); err != nil {
		t.Fatalf("SetLang zh: %v", err)
	}

	p := testPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["model"] = ""

	m := NewPresetEditorModelWithBuiltinFlag(p, "zh", nil, "", false)
	updated, cmd := m.commit()
	if cmd != nil {
		t.Fatal("invalid preset commit must not emit a command")
	}
	if got, want := updated.saveErr, "模型不能为空。"; got != want {
		t.Fatalf("saveErr = %q, want %q", got, want)
	}
	if strings.Contains(updated.saveErr, "manifest.llm.model") {
		t.Fatalf("localized saveErr leaked schema text: %q", updated.saveErr)
	}
}

func testCodexPresetEditorPreset(serviceTier interface{}) preset.Preset {
	return testCodexPresetEditorPresetWithThinking(serviceTier, nil)
}

func testCodexPresetEditorPresetWithThinking(serviceTier interface{}, thinking interface{}) preset.Preset {
	llm := map[string]interface{}{
		"provider":    "codex",
		"model":       "gpt-5.6-sol",
		"api_key":     nil,
		"api_key_env": "",
		"base_url":    "https://chatgpt.com/backend-api/codex",
	}
	if serviceTier != nil {
		llm["service_tier"] = serviceTier
	}
	if thinking != nil {
		llm["thinking"] = thinking
	}
	return preset.Preset{
		Name:        "codex-test",
		Description: preset.PresetDescription{Summary: "Codex editor test preset"},
		Manifest: map[string]interface{}{
			"llm": llm,
			"capabilities": map[string]interface{}{
				"web_search": map[string]interface{}{"provider": "codex"},
				"vision":     map[string]interface{}{"provider": "codex"},
			},
		},
	}
}

func builtinPresetForEditorTest(t *testing.T, name string) preset.Preset {
	t.Helper()
	for _, p := range preset.BuiltinPresets() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("built-in preset %q not found", name)
	return preset.Preset{}
}

func TestPresetEditorProviderModelLineupsPinRequestedDefaults(t *testing.T) {
	if got := providerModels["deepseek"][0]; got != "deepseek-v4-pro" {
		t.Fatalf("deepseek default picker model = %q, want deepseek-v4-pro", got)
	}
	// Zhipu carries two spellings of the SAME two generations: uppercase for
	// the native CN/INTL rows, lowercase for OpenCode Go (which rejects the
	// uppercase form). There is no standalone GLM-5 in the catalog, so no
	// `glm-5` alias exists for one — the picker must not invent it.
	wantZhipuModels := []string{"GLM-5.2", "GLM-5.1", "glm-5.2", "glm-5.1"}
	if got := providerModels["zhipu"]; !reflect.DeepEqual(got, wantZhipuModels) {
		t.Fatalf("zhipu provider models = %#v, want %#v", got, wantZhipuModels)
	}
	// Native CN MiniMax generations: M2.7 and M2.5, with their highspeed
	// variants. OpenCode Go keeps the protected pre-PR list below.
	wantMiniMaxModels := []string{"MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed"}
	if got := providerModels["minimax"]; !reflect.DeepEqual(got, wantMiniMaxModels) {
		t.Fatalf("minimax provider models = %#v, want %#v", got, wantMiniMaxModels)
	}
	for _, model := range wantMiniMaxModels {
		if modelHasVision[model] {
			t.Fatalf("MiniMax native text model %s must remain text-only", model)
		}
	}
	// Native MiMo curation keeps V2.5 and its text-only Pro variant. The
	// deprecated V2 entries remain only on the protected OpenCode Go route.
	wantMiMoModels := []string{"mimo-v2.5", "mimo-v2.5-pro"}
	if got := providerModels["mimo"]; !reflect.DeepEqual(got, wantMiMoModels) {
		t.Fatalf("mimo provider models = %#v, want %#v", got, wantMiMoModels)
	}
	if got := providerModels["grok"]; !reflect.DeepEqual(got, []string{"grok-4.5"}) {
		t.Fatalf("grok provider models = %#v, want [grok-4.5]", got)
	}
	// Kimi must NOT gain a provider-global picker: only the exact native
	// Kimi Code route is curated, while OpenCode Go and Custom stay free text
	// so a typed off-list gateway or proxy id remains editable.
	if models := providerModels["kimi"]; models != nil {
		t.Fatalf("kimi must keep no provider-global model row, got %#v", models)
	}
	wantClaudeModels := []string{"opus", "fable", "sonnet", "haiku"}
	if got := providerModels["claude-code"]; !reflect.DeepEqual(got, wantClaudeModels) {
		t.Fatalf("claude-code provider models = %#v, want %#v", got, wantClaudeModels)
	}
	wantNVIDIAModels := []string{
		"nvidia/nemotron-3-ultra-550b-a55b",
		"nvidia/nemotron-3-super-120b-a12b",
		"nvidia/nemotron-3-nano-omni-30b-a3b-reasoning",
		"deepseek-ai/deepseek-v4-pro-0813",
		"deepseek-ai/deepseek-v4-flash-0731",
		"moonshotai/kimi-k3",
		"minimaxai/minimax-m3",
		"mistralai/mistral-nemotron",
		"openai/gpt-oss-20b",
		"nvidia/llama-3.1-nemotron-ultra-253b-v1",
	}
	if got := providerModels["nvidia"]; !reflect.DeepEqual(got, wantNVIDIAModels) {
		t.Fatalf("nvidia provider models = %#v, want %#v", got, wantNVIDIAModels)
	}
	if !modelHasVision["mimo-v2.5"] {
		t.Fatal("MiMo picker must keep native vision for mimo-v2.5")
	}
	for _, model := range []string{"mimo-v2.5-pro"} {
		if modelHasVision[model] {
			t.Fatalf("MiMo %s has no verified image route; it must stay text-only (a name is not evidence)", model)
		}
	}
	for _, model := range providerModels["zhipu"] {
		if modelHasVision[model] {
			t.Fatalf("Zhipu coding-plan model %s must remain text-only; GLM-4.6V uses the optional manual MCP path", model)
		}
	}
	if modelHasVision["grok-4.5"] {
		t.Fatal("grok-4.5 has no verified image-input route on OpenCode Go; it must stay text-only")
	}

	// GPT-6 Astra is documented but account/client rollout is not proven, so
	// Sol remains the default-first entry. The named GPT-5.6 routes are one
	// generation's variants; codex-pool changes account selection only.
	wantCodexModels := []string{
		"gpt-5.6-sol", "gpt-6-astra", "gpt-5.6-terra", "gpt-5.6-luna",
	}
	for _, provider := range []string{"codex", "codex-pool"} {
		models := providerModels[provider]
		if !reflect.DeepEqual(models, wantCodexModels) {
			t.Fatalf("%s provider models = %#v, want %#v", provider, models, wantCodexModels)
		}
	}
	for _, model := range wantCodexModels {
		if !modelHasVision[model] {
			t.Fatalf("%s should be treated as vision-capable like the documented Codex lineup", model)
		}
	}
}

func TestPresetEditorModelCatalogsAreExactByRoute(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		label    string
		want     []string
		freeText bool
	}{
		{
			name:     "minimax native CN",
			provider: "minimax",
			label:    "CN",
			want:     []string{"MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed"},
		},
		{
			name:     "minimax OpenCode Go protected list",
			provider: "minimax",
			label:    "OpenCode Go",
			want:     []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed"},
		},
		{
			name:     "minimax INTL remains free text",
			provider: "minimax",
			label:    "INTL",
			freeText: true,
		},
		{
			name:     "mimo native",
			provider: "mimo",
			label:    "MiMo",
			want:     []string{"mimo-v2.5", "mimo-v2.5-pro"},
		},
		{
			name:     "mimo OpenCode Go protected list",
			provider: "mimo",
			label:    "OpenCode Go",
			want:     []string{"mimo-v2.5", "mimo-v2.5-pro", "mimo-v2-pro", "mimo-v2-omni"},
		},
		{
			name:     "mimo Custom remains free text",
			provider: "mimo",
			label:    "Custom",
			freeText: true,
		},
		{
			name:     "kimi native",
			provider: "kimi",
			label:    "Kimi Code",
			want:     []string{"k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"},
		},
		{
			name:     "kimi OpenCode Go remains free text",
			provider: "kimi",
			label:    "OpenCode Go",
			freeText: true,
		},
		{
			name:     "kimi Custom remains free text",
			provider: "kimi",
			label:    "Custom",
			freeText: true,
		},
		{
			name:     "zhipu protected mixed catalog",
			provider: "zhipu",
			label:    "OpenCode Go",
			want:     []string{"GLM-5.2", "GLM-5.1", "glm-5.2", "glm-5.1"},
		},
		{
			name:     "custom remains free text",
			provider: "custom",
			freeText: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := ""
			if tc.label != "" {
				for _, region := range preset.ProviderRegionURLs[tc.provider] {
					if region.Label == tc.label {
						baseURL = region.URL
						break
					}
				}
			}
			if tc.label != "" && tc.label != "Custom" && baseURL == "" {
				t.Fatalf("%s route %q has no URL", tc.provider, tc.label)
			}
			got := modelOptions(tc.provider, baseURL)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("modelOptions(%q, %q) = %#v, want %#v", tc.provider, baseURL, got, tc.want)
			}

			p := builtinPresetForEditorTest(t, tc.provider)
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
			m.llmMap()["base_url"] = baseURL
			model := "typed-route-model"
			if len(tc.want) > 0 {
				model = tc.want[0]
			}
			m.llmMap()["model"] = model
			m.cursor = editorFieldOrderIndex(t, feModel)
			if gotPicker := m.isCyclable(feModel); gotPicker != !tc.freeText {
				t.Fatalf("isCyclable(feModel) = %v, freeText=%v", gotPicker, tc.freeText)
			}
			if strip := m.modelRadioStrip(false, lipgloss.NewStyle()); (strip == "") != tc.freeText {
				t.Fatalf("modelRadioStrip() empty = %v, freeText=%v; strip=%q", strip == "", tc.freeText, strip)
			}
			opened, _ := m.openInline()
			if tc.freeText {
				if opened.mode != emInline {
					t.Fatalf("free-text route Enter mode = %v, want emInline", opened.mode)
				}
				opened.applyInline("edited-route-model")
				if got := asString(opened.llmMap()["model"]); got != "edited-route-model" {
					t.Fatalf("free-text route model = %q, want edited-route-model", got)
				}
			} else if opened.mode != emBrowse {
				t.Fatalf("picker route Enter mode = %v, want emBrowse", opened.mode)
			} else if got := asString(opened.llmMap()["model"]); got != tc.want[1] {
				t.Fatalf("picker route model after Enter = %q, want %q", got, tc.want[1])
			}
		})
	}
}

func TestPresetEditorRegionChangePreservesModelText(t *testing.T) {
	tests := []struct {
		provider string
		steps    int
	}{
		{provider: "minimax", steps: 2}, // CN -> INTL -> OpenCode Go
		{provider: "mimo", steps: 1},    // native -> OpenCode Go
		{provider: "kimi", steps: 1},    // native -> OpenCode Go
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			regions := preset.ProviderRegionURLs[tc.provider]
			if len(regions) < 2 || regions[0].URL == "" || regions[1].URL == "" {
				t.Fatalf("%s route table lacks two non-empty regions: %#v", tc.provider, regions)
			}
			m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, tc.provider), "en", nil, "", false)
			m.llmMap()["base_url"] = regions[0].URL
			m.llmMap()["model"] = "user-typed-model"
			m.cursor = editorFieldOrderIndex(t, feBaseURL)
			for i := 0; i < tc.steps; i++ {
				m.cycleFocused(+1)
			}
			if got := asString(m.llmMap()["model"]); got != "user-typed-model" {
				t.Fatalf("model after %s region change = %q, want preserved user text", tc.provider, got)
			}
		})
	}
}

func TestPresetEditorRegionChangeReconcilesKnownRouteModels(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		startIndex int
		direction  int
		steps      int
		model      string
		wantModel  string
		wantIndex  int
	}{
		{
			name:       "MiniMax native-only model falls back to protected Go default",
			provider:   "minimax",
			startIndex: 0,
			direction:  +1,
			steps:      2,
			model:      "MiniMax-M2.5",
			wantModel:  "MiniMax-M3",
			wantIndex:  2,
		},
		{
			name:       "MiMo retired Go model falls back to native default",
			provider:   "mimo",
			startIndex: 1,
			direction:  -1,
			steps:      1,
			model:      "mimo-v2-omni",
			wantModel:  "mimo-v2.5",
			wantIndex:  0,
		},
		{
			name:       "Kimi native id is cleared on free-text Go route",
			provider:   "kimi",
			startIndex: 0,
			direction:  +1,
			steps:      1,
			model:      "k3",
			wantModel:  "",
			wantIndex:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			regions := preset.ProviderRegionURLs[tc.provider]
			m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, tc.provider), "en", nil, "", false)
			m.llmMap()["base_url"] = regions[tc.startIndex].URL
			m.llmMap()["model"] = tc.model
			m.cursor = editorFieldOrderIndex(t, feBaseURL)
			for i := 0; i < tc.steps; i++ {
				m.cycleFocused(tc.direction)
			}
			if got := asString(m.llmMap()["base_url"]); got != regions[tc.wantIndex].URL {
				t.Fatalf("base_url = %q, want %q", got, regions[tc.wantIndex].URL)
			}
			if got := asString(m.llmMap()["model"]); got != tc.wantModel {
				t.Fatalf("model = %q, want %q", got, tc.wantModel)
			}
		})
	}
}

// TestModelHasVisionDeclaresEveryShippedModel is the F8 contract for the
// native/default catalog: a shipped model that is absent from modelHasVision
// reads as text-only through Go's zero value, which is an omission rather than
// a declaration. Route-only gateway overrides intentionally have no global
// modality entries. The map must not accumulate entries for models the
// two-generation curation rule has retired.
func TestModelHasVisionDeclaresEveryShippedModel(t *testing.T) {
	shipped := map[string]bool{}
	for provider, models := range providerModels {
		// The Claude Code aliases name CLI tiers, not API model ids; vision
		// there is a property of the resolved model, not the alias.
		if strings.HasPrefix(provider, "claude") {
			continue
		}
		// NVIDIA's bounded route-served snapshot does not assert a curated
		// per-model vision inventory, so this completeness gate does not apply.
		if provider == "nvidia" {
			continue
		}
		for _, m := range models {
			shipped[m] = true
			if _, ok := modelHasVision[m]; !ok {
				t.Errorf("providerModels[%q] ships %q with no modelHasVision entry — declare it false rather than relying on the zero value", provider, m)
			}
		}
	}
	for m := range modelHasVision {
		if !shipped[m] {
			t.Errorf("modelHasVision documents %q, which no provider ships any more — remove it with the id (SKILL.md, \"When you remove a retired model\")", m)
		}
	}
}

// TestPresetEditorVisionProviderIdentityIsCommitImmutable is the
// regression test for the always-included-capabilities change: there is
// no longer any editor control (checkbox, provider cycle, or model
// switch) that can change a capability's provider or other config.
// Committing a built-in preset with a non-default vision provider must
// round-trip that provider (and any credential fields alongside it)
// byte-for-byte, for every built-in that ships one.
func TestPresetEditorVisionProviderIdentityIsCommitImmutable(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		currentKeyEnv string
	}{
		{name: "gemini", current: "gemini", currentKeyEnv: "GEMINI_API_KEY"},
		{name: "codex-pool", current: "codex-pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := builtinPresetForEditorTest(t, tt.name)
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", true)

			_, cmd := m.commit()
			commit := cmd().(PresetEditorCommitMsg)
			caps := commit.Preset.Manifest["capabilities"].(map[string]interface{})
			vision := caps["vision"].(map[string]interface{})
			if got := vision["provider"]; got != tt.current {
				t.Fatalf("commit changed vision provider: got %#v, want %q", got, tt.current)
			}
			if tt.currentKeyEnv != "" {
				if got := vision["api_key_env"]; got != tt.currentKeyEnv {
					t.Fatalf("commit changed vision credential source: got %#v, want %q", got, tt.currentKeyEnv)
				}
			}
		})
	}
}

func TestPresetEditorCodexPoolThinkingIsEditableAndPreserved(t *testing.T) {
	p := builtinPresetForEditorTest(t, "codex-pool")
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", true)

	if !m.fieldVisible(feThinking) {
		t.Fatal("codex-pool thinking field must be visible")
	}
	if !m.isCyclable(feThinking) {
		t.Fatal("codex-pool thinking field must be cyclable")
	}

	_, unchangedCmd := m.commit()
	unchanged := unchangedCmd().(PresetEditorCommitMsg)
	unchangedLLM := unchanged.Preset.Manifest["llm"].(map[string]interface{})
	if got := unchangedLLM["thinking"]; got != "xhigh" {
		t.Fatalf("ordinary codex-pool commit changed thinking: got %#v, want xhigh", got)
	}

	m.setCodexThinking("high")
	_, changedCmd := m.commit()
	changed := changedCmd().(PresetEditorCommitMsg)
	changedLLM := changed.Preset.Manifest["llm"].(map[string]interface{})
	if got := changedLLM["thinking"]; got != "high" {
		t.Fatalf("codex-pool thinking edit was not preserved: got %#v, want high", got)
	}
}

func TestPresetEditorSmallHeightKeepsSaveVisible(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	var cmd tea.Cmd
	m, cmd = m.Update(tea.WindowSizeMsg{Width: 100, Height: 14})
	if cmd != nil {
		t.Fatalf("WindowSizeMsg returned unexpected cmd")
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	view := m.View()

	if !strings.Contains(view, "Save preset") {
		t.Fatalf("small editor view after End should contain save button; view:\n%s", view)
	}
	if got := renderedLineCount(view); got > 14 {
		t.Fatalf("small editor view after End must fit terminal height, got %d lines; view:\n%s", got, view)
	}
	if strings.Contains(view, "scroll-test") && strings.Contains(view, "good at testing") {
		t.Fatalf("expected top identity rows to scroll away when save is focused; view:\n%s", view)
	}
}

func TestPresetEditorTabJumpsToSaveInSmallHeight(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 14})

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	view := m.View()

	if !strings.Contains(view, "Save preset") {
		t.Fatalf("small editor view after Tab should contain save button; view:\n%s", view)
	}
	if got := renderedLineCount(view); got > 14 {
		t.Fatalf("small editor view after Tab must fit terminal height, got %d lines; view:\n%s", got, view)
	}
}

func TestPresetEditorShortTerminalDoesNotWrapRowsPastHeight(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 50, height: 10},
		{width: 60, height: 12},
		{width: 80, height: 14},
		{width: 100, height: 16},
	} {
		m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
		m, _ = m.Update(tea.WindowSizeMsg{Width: size.width, Height: size.height})
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		view := m.View()
		if !strings.Contains(view, "Save preset") {
			t.Fatalf("%dx%d view after End should contain save button; view:\n%s", size.width, size.height, view)
		}
		if got := renderedLineCount(view); got > size.height {
			t.Fatalf("%dx%d view must fit terminal height, got %d lines; view:\n%s", size.width, size.height, got, view)
		}
	}
}

// TestPresetEditorAPIKeyEditableWhenAlreadyStored verifies that opening the
// editor on a preset whose api_key_env slot already holds a value still allows
// an explicit replacement. The existing key is shown masked, Enter opens a
// blank paste target, and commit emits APIKeySet only after the user edits.
func TestPresetEditorAPIKeyEditableWhenAlreadyStored(t *testing.T) {
	keys := map[string]string{"MINIMAX_API_KEY": "sk-existing-value"}
	p := testPresetEditorPreset()
	p.Source = preset.SourceSaved
	m := NewPresetEditorModel(p, "en", keys, "")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if got := m.fieldString(feAPIKey); got == "" || got == "sk-existing-value" {
		t.Fatalf("expected existing key to render masked, got %q", got)
	}

	m.cursor = editorFieldOrderIndex(t, feAPIKey)
	if editorFieldOrder[m.cursor] != feAPIKey {
		t.Fatalf("expected cursor on feAPIKey, got %v", editorFieldOrder[m.cursor])
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.mode != emInline {
		t.Fatalf("expected emInline after Enter on api_key with stored key, got mode=%v", m.mode)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("api_key replacement input should start blank for easy paste, got %q", got)
	}

	m.input.SetValue("sk-replacement-value")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.apiKeySet || m.apiKey != "sk-replacement-value" {
		t.Fatalf("expected replacement key to be staged; apiKeySet=%v apiKey=%q", m.apiKeySet, m.apiKey)
	}
}

func TestPresetEditorAPIKeyUnchangedWhenStoredKeyUntouched(t *testing.T) {
	keys := map[string]string{"MINIMAX_API_KEY": "sk-existing-value"}
	p := testPresetEditorPreset()
	p.Source = preset.SourceSaved
	m := NewPresetEditorModel(p, "en", keys, "")

	_, cmd := m.commit()
	if cmd == nil {
		t.Fatalf("commit returned nil cmd")
	}
	msg := cmd()
	commit, ok := msg.(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg", msg)
	}
	if commit.APIKeySet {
		t.Fatalf("untouched stored API key should not be emitted as a replacement")
	}
}

func TestPresetEditorAPIKeyBlankEditKeepsStoredKey(t *testing.T) {
	keys := map[string]string{"MINIMAX_API_KEY": "sk-existing-value"}
	p := testPresetEditorPreset()
	p.Source = preset.SourceSaved
	m := NewPresetEditorModel(p, "en", keys, "")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.cursor = editorFieldOrderIndex(t, feAPIKey)
	if editorFieldOrder[m.cursor] != feAPIKey {
		t.Fatalf("expected cursor on feAPIKey, got %v", editorFieldOrder[m.cursor])
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mode != emInline {
		t.Fatalf("expected emInline after opening api_key row, got mode=%v", m.mode)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.apiKeySet {
		t.Fatalf("blank API key edit should be a no-op, not stage a clear")
	}

	_, cmd := m.commit()
	if cmd == nil {
		t.Fatalf("commit returned nil cmd")
	}
	commit, ok := cmd().(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned non-commit msg")
	}
	if commit.APIKeySet {
		t.Fatalf("blank API key edit should not emit APIKeySet=true")
	}
}

func TestPresetEditorTemplateDoesNotInheritStoredProviderKey(t *testing.T) {
	keys := map[string]string{"MINIMAX_API_KEY": "sk-existing-value"}
	p := testPresetEditorPreset()
	p.Source = preset.SourceTemplate
	m := NewPresetEditorModel(p, "en", keys, "")

	if m.apiKey != "" {
		t.Fatalf("template editor should not preload old provider key, apiKey=%q", m.apiKey)
	}
	if got := m.fieldString(feAPIKey); got == "sk-existing-value" {
		t.Fatalf("template editor should not render old provider key, got %q", got)
	}

	_, cmd := m.commit()
	if cmd == nil {
		t.Fatalf("commit returned nil cmd")
	}
	commit, ok := cmd().(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned non-commit msg")
	}
	if commit.APIKeySet || commit.APIKey != "" {
		t.Fatalf("untouched template key should not emit old provider key; APIKeySet=%v APIKey=%q", commit.APIKeySet, commit.APIKey)
	}
}

// Every editor result is destined for preset.Save, which always writes under
// presets/saved/. The emitted runtime-only Source must therefore agree with
// that host write even when the committed name matches a built-in template.
func TestPresetEditorCommitPathsIdentifySavedSource(t *testing.T) {
	template := builtinPresetForEditorTest(t, "minimax")
	template.Source = preset.SourceTemplate

	tests := []struct {
		name string
		cmd  func(t *testing.T) tea.Cmd
	}{
		{
			name: "normal save",
			cmd: func(t *testing.T) tea.Cmd {
				m := NewPresetEditorModelWithBuiltinFlag(template, "en", nil, "", false)
				_, cmd := m.commit()
				return cmd
			},
		},
		{
			name: "clone prompt enter",
			cmd: func(t *testing.T) tea.Cmd {
				m := NewPresetEditorModel(template, "en", nil, "")
				m.mode = emClonePrompt
				m.cloneNameInput.SetValue("minimax-copy")
				_, cmd := m.updateClonePrompt(tea.KeyPressMsg{Code: tea.KeyEnter})
				return cmd
			},
		},
		{
			name: "expert built-in overwrite",
			cmd: func(t *testing.T) tea.Cmd {
				m := NewPresetEditorModel(template, "en", nil, "")
				m.mode = emClonePrompt
				_, cmd := m.updateClonePrompt(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
				return cmd
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.cmd(t)
			if cmd == nil {
				t.Fatal("commit returned nil cmd")
			}
			commit, ok := cmd().(PresetEditorCommitMsg)
			if !ok {
				t.Fatalf("commit cmd returned a non-commit message")
			}
			if commit.Preset.Source != preset.SourceSaved {
				t.Fatalf("commit source = %v, want SourceSaved", commit.Preset.Source)
			}
			wantRef := "~/.lingtai-tui/presets/saved/" + commit.Preset.Name + ".json"
			if got := preset.RefFor(commit.Preset); got != wantRef {
				t.Fatalf("commit ref = %q, want %q", got, wantRef)
			}
		})
	}
}

// TestPresetEditorAPIKeyEditableWhenNoStoredKey verifies that a preset
// with no stored key (typical for first-run flow on a fresh template)
// allows inline edit so initial setup works.
func TestPresetEditorAPIKeyEditableWhenNoStoredKey(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	m.cursor = editorFieldOrderIndex(t, feAPIKey)
	if editorFieldOrder[m.cursor] != feAPIKey {
		t.Fatalf("expected cursor on feAPIKey, got %v", editorFieldOrder[m.cursor])
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if m.mode != emInline {
		t.Fatalf("expected emInline after Enter on editable api_key, got mode=%v", m.mode)
	}
}

func TestPresetEditorCanonicalizesLegacyShellForDisplay(t *testing.T) {
	p := testPresetEditorPreset()
	caps := p.Manifest["capabilities"].(map[string]interface{})
	legacy := caps["shell"]
	delete(caps, "shell")
	caps["bash"] = legacy

	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	workingCaps := m.working.Manifest["capabilities"].(map[string]interface{})
	if _, ok := workingCaps["bash"]; ok {
		t.Fatalf("editor retained legacy bash capability: %#v", workingCaps)
	}
	if got := workingCaps["shell"].(map[string]interface{})["yolo"]; got != true {
		t.Fatalf("editor lost legacy shell configuration: %#v", workingCaps["shell"])
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	if !strings.Contains(m.View(), "shell") {
		t.Fatalf("editor view does not display canonical shell capability")
	}
}

// TestPresetEditorNoCapabilityFieldsInEditorFieldOrder is the regression
// test for removing the separate editable-capability concept. The prior
// editorField enum had feCapFile/feCapBash/feCapWebSearch/feCapAvatar/
// feCapDaemon/feCapVision entries; none of them exist anymore, so
// editorFieldOrder's fixed length below is a compile-time-checked proxy
// for "no capability slot was added back in". The Capabilities section
// is rendered entirely from formRows' fixed capabilityRows list, never
// from editorFieldOrder or the cursor.
func TestPresetEditorNoCapabilityFieldsInEditorFieldOrder(t *testing.T) {
	wantOrder := []editorField{
		feName, feSummary, feTier, feGains, feLoses,
		feProvider, feModel, feServiceTier, feThinking, feAPICompat, feWireAPI, feResponsesTransport, feBaseURL, feAPIKey,
		feSave,
	}
	if !reflect.DeepEqual(editorFieldOrder, wantOrder) {
		t.Fatalf("editorFieldOrder = %#v, want %#v (no capability field should ever appear here)", editorFieldOrder, wantOrder)
	}
}

func TestPresetEditorCommitDoesNotInjectLegacyCoreCaps(t *testing.T) {
	p := testPresetEditorPreset()
	p.Manifest["capabilities"] = map[string]interface{}{
		"web_search": map[string]interface{}{"provider": "duckduckgo"},
	}
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	_, cmd := m.commit()
	if cmd == nil {
		t.Fatalf("commit returned nil cmd")
	}
	msg := cmd()
	commit, ok := msg.(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg", msg)
	}
	caps, ok := commit.Preset.Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("committed capabilities missing/wrong type: %T", commit.Preset.Manifest["capabilities"])
	}
	for _, capName := range []string{"library", "skills", "file", "shell", "avatar", "daemon"} {
		if _, ok := caps[capName]; ok {
			t.Fatalf("commit injected core/legacy capability %q: %#v", capName, caps)
		}
	}
	if _, ok := caps["web_search"]; !ok {
		t.Fatalf("commit lost optional web_search capability: %#v", caps)
	}
}

// TestModelSwitchNeverTouchesCapabilities is the regression test for the
// always-included-capabilities change: switching the LLM model — via
// direct field edit (applyInline) or via cycling (cycleFocused), including
// a switch from a vision-capable model to a cataloged text-only one and
// back — must never add, remove, or modify any capability. The prior
// syncCapsToModel behavior (dropping vision on text-only models, resetting
// web_search's provider) is gone: no reachable model-switch path may
// change a capability.
func TestModelSwitchNeverTouchesCapabilities(t *testing.T) {
	skillsPaths := []interface{}{"../.library_shared", "~/.lingtai-tui/utilities"}
	p := testPresetEditorPreset()
	p.Manifest["capabilities"] = map[string]interface{}{
		"web_search": map[string]interface{}{"provider": "zhipu"},
		"vision":     map[string]interface{}{"provider": "inherit"},
		"skills":     map[string]interface{}{"paths": skillsPaths},
		"shell":      map[string]interface{}{"yolo": true},
	}
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	before := deepCopyCaps(t, m.working.Manifest["capabilities"])

	// mimo-v2.5-pro is a cataloged text-only model. Direct field edit
	// (feModel + applyInline) must not touch capabilities even though the
	// model no longer supports vision.
	m.cursor = editorFieldOrderIndex(t, feModel)
	m.applyInline("mimo-v2.5-pro")
	assertCapsUnchanged(t, "applyInline to text-only model", m, before)

	// Cycling the model field (feModel + cycleFocused) must not touch
	// capabilities either, in either direction.
	m.llmMap()["provider"] = "mimo"
	m.llmMap()["base_url"] = preset.ProviderRegionURLs["mimo"][0].URL
	m.llmMap()["model"] = "mimo-v2.5-pro"
	m.cycleFocused(+1) // mimo-v2.5-pro -> mimo-v2.5 (vision-capable)
	assertCapsUnchanged(t, "cycleFocused to vision-capable model", m, before)
	m.cycleFocused(-1) // back to mimo-v2.5-pro
	assertCapsUnchanged(t, "cycleFocused back to text-only model", m, before)

	// Switching provider (which can also reset the model) must not touch
	// capabilities.
	m.cursor = editorFieldOrderIndex(t, feProvider)
	m.cycleFocused(+1)
	assertCapsUnchanged(t, "provider switch", m, before)
}

func deepCopyCaps(t *testing.T, caps interface{}) map[string]interface{} {
	t.Helper()
	m, ok := caps.(map[string]interface{})
	if !ok {
		t.Fatalf("capabilities missing/wrong type: %T", caps)
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func assertCapsUnchanged(t *testing.T, step string, m PresetEditorModel, before map[string]interface{}) {
	t.Helper()
	after, ok := m.working.Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: capabilities missing/wrong type: %T", step, m.working.Manifest["capabilities"])
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("%s: capabilities changed: got %#v, want %#v", step, after, before)
	}
}

func TestPresetEditorCodexServiceTierFastAndNormal(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset(nil), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feServiceTier)

	if !m.fieldVisible(feServiceTier) {
		t.Fatalf("codex service tier row should be visible")
	}
	if got := m.fieldString(feServiceTier); got != "normal" {
		t.Fatalf("empty llm.service_tier displays %q, want normal", got)
	}

	m.cycleFocused(+1)
	llm := m.working.Manifest["llm"].(map[string]interface{})
	if got, _ := llm["service_tier"].(string); got != "fast" {
		t.Fatalf("cycling normal -> fast wrote service_tier=%#v, want fast", llm["service_tier"])
	}
	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if got, _ := committedLLM["service_tier"].(string); got != "fast" {
		t.Fatalf("committed fast service_tier=%#v, want fast", committedLLM["service_tier"])
	}

	m.cycleFocused(+1)
	if _, ok := llm["service_tier"]; ok {
		t.Fatalf("cycling fast -> normal should remove llm.service_tier; got %#v", llm["service_tier"])
	}
	_, cmd = m.commit()
	commit = cmd().(PresetEditorCommitMsg)
	committedLLM = commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["service_tier"]; ok {
		t.Fatalf("committed normal service tier should omit llm.service_tier; got %#v", committedLLM["service_tier"])
	}
}

func TestPresetEditorCodexServiceTierDisplayAndCommitNormalization(t *testing.T) {
	cases := []struct {
		name        string
		serviceTier interface{}
		wantDisplay string
		wantSaved   bool
	}{
		{name: "absent", serviceTier: nil, wantDisplay: "normal"},
		{name: "fast", serviceTier: "fast", wantDisplay: "fast", wantSaved: true},
		{name: "unknown", serviceTier: "flex", wantDisplay: "normal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset(tc.serviceTier), "en", nil, "", false)
			if got := m.fieldString(feServiceTier); got != tc.wantDisplay {
				t.Fatalf("service tier display = %q, want %q", got, tc.wantDisplay)
			}
			_, cmd := m.commit()
			commit := cmd().(PresetEditorCommitMsg)
			llm := commit.Preset.Manifest["llm"].(map[string]interface{})
			got, saved := llm["service_tier"].(string)
			if saved != tc.wantSaved {
				t.Fatalf("committed service_tier saved=%v, want %v; value=%#v", saved, tc.wantSaved, llm["service_tier"])
			}
			if saved && got != "fast" {
				t.Fatalf("committed service_tier=%q, want fast", got)
			}
		})
	}
}

func TestPresetEditorCodexThinkingRowAndOptions(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset(nil), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()

	if !m.fieldVisible(feThinking) {
		t.Fatalf("codex reasoning effort row should be visible")
	}
	if got := m.fieldString(feThinking); got != "xhigh" {
		t.Fatalf("empty llm.thinking displays %q, want xhigh", got)
	}
	if !strings.Contains(view, "Reasoning effort") {
		t.Fatalf("codex editor should render Reasoning effort row; view:\n%s", view)
	}
	for _, effort := range codexThinkingOptions {
		if !strings.Contains(view, effort) {
			t.Fatalf("codex editor should render thinking option %q; view:\n%s", effort, view)
		}
	}
}

func TestPresetEditorCodexThinkingSelectionAndCommit(t *testing.T) {
	cases := []struct {
		effort    string
		wantSaved bool
	}{
		{effort: "low", wantSaved: true},
		{effort: "medium", wantSaved: true},
		{effort: "high", wantSaved: true},
		{effort: "xhigh", wantSaved: true},
	}

	for _, tc := range cases {
		t.Run(tc.effort, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset(nil), "en", nil, "", false)
			m.cursor = editorFieldOrderIndex(t, feThinking)
			m.setCodexThinking(tc.effort)
			if got := m.fieldString(feThinking); got != tc.effort {
				t.Fatalf("thinking display = %q, want %q", got, tc.effort)
			}

			_, cmd := m.commit()
			commit := cmd().(PresetEditorCommitMsg)
			llm := commit.Preset.Manifest["llm"].(map[string]interface{})
			got, saved := llm["thinking"].(string)
			if saved != tc.wantSaved {
				t.Fatalf("committed thinking saved=%v, want %v; value=%#v", saved, tc.wantSaved, llm["thinking"])
			}
			if saved && got != tc.effort {
				t.Fatalf("committed thinking=%q, want %q", got, tc.effort)
			}
		})
	}
}

func TestPresetEditorCodexThinkingDisplayAndCommitNormalization(t *testing.T) {
	cases := []struct {
		name        string
		thinking    interface{}
		wantDisplay string
		wantSaved   bool
		wantValue   string
	}{
		{name: "absent", thinking: nil, wantDisplay: "xhigh", wantSaved: true, wantValue: "xhigh"},
		{name: "explicit high", thinking: "high", wantDisplay: "high", wantSaved: true, wantValue: "high"},
		{name: "low", thinking: "low", wantDisplay: "low", wantSaved: true, wantValue: "low"},
		{name: "explicit xhigh", thinking: "xhigh", wantDisplay: "xhigh", wantSaved: true, wantValue: "xhigh"},
		{name: "unknown", thinking: "turbo", wantDisplay: "xhigh", wantSaved: true, wantValue: "xhigh"},
		{name: "wrong type", thinking: 12, wantDisplay: "xhigh", wantSaved: true, wantValue: "xhigh"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPresetWithThinking(nil, tc.thinking), "en", nil, "", false)
			if got := m.fieldString(feThinking); got != tc.wantDisplay {
				t.Fatalf("thinking display = %q, want %q", got, tc.wantDisplay)
			}
			_, cmd := m.commit()
			commit := cmd().(PresetEditorCommitMsg)
			llm := commit.Preset.Manifest["llm"].(map[string]interface{})
			got, saved := llm["thinking"].(string)
			if saved != tc.wantSaved {
				t.Fatalf("committed thinking saved=%v, want %v; value=%#v", saved, tc.wantSaved, llm["thinking"])
			}
			if saved && got != tc.wantValue {
				t.Fatalf("committed thinking=%q, want %q", got, tc.wantValue)
			}
		})
	}
}

// Providers with a native, non-OpenAI/Anthropic kernel adapter (gemini,
// claude-code) do not accept manifest.llm.thinking at all: the row stays
// hidden and a stale value is stripped on commit.
func TestPresetEditorThinkingHiddenAndRemovedForNonThinkingProvider(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	llm := m.working.Manifest["llm"].(map[string]interface{})
	llm["provider"] = "gemini"
	delete(llm, "api_compat")
	llm["thinking"] = "low"
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()

	if m.fieldVisible(feThinking) {
		t.Fatalf("reasoning effort row should be hidden for a non-thinking provider")
	}
	if m.isCyclable(feThinking) {
		t.Fatalf("reasoning effort row should not be cyclable for a non-thinking provider")
	}
	if strings.Contains(view, "Reasoning effort") || strings.Contains(view, "llm.thinking") {
		t.Fatalf("non-thinking editor should not render thinking row; view:\n%s", view)
	}

	m.cursor = editorFieldOrderIndex(t, feServiceTier)
	m.normalizeCursor()
	if editorFieldOrder[m.cursor] == feThinking {
		t.Fatalf("cursor landed on hidden thinking field for non-thinking preset")
	}

	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["thinking"]; ok {
		t.Fatalf("non-thinking commit should remove llm.thinking; got %#v", committedLLM["thinking"])
	}
}

func TestPresetEditorProviderSwitchClearsThinking(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPresetWithThinking(nil, "low"), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feProvider)

	m.cycleFocused(+1) // codex -> custom in provider picker order.
	llm := m.working.Manifest["llm"].(map[string]interface{})
	if got := llm["provider"]; got != "custom" {
		t.Fatalf("provider after cycling from codex = %#v, want custom", got)
	}
	if _, ok := llm["thinking"]; ok {
		t.Fatalf("provider switch away from codex should remove llm.thinking; got %#v", llm["thinking"])
	}

	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["thinking"]; ok {
		t.Fatalf("non-codex commit after provider switch should omit llm.thinking; got %#v", committedLLM["thinking"])
	}
}

func TestPresetEditorServiceTierAvailableForEveryBuiltin(t *testing.T) {
	wantNames := []string{
		"minimax", "zhipu", "mimo", "deepseek", "gemini", "kimi", "grok",
		"nvidia", "openrouter", "codex", "codex-pool", "claude", "custom",
	}
	gotNames := make([]string, 0, len(preset.BuiltinPresets()))
	for _, p := range preset.BuiltinPresets() {
		gotNames = append(gotNames, p.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("BuiltinPresets() names = %#v, want %#v", gotNames, wantNames)
	}

	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, name), "en", nil, "", false)
			// The custom template intentionally starts with an empty model; give
			// this editor behavior test a valid user value before commit.
			if asString(m.llmMap()["model"]) == "" {
				m.llmMap()["model"] = "test-model"
			}
			m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
			view := m.View()

			if !m.fieldVisible(feServiceTier) {
				t.Fatalf("service tier row should be visible for %s", name)
			}
			if !m.isCyclable(feServiceTier) {
				t.Fatalf("service tier row should be cyclable for %s", name)
			}
			if !strings.Contains(view, i18n.T("preset_editor.field_service_tier")) {
				t.Fatalf("view missing service tier label for %s; view:\n%s", name, view)
			}
			for _, option := range serviceTierOptions {
				if !strings.Contains(view, option) {
					t.Fatalf("view missing service tier option %q for %s; view:\n%s", option, name, view)
				}
			}

			// The cursor must be able to reach the shared row directly after the
			// model row, including free-text and CLI-backed built-ins.
			m.cursor = editorFieldOrderIndex(t, feModel)
			m.moveCursor(+1)
			if editorFieldOrder[m.cursor] != feServiceTier {
				t.Fatalf("cursor after model = %v for %s, want feServiceTier", editorFieldOrder[m.cursor], name)
			}
			if got := m.fieldString(feServiceTier); got != "normal" {
				t.Fatalf("missing service tier displays %q for %s, want normal", got, name)
			}

			m.llmMap()["service_tier"] = "normal"
			_, cmd := m.commit()
			if cmd == nil {
				t.Fatalf("explicit normal commit returned nil command for %s", name)
			}
			committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			if _, ok := committed["service_tier"]; ok {
				t.Fatalf("explicit normal commit should omit service_tier for %s: %#v", name, committed["service_tier"])
			}

			m.cycleFocused(+1)
			if got := m.llmMap()["service_tier"]; got != "fast" {
				t.Fatalf("normal -> fast stored %#v for %s, want string fast", got, name)
			}
			_, cmd = m.commit()
			if cmd == nil {
				t.Fatalf("fast commit returned nil command for %s", name)
			}
			committed = cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			if got := committed["service_tier"]; got != "fast" {
				t.Fatalf("fast commit stored %#v for %s, want string fast", got, name)
			}

			m.cycleFocused(+1)
			if got := m.fieldString(feServiceTier); got != "normal" {
				t.Fatalf("fast -> normal displays %q for %s, want normal", got, name)
			}
			if _, ok := m.llmMap()["service_tier"]; ok {
				t.Fatalf("fast -> normal kept service_tier for %s: %#v", name, m.llmMap()["service_tier"])
			}
			_, cmd = m.commit()
			if cmd == nil {
				t.Fatalf("normal commit returned nil command for %s", name)
			}
			committed = cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			if _, ok := committed["service_tier"]; ok {
				t.Fatalf("normal commit should omit service_tier for %s: %#v", name, committed["service_tier"])
			}

			// Unknown legacy/provider-specific values are displayed as normal. Keep
			// the old non-Codex preservation behavior until the user cycles the row;
			// Codex retains its existing normalization to omission.
			m.llmMap()["service_tier"] = "provider-specific"
			if got := m.fieldString(feServiceTier); got != "normal" {
				t.Fatalf("unknown service tier displays %q for %s, want normal", got, name)
			}
			_, cmd = m.commit()
			if cmd == nil {
				t.Fatalf("unknown service tier commit returned nil command for %s", name)
			}
			committed = cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			_, saved := committed["service_tier"]
			if name == "codex" {
				if saved {
					t.Fatalf("unknown Codex service tier should be omitted: %#v", committed["service_tier"])
				}
			} else if !saved || committed["service_tier"] != "provider-specific" {
				t.Fatalf("unknown non-Codex service tier should be preserved for %s: %#v", name, committed["service_tier"])
			}
		})
	}
}

func TestPresetEditorProviderSwitchPreservesServiceTier(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset("fast"), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feProvider)

	m.cycleFocused(+1) // codex -> custom in provider picker order.
	llm := m.working.Manifest["llm"].(map[string]interface{})
	if got := llm["provider"]; got != "custom" {
		t.Fatalf("provider after cycling from codex = %#v, want custom", got)
	}
	if got, _ := llm["service_tier"].(string); got != "fast" {
		t.Fatalf("provider switch should preserve existing service_tier; got %#v", llm["service_tier"])
	}

	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if got, _ := committedLLM["service_tier"].(string); got != "fast" {
		t.Fatalf("non-codex commit after provider switch should preserve service_tier; got %#v", committedLLM["service_tier"])
	}
}

// TestPresetEditorViewShowsOneCapabilitiesSectionWithAllRows verifies the
// human-facing contract: a single "Capabilities" section (not "Always
// Included") lists every capability the runtime can grant an agent,
// including web_search and vision, alongside the kernel core floor.
func TestPresetEditorViewShowsOneCapabilitiesSectionWithAllRows(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()

	if !strings.Contains(view, "Capabilities") {
		t.Fatalf("view missing renamed \"Capabilities\" section header; view:\n%s", view)
	}
	if strings.Contains(view, "Always Included") {
		t.Fatalf("view still shows the old \"Always Included\" section header; view:\n%s", view)
	}
	for _, capName := range []string{
		"knowledge", "skills", "shell", "avatar", "daemon", "mcp", "file",
		"web_search", "vision",
	} {
		if !strings.Contains(view, capName) {
			t.Fatalf("view missing always-included capability %q; view:\n%s", capName, view)
		}
	}
}

// TestPresetEditorViewShowsCapabilitiesGuidanceLine asserts the one-line
// explanation of how to customize capabilities (ask the agent to explain
// init.json, then edit init.json) is present in the rendered view, and
// that it is actually localized — not just falling back to English —
// in all three shipped locales.
func TestPresetEditorViewShowsCapabilitiesGuidanceLine(t *testing.T) {
	for _, tc := range []struct {
		lang string
		want string
	}{
		{lang: "en", want: "init.json"},
		{lang: "zh", want: "init.json"},
		{lang: "wen", want: "init.json"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			i18n.SetLang(tc.lang)
			t.Cleanup(func() { i18n.SetLang("en") })

			m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), tc.lang, nil, "", false)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
			view := m.View()

			guidance := i18n.T("preset_editor.capabilities_guidance")
			if guidance == "" || guidance == "preset_editor.capabilities_guidance" {
				t.Fatalf("lang %q: capabilities_guidance key missing/untranslated", tc.lang)
			}
			if !strings.Contains(view, tc.want) {
				t.Fatalf("lang %q: view missing capabilities guidance mentioning %q; view:\n%s", tc.lang, tc.want, view)
			}
		})
	}
}

// TestPresetEditorWebSearchAndVisionViewHasNoCheckboxOrProviderNames
// asserts the web_search/vision rows in the live editor view render
// exactly like the other always-included tools (plain "[✓] name  desc"
// via mandatoryCapRow) with no radio strip of provider options — those
// are the fixed/default tool routes, not user choices. Scoped to just
// those two row lines, since provider names like "minimax"/"gemini"
// legitimately appear elsewhere in the view (the LLM provider/model
// rows and their radio strips).
func TestPresetEditorWebSearchAndVisionViewHasNoCheckboxOrProviderNames(t *testing.T) {
	p := testPresetEditorPreset()
	caps := p.Manifest["capabilities"].(map[string]interface{})
	caps["web_search"] = map[string]interface{}{"provider": "zhipu"}
	caps["vision"] = map[string]interface{}{"provider": "gemini"}

	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()

	for _, capName := range []string{"web_search", "vision"} {
		line := findLineContaining(t, view, capName)
		if !strings.Contains(line, "[✓]") {
			t.Fatalf("%s row must render the informational [✓] marker; got: %q", capName, line)
		}
		if strings.Contains(line, "[ ]") {
			t.Fatalf("%s row must not render an unchecked/toggleable checkbox; got: %q", capName, line)
		}
		if strings.Contains(line, "●") || strings.Contains(line, "○") {
			t.Fatalf("%s row must not render a provider radio strip; got: %q", capName, line)
		}
		for _, providerName := range []string{"duckduckgo", "zhipu", "gemini", "inherit"} {
			if strings.Contains(line, providerName) {
				t.Fatalf("%s row must not display provider name %q; got: %q", capName, providerName, line)
			}
		}
	}
}

func findLineContaining(t *testing.T, view, substr string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("view has no line containing %q", substr)
	return ""
}

// TestPresetEditorNoKeypressAtAnyCursorPositionChangesCapabilities is the
// acceptance test for "no reachable checkbox/provider control can remove
// or change a capability on this page": walk every cursor position in the
// form and, at each one, try every input this page recognizes as a
// mutation trigger (Space, Enter, Left, Right). None of it may change
// manifest.capabilities.
func TestPresetEditorNoKeypressAtAnyCursorPositionChangesCapabilities(t *testing.T) {
	p := testPresetEditorPreset()
	before := deepCopyCaps(t, p.Manifest["capabilities"])

	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})

	for i := 0; i < len(editorFieldOrder); i++ {
		m.cursor = i
		for _, key := range []tea.KeyPressMsg{
			{Text: " "},
			{Code: tea.KeyEnter},
			{Code: tea.KeyLeft},
			{Code: tea.KeyRight},
		} {
			trial := m
			trial, _ = trial.Update(key)
			assertCapsUnchanged(t, "keypress at cursor "+lbl(editorFieldOrder[i]), trial, before)
		}
	}
}

func lbl(f editorField) string {
	return fmt.Sprintf("field=%d", int(f))
}

// TestPresetEditorCommitPreservesExistingCapabilityValuesByteForValue
// ensures saving an existing preset with old init.json-style capability
// values (including a non-default web_search provider) round-trips them
// unchanged. The editor's field-list change must not normalize, rewrite,
// or migrate values it no longer exposes as editable UI.
func TestPresetEditorCommitPreservesExistingCapabilityValuesByteForValue(t *testing.T) {
	p := testPresetEditorPreset()
	caps := p.Manifest["capabilities"].(map[string]interface{})
	caps["web_search"] = map[string]interface{}{"provider": "zhipu"}
	caps["vision"] = map[string]interface{}{"provider": "gemini", "api_key_env": "GEMINI_API_KEY"}

	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	_, cmd := m.commit()
	msg := cmd()
	commit, ok := msg.(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg", msg)
	}
	gotCaps, ok := commit.Preset.Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("committed capabilities missing/wrong type: %T", commit.Preset.Manifest["capabilities"])
	}
	if !reflect.DeepEqual(gotCaps["web_search"], caps["web_search"]) {
		t.Fatalf("web_search capability changed on save: got %#v, want %#v", gotCaps["web_search"], caps["web_search"])
	}
	if !reflect.DeepEqual(gotCaps["vision"], caps["vision"]) {
		t.Fatalf("vision capability changed on save: got %#v, want %#v", gotCaps["vision"], caps["vision"])
	}
}

// TestPresetEditorSaveNotBlockedByCapabilityState confirms Save is
// unaffected by web_search/vision capability presence or absence —
// capability state must never block Save/Next. Save only performs local
// structural validation (Preset.Validate); it never blocks on a live
// provider/model check.
func TestPresetEditorSaveNotBlockedByCapabilityState(t *testing.T) {
	p := testPresetEditorPreset()
	caps := p.Manifest["capabilities"].(map[string]interface{})
	delete(caps, "web_search")
	delete(caps, "vision")

	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	_, cmd := m.commit()
	if cmd == nil {
		t.Fatalf("commit returned nil cmd with no capabilities present and a valid model")
	}
	msg := cmd()
	if _, ok := msg.(PresetEditorCommitMsg); !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg (save must not be blocked by missing capabilities)", msg)
	}
}

// TestPresetEditorSaveDoesNotProbeAPIKeyProvider is the regression test
// for the reported (Jason, 2026-07-23) bug where every provider — Codex
// and API-key providers such as DeepSeek alike — was rejected by a
// save-time live-availability check even though the configured provider
// worked. Save must only run local structural validation
// (Preset.Validate) and must never make a live network call: base_url
// points at a closed local port, so any HTTP attempt would fail/hang and
// this test would time out or fail if commit() still probed.
func TestPresetEditorSaveDoesNotProbeAPIKeyProvider(t *testing.T) {
	unreachable := "http://127.0.0.1:1" // reserved port; connection refused instantly, never a real server
	p := preset.Preset{
		Name:        "deepseek-test",
		Description: preset.PresetDescription{Summary: "DeepSeek editor test preset"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider":    "deepseek",
				"model":       "deepseek-v4-pro",
				"api_compat":  "openai",
				"base_url":    unreachable,
				"api_key_env": "DEEPSEEK_API_KEY",
			},
		},
	}
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m.apiKey = "sk-deepseek-test"

	updated, cmd := m.commit()
	if updated.saveErr != "" {
		t.Fatalf("save must not be blocked by a pending/failed availability check; saveErr=%q", updated.saveErr)
	}
	if cmd == nil {
		t.Fatalf("expected commit() to return the commit cmd immediately, not a pending validity-check cmd")
	}
	msg := cmd()
	commit, ok := msg.(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg (save must succeed without a live provider probe)", msg)
	}
	if commit.Preset.Manifest["llm"].(map[string]interface{})["provider"] != "deepseek" {
		t.Fatalf("committed preset lost its provider: %#v", commit.Preset.Manifest["llm"])
	}
}

// TestPresetEditorSaveDoesNotProbeCodexProvider is the Codex half of the
// same regression: Codex presets (OAuth-based, no api_key_env) must also
// reach PresetEditorCommitMsg on the first Save with no pending/checking
// state and no live Responses-endpoint call.
func TestPresetEditorSaveDoesNotProbeCodexProvider(t *testing.T) {
	p := testCodexPresetEditorPreset(nil)
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["base_url"] = "http://127.0.0.1:1" // reserved port; would fail/hang if ever dialed
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", true)

	updated, cmd := m.commit()
	if updated.saveErr != "" {
		t.Fatalf("save must not be blocked by a pending/failed availability check; saveErr=%q", updated.saveErr)
	}
	if cmd == nil {
		t.Fatalf("expected commit() to return the commit cmd immediately, not a pending validity-check cmd")
	}
	msg := cmd()
	commit, ok := msg.(PresetEditorCommitMsg)
	if !ok {
		t.Fatalf("commit cmd returned %T, want PresetEditorCommitMsg (save must succeed without a live Codex probe)", msg)
	}
	if commit.Preset.Manifest["llm"].(map[string]interface{})["provider"] != "codex" {
		t.Fatalf("committed preset lost its provider: %#v", commit.Preset.Manifest["llm"])
	}
}

func editorFieldOrderIndex(t *testing.T, want editorField) int {
	t.Helper()
	for i, got := range editorFieldOrder {
		if got == want {
			return i
		}
	}
	t.Fatalf("field %v missing from editorFieldOrder", want)
	return -1
}

func renderedLineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

// ─────────────────────────────────────────────────────────────────────────────
// wire_api (OpenAI wire-format selector for custom+openai presets)
// ─────────────────────────────────────────────────────────────────────────────

func testCustomOpenAIPresetEditorPreset() preset.Preset {
	return preset.Preset{
		Name:        "custom-openai-test",
		Description: preset.PresetDescription{Summary: "Custom OpenAI-compat test preset"},
		Manifest: map[string]interface{}{
			"llm": map[string]interface{}{
				"provider":    "custom",
				"model":       "gpt-oss-test",
				"api_compat":  "openai",
				"base_url":    "https://api.example.com/v1",
				"api_key_env": "CUSTOM_API_KEY",
			},
			"capabilities": map[string]interface{}{},
		},
	}
}

func TestPresetEditorWireAPIVisibleForCustomOpenAI(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCustomOpenAIPresetEditorPreset(), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()

	if !m.fieldVisible(feWireAPI) {
		t.Fatalf("wire_api row should be visible for custom+openai provider")
	}
	if !m.isCyclable(feWireAPI) {
		t.Fatalf("wire_api should be cyclable for custom+openai provider")
	}
	if !strings.Contains(view, "wire_api") {
		t.Fatalf("custom+openai editor should render wire_api row; view:\n%s", view)
	}
}

func TestPresetEditorWireAPIHiddenOutsideCustomOpenAI(t *testing.T) {
	// Built-in provider (minimax) with api_compat=openai: must NOT surface.
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	view := m.View()
	if m.fieldVisible(feWireAPI) {
		t.Fatalf("wire_api row should be hidden for non-custom provider")
	}
	if strings.Contains(view, "wire_api") {
		t.Fatalf("minimax editor should not render wire_api row; view:\n%s", view)
	}

	// Custom with api_compat=anthropic: must NOT surface.
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["api_compat"] = "anthropic"
	m2 := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	if m2.fieldVisible(feWireAPI) {
		t.Fatalf("wire_api row should be hidden for custom+anthropic")
	}

	// Custom with api_compat unset: must NOT surface.
	p2 := testCustomOpenAIPresetEditorPreset()
	llm2 := p2.Manifest["llm"].(map[string]interface{})
	delete(llm2, "api_compat")
	m3 := NewPresetEditorModelWithBuiltinFlag(p2, "en", nil, "", false)
	if m3.fieldVisible(feWireAPI) {
		t.Fatalf("wire_api row should be hidden for custom with no api_compat")
	}
}

func TestPresetEditorWireAPICursorSkipsHiddenField(t *testing.T) {
	// For a non-custom-openai preset, cursor navigation must skip the
	// hidden feWireAPI and advance from feAPICompat to feBaseURL.
	m := NewPresetEditorModelWithBuiltinFlag(testPresetEditorPreset(), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feAPICompat)
	m.moveCursor(+1)
	if editorFieldOrder[m.cursor] == feWireAPI {
		t.Fatalf("cursor landed on hidden wire_api field for non-custom-openai preset")
	}
	if editorFieldOrder[m.cursor] != feBaseURL {
		t.Fatalf("cursor after api_compat = %v, want feBaseURL", editorFieldOrder[m.cursor])
	}
}

func TestPresetEditorWireAPIDefaultsToAuto(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCustomOpenAIPresetEditorPreset(), "en", nil, "", false)
	if got := m.fieldString(feWireAPI); got != "auto" {
		t.Fatalf("absent wire_api displays %q, want auto", got)
	}
}

func TestPresetEditorWireAPICycling(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCustomOpenAIPresetEditorPreset(), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feWireAPI)

	// auto -> chat_completions
	m.cycleFocused(+1)
	if got := m.fieldString(feWireAPI); got != "chat_completions" {
		t.Fatalf("cycling auto -> +1 = %q, want chat_completions", got)
	}
	llm := m.working.Manifest["llm"].(map[string]interface{})
	if got, _ := llm["wire_api"].(string); got != "chat_completions" {
		t.Fatalf("wire_api should be persisted as chat_completions, got %#v", llm["wire_api"])
	}

	// chat_completions -> responses
	m.cycleFocused(+1)
	if got := m.fieldString(feWireAPI); got != "responses" {
		t.Fatalf("cycling chat_completions -> +1 = %q, want responses", got)
	}

	// responses -> auto (absent)
	m.cycleFocused(+1)
	if got := m.fieldString(feWireAPI); got != "auto" {
		t.Fatalf("cycling responses -> +1 = %q, want auto", got)
	}
	llm = m.working.Manifest["llm"].(map[string]interface{})
	if _, ok := llm["wire_api"]; ok {
		t.Fatalf("cycling to auto should delete wire_api key; got %#v", llm["wire_api"])
	}

	// Reverse: auto -> responses
	m.cycleFocused(-1)
	if got := m.fieldString(feWireAPI); got != "responses" {
		t.Fatalf("cycling auto -> -1 = %q, want responses", got)
	}
}

func TestPresetEditorWireAPICommitPersistsAndOmitsAuto(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCustomOpenAIPresetEditorPreset(), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feWireAPI)

	// Select responses and commit — should persist.
	m.cycleFocused(+1) // auto -> chat_completions
	m.cycleFocused(+1) // chat_completions -> responses
	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if got, _ := committedLLM["wire_api"].(string); got != "responses" {
		t.Fatalf("committed wire_api=%#v, want responses", committedLLM["wire_api"])
	}

	// Cycle back to auto and commit — should omit the key entirely.
	m.cycleFocused(+1) // responses -> auto
	_, cmd = m.commit()
	commit = cmd().(PresetEditorCommitMsg)
	committedLLM = commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["wire_api"]; ok {
		t.Fatalf("committing auto wire_api should omit the key; got %#v", committedLLM["wire_api"])
	}
}

func TestPresetEditorWireAPICleanupOnScopeExit(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	// Switch api_compat away from openai while keeping provider=custom.
	m.cursor = editorFieldOrderIndex(t, feAPICompat)
	m.cycleFocused(+1) // openai -> anthropic
	if _, ok := m.llmMap()["wire_api"]; ok {
		t.Fatalf("leaving openai compat should remove wire_api immediately")
	}
	if m.fieldVisible(feWireAPI) {
		t.Fatalf("wire_api should be hidden after api_compat leaves openai")
	}

	// Commit must strip the stale wire_api.
	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["wire_api"]; ok {
		t.Fatalf("commit after scope exit should remove wire_api; got %#v", committedLLM["wire_api"])
	}
}

func TestPresetEditorWireAPICleanupOnProviderSwitch(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "chat_completions"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	// Switch provider away from custom (custom is last in picker order,
	// so cycling wraps to the first provider, minimax).
	m.cursor = editorFieldOrderIndex(t, feProvider)
	m.cycleFocused(+1)
	if _, ok := m.llmMap()["wire_api"]; ok {
		t.Fatalf("leaving the custom provider should remove wire_api immediately")
	}

	// Commit must strip the stale wire_api.
	_, cmd := m.commit()
	commit := cmd().(PresetEditorCommitMsg)
	committedLLM := commit.Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committedLLM["wire_api"]; ok {
		t.Fatalf("commit after provider switch should remove wire_api; got %#v", committedLLM["wire_api"])
	}
}

func TestPresetEditorWireAPIEnterCyclesAndPreservesLegacyFlags(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["use_responses_api"] = true
	llm["force_responses"] = true
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feWireAPI)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.fieldString(feWireAPI); got != "chat_completions" {
		t.Fatalf("Enter on wire_api = %q, want chat_completions", got)
	}
	for _, key := range []string{"use_responses_api", "force_responses"} {
		if got := m.llmMap()[key]; got != true {
			t.Fatalf("wire selection must preserve legacy %s=true, got %#v", key, got)
		}
	}

	m.cycleFocused(+1) // chat_completions -> responses
	m.cycleFocused(+1) // responses -> auto (canonical key omitted)
	if _, ok := m.llmMap()["wire_api"]; ok {
		t.Fatalf("auto should omit only canonical wire_api")
	}
	for _, key := range []string{"use_responses_api", "force_responses"} {
		if got := m.llmMap()[key]; got != true {
			t.Fatalf("auto must keep legacy delegation flag %s=true, got %#v", key, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// responses_transport (HTTP default / WebSocket v2 opt-in)
// ─────────────────────────────────────────────────────────────────────────────

func TestPresetEditorResponsesTransportVisibleOnlyForCustomResponses(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})

	if !m.fieldVisible(feResponsesTransport) || !m.isCyclable(feResponsesTransport) {
		t.Fatal("Responses transport should be visible and cyclable for custom OpenAI Responses")
	}
	if got := m.fieldString(feResponsesTransport); got != "http" {
		t.Fatalf("absent responses_transport displays %q, want http", got)
	}
	view := m.View()
	if !strings.Contains(view, "Transport") || !strings.Contains(view, "websocket") {
		t.Fatalf("custom Responses editor did not render the transport choices; view:\n%s", view)
	}

	delete(llm, "wire_api")
	m = NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	if m.fieldVisible(feResponsesTransport) || m.isCyclable(feResponsesTransport) {
		t.Fatal("Responses transport must be hidden when wire_api is not responses")
	}
}

func TestPresetEditorResponsesTransportCyclesAndCommits(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feResponsesTransport)

	// Enter uses the same enum path as Right: HTTP (omitted) -> WebSocket.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.fieldString(feResponsesTransport); got != "websocket" {
		t.Fatalf("transport cycle = %q, want websocket", got)
	}
	if got := m.llmMap()["responses_transport"]; got != "websocket" {
		t.Fatalf("responses_transport not persisted: %#v", got)
	}

	_, cmd := m.commit()
	committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
	if got := committed["responses_transport"]; got != "websocket" {
		t.Fatalf("committed responses_transport = %#v, want websocket", got)
	}

	// WebSocket -> HTTP removes the key because HTTP is the default.
	m.cycleFocused(+1)
	if got := m.fieldString(feResponsesTransport); got != "http" {
		t.Fatalf("transport cycle back = %q, want http", got)
	}
	if _, ok := m.llmMap()["responses_transport"]; ok {
		t.Fatal("HTTP transport should be represented by an omitted manifest key")
	}
}

func TestPresetEditorResponsesTransportCleansWhenScopeEnds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field editorField
	}{
		{name: "provider", field: feProvider},
		{name: "api compat", field: feAPICompat},
		{name: "wire api", field: feWireAPI},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testCustomOpenAIPresetEditorPreset()
			llm := p.Manifest["llm"].(map[string]interface{})
			llm["wire_api"] = "responses"
			llm["responses_transport"] = "websocket"
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

			m.cursor = editorFieldOrderIndex(t, tc.field)
			m.cycleFocused(+1)
			if _, ok := m.llmMap()["responses_transport"]; ok {
				t.Fatal("leaving custom Responses scope must remove responses_transport")
			}
			if m.fieldVisible(feResponsesTransport) {
				t.Fatal("transport must be hidden after leaving custom Responses scope")
			}
		})
	}

	// Commit is a second fail-closed boundary for stale or explicit HTTP values.
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	llm["responses_transport"] = "http"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	_, cmd := m.commit()
	committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committed["responses_transport"]; ok {
		t.Fatal("commit must omit explicit HTTP responses_transport")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// custom OpenAI Responses reasoning effort
// ─────────────────────────────────────────────────────────────────────────────

func TestPresetEditorCustomResponsesThinkingShowsDefaultAndAllEfforts(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 80})

	if !m.fieldVisible(feThinking) || !m.isCyclable(feThinking) {
		t.Fatal("thinking should be visible and cyclable for custom OpenAI Responses")
	}
	if got := m.fieldString(feThinking); got != "default" {
		t.Fatalf("absent custom thinking displays %q, want default", got)
	}
	if got := m.thinkingOptions(); !reflect.DeepEqual(got, customResponsesThinkingOptions) {
		t.Fatalf("custom thinking options = %#v, want %#v", got, customResponsesThinkingOptions)
	}
	view := m.View()
	for _, effort := range customResponsesThinkingOptions {
		if !strings.Contains(view, effort) {
			t.Fatalf("custom thinking picker does not render %q; view:\n%s", effort, view)
		}
	}
}

func TestPresetEditorCustomResponsesThinkingCyclesAndOmitsDefault(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feThinking)

	for _, want := range customResponsesThinkingOptions[1:] {
		m.cycleFocused(+1)
		if got := m.fieldString(feThinking); got != want {
			t.Fatalf("custom thinking cycle = %q, want %q", got, want)
		}
		if got := m.llmMap()["thinking"]; got != want {
			t.Fatalf("manifest thinking = %#v, want %q", got, want)
		}
	}

	// xhigh wraps to default, represented by an omitted field.
	m.cycleFocused(+1)
	if got := m.fieldString(feThinking); got != "default" {
		t.Fatalf("custom thinking wrap = %q, want default", got)
	}
	if _, ok := m.llmMap()["thinking"]; ok {
		t.Fatal("custom thinking default must omit the manifest field")
	}

	_, cmd := m.commit()
	committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
	if _, ok := committed["thinking"]; ok {
		t.Fatal("committed custom default must omit manifest.llm.thinking")
	}
}

// Leaving the thinking-capable scope entirely (api_compat cleared, so the
// preset declares no OpenAI/Anthropic wire) strips the value and hides the
// row. Switching wire_api does NOT: manifest.llm.thinking is accepted on
// either OpenAI wire, so the level survives.
func TestPresetEditorThinkingCleansWhenCompatScopeEnds(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	llm["thinking"] = "xhigh"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	m.cursor = editorFieldOrderIndex(t, feAPICompat)
	m.cycleFocused(-1) // openai -> "" (no declared wire compatibility)
	if got := m.llmMap()["api_compat"]; got != "" {
		t.Fatalf("api_compat after cycling = %#v, want empty", got)
	}
	if _, ok := m.llmMap()["thinking"]; ok {
		t.Fatal("leaving the OpenAI/Anthropic compat scope must remove thinking")
	}
	if m.fieldVisible(feThinking) {
		t.Fatal("thinking must be hidden after leaving the compat scope")
	}
}

func TestPresetEditorThinkingSurvivesWireAPIChange(t *testing.T) {
	p := testCustomOpenAIPresetEditorPreset()
	llm := p.Manifest["llm"].(map[string]interface{})
	llm["wire_api"] = "responses"
	llm["thinking"] = "high"
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)

	m.cursor = editorFieldOrderIndex(t, feWireAPI)
	m.cycleFocused(+1) // responses -> auto (wraps)
	if got := m.fieldString(feWireAPI); got != "auto" {
		t.Fatalf("wire_api after cycling = %q, want auto", got)
	}
	if got := m.llmMap()["thinking"]; got != "high" {
		t.Fatalf("thinking after wire_api change = %#v, want high", got)
	}
	if !m.fieldVisible(feThinking) || !m.isCyclable(feThinking) {
		t.Fatal("thinking must stay visible and cyclable on any OpenAI wire")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// reasoning effort for every non-Codex thinking-capable provider
// ─────────────────────────────────────────────────────────────────────────────

// testLevelThinkingPreset builds a minimal preset for a provider that takes
// the canonical THINKING_LEVELS ladder. A nil thinking omits the field.
func testLevelThinkingPreset(provider, apiCompat string, thinking interface{}) preset.Preset {
	llm := map[string]interface{}{
		"provider":    provider,
		"model":       "test-model",
		"base_url":    "https://api.example.com/v1",
		"api_key_env": "TEST_API_KEY",
	}
	if apiCompat != "" {
		llm["api_compat"] = apiCompat
	}
	if thinking != nil {
		llm["thinking"] = thinking
	}
	return preset.Preset{
		Name:        provider + "-thinking-test",
		Description: preset.PresetDescription{Summary: "Thinking-capable editor test preset"},
		Manifest: map[string]interface{}{
			"llm":          llm,
			"capabilities": map[string]interface{}{},
		},
	}
}

func TestPresetEditorThinkingRowVisibleForThinkingCapableProviders(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		compat    string
		wantShown bool
	}{
		{name: "anthropic", provider: "anthropic", compat: "", wantShown: true},
		{name: "custom anthropic compat", provider: "custom", compat: "anthropic", wantShown: true},
		{name: "openai", provider: "openai", compat: "", wantShown: true},
		// No wire_api anywhere in these manifests: the row must not depend on
		// an explicit Responses selection.
		{name: "deepseek openai compat", provider: "deepseek", compat: "openai", wantShown: true},
		{name: "custom openai compat", provider: "custom", compat: "openai", wantShown: true},
		{name: "minimax native", provider: "minimax", compat: "", wantShown: false},
		{name: "gemini", provider: "gemini", compat: "", wantShown: false},
		{name: "claude code", provider: "claude-code", compat: "", wantShown: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testLevelThinkingPreset(tc.provider, tc.compat, nil), "en", nil, "", false)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 80})

			if got := m.fieldVisible(feThinking); got != tc.wantShown {
				t.Fatalf("fieldVisible(feThinking) = %v, want %v", got, tc.wantShown)
			}
			if got := m.isCyclable(feThinking); got != tc.wantShown {
				t.Fatalf("isCyclable(feThinking) = %v, want %v", got, tc.wantShown)
			}
			view := m.View()
			if got := strings.Contains(view, "Reasoning effort"); got != tc.wantShown {
				t.Fatalf("view renders Reasoning effort = %v, want %v; view:\n%s", got, tc.wantShown, view)
			}
			if !tc.wantShown {
				if got := m.thinkingValue(); got != "" {
					t.Fatalf("thinkingValue() = %q, want empty for a non-thinking provider", got)
				}
				if got := m.thinkingOptions(); got != nil {
					t.Fatalf("thinkingOptions() = %#v, want nil", got)
				}
				return
			}
			if got := m.thinkingValue(); got != "default" {
				t.Fatalf("absent thinking displays %q, want default", got)
			}
			if got := m.thinkingOptions(); !reflect.DeepEqual(got, customResponsesThinkingOptions) {
				t.Fatalf("thinking options = %#v, want %#v", got, customResponsesThinkingOptions)
			}
			for _, effort := range customResponsesThinkingOptions {
				if !strings.Contains(view, effort) {
					t.Fatalf("thinking picker does not render %q; view:\n%s", effort, view)
				}
			}
		})
	}
}

func TestPresetEditorLevelThinkingCyclesAndCommits(t *testing.T) {
	for _, provider := range []struct{ name, compat string }{
		{name: "anthropic", compat: ""},
		{name: "deepseek", compat: "openai"},
	} {
		t.Run(provider.name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testLevelThinkingPreset(provider.name, provider.compat, nil), "en", nil, "", false)
			m.cursor = editorFieldOrderIndex(t, feThinking)

			for _, want := range customResponsesThinkingOptions[1:] {
				m.cycleFocused(+1)
				if got := m.fieldString(feThinking); got != want {
					t.Fatalf("thinking cycle = %q, want %q", got, want)
				}
				if got := m.llmMap()["thinking"]; got != want {
					t.Fatalf("manifest thinking = %#v, want %q", got, want)
				}
			}

			// xhigh wraps back to default, represented by an omitted field.
			m.cycleFocused(+1)
			if got := m.fieldString(feThinking); got != "default" {
				t.Fatalf("thinking wrap = %q, want default", got)
			}
			if _, ok := m.llmMap()["thinking"]; ok {
				t.Fatal("default must omit manifest.llm.thinking")
			}

			_, cmd := m.commit()
			committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			if _, ok := committed["thinking"]; ok {
				t.Fatal("committed default must omit manifest.llm.thinking")
			}
		})
	}
}

func TestPresetEditorLevelThinkingSetPersistsThroughCommit(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		compat    string
		set       string
		wantSaved bool
		wantValue string
	}{
		{name: "anthropic high", provider: "anthropic", set: "high", wantSaved: true, wantValue: "high"},
		{name: "anthropic none", provider: "anthropic", set: "none", wantSaved: true, wantValue: "none"},
		{name: "anthropic default", provider: "anthropic", set: "default"},
		{name: "anthropic invalid", provider: "anthropic", set: "turbo"},
		{name: "openai compat xhigh", provider: "deepseek", compat: "openai", set: "xhigh", wantSaved: true, wantValue: "xhigh"},
		{name: "openai compat default", provider: "deepseek", compat: "openai", set: "default"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewPresetEditorModelWithBuiltinFlag(testLevelThinkingPreset(tc.provider, tc.compat, "medium"), "en", nil, "", false)
			m.setThinking(tc.set)

			wantDisplay := tc.wantValue
			if !tc.wantSaved {
				wantDisplay = "default"
			}
			if got := m.fieldString(feThinking); got != wantDisplay {
				t.Fatalf("thinking display = %q, want %q", got, wantDisplay)
			}

			_, cmd := m.commit()
			committed := cmd().(PresetEditorCommitMsg).Preset.Manifest["llm"].(map[string]interface{})
			got, saved := committed["thinking"].(string)
			if saved != tc.wantSaved {
				t.Fatalf("committed thinking saved=%v, want %v; value=%#v", saved, tc.wantSaved, committed["thinking"])
			}
			if saved && got != tc.wantValue {
				t.Fatalf("committed thinking = %q, want %q", got, tc.wantValue)
			}
		})
	}
}

// An invalid stored value is normalized away for level providers (omitted ==
// the kernel default) while Codex keeps its explicit xhigh default.
func TestNormalizeThinkingByProviderScope(t *testing.T) {
	cases := []struct {
		name      string
		llm       map[string]interface{}
		wantSaved bool
		wantValue string
	}{
		{
			name:      "anthropic invalid dropped",
			llm:       map[string]interface{}{"provider": "anthropic", "thinking": "turbo"},
			wantSaved: false,
		},
		{
			name:      "anthropic wrong type dropped",
			llm:       map[string]interface{}{"provider": "anthropic", "thinking": 12},
			wantSaved: false,
		},
		{
			name:      "anthropic valid kept",
			llm:       map[string]interface{}{"provider": "anthropic", "thinking": "medium"},
			wantSaved: true,
			wantValue: "medium",
		},
		{
			name:      "anthropic absent stays absent",
			llm:       map[string]interface{}{"provider": "anthropic"},
			wantSaved: false,
		},
		{
			name:      "openai compat invalid dropped",
			llm:       map[string]interface{}{"provider": "deepseek", "api_compat": "openai", "thinking": "turbo"},
			wantSaved: false,
		},
		{
			name:      "openai compat valid kept without wire_api",
			llm:       map[string]interface{}{"provider": "deepseek", "api_compat": "openai", "thinking": "minimal"},
			wantSaved: true,
			wantValue: "minimal",
		},
		{
			name:      "out of scope dropped",
			llm:       map[string]interface{}{"provider": "gemini", "thinking": "high"},
			wantSaved: false,
		},
		{
			name:      "codex invalid falls back to xhigh",
			llm:       map[string]interface{}{"provider": "codex", "thinking": "turbo"},
			wantSaved: true,
			wantValue: "xhigh",
		},
		{
			name:      "codex absent gains xhigh",
			llm:       map[string]interface{}{"provider": "codex"},
			wantSaved: true,
			wantValue: "xhigh",
		},
		{
			name:      "codex valid kept",
			llm:       map[string]interface{}{"provider": "codex-pool", "thinking": "low"},
			wantSaved: true,
			wantValue: "low",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := map[string]interface{}{"llm": tc.llm}
			normalizeThinking(manifest)
			got, saved := tc.llm["thinking"].(string)
			if saved != tc.wantSaved {
				t.Fatalf("normalized thinking saved=%v, want %v; value=%#v", saved, tc.wantSaved, tc.llm["thinking"])
			}
			if saved && got != tc.wantValue {
				t.Fatalf("normalized thinking = %q, want %q", got, tc.wantValue)
			}
		})
	}
}

func TestPresetEditorCodexThinkingOptionsRemainUnchanged(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(testCodexPresetEditorPreset(nil), "en", nil, "", false)

	if got := m.thinkingValue(); got != "xhigh" {
		t.Fatalf("Codex default thinking = %q, want xhigh", got)
	}
	if got := m.thinkingOptions(); !reflect.DeepEqual(got, codexThinkingOptions) {
		t.Fatalf("Codex thinking options = %#v, want %#v", got, codexThinkingOptions)
	}
}

func TestPresetEditorCodexSingleAPIKeyDisplayKeepsBoundAccount(t *testing.T) {
	globalDir := t.TempDir()
	writeStubCodexToken(t, legacyCodexAuthPath(globalDir), "bound@example.test")
	m := NewPresetEditorModel(testCodexPresetEditorPreset(nil), "en", nil, globalDir)
	if got := m.fieldString(feAPIKey); got != "✓ bound@example.test" {
		t.Fatalf("CodexSingle API-key row = %q, want bound-account display", got)
	}
}

func TestPresetEditorCredentialFamilyGates(t *testing.T) {
	tests := []struct {
		provider       string
		account        bool
		thinking       bool
		serviceTierKey bool
	}{
		{"codex", true, true, true},
		{"codex_oauth", true, true, true},
		{"codex-pool", false, true, true},
		{"codex_pool", false, true, true},
		{"claude-code", false, false, true},
		{"claude_code", false, false, true},
		{"claude-agent-sdk", false, false, true},
		{"claude_agent_sdk", false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			p := testCodexPresetEditorPreset(nil)
			llm := p.Manifest["llm"].(map[string]interface{})
			llm["provider"] = tc.provider
			m := NewPresetEditorModel(p, "en", nil, t.TempDir())
			if got := m.isCodexProvider(); got != tc.account {
				t.Fatalf("isCodexProvider() = %v, want %v", got, tc.account)
			}
			if got := m.hasCodexThinking(); got != tc.thinking {
				t.Fatalf("hasCodexThinking() = %v, want %v", got, tc.thinking)
			}
			m.setServiceTier("fast")
			workingLLM := m.llmMap()
			_, hasTier := workingLLM["service_tier"]
			if hasTier != tc.serviceTierKey {
				t.Fatalf("service_tier presence = %v, want %v", hasTier, tc.serviceTierKey)
			}
			m.setCodexAuthRef("codex-auth/account.json")
			_, hasRef := workingLLM["codex_auth_path"]
			if hasRef != tc.account {
				t.Fatalf("codex_auth_path presence = %v, want %v", hasRef, tc.account)
			}
		})
	}
}

func TestPresetEditorCredentialFamilyAPIKeyRowsAreReadOnly(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		readOnly   bool
		messageKey string
	}{
		{name: "codex", provider: "codex", readOnly: true, messageKey: "preset_editor.api_key_codex_readonly"},
		{name: "codex oauth alias", provider: "codex_oauth", readOnly: true, messageKey: "preset_editor.api_key_codex_readonly"},
		{name: "codex pool", provider: "codex-pool", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "codex pool alias", provider: "codex_pool", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "claude cli", provider: "claude-code", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "claude cli alias", provider: "claude_code", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "claude agent sdk", provider: "claude-agent-sdk", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "claude agent sdk alias", provider: "claude_agent_sdk", readOnly: true, messageKey: "preset_editor.api_key_managed_externally"},
		{name: "other", provider: "minimax", readOnly: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testPresetEditorPreset()
			llm := p.Manifest["llm"].(map[string]interface{})
			llm["provider"] = tt.provider
			llm["api_key_env"] = "TEST_API_KEY"
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", map[string]string{"TEST_API_KEY": "existing"}, "", false)
			if tt.readOnly {
				wantDisplay := i18n.T(tt.messageKey)
				if tt.provider == "codex" || tt.provider == "codex_oauth" {
					wantDisplay = i18n.T("codex.oauth_not_logged_in")
				}
				if got := m.fieldString(feAPIKey); got != wantDisplay {
					t.Fatalf("API-key display = %q, want managed/read-only label %q", got, wantDisplay)
				}
			} else if got := m.fieldString(feAPIKey); got == "" || got == "existing" {
				t.Fatalf("Other API-key display = %q, want a masked existing key", got)
			}
			m.cursor = int(feAPIKey)
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if tt.readOnly {
				if m.mode != emBrowse {
					t.Fatalf("mode after Enter = %v, want browse", m.mode)
				}
				if m.apiKeySet {
					t.Fatal("read-only credential row set apiKeySet")
				}
				if got := m.saveErr; got != i18n.T(tt.messageKey) {
					t.Fatalf("hint = %q, want %q", got, i18n.T(tt.messageKey))
				}
				return
			}
			if m.mode != emInline {
				t.Fatalf("Other API-key row mode after Enter = %v, want inline", m.mode)
			}
		})
	}
}

// TestPresetEditorDeepseekBaseURLCyclingAndCustomSentinel covers the
// deepseek preset's three base_url options: cycling DeepSeek API → OpenCode
// adopts the OpenCode credential env-var, cycling to Custom clears base_url
// (the free-text sentinel), and Enter on Custom/unknown URLs opens inline
// edit while Enter on a known region URL stays a no-op.
func TestPresetEditorDeepseekBaseURLCyclingAndCustomSentinel(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "deepseek"), "en", nil, "", false)
	llm := m.llmMap()
	if got, _ := llm["provider"].(string); got != "deepseek" {
		t.Fatalf("test preset provider = %q, want deepseek", got)
	}
	m.cursor = editorFieldOrderIndex(t, feBaseURL)

	// DeepSeek API -> OpenCode: base_url switches and api_key_env follows.
	m.cycleFocused(+1)
	if got, _ := llm["base_url"].(string); got != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("after first cycle base_url = %q, want OpenCode endpoint", llm["base_url"])
	}
	if got, _ := llm["api_key_env"].(string); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("after first cycle api_key_env = %q, want OPENCODE_GO_API_KEY", llm["api_key_env"])
	}

	// OpenCode -> Custom: base_url clears (free-text sentinel); api_key_env
	// is left untouched (the Custom option declares no env).
	m.cycleFocused(+1)
	if got, _ := llm["base_url"].(string); got != "" {
		t.Fatalf("after second cycle base_url = %q, want empty (Custom sentinel)", llm["base_url"])
	}
	if got, _ := llm["api_key_env"].(string); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("after second cycle api_key_env = %q, want untouched OPENCODE_GO_API_KEY", llm["api_key_env"])
	}

	// Cycling back to the first region restores DeepSeek API + its env.
	m.cycleFocused(-1)
	m.cycleFocused(-1)
	if got, _ := llm["base_url"].(string); got != "https://api.deepseek.com" {
		t.Fatalf("cycling back base_url = %q, want DeepSeek API endpoint", llm["base_url"])
	}
	if got, _ := llm["api_key_env"].(string); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("cycling back api_key_env = %q, want DEEPSEEK_API_KEY", llm["api_key_env"])
	}

	// Enter on a known region URL stays a no-op (←/→ cycle).
	m2, _ := m.openInline()
	if m2.mode == emInline {
		t.Fatal("Enter on a known region URL must not open inline edit")
	}

	// Enter on the Custom (empty) URL opens free-text inline edit prefilled
	// with the current value.
	m.cursor = editorFieldOrderIndex(t, feBaseURL)
	m.cycleFocused(+1) // DeepSeek API -> OpenCode
	m.cycleFocused(+1) // OpenCode -> Custom (empty)
	m3, _ := m.openInline()
	if m3.mode != emInline {
		t.Fatalf("Enter on Custom (empty base_url) mode = %v, want inline", m3.mode)
	}

	// Enter on an unknown (free-typed) URL also opens inline edit.
	m4 := m
	m4.llmMap()["base_url"] = "https://myhost.example/v1"
	m5, _ := m4.openInline()
	if m5.mode != emInline {
		t.Fatalf("Enter on unknown base_url mode = %v, want inline", m5.mode)
	}
}

// TestPresetEditorDeepseekTypedURLCycling covers the free-typed Custom URL
// path: an off-list endpoint resolves to the Custom row, so cycling away
// moves to the adjacent real option (never the wrong slot), and Enter still
// opens inline edit.
func TestPresetEditorDeepseekTypedURLCycling(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "deepseek"), "en", nil, "", false)
	llm := m.llmMap()
	m.cursor = editorFieldOrderIndex(t, feBaseURL)

	// Simulate the user typing a custom endpoint on the Custom row.
	llm["base_url"] = "https://myhost.example/v1"

	// +1 from Custom lands on DeepSeek API (index 0), not OpenCode (1).
	m.cycleFocused(+1)
	if got, _ := llm["base_url"].(string); got != "https://api.deepseek.com" {
		t.Fatalf("cycle +1 from typed URL base_url = %q, want DeepSeek API endpoint", llm["base_url"])
	}
	if got, _ := llm["api_key_env"].(string); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("cycle +1 from typed URL api_key_env = %q, want DEEPSEEK_API_KEY", llm["api_key_env"])
	}

	// -1 from a fresh typed URL lands on OpenCode Go (index 1).
	llm["base_url"] = "https://myhost.example/v1"
	m.cycleFocused(-1)
	if got, _ := llm["base_url"].(string); got != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("cycle -1 from typed URL base_url = %q, want OpenCode endpoint", llm["base_url"])
	}
	if got, _ := llm["api_key_env"].(string); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("cycle -1 from typed URL api_key_env = %q, want OPENCODE_GO_API_KEY", llm["api_key_env"])
	}

	// Enter on the typed URL still opens inline edit.
	llm["base_url"] = "https://myhost.example/v1"
	m2, _ := m.openInline()
	if m2.mode != emInline {
		t.Fatalf("Enter on typed base_url mode = %v, want inline", m2.mode)
	}
}

// TestPresetEditorDeepseekBaseURLStripRendering locks the radio strip's
// three-state selection and the trailing typed-URL echo.
func TestPresetEditorDeepseekBaseURLStripRendering(t *testing.T) {
	newModel := func() PresetEditorModel {
		return NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "deepseek"), "en", nil, "", false)
	}
	style := lipgloss.NewStyle()

	m := newModel()
	if strip := m.baseURLRadioStrip(false, style); !strings.Contains(strip, "● DeepSeek API") || strings.Contains(strip, "● OpenCode Go") || strings.Contains(strip, "● Custom") {
		t.Fatalf("DeepSeek API strip = %q, want only DeepSeek API selected", strip)
	}
	m.llmMap()["base_url"] = "https://opencode.ai/zen/go/v1"
	if strip := m.baseURLRadioStrip(false, style); !strings.Contains(strip, "● OpenCode Go") || strings.Contains(strip, "● DeepSeek API") || strings.Contains(strip, "● Custom") {
		t.Fatalf("OpenCode strip = %q, want only OpenCode selected", strip)
	}
	m.llmMap()["base_url"] = ""
	if strip := m.baseURLRadioStrip(false, style); !strings.Contains(strip, "● Custom") || strings.Contains(strip, "● DeepSeek API") || strings.Contains(strip, "● OpenCode Go") {
		t.Fatalf("empty Custom strip = %q, want only Custom selected", strip)
	}
	m.llmMap()["base_url"] = "https://myhost.example/v1"
	if strip := m.baseURLRadioStrip(false, style); !strings.Contains(strip, "● Custom") || !strings.Contains(strip, "https://myhost.example/v1") {
		t.Fatalf("typed Custom strip = %q, want Custom selected with endpoint echoed", strip)
	}
}

// TestPresetEditorNoCustomRowOffListBaseURLStrip covers the providers that have
// no Custom row (zhipu, minimax): an off-list base_url must select nothing and
// echo the raw value, never claim a region the preset does not point at.
func TestPresetEditorNoCustomRowOffListBaseURLStrip(t *testing.T) {
	style := lipgloss.NewStyle()
	for _, provider := range []string{"zhipu", "minimax"} {
		m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, provider), "en", nil, "", false)
		m.llmMap()["base_url"] = "https://my-proxy.example/v4"
		strip := m.baseURLRadioStrip(false, style)
		if strings.Contains(strip, "● ") {
			t.Errorf("%s off-list strip = %q, want no filled dot", provider, strip)
		}
		if !strings.Contains(strip, "○ CN") || !strings.Contains(strip, "○ INTL") {
			t.Errorf("%s off-list strip = %q, want both regions hollow", provider, strip)
		}
		if !strings.Contains(strip, "https://my-proxy.example/v4") {
			t.Errorf("%s off-list strip = %q, want the raw endpoint echoed", provider, strip)
		}

		// The known region URLs still select normally.
		m.llmMap()["base_url"] = preset.ProviderRegionURLs[provider][1].URL
		if strip := m.baseURLRadioStrip(false, style); !strings.Contains(strip, "● INTL") {
			t.Errorf("%s INTL strip = %q, want INTL selected", provider, strip)
		}
	}
}

// TestPresetEditorNoCustomRowOffListCyclingAndEnter checks the other two
// selectedRegionIndex consumers agree with the strip when nothing is selected:
// cycling enters the list at its edge (never indexing -1), and Enter opens
// inline edit so the value is correctable from the editor.
func TestPresetEditorNoCustomRowOffListCyclingAndEnter(t *testing.T) {
	regions := preset.ProviderRegionURLs["zhipu"]

	m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "zhipu"), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feBaseURL)
	m.llmMap()["base_url"] = "https://my-proxy.example/v4"
	m.cycleFocused(+1)
	if got := m.fieldString(feBaseURL); got != regions[0].URL {
		t.Errorf("cycle +1 from off-list = %q, want first region %q", got, regions[0].URL)
	}

	m.llmMap()["base_url"] = "https://my-proxy.example/v4"
	m.cycleFocused(-1)
	if got := m.fieldString(feBaseURL); got != regions[len(regions)-1].URL {
		t.Errorf("cycle -1 from off-list = %q, want last region %q", got, regions[len(regions)-1].URL)
	}

	m.llmMap()["base_url"] = "https://my-proxy.example/v4"
	opened, _ := m.openInline()
	if opened.mode != emInline {
		t.Errorf("Enter on off-list base_url mode = %v, want emInline so the value is editable", opened.mode)
	}
}

// TestPresetEditorProviderSwitchResetsAdoptedAPIKeyEnv is the F3 regression:
// a credential slot adopted from a base_url region must not follow the user
// into the next provider.
func TestPresetEditorProviderSwitchResetsAdoptedAPIKeyEnv(t *testing.T) {
	m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "deepseek"), "en", nil, "", false)
	m.cursor = editorFieldOrderIndex(t, feBaseURL)
	m.cycleFocused(+1) // DeepSeek API -> OpenCode Go
	if got := asString(m.llmMap()["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("after base_url cycle api_key_env = %q, want OPENCODE_GO_API_KEY", got)
	}

	// Switch the provider until it lands on each region-table provider and
	// assert the stale OpenCode slot is gone every time.
	//
	// Coverage here is bounded by cycleFocused's `opts` list, NOT by
	// ProviderRegionURLs: kimi is a region-table provider but is not on the
	// provider cycle (it is reached only by opening its own template), so it
	// is deliberately never seen by this loop. See the feProvider comment in
	// preset_editor.go.
	m.cursor = editorFieldOrderIndex(t, feProvider)
	for i := 0; i < 24; i++ {
		m.cycleFocused(+1)
		provider := asString(m.llmMap()["provider"])
		regions, ok := preset.ProviderRegionURLs[provider]
		if !ok {
			continue
		}
		// The slot must match the row the switch lands on: the landing row's
		// own declared Env wins over ProviderDefaultEnv when it has one. Only
		// grok differs today — its regions[0] is OpenCode Go, so a switch to
		// grok must adopt OPENCODE_GO_API_KEY, not GROK_API_KEY, or the user
		// would be pointed at the Go endpoint holding a slot it does not use.
		want := preset.ProviderDefaultEnv[provider]
		if len(regions) > 0 && regions[0].Env != "" {
			want = regions[0].Env
		}
		if got := asString(m.llmMap()["api_key_env"]); got != want {
			t.Fatalf("after switch to %s api_key_env = %q, want %q (the slot declared by the landing base_url row, else the provider default)", provider, got, want)
		}
		if got := asString(m.llmMap()["base_url"]); got != regions[0].URL {
			t.Fatalf("after switch to %s base_url = %q, want %q", provider, got, regions[0].URL)
		}
	}
}

// TestPresetEditorRegionCyclePreservesRegionSuffixedAPIKeyEnv pins the F3a
// contract: cycling zhipu/minimax between CN and INTL must NOT rewrite
// api_key_env. The host stamps region-suffixed slots (ZHIPU_INTL_1_API_KEY,
// MINIMAX_CN_1_API_KEY) for saved presets, and those are separate accounts;
// the base_url cycle only adopts an Env when the selected option declares one
// (the OpenCode Go row, which is a genuinely distinct credential), never for
// a plain CN<->INTL region toggle.
func TestPresetEditorRegionCyclePreservesRegionSuffixedAPIKeyEnv(t *testing.T) {
	for _, provider := range []string{"zhipu", "minimax"} {
		regions := preset.ProviderRegionURLs[provider]
		if len(regions) < 3 {
			t.Fatalf("%s needs at least CN/INTL/OpenCode Go for the cycle test", provider)
		}
		slot := strings.ToUpper(provider) + "_INTL_1_API_KEY"
		p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
			"llm": map[string]interface{}{"provider": provider, "model": "m",
				"base_url":    regions[1].URL, // INTL
				"api_key_env": slot}}}
		m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
		llm := m.llmMap()
		m.cursor = editorFieldOrderIndex(t, feBaseURL)

		// INTL -> CN (cycle backwards): slot must survive untouched.
		m.cycleFocused(-1)
		if got := asString(llm["base_url"]); got != regions[0].URL {
			t.Fatalf("[%s] INTL->CN base_url = %q, want %q", provider, got, regions[0].URL)
		}
		if got := asString(llm["api_key_env"]); got != slot {
			t.Fatalf("[%s] INTL->CN api_key_env = %q, want preserved %q", provider, got, slot)
		}

		// CN -> INTL (cycle forward): still preserved.
		m.cycleFocused(+1)
		if got := asString(llm["base_url"]); got != regions[1].URL {
			t.Fatalf("[%s] CN->INTL base_url = %q, want %q", provider, got, regions[1].URL)
		}
		if got := asString(llm["api_key_env"]); got != slot {
			t.Fatalf("[%s] CN->INTL api_key_env = %q, want preserved %q", provider, got, slot)
		}

		// INTL -> OpenCode Go: adopts the OpenCode credential (distinct account).
		m.cycleFocused(+1)
		if got := asString(llm["base_url"]); got != regions[2].URL {
			t.Fatalf("[%s] INTL->OpenCodeGo base_url = %q, want %q", provider, got, regions[2].URL)
		}
		if got := asString(llm["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
			t.Fatalf("[%s] INTL->OpenCodeGo api_key_env = %q, want OPENCODE_GO_API_KEY", provider, got)
		}

		// OpenCode Go -> INTL (back the way we came): the adopted slot must
		// be handed back, not left stuck on the plain region row.
		m.cycleFocused(-1)
		if got := asString(llm["base_url"]); got != regions[1].URL {
			t.Fatalf("[%s] OpenCodeGo->INTL base_url = %q, want %q", provider, got, regions[1].URL)
		}
		if got := asString(llm["api_key_env"]); got != slot {
			t.Fatalf("[%s] OpenCodeGo->INTL api_key_env = %q, want restored %q", provider, got, slot)
		}
	}
}

// TestPresetEditorRegionCycleRoundTripRestoresAdoptedSlot is the F2 regression
// (fable r4): the OpenCode Go row's Env adoption must be a two-way door.
// zhipu/minimax CN and INTL declare no Env of their own, so before this fix
// one extra → press past OpenCode Go wrapped around to CN and left the preset
// pointing at bigmodel.cn while still resolving through OPENCODE_GO_API_KEY —
// silently destroying the user's ZHIPU_INTL_1_API_KEY with no way back.
// Walks the FULL round trip, wrap included: CN → INTL → OpenCode Go → CN.
func TestPresetEditorRegionCycleRoundTripRestoresAdoptedSlot(t *testing.T) {
	for _, provider := range []string{"zhipu", "minimax"} {
		regions := preset.ProviderRegionURLs[provider]
		if len(regions) != 3 {
			t.Fatalf("%s wants exactly CN/INTL/OpenCode Go for the wrap test, got %d rows", provider, len(regions))
		}
		slot := strings.ToUpper(provider) + "_INTL_1_API_KEY"
		p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
			"llm": map[string]interface{}{"provider": provider, "model": "m",
				"base_url":    regions[0].URL, // CN
				"api_key_env": slot}}}
		m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
		llm := m.llmMap()
		m.cursor = editorFieldOrderIndex(t, feBaseURL)

		steps := []struct {
			label   string
			wantURL string
			wantEnv string
		}{
			{"CN->INTL", regions[1].URL, slot},
			{"INTL->OpenCodeGo", regions[2].URL, "OPENCODE_GO_API_KEY"},
			// The wrap. This is the press the r3 test stopped one short of.
			{"OpenCodeGo->CN (wrap)", regions[0].URL, slot},
			// And round again, to prove the memo survives a second lap.
			{"CN->INTL (lap 2)", regions[1].URL, slot},
			{"INTL->OpenCodeGo (lap 2)", regions[2].URL, "OPENCODE_GO_API_KEY"},
			{"OpenCodeGo->CN (wrap, lap 2)", regions[0].URL, slot},
		}
		for _, s := range steps {
			m.cycleFocused(+1)
			if got := asString(llm["base_url"]); got != s.wantURL {
				t.Fatalf("[%s] %s base_url = %q, want %q", provider, s.label, got, s.wantURL)
			}
			if got := asString(llm["api_key_env"]); got != s.wantEnv {
				t.Fatalf("[%s] %s api_key_env = %q, want %q", provider, s.label, got, s.wantEnv)
			}
		}
	}
}

// TestPresetEditorRegionCycleFallsBackToProviderDefaultEnv covers the branch
// where there is nothing memoized to restore: the preset was already sitting
// on OpenCode Go when the editor opened, so cycling off it has no prior slot
// to hand back and must fall back to the provider default rather than leaving
// OPENCODE_GO_API_KEY stuck on a bigmodel.cn / minimaxi.com endpoint.
func TestPresetEditorRegionCycleFallsBackToProviderDefaultEnv(t *testing.T) {
	for _, provider := range []string{"zhipu", "minimax"} {
		regions := preset.ProviderRegionURLs[provider]
		p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
			"llm": map[string]interface{}{"provider": provider, "model": "m",
				"base_url":    regions[2].URL, // OpenCode Go
				"api_key_env": "OPENCODE_GO_API_KEY"}}}
		m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
		llm := m.llmMap()
		m.cursor = editorFieldOrderIndex(t, feBaseURL)

		m.cycleFocused(+1) // wraps to CN
		if got := asString(llm["base_url"]); got != regions[0].URL {
			t.Fatalf("[%s] OpenCodeGo->CN base_url = %q, want %q", provider, got, regions[0].URL)
		}
		want := preset.ProviderDefaultEnv[provider]
		if got := asString(llm["api_key_env"]); got != want {
			t.Fatalf("[%s] OpenCodeGo->CN api_key_env = %q, want provider default %q", provider, got, want)
		}
	}
}

// TestPresetEditorRegionCycleRestoresFromOffListBaseURL is the residual edge of
// F2 (fable r5): zhipu/minimax have no Custom sentinel row, so an off-list
// base_url makes selectedRegionIndex return -1 and there is no "previous row"
// to key the restore on. A preset that pairs such a URL with the adopted
// OPENCODE_GO_API_KEY must still be healed on the first cycle — otherwise the
// stale slot rides along onto CN and bigmodel.cn resolves through the OpenCode
// account, which is the exact harm the F2 fix exists to prevent. The rule is
// therefore about the STATE (is api_key_env a slot some region row declares?),
// not about the row we came from.
func TestPresetEditorRegionCycleRestoresFromOffListBaseURL(t *testing.T) {
	for _, provider := range []string{"zhipu", "minimax"} {
		regions := preset.ProviderRegionURLs[provider]
		for _, tc := range []struct {
			label string
			memo  string
			want  string
		}{
			{"no memo -> provider default", "", preset.ProviderDefaultEnv[provider]},
			{"memoized slot", strings.ToUpper(provider) + "_INTL_1_API_KEY", strings.ToUpper(provider) + "_INTL_1_API_KEY"},
		} {
			offList := "https://proxy.internal.example/v1"
			if idx := selectedRegionIndex(regions, offList); idx >= 0 {
				t.Fatalf("[%s] test setup wrong: %q resolves to region %d", provider, offList, idx)
			}
			p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
				"llm": map[string]interface{}{"provider": provider, "model": "m",
					"base_url":    offList,
					"api_key_env": "OPENCODE_GO_API_KEY"}}}
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
			m.regionEnvBeforeAdopt = tc.memo
			llm := m.llmMap()
			m.cursor = editorFieldOrderIndex(t, feBaseURL)

			m.cycleFocused(+1) // off-list -> CN (enters the list at its edge)
			if got := asString(llm["base_url"]); got != regions[0].URL {
				t.Fatalf("[%s/%s] base_url = %q, want CN %q", provider, tc.label, got, regions[0].URL)
			}
			if got := asString(llm["api_key_env"]); got != tc.want {
				t.Fatalf("[%s/%s] api_key_env = %q, want restored %q — the adopted OpenCode slot must not ride along onto CN",
					provider, tc.label, got, tc.want)
			}
			if m.regionEnvBeforeAdopt != "" {
				t.Fatalf("[%s/%s] memo = %q after restore, want cleared", provider, tc.label, m.regionEnvBeforeAdopt)
			}
		}
	}
}

// TestPresetEditorOpenCodeGoRowAdoptsAndRestoresForEveryProvider is the F5
// coverage fix: the five hand-written cycle tests above are all hardcoded to
// zhipu/minimax, so kimi, mimo and grok shipped an OpenCode Go row whose Env
// adoption and restore nothing pinned. This one is generic — it reads the
// table, so a provider that gains a Go row is covered the moment it is added.
//
// It walks the single transition that matters, in both directions: the row
// immediately before OpenCode Go -> OpenCode Go -> back. The expected slot on
// the way back follows the three documented rules, in order: the row's own
// declared Env when it has one (deepseek's DeepSeek API row); an untouched
// api_key_env on the Custom sentinel (pinned by
// TestPresetEditorDeepseekRegionCycleRoundTrip — a free-typed endpoint keeps
// whatever slot it actually uses); otherwise the memoized pre-adoption slot
// (every Env-less native row).
func TestPresetEditorOpenCodeGoRowAdoptsAndRestoresForEveryProvider(t *testing.T) {
	covered := 0
	for _, provider := range sortedRegionProviders() {
		regions := preset.ProviderRegionURLs[provider]
		ocg := -1
		for i, r := range regions {
			if r.URL == openCodeGoURL {
				ocg = i
				break
			}
		}
		if ocg < 0 {
			continue
		}
		covered++
		t.Run(provider, func(t *testing.T) {
			from := (ocg - 1 + len(regions)) % len(regions)
			// A host-stamped private slot, the thing the restore exists to
			// hand back. Env-less rows carry one; an Env row overrides it.
			slot := strings.ToUpper(provider) + "_7_API_KEY"
			startEnv := regions[from].Env
			if startEnv == "" {
				startEnv = slot
			}
			p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
				"llm": map[string]interface{}{"provider": provider, "model": "m",
					"base_url":    regions[from].URL,
					"api_key_env": startEnv}}}
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
			llm := m.llmMap()
			m.cursor = editorFieldOrderIndex(t, feBaseURL)

			m.cycleFocused(+1) // -> OpenCode Go
			if got := asString(llm["base_url"]); got != openCodeGoURL {
				t.Fatalf("[%s] +1 from %q base_url = %q, want the OpenCode Go row", provider, regions[from].Label, got)
			}
			if got := asString(llm["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
				t.Fatalf("[%s] on OpenCode Go api_key_env = %q, want OPENCODE_GO_API_KEY", provider, got)
			}

			wantBack := startEnv
			switch {
			case regions[from].Env != "":
				wantBack = regions[from].Env
			case regions[from].URL == "":
				// Custom sentinel: documented to keep whatever slot is set.
				wantBack = "OPENCODE_GO_API_KEY"
			}
			m.cycleFocused(-1) // -> back where we came from
			if got := asString(llm["base_url"]); got != regions[from].URL {
				t.Fatalf("[%s] -1 base_url = %q, want %q", provider, got, regions[from].URL)
			}
			if got := asString(llm["api_key_env"]); got != wantBack {
				t.Fatalf("[%s] -1 api_key_env = %q, want %q on the %q row",
					provider, got, wantBack, regions[from].Label)
			}
		})
	}
	// Guards against the loop silently covering nothing if the table shape
	// or the OpenCode Go URL ever changes.
	if covered < 6 {
		t.Fatalf("only %d providers offer an OpenCode Go row; want at least 6 (deepseek, zhipu, minimax, kimi, mimo, grok)", covered)
	}
}

// sortedRegionProviders makes the generic table-driven tests deterministic —
// Go map iteration order is randomized per run.
func sortedRegionProviders() []string {
	names := make([]string, 0, len(preset.ProviderRegionURLs))
	for name := range preset.ProviderRegionURLs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// F1-r6 mirror: an off-list base_url paired with the adopted slot, then
// cycle -1 (off-list -> OpenCode Go) and +1 (OpenCode Go -> CN wrap). The
// memo write must NOT capture OPENCODE_GO_API_KEY itself on the way onto
// the Env row (cur is already region-declared, so the write is skipped), and
// the +1 restore must still hand back the original slot or provider default.
func TestPresetEditorRegionCycleRestoresFromOffListBaseURLMirror(t *testing.T) {
	for _, provider := range []string{"zhipu", "minimax"} {
		regions := preset.ProviderRegionURLs[provider]
		for _, tc := range []struct {
			label string
			memo  string
			want  string
		}{
			{"no memo -> provider default", "", preset.ProviderDefaultEnv[provider]},
			{"memoized slot", strings.ToUpper(provider) + "_INTL_1_API_KEY", strings.ToUpper(provider) + "_INTL_1_API_KEY"},
		} {
			offList := "https://proxy.internal.example/v1"
			p := preset.Preset{Name: "my-" + provider, Source: preset.SourceSaved, Manifest: map[string]interface{}{
				"llm": map[string]interface{}{"provider": provider, "model": "m",
					"base_url":    offList,
					"api_key_env": "OPENCODE_GO_API_KEY"}}}
			m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
			m.regionEnvBeforeAdopt = tc.memo
			llm := m.llmMap()
			m.cursor = editorFieldOrderIndex(t, feBaseURL)

			m.cycleFocused(-1) // off-list -> OpenCode Go (Env row)
			if got := asString(llm["base_url"]); got != regions[2].URL {
				t.Fatalf("[%s/%s] base_url after -1 = %q, want OpenCode Go %q", provider, tc.label, got, regions[2].URL)
			}
			if m.regionEnvBeforeAdopt != tc.memo {
				t.Fatalf("[%s/%s] memo = %q after -1, want %q -- the adopted slot must not memoize itself",
					provider, tc.label, m.regionEnvBeforeAdopt, tc.memo)
			}

			m.cycleFocused(+1) // OpenCode Go -> CN (wrap)
			if got := asString(llm["base_url"]); got != regions[0].URL {
				t.Fatalf("[%s/%s] base_url after +1 = %q, want CN %q", provider, tc.label, got, regions[0].URL)
			}
			if got := asString(llm["api_key_env"]); got != tc.want {
				t.Fatalf("[%s/%s] api_key_env after +1 = %q, want %q -- the OpenCode slot must not survive onto CN",
					provider, tc.label, got, tc.want)
			}
			if m.regionEnvBeforeAdopt != "" {
				t.Fatalf("[%s/%s] memo = %q after +1, want cleared", provider, tc.label, m.regionEnvBeforeAdopt)
			}
		}
	}
}

// TestPresetEditorProviderSwitchClearsAdoptedSlotMemo pins the other half of the
// same rule: the memo records a slot taken from THIS provider's region cycle, so
// switching providers must drop it along with base_url and api_key_env. Left
// behind, it could be handed back onto a provider it never belonged to.
func TestPresetEditorProviderSwitchClearsAdoptedSlotMemo(t *testing.T) {
	regions := preset.ProviderRegionURLs["zhipu"]
	p := preset.Preset{Name: "my-zhipu", Source: preset.SourceSaved, Manifest: map[string]interface{}{
		"llm": map[string]interface{}{"provider": "zhipu", "model": "m",
			"base_url":    regions[2].URL, // OpenCode Go
			"api_key_env": "OPENCODE_GO_API_KEY"}}}
	m := NewPresetEditorModelWithBuiltinFlag(p, "en", nil, "", false)
	m.regionEnvBeforeAdopt = "ZHIPU_INTL_1_API_KEY"
	m.cursor = editorFieldOrderIndex(t, feProvider)

	m.cycleFocused(+1)
	if got := asString(m.llmMap()["provider"]); got == "zhipu" {
		t.Fatal("provider cycle did not move off zhipu")
	}
	if m.regionEnvBeforeAdopt != "" {
		t.Fatalf("memo = %q after provider switch, want cleared", m.regionEnvBeforeAdopt)
	}
}

// TestPresetEditorDeepseekRegionCycleRoundTrip is the deepseek half of the F2
// contract. deepseek's table has a Custom sentinel instead of a second real
// region, and its first row declares DEEPSEEK_API_KEY, so the full lap is
// DeepSeek API → OpenCode Go → Custom → DeepSeek API. Landing on Custom must
// leave api_key_env alone (documented, intended: the user's free-typed
// endpoint keeps whatever slot it actually uses), and the lap must end back
// on DEEPSEEK_API_KEY.
func TestPresetEditorDeepseekRegionCycleRoundTrip(t *testing.T) {
	regions := preset.ProviderRegionURLs["deepseek"]
	if len(regions) != 3 || regions[2].URL != "" {
		t.Fatalf("deepseek table changed shape: %#v", regions)
	}
	m := NewPresetEditorModelWithBuiltinFlag(builtinPresetForEditorTest(t, "deepseek"), "en", nil, "", false)
	llm := m.llmMap()
	m.cursor = editorFieldOrderIndex(t, feBaseURL)

	if got := asString(llm["api_key_env"]); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("template api_key_env = %q, want DEEPSEEK_API_KEY", got)
	}

	m.cycleFocused(+1) // DeepSeek API -> OpenCode Go
	if got := asString(llm["base_url"]); got != regions[1].URL {
		t.Fatalf("->OpenCodeGo base_url = %q, want %q", got, regions[1].URL)
	}
	if got := asString(llm["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("->OpenCodeGo api_key_env = %q, want OPENCODE_GO_API_KEY", got)
	}

	m.cycleFocused(+1) // OpenCode Go -> Custom (sentinel: clears base_url, keeps the slot)
	if got := asString(llm["base_url"]); got != "" {
		t.Fatalf("->Custom base_url = %q, want cleared", got)
	}
	if got := asString(llm["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("->Custom api_key_env = %q, want untouched OPENCODE_GO_API_KEY", got)
	}

	m.cycleFocused(+1) // Custom -> DeepSeek API (wrap): row declares its own slot
	if got := asString(llm["base_url"]); got != regions[0].URL {
		t.Fatalf("->DeepSeek API base_url = %q, want %q", got, regions[0].URL)
	}
	if got := asString(llm["api_key_env"]); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("round trip ended on api_key_env = %q, want DEEPSEEK_API_KEY", got)
	}

	// And the reverse lap, so neither direction is a one-way door.
	m.cycleFocused(-1) // DeepSeek API -> Custom
	if got := asString(llm["api_key_env"]); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("reverse ->Custom api_key_env = %q, want untouched DEEPSEEK_API_KEY", got)
	}
	m.cycleFocused(-1) // Custom -> OpenCode Go
	if got := asString(llm["api_key_env"]); got != "OPENCODE_GO_API_KEY" {
		t.Fatalf("reverse ->OpenCodeGo api_key_env = %q, want OPENCODE_GO_API_KEY", got)
	}
	m.cycleFocused(-1) // OpenCode Go -> DeepSeek API
	if got := asString(llm["api_key_env"]); got != "DEEPSEEK_API_KEY" {
		t.Fatalf("reverse ->DeepSeek API api_key_env = %q, want DEEPSEEK_API_KEY", got)
	}
}
