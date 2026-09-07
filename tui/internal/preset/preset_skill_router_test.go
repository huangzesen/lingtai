package preset

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const wantMaintenance = `If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths.`

var (
	maintenanceRE  = regexp.MustCompile(`(?m)^maintenance:\s*(.+?)\s*$`)
	catalogEntryRE = regexp.MustCompile(`(?m)^- name:\s*(\S+)\n  location:\s*(\S+)$`)
	catalogNameRE  = regexp.MustCompile(`(?m)^- name:`)
	relatedFileRE  = regexp.MustCompile(`^  - ([^\s#].+)$`)
)

// frontmatter returns only the YAML frontmatter, reporting malformed or short
// files before attempting to slice past the opening delimiter.
func frontmatter(path string, data []byte) (string, error) {
	body := string(data)
	if !strings.HasPrefix(body, "---\n") {
		return "", fmt.Errorf("%s missing opening frontmatter delimiter", path)
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return "", fmt.Errorf("%s missing closing frontmatter delimiter", path)
	}
	return body[:4+end], nil
}

func maintenanceValue(frontmatter string) (string, bool) {
	match := maintenanceRE.FindStringSubmatch(frontmatter)
	if match == nil {
		return "", false
	}
	value := strings.TrimSpace(match[1])
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return value, true
}

// wantOperations is the exact 6-item operation-axis inventory required by
// the #691 preset-skill dual-axis router shape. Unlike the provider axis
// (sourced from BuiltinPresets()), there is no runtime source list for
// operations, so this literal is the single source of truth the tests below
// check the catalog, embedded children, and extracted tree against.
var wantOperations = map[string]bool{
	"saved-presets":              true,
	"endpoint-capabilities":      true,
	"availability-save-gate":     true,
	"activation-session-refresh": true,
	"troubleshooting-migration":  true,
	"revision-pipeline":          true,
}

// TestPresetSkillRouter_BuiltinBijection keeps the source preset list, the
// embedded direct-provider manuals, the parent router, and extracted utility
// tree aligned. Direct providers are exactly the 13 BuiltinPresets() names —
// nested operation children live under reference/operations/ and are
// validated separately by TestPresetSkillRouter_OperationBijection so a
// provider directory can never silently absorb an operation, or vice versa.
func TestPresetSkillRouter_BuiltinBijection(t *testing.T) {
	want := map[string]bool{}
	for _, p := range BuiltinPresets() {
		if want[p.Name] {
			t.Errorf("BuiltinPresets() contains duplicate name %q", p.Name)
		}
		want[p.Name] = true
	}

	children := map[string]bool{}
	err := fs.WalkDir(skillsFS, "skills/lingtai-preset-skill/reference", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		rel := strings.TrimPrefix(path, "skills/lingtai-preset-skill/reference/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 || parts[1] != "SKILL.md" {
			return nil
		}
		// Direct providers are top-level reference/<name>/SKILL.md. The
		// operations subtree is reference/operations/<op>/SKILL.md — skip it
		// here; it has its own bijection test below.
		if parts[0] == "operations" {
			return nil
		}
		if children[parts[0]] {
			t.Errorf("embedded children contains duplicate %q", parts[0])
		}
		children[parts[0]] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSameNames(t, "embedded children", want, children)

	parentData, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/SKILL.md")
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	_, err = frontmatter("parent", parentData)
	if err != nil {
		t.Fatal(err)
	}
	parent := string(parentData)
	if !strings.Contains(parent, "When `BuiltinPresets()` gains a new template name") {
		t.Error("parent does not state the new-preset maintenance contract")
	}
	if !strings.Contains(parent, "new cross-cutting mechanic is added") {
		t.Error("parent does not state the new-operation maintenance contract")
	}

	catalogNames := map[string]bool{}
	catalogLocations := map[string]bool{}
	catalogEntries := catalogEntryRE.FindAllStringSubmatch(parent, -1)
	if len(catalogEntries) != len(catalogNameRE.FindAllString(parent, -1)) {
		t.Errorf("parent catalog has malformed or short name/location entries")
	}
	nameCount := map[string]int{}
	locationCount := map[string]int{}
	for _, match := range catalogEntries {
		name, location := match[1], match[2]
		nameCount[name]++
		locationCount[location]++
		switch {
		case strings.HasPrefix(name, "preset-skill-op-"):
			// Operation-axis catalog entry — validated by
			// TestPresetSkillRouter_OperationBijection.
			continue
		case strings.HasPrefix(name, "preset-skill-"):
			child := strings.TrimPrefix(name, "preset-skill-")
			wantLocation := "reference/" + child + "/SKILL.md"
			if location != wantLocation {
				t.Errorf("parent catalog pairs name %q with location %q, want %q", name, location, wantLocation)
			}
			catalogNames[name] = true
			catalogLocations[location] = true
		default:
			t.Errorf("parent catalog has unexpected name %q", name)
		}
	}
	for name, count := range nameCount {
		if count > 1 {
			t.Errorf("parent catalog duplicates name %q", name)
		}
	}
	for location, count := range locationCount {
		if count > 1 {
			t.Errorf("parent catalog duplicates location %q", location)
		}
	}
	wantCatalogNames := map[string]bool{}
	for name := range want {
		wantCatalogNames["preset-skill-"+name] = true
	}
	assertSameNames(t, "parent catalog names", wantCatalogNames, catalogNames)
	wantLocations := map[string]bool{}
	for name := range want {
		wantLocations["reference/"+name+"/SKILL.md"] = true
	}
	assertSameNames(t, "parent catalog locations", wantLocations, catalogLocations)

	globalDir := t.TempDir()
	PopulateBundledLibrary(globalDir)
	referenceDir := filepath.Join(globalDir, "utilities", "lingtai-preset-skill", "reference")
	entries, err := os.ReadDir(referenceDir)
	if err != nil {
		t.Fatal(err)
	}
	extracted := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("extracted reference has unexpected file %q", entry.Name())
			continue
		}
		if entry.Name() == "operations" {
			continue
		}
		extracted[entry.Name()] = true
		if _, err := os.Stat(filepath.Join(referenceDir, entry.Name(), "SKILL.md")); err != nil {
			t.Errorf("extracted child %q: %v", entry.Name(), err)
		}
	}
	assertSameNames(t, "extracted children", want, extracted)
	if !BundledSkillNames()["lingtai-preset-skill"] {
		t.Error("parent router is not a bundled skill")
	}
}

