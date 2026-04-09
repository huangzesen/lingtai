package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBundledRecipes(t *testing.T) {
	got := BundledRecipes()
	want := []string{RecipeAdaptive, RecipeGreeter, RecipePlain, RecipeTutorial}
	if len(got) != len(want) {
		t.Fatalf("BundledRecipes len = %d, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("BundledRecipes()[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestRecipeDir(t *testing.T) {
	got := RecipeDir("/home/user/.lingtai-tui", "greeter")
	want := filepath.Join("/home/user/.lingtai-tui", "recipes", "greeter")
	if got != want {
		t.Errorf("RecipeDir = %q, want %q", got, want)
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
