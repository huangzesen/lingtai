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

	state := RecipeState{Recipe: "greeter"}
	if err := SaveRecipeState(lingtaiDir, state); err != nil {
		t.Fatalf("SaveRecipeState err = %v", err)
	}

	got, err := LoadRecipeState(lingtaiDir)
	if err != nil {
		t.Fatalf("LoadRecipeState err = %v", err)
	}
	if got.Recipe != "greeter" {
		t.Errorf("Recipe = %q, want %q", got.Recipe, "greeter")
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

	if err := SaveRecipeState(lingtaiDir, RecipeState{Recipe: "plain"}); err != nil {
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
	for _, cat := range RecipeCategories {
		recipes := ScanCategory(globalDir, cat, "en")
		if len(recipes) == 0 {
			t.Errorf("ScanCategory(%s) returned no recipes", cat)
		}
		for _, r := range recipes {
			if _, err := os.Stat(r.Dir); err != nil {
				t.Errorf("recipe dir %s not created: %v", r.Dir, err)
			}
		}
	}
	greetDir := RecipeDir(globalDir, "greeter")
	if greetDir == "" {
		t.Fatalf("RecipeDir(greeter) not found after Bootstrap")
	}
	greet := ResolveGreetPath(greetDir, "en")
	if greet == "" {
		t.Errorf("greeter/en/greet.md not resolvable after Bootstrap")
	}
}