// TestPresetSkillRouter_OperationBijection keeps the fixed 6-item operation
// inventory, the parent's operation catalog, the embedded
// reference/operations/ tree, and the extracted utility tree aligned —
// mirroring TestPresetSkillRouter_BuiltinBijection but for the operation
// axis instead of the provider axis.
func TestPresetSkillRouter_OperationBijection(t *testing.T) {
	children := map[string]bool{}
	err := fs.WalkDir(skillsFS, "skills/lingtai-preset-skill/reference/operations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		rel := strings.TrimPrefix(path, "skills/lingtai-preset-skill/reference/operations/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 2 && parts[1] == "SKILL.md" {
			if children[parts[0]] {
				t.Errorf("embedded operation children contains duplicate %q", parts[0])
			}
			children[parts[0]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSameNames(t, "embedded operation children", wantOperations, children)

	parentData, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/SKILL.md")
	if err != nil {
		t.Fatalf("read parent: %v", err)
	}
	parent := string(parentData)

	catalogEntries := catalogEntryRE.FindAllStringSubmatch(parent, -1)
	catalogNames := map[string]bool{}
	catalogLocations := map[string]bool{}
	for _, match := range catalogEntries {
		name, location := match[1], match[2]
		if !strings.HasPrefix(name, "preset-skill-op-") {
			continue
		}
		op := strings.TrimPrefix(name, "preset-skill-op-")
		wantLocation := "reference/operations/" + op + "/SKILL.md"
		if location != wantLocation {
			t.Errorf("parent operation catalog pairs name %q with location %q, want %q", name, location, wantLocation)
		}
		if catalogNames[name] {
			t.Errorf("parent operation catalog duplicates name %q", name)
		}
		if catalogLocations[location] {
			t.Errorf("parent operation catalog duplicates location %q", location)
		}
		catalogNames[name] = true
		catalogLocations[location] = true
	}
	wantCatalogNames := map[string]bool{}
	wantLocations := map[string]bool{}
	for op := range wantOperations {
		wantCatalogNames["preset-skill-op-"+op] = true
		wantLocations["reference/operations/"+op+"/SKILL.md"] = true
	}
	assertSameNames(t, "parent operation catalog names", wantCatalogNames, catalogNames)
	assertSameNames(t, "parent operation catalog locations", wantLocations, catalogLocations)

	for op := range wantOperations {
		path := "skills/lingtai-preset-skill/reference/operations/" + op + "/SKILL.md"
		data, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			t.Fatalf("read operation child %s: %v", op, err)
		}
		fm, err := frontmatter(path, data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(fm, "\nrelated_files:\n") {
			t.Errorf("operation child %s missing related_files frontmatter", op)
			continue
		}
		relatedCount := 0
		for _, line := range strings.Split(fm, "\n") {
			match := relatedFileRE.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			relatedCount++
			rel := match[1]
			if _, err := os.Stat(filepath.Join("..", "..", "..", rel)); err != nil {
				t.Errorf("operation child %s related_files entry %q does not resolve: %v", op, rel, err)
			}
		}
		if relatedCount == 0 {
			t.Errorf("operation child %s has empty related_files frontmatter", op)
		}
	}

	globalDir := t.TempDir()
	PopulateBundledLibrary(globalDir)
	operationsDir := filepath.Join(globalDir, "utilities", "lingtai-preset-skill", "reference", "operations")
	entries, err := os.ReadDir(operationsDir)
	if err != nil {
		t.Fatal(err)
	}
	extracted := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			t.Errorf("extracted operations reference has unexpected file %q", entry.Name())
			continue
		}
		extracted[entry.Name()] = true
		if _, err := os.Stat(filepath.Join(operationsDir, entry.Name(), "SKILL.md")); err != nil {
			t.Errorf("extracted operation child %q: %v", entry.Name(), err)
		}
	}
	assertSameNames(t, "extracted operation children", wantOperations, extracted)
}

// TestPresetSkillRouter_ProviderChildContracts checks the per-provider
// requirements added by the #691 dual-axis shape: an exact provider/manual
// mapping, a "Template-specific settings" heading, and a one-line route into
// the operations tree — without duplicating operation prose into the provider
// page itself.
func TestPresetSkillRouter_ProviderChildContracts(t *testing.T) {
	wantProviders := []string{
		"minimax",
		"zhipu",
		"mimo",
		"deepseek",
		"gemini",
		"kimi",
		"grok",
		"nvidia",
		"openrouter",
		"codex",
		"codex-pool",
		"claude",
		"custom",
	}
	want := map[string]bool{}
	for _, p := range BuiltinPresets() {
		want[p.Name] = true
	}
	manuals := map[string]bool{}
	for _, name := range wantProviders {
		manuals[name] = true
	}
	assertSameNames(t, "provider manual inventory", want, manuals)

	for _, name := range wantProviders {
		data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("read %s manual: %v", name, err)
		}
		body := string(data)
		if !strings.Contains(body, "## Template-specific settings") {
			t.Errorf("%s manual missing \"## Template-specific settings\" heading", name)
		}
		if !strings.Contains(body, "## Operations") {
			t.Errorf("%s manual missing \"## Operations\" route heading", name)
		}
		if !strings.Contains(body, "reference/operations/") {
			t.Errorf("%s manual does not route to reference/operations/", name)
		}
	}
}

