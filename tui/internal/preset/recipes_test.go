package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecipeDir(t *testing.T) {
	globalDir := t.TempDir()
	if err := Bootstrap(globalDir); err != nil {
		t.Fatalf("Bootstrap err = %v", err)
	}
	got := RecipeDir(globalDir, "greeter")
	want := filepath.Join(globalDir, "recipes", "recommended", "greeter")
	if got != want {
		t.Errorf("RecipeDir(greeter) = %q, want %q", got, want)
	}
	got = RecipeDir(globalDir, "tutorial")
	want = filepath.Join(globalDir, "recipes", "examples", "tutorial")
	if got != want {
		t.Errorf("RecipeDir(tutorial) = %q, want %q", got, want)
	}
	got = RecipeDir(globalDir, "nonexistent")
	if got != "" {
		t.Errorf("RecipeDir(nonexistent) = %q, want empty", got)
	}
}

func TestScanCategory(t *testing.T) {
	globalDir := t.TempDir()
	if err := Bootstrap(globalDir); err != nil {
		t.Fatalf("Bootstrap err = %v", err)
	}
	recipes := ScanCategory(globalDir, "recommended", "en")
	if len(recipes) == 0 {
		t.Fatalf("ScanCategory(recommended) returned no recipes")
	}
	found := false
	for _, r := range recipes {
		if r.ID == "greeter" {
			found = true
			if r.Info.Name == "" {
				t.Errorf("greeter recipe has empty name")
			}
			if r.Dir == "" {
				t.Errorf("greeter recipe has empty dir")
			}
		}
	}
	if !found {
		t.Errorf("ScanCategory(recommended) did not find greeter")
	}
}

func TestScanCategory_Intrinsic(t *testing.T) {
	globalDir := t.TempDir()
	if err := Bootstrap(globalDir); err != nil {
		t.Fatalf("Bootstrap err = %v", err)
	}
	recipes := ScanCategory(globalDir, "intrinsic", "en")
	if len(recipes) == 0 {
		t.Fatalf("ScanCategory(intrinsic) returned no recipes")
	}
	ids := make(map[string]bool)
	for _, r := range recipes {
		ids[r.ID] = true
	}
	for _, want := range []string{"adaptive", "plain"} {
		if !ids[want] {
			t.Errorf("ScanCategory(intrinsic) missing %q", want)
		}
	}
}

func TestScanCategory_Examples(t *testing.T) {
	globalDir := t.TempDir()
	if err := Bootstrap(globalDir); err != nil {
		t.Fatalf("Bootstrap err = %v", err)
	}
	recipes := ScanCategory(globalDir, "examples", "en")
	if len(recipes) == 0 {
		t.Fatalf("ScanCategory(examples) returned no recipes")
	}
	found := false
	for _, r := range recipes {
		if r.ID == "tutorial" {
			found = true
		}
	}
	if !found {
		t.Errorf("ScanCategory(examples) did not find tutorial")
	}
}

func TestScanCategory_Empty(t *testing.T) {
	globalDir := t.TempDir()
	recipes := ScanCategory(globalDir, "nonexistent", "en")
	if len(recipes) != 0 {
		t.Errorf("ScanCategory(nonexistent) = %d recipes, want 0", len(recipes))
	}
}

func TestResolveGreetPath_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "en"), 0o755)
	os.WriteFile(filepath.Join(dir, "en", "greet.md"), []byte("en greet"), 0o644)
	os.WriteFile(filepath.Join(dir, "greet.md"), []byte("root greet"), 0o644)

	got := ResolveGreetPath(dir, "en")
	want := filepath.Join(dir, "en", "greet.md")
	if got != want {
		t.Errorf("ResolveGreetPath prefers lang-specific, got %q, want %q", got, want)
	}
}

func TestResolveGreetPath_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "greet.md"), []byte("root greet"), 0o644)

	got := ResolveGreetPath(dir, "en")
	want := filepath.Join(dir, "greet.md")
	if got != want {
		t.Errorf("ResolveGreetPath fallback to root, got %q, want %q", got, want)
	}
}

func TestResolveGreetPath_Empty(t *testing.T) {
	dir := t.TempDir()
	got := ResolveGreetPath(dir, "en")
	if got != "" {
		t.Errorf("ResolveGreetPath empty dir = %q, want empty string", got)
	}
}

func TestResolveGreetPath_EmptyLang(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "greet.md"), []byte("root greet"), 0o644)

	got := ResolveGreetPath(dir, "")
	want := filepath.Join(dir, "greet.md")
	if got != want {
		t.Errorf("ResolveGreetPath empty lang = %q, want %q", got, want)
	}
}

