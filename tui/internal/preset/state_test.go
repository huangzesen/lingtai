package preset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRecipeState_Missing(t *testing.T) {
	lingtaiDir := t.TempDir()
	got, err := LoadRecipeState(lingtaiDir)
	if err != nil {
		t.Fatalf("LoadRecipeState(missing) err = %v, want nil", err)
	}
	if got.Recipe != "" || got.CustomDir != "" {
		t.Errorf("LoadRecipeState(missing) = %+v, want zero value", got)
	}
}

func TestSaveAndLoadRecipeState_Bundled(t *testing.T) {
	lingtaiDir := t.TempDir()

	state := RecipeState{Recipe: RecipeGreeter}
	if err := SaveRecipeState(lingtaiDir, state); err != nil {
		t.Fatalf("SaveRecipeState err = %v", err)
	}

	got, err := LoadRecipeState(lingtaiDir)
	if err != nil {
		t.Fatalf("LoadRecipeState err = %v", err)
	}
	if got.Recipe != RecipeGreeter {
		t.Errorf("Recipe = %q, want %q", got.Recipe, RecipeGreeter)
	}
	if got.CustomDir != "" {
		t.Errorf("CustomDir = %q, want empty", got.CustomDir)
	}
}

func TestSaveAndLoadRecipeState_Custom(t *testing.T) {
	lingtaiDir := t.TempDir()

	state := RecipeState{Recipe: RecipeCustom, CustomDir: "/some/path"}
	if err := SaveRecipeState(lingtaiDir, state); err != nil {
		t.Fatalf("SaveRecipeState err = %v", err)
	}

	got, err := LoadRecipeState(lingtaiDir)
	if err != nil {
		t.Fatalf("LoadRecipeState err = %v", err)
	}
	if got.Recipe != RecipeCustom {
		t.Errorf("Recipe = %q, want %q", got.Recipe, RecipeCustom)
	}
	if got.CustomDir != "/some/path" {
		t.Errorf("CustomDir = %q, want /some/path", got.CustomDir)
	}
}

func TestSaveRecipeState_CreatesTuiAssetDir(t *testing.T) {
	lingtaiDir := t.TempDir()
	// Pre-condition: .tui-asset/ does not exist
	tuiAsset := filepath.Join(lingtaiDir, ".tui-asset")
	if _, err := os.Stat(tuiAsset); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: .tui-asset/ should not exist")
	}

	if err := SaveRecipeState(lingtaiDir, RecipeState{Recipe: RecipePlain}); err != nil {
		t.Fatalf("SaveRecipeState err = %v", err)
	}

	// .tui-asset/ should now exist
	if _, err := os.Stat(tuiAsset); err != nil {
		t.Errorf(".tui-asset/ not created: %v", err)
	}
	// .recipe file inside
	if _, err := os.Stat(filepath.Join(tuiAsset, ".recipe")); err != nil {
		t.Errorf(".recipe file not created: %v", err)
	}
}

func TestLoadRecipeState_MalformedJSON(t *testing.T) {
	lingtaiDir := t.TempDir()
	tuiAsset := filepath.Join(lingtaiDir, ".tui-asset")
	os.MkdirAll(tuiAsset, 0o755)
	os.WriteFile(filepath.Join(tuiAsset, ".recipe"), []byte("{not json"), 0o644)

	_, err := LoadRecipeState(lingtaiDir)
	if err == nil {
		t.Errorf("LoadRecipeState(malformed) err = nil, want error")
	}
}

func TestBootstrap_CreatesRecipeDirs(t *testing.T) {
	globalDir := t.TempDir()
	if err := Bootstrap(globalDir); err != nil {
		t.Fatalf("Bootstrap err = %v", err)
	}
	// All four bundled recipes should have directories
	for _, name := range BundledRecipes() {
		dir := RecipeDir(globalDir, name)
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("recipe dir %s not created: %v", name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("recipe dir %s is not a directory", name)
		}
	}
	// Greeter should have a greet.md resolvable in en
	greet := ResolveGreetPath(RecipeDir(globalDir, RecipeGreeter), "en")
	if greet == "" {
		t.Errorf("greeter/en/greet.md not resolvable after Bootstrap")
	}
}