// TestPresetSkillRouter_CodexPoolContract checks the codex-pool manual holds
// the critical pool-format/selection/manual-edit facts the #691 brief
// requires, without asserting on prose wording beyond the load-bearing
// technical terms.
func TestPresetSkillRouter_CodexPoolContract(t *testing.T) {
	data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/codex-pool/SKILL.md")
	if err != nil {
		t.Fatalf("read codex-pool manual: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"$LINGTAI_TUI_DIR/codex-auth-pool.json",
		"~/.lingtai-tui/codex-auth-pool.json",
		`"version": 1, "accounts"`,
		`"version": 2, "models"`,
		"of the `models` key is what classifies",
		"errCodexPoolModelClassified",
		"stores only refs and integer weights",
		"never token",
		"Weight 0 means the account is present but disabled",
		"sticky within one agent wake/session",
		"excludes `molt_count`",
		"Selection happens at adapter/service construction",
		"does **not** reselect an",
		"already-running session",
		"Configured weights are inputs, not measured shares",
		"falls back to the legacy",
		"Exact authorization",
		"Timestamped backup",
		"Exact-old-value or hash gate",
		"atomic rename",
		"load_codex_auth_pool",
		"Preserve the original file on any validation failure",
		"Never print token/auth contents or absolute auth paths",
		"tui/internal/tui/codex_pool_store.go:11-330",
		"login.go:171-201,285-299,603-702",
		"auth/codex_pool.py:72-323",
		"_register.py:424-497",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("codex-pool manual missing %q", want)
		}
	}
}