func TestResolveGreetPath_EmptyRecipeDir(t *testing.T) {
	got := ResolveGreetPath("", "en")
	if got != "" {
		t.Errorf("ResolveGreetPath empty recipeDir = %q, want empty", got)
	}
}

func TestResolveCovenantPath_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "en"), 0o755)
	want := filepath.Join(dir, "en", "covenant.md")
	os.WriteFile(want, []byte("test covenant"), 0o644)
	got := ResolveCovenantPath(dir, "en")
	if got != want {
		t.Errorf("ResolveCovenantPath prefers lang-specific, got %q, want %q", got, want)
	}
}

func TestResolveCovenantPath_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "covenant.md")
	os.WriteFile(want, []byte("root covenant"), 0o644)
	got := ResolveCovenantPath(dir, "en")
	if got != want {
		t.Errorf("ResolveCovenantPath fallback to root, got %q, want %q", got, want)
	}
}

func TestResolveCovenantPath_Empty(t *testing.T) {
	dir := t.TempDir()
	got := ResolveCovenantPath(dir, "en")
	if got != "" {
		t.Errorf("ResolveCovenantPath empty dir = %q, want empty string", got)
	}
}

func TestResolveCovenantPath_EmptyRecipeDir(t *testing.T) {
	got := ResolveCovenantPath("", "en")
	if got != "" {
		t.Errorf("ResolveCovenantPath empty recipeDir = %q, want empty", got)
	}
}

func TestResolveProceduresPath_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "en"), 0o755)
	want := filepath.Join(dir, "en", "procedures.md")
	os.WriteFile(want, []byte("test procedures"), 0o644)
	got := ResolveProceduresPath(dir, "en")
	if got != want {
		t.Errorf("ResolveProceduresPath prefers lang-specific, got %q, want %q", got, want)
	}
}

func TestResolveProceduresPath_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, "procedures.md")
	os.WriteFile(want, []byte("root procedures"), 0o644)
	got := ResolveProceduresPath(dir, "en")
	if got != want {
		t.Errorf("ResolveProceduresPath fallback to root, got %q, want %q", got, want)
	}
}

func TestResolveProceduresPath_Empty(t *testing.T) {
	dir := t.TempDir()
	got := ResolveProceduresPath(dir, "en")
	if got != "" {
		t.Errorf("ResolveProceduresPath empty dir = %q, want empty string", got)
	}
}

func TestResolveProceduresPath_EmptyRecipeDir(t *testing.T) {
	got := ResolveProceduresPath("", "en")
	if got != "" {
		t.Errorf("ResolveProceduresPath empty recipeDir = %q, want empty", got)
	}
}

func TestResolveCommentPath_SameRules(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "zh"), 0o755)
	os.WriteFile(filepath.Join(dir, "zh", "comment.md"), []byte("zh comment"), 0o644)

	got := ResolveCommentPath(dir, "zh")
	want := filepath.Join(dir, "zh", "comment.md")
	if got != want {
		t.Errorf("ResolveCommentPath = %q, want %q", got, want)
	}
}

func TestResolveCommentPath_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "comment.md"), []byte("root comment"), 0o644)

	got := ResolveCommentPath(dir, "en")
	want := filepath.Join(dir, "comment.md")
	if got != want {
		t.Errorf("ResolveCommentPath fallback to root, got %q, want %q", got, want)
	}
}

func TestValidateCustomDir_OK(t *testing.T) {
	dir := t.TempDir()
	if err := ValidateCustomDir(dir); err != nil {
		t.Errorf("ValidateCustomDir(existing empty dir) = %v, want nil", err)
	}
}

func TestValidateCustomDir_Missing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := ValidateCustomDir(missing); err == nil {
		t.Errorf("ValidateCustomDir(missing) = nil, want error")
	}
}

func TestValidateCustomDir_IsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir")
	os.WriteFile(filePath, []byte("x"), 0o644)
	if err := ValidateCustomDir(filePath); err == nil {
		t.Errorf("ValidateCustomDir(file) = nil, want error")
	}
}

func TestProjectLocalRecipeDir_Present(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".lingtai-recipe")
	os.MkdirAll(local, 0o755)

	got := ProjectLocalRecipeDir(root)
	if got != local {
		t.Errorf("ProjectLocalRecipeDir = %q, want %q", got, local)
	}
}

func TestProjectLocalRecipeDir_Absent(t *testing.T) {
	root := t.TempDir()
	got := ProjectLocalRecipeDir(root)
	if got != "" {
		t.Errorf("ProjectLocalRecipeDir = %q, want empty", got)
	}
}

func TestProjectLocalRecipeDir_IsFile(t *testing.T) {
	root := t.TempDir()
	fakeFile := filepath.Join(root, ".lingtai-recipe")
	os.WriteFile(fakeFile, []byte("x"), 0o644)

	got := ProjectLocalRecipeDir(root)
	if got != "" {
		t.Errorf("ProjectLocalRecipeDir(file) = %q, want empty", got)
	}
}