func TestPresetSkillRouter_SavedAndAvailabilitySourceContracts(t *testing.T) {
	readOperation := func(name string) string {
		t.Helper()
		data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/operations/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("read %s operation: %v", name, err)
		}
		return string(data)
	}

	saved := readOperation("saved-presets")
	for _, want := range []string{
		"malformed saved override blocks fallback",
		"Neither `Save` nor `Load` calls",
		"direct, non-atomic `os.WriteFile`",
		"removes a file from `saved/` only",
		"never touches\n`saved/`",
		"exact relative order as an implementation detail",
		"a **non-empty** `tier`",
		"while an empty tier is allowed",
	} {
		if !strings.Contains(saved, want) {
			t.Errorf("saved-presets manual missing %q", want)
		}
	}
	if strings.Contains(saved, "canonical product order") {
		t.Fatal("saved-presets manual must not promise a canonical template order the current comparator does not guarantee")
	}

	presetWithEmptyTier := BuiltinPresets()[0]
	presetWithEmptyTier.Description.Tier = ""
	for _, err := range presetWithEmptyTier.Validate() {
		if strings.Contains(err.Error(), "description.tier") {
			t.Fatalf("Validate rejected the documented optional empty tier: %v", err)
		}
	}

	gate := readOperation("availability-save-gate")
	for _, want := range []string{
		"Save is structural-only",
		"never makes a live provider/model network call",
		"Codex, Codex-pool, and API-key providers like",
		"DeepSeek",
		"not been replaced by another probe",
		"`/doctor`",
	} {
		if !strings.Contains(gate, want) {
			t.Errorf("availability-save-gate manual missing %q", want)
		}
	}
	for _, mustNotContain := range []string{
		"hard-block Save as `probeNoKey`",
		"pressing Save again with the unchanged tuple does not",
	} {
		if strings.Contains(gate, mustNotContain) {
			t.Errorf("availability-save-gate manual still describes the removed live-probe save gate: %q", mustNotContain)
		}
	}
}

// TestPresetSkillRouter_QuotaContract checks the endpoint-capabilities
// operation child holds the Codex OAuth quota inspection facts required by
// the #691 brief, cross-linked from codex and codex-pool. Updated for the
// 2026-07-19 CORRECTION-372K post-run fix: exact agent-facing app-server
// query routing (initialize -> account/rateLimits/read with params:null,
// plus the account/rateLimits/updated notification as a rolling
// supplement) and the official-272K-vs-measured-372K context-window
// distinction, each with its exact PASS/FAIL evidence.
func TestPresetSkillRouter_QuotaContract(t *testing.T) {
	data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/operations/endpoint-capabilities/SKILL.md")
	if err != nil {
		t.Fatalf("read endpoint-capabilities manual: %v", err)
	}
	body := string(data)
	for _, want := range []string{
		"0.144.3",
		"/status",
		"account/rateLimits/read",
		"account/rateLimits/updated",
		"GetAccountRateLimitsResponse",
		"AccountRateLimitsUpdatedNotification",
		"rateLimitsByLimitId",
		"usedPercent",
		"windowDurationMins",
		"resetsAt",
		"does NOT expose any of this",
		"remaining = max(0, 100 - usedPercent)",
		"Never expose auth paths or tokens",
		"verify any adapter-facing convenience surface",
		// Exact agent query routing (Jason's 7927).
		"Complete the app-server `initialize` handshake first",
		"structurally `null`",
		`"params": {"type": "null"}`,
		"not a substitute for step 2",
		"Secret-safe fields and limitations",
		"none of it is a token or credential",
		// Official-vs-measured context window distinction.
		"Official current metadata/changelog figure: 272K tokens",
		"Measured live A/B boundary: ~372,000 total tokens",
		"gpt-5.6-sol",
		"codex-cli `0.144.3`",
		"2026-07-19",
		"o200k_base 0.12.0",
		"input_tokens=312684",
		"CONTEXT_PROBE_359K_OK",
		"371684",
		"360,000 user tokens",
		"FAIL",
		"500,000 user tokens",
		"empirical bracket",
		"371,684 total",
		"372,684 total",
		"rounded 372,000",
		"12,684-token setup overhead",
		"359,316-token same-setup",
		"exact empirical claim remains only",
		"Never present 272K as live proof of anything measured",
		"present 372K",
		"as timeless or",
		"universal",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("endpoint-capabilities manual missing %q", want)
		}
	}

	for _, name := range []string{"codex", "codex-pool"} {
		child, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("read %s manual: %v", name, err)
		}
		body := string(child)
		if !strings.Contains(body, "reference/operations/endpoint-capabilities/SKILL.md") {
			t.Errorf("%s manual does not cross-link the quota-inspection operation child", name)
		}
		for _, want := range []string{
			"initialize",
			"account/rateLimits/read",
			"structurally `null`",
			"account/rateLimits/updated",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s manual does not route agents to the exact quota-query operation (missing %q)", name, want)
			}
		}
	}
}

// TestPresetSkillRouter_OperationTaxonomyPreservesExisting pins the exact
// operation-axis inventory so a future correction cannot silently add or
// rename an operation instead of extending an existing one — the
// CORRECTION-372K fix, for example, was required to land entirely inside
// the existing endpoint-capabilities child.
func TestPresetSkillRouter_OperationTaxonomyPreservesExisting(t *testing.T) {
	want := map[string]bool{
		"revision-pipeline":          true,
		"saved-presets":              true,
		"endpoint-capabilities":      true,
		"availability-save-gate":     true,
		"activation-session-refresh": true,
		"troubleshooting-migration":  true,
	}
	assertSameNames(t, "operation taxonomy", want, wantOperations)
}