func TestLangFallbackChain(t *testing.T) {
	tests := []struct {
		lang string
		want []string
	}{
		{"wen", []string{"wen", ""}},
		{"zh", []string{"zh", ""}},
		{"en", []string{"en", ""}},
		{"", []string{""}},
		{"fr", []string{"fr", ""}},
	}
	for _, tt := range tests {
		got := langFallbackChain(tt.lang)
		if len(got) != len(tt.want) {
			t.Errorf("langFallbackChain(%q) len = %d, want %d", tt.lang, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("langFallbackChain(%q)[%d] = %q, want %q", tt.lang, i, got[i], tt.want[i])
			}
		}
	}
}

func TestResolveGreetPath_FallsBackToRoot(t *testing.T) {
	dir := t.TempDir()
	// Only root exists — wen user gets root, not zh or en
	os.WriteFile(filepath.Join(dir, "greet.md"), []byte("root greet"), 0o644)

	got := ResolveGreetPath(dir, "wen")
	want := filepath.Join(dir, "greet.md")
	if got != want {
		t.Errorf("ResolveGreetPath wen→root fallback, got %q, want %q", got, want)
	}
}

func TestResolveSkillDir_FallsBackToRoot(t *testing.T) {
	dir := t.TempDir()
	// Only root SKILL.md — wen user gets root
	rootDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(rootDir, 0o755)
	os.WriteFile(filepath.Join(rootDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "wen")
	if got != rootDir {
		t.Errorf("ResolveSkillDir wen→root fallback = %q, want %q", got, rootDir)
	}
}

func TestResolveSkillDir_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill", "en")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "en")
	if got != skillDir {
		t.Errorf("ResolveSkillDir lang-specific = %q, want %q", got, skillDir)
	}
}

func TestResolveSkillDir_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "zh")
	if got != skillDir {
		t.Errorf("ResolveSkillDir fallback = %q, want %q", got, skillDir)
	}
}

func TestResolveSkillDir_NoMatch(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "my-skill"), 0o755)

	got := ResolveSkillDir(dir, "my-skill", "en")
	if got != "" {
		t.Errorf("ResolveSkillDir no match = %q, want empty", got)
	}
}

func TestResolveSkillDir_EmptyRecipeDir(t *testing.T) {
	got := ResolveSkillDir("", "my-skill", "en")
	if got != "" {
		t.Errorf("ResolveSkillDir empty recipeDir = %q, want empty", got)
	}
}

func TestResolveSkillDir_EmptyLang(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0o644)

	got := ResolveSkillDir(dir, "my-skill", "")
	if got != skillDir {
		t.Errorf("ResolveSkillDir empty lang = %q, want %q", got, skillDir)
	}
}

func TestLoadRecipeInfo_Valid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Test Recipe","description":"A test"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "en")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Test Recipe" {
		t.Errorf("Name = %q, want %q", info.Name, "Test Recipe")
	}
	if info.Description != "A test" {
		t.Errorf("Description = %q, want %q", info.Description, "A test")
	}
}

func TestLoadRecipeInfo_LangSpecific(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Root","description":"root"}`), 0o644)
	os.MkdirAll(filepath.Join(dir, "zh"), 0o755)
	os.WriteFile(filepath.Join(dir, "zh", "recipe.json"), []byte(`{"name":"中文名","description":"中文描述"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "zh")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "中文名" {
		t.Errorf("Name = %q, want %q", info.Name, "中文名")
	}
}

func TestLoadRecipeInfo_FallbackToRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Root Name","description":"root"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "wen")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Root Name" {
		t.Errorf("Name = %q, want %q", info.Name, "Root Name")
	}
}

func TestLoadRecipeInfo_Missing(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadRecipeInfo(dir, "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error when recipe.json missing")
	}
}

func TestLoadRecipeInfo_EmptyName(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"","description":"has desc"}`), 0o644)

	_, err := LoadRecipeInfo(dir, "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error when name is empty")
	}
}

func TestLoadRecipeInfo_ExtraFieldsIgnored(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "recipe.json"), []byte(`{"name":"Test","description":"d","version":"1.0","author":"me"}`), 0o644)

	info, err := LoadRecipeInfo(dir, "en")
	if err != nil {
		t.Fatalf("LoadRecipeInfo error: %v", err)
	}
	if info.Name != "Test" {
		t.Errorf("Name = %q, want %q", info.Name, "Test")
	}
}

func TestLoadRecipeInfo_EmptyDir(t *testing.T) {
	_, err := LoadRecipeInfo("", "en")
	if err == nil {
		t.Errorf("LoadRecipeInfo should error on empty dir")
	}
}