func assertSameNames(t *testing.T, label string, want, got map[string]bool) {
	t.Helper()
	for name := range want {
		if !got[name] {
			t.Errorf("%s missing %q", label, name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("%s has unexpected %q", label, name)
		}
	}
}

func TestPresetSkillRouter_ChildMetadata(t *testing.T) {
	err := fs.WalkDir(skillsFS, "skills/lingtai-preset-skill/reference", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		data, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return err
		}
		fm, err := frontmatter(path, data)
		if err != nil {
			t.Error(err)
			return nil
		}
		rel := strings.TrimPrefix(path, "skills/lingtai-preset-skill/reference/")
		parts := strings.SplitN(rel, "/", 2)
		var wantName string
		if parts[0] == "operations" {
			// reference/operations/<op>/SKILL.md -> preset-skill-op-<op>,
			// the operation axis's own naming (see TestPresetSkillRouter_OperationBijection).
			opParts := strings.SplitN(strings.TrimPrefix(rel, "operations/"), "/", 2)
			wantName = "preset-skill-op-" + opParts[0]
		} else {
			wantName = "preset-skill-" + parts[0]
		}
		if !regexp.MustCompile(`(?m)^name:\s*` + regexp.QuoteMeta(wantName) + `\s*$`).MatchString(fm) {
			t.Errorf("%s has no name %q", path, wantName)
		}
		value, ok := maintenanceValue(fm)
		if !ok || value != wantMaintenance {
			t.Errorf("%s has maintenance %q, want %q", path, value, wantMaintenance)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPresetSkillRouter_AllBundledMaintenance(t *testing.T) {
	err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		data, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return err
		}
		fm, err := frontmatter(path, data)
		if err != nil {
			t.Error(err)
			return nil
		}
		value, ok := maintenanceValue(fm)
		if !ok {
			t.Errorf("%s missing maintenance frontmatter", path)
		} else if value != wantMaintenance {
			t.Errorf("%s maintenance value %q, want %q", path, value, wantMaintenance)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinPresetVisionWiring(t *testing.T) {
	presets := map[string]Preset{}
	for _, p := range BuiltinPresets() {
		presets[p.Name] = p
	}

	geminiCaps, ok := presets["gemini"].Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("gemini capabilities has unexpected type")
	}
	geminiVision, ok := geminiCaps["vision"].(map[string]interface{})
	if !ok {
		t.Fatal("gemini must expose a vision capability")
	}
	if got := geminiVision["provider"]; got != "gemini" {
		t.Fatalf("gemini vision provider = %#v, want gemini", got)
	}
	if got := geminiVision["api_key_env"]; got != "GEMINI_API_KEY" {
		t.Fatalf("gemini vision api_key_env = %#v, want GEMINI_API_KEY", got)
	}

	zhipuCaps, ok := presets["zhipu"].Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("zhipu capabilities has unexpected type")
	}
	if _, ok := zhipuCaps["vision"]; ok {
		t.Fatal("zhipu must not expose a default vision capability for text-only GLM-5.2")
	}

	openrouterCaps, ok := presets["openrouter"].Manifest["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("openrouter capabilities has unexpected type")
	}
	if _, ok := openrouterCaps["vision"]; ok {
		t.Fatal("openrouter stock template must not expose a default vision capability")
	}
}

func TestPresetVisionManualContracts(t *testing.T) {
	readChild := func(t *testing.T, name string) string {
		t.Helper()
		data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/reference/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("read %s manual: %v", name, err)
		}
		return string(data)
	}

	gemini := readChild(t, "gemini")
	for _, want := range []string{"gemini-3-flash-preview", "GEMINI_API_KEY", "explicit LingTai `vision` capability"} {
		if !strings.Contains(gemini, want) {
			t.Errorf("gemini manual missing %q", want)
		}
	}

	zhipu := readChild(t, "zhipu")
	for _, want := range []string{"GLM-5.2", "@z_ai/mcp-server", "GLM-4.6V", "ZHIPU_API_KEY", "Z_AI_API_KEY", "Z_AI_MODE", "5-hour prompt pool"} {
		if !strings.Contains(zhipu, want) {
			t.Errorf("zhipu manual missing %q", want)
		}
	}

	openrouter := readChild(t, "openrouter")
	for _, want := range []string{"stock template is text-only", "explicitly adds\n`capabilities.vision`", "only then may the vision tool attempt"} {
		if !strings.Contains(openrouter, want) {
			t.Errorf("openrouter manual missing %q", want)
		}
	}
	if strings.Contains(openrouter, "route by default") {
		t.Fatal("openrouter manual must not claim the stock text-only template routes vision by default")
	}

	retiredModel := strings.Join([]string{"mimo", "v2", "flash"}, "-")
	if mimo := readChild(t, "mimo"); strings.Contains(mimo, retiredModel) {
		t.Fatalf("mimo manual still mentions retired model %q", retiredModel)
	}
}

func TestPresetSkillRouter_ParentMaintenanceAndRelated(t *testing.T) {
	data, err := fs.ReadFile(skillsFS, "skills/lingtai-preset-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	fm, err := frontmatter("parent", data)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := maintenanceValue(fm)
	if !ok || value != wantMaintenance {
		t.Errorf("parent maintenance value %q, want %q", value, wantMaintenance)
	}

	seenRelated := false
	relatedCount := 0
	for _, line := range strings.Split(fm, "\n") {
		if strings.HasPrefix(line, "related_files:") {
			seenRelated = true
			continue
		}
		if !seenRelated {
			continue
		}
		match := relatedFileRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		rel := match[1]
		relatedCount++
		if rel == "" {
			t.Error("parent related_files contains an empty path")
			continue
		}
		if _, err := os.Stat(filepath.Join("..", "..", "..", rel)); err != nil {
			t.Errorf("related_files entry %q does not resolve to a repo file: %v", rel, err)
		}
	}
	if !seenRelated {
		t.Fatal("parent missing related_files field")
	}
	if relatedCount == 0 {
		t.Fatal("parent related_files field is empty")
	}
}
