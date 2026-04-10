// Package secretary provides the embedded recipe and launch logic for the
// secretary agent — a background utility that consolidates session history
// into brief files consumed by other agents.
//
// The secretary recipe is internal infrastructure, not a user-selectable
// recipe. It is never shown in the recipe picker or bootstrapped to
// ~/.lingtai-tui/recipes/.
package secretary

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:assets
var assetsFS embed.FS

// ProjectDir returns the secretary's project root directory:
// ~/.lingtai-tui/secretary/
func ProjectDir(globalDir string) string {
	return filepath.Join(globalDir, "secretary")
}

// LingtaiDir returns the secretary's .lingtai directory:
// ~/.lingtai-tui/secretary/.lingtai/
func LingtaiDir(globalDir string) string {
	return filepath.Join(globalDir, "secretary", ".lingtai")
}

// AgentDir returns the secretary agent's working directory:
// ~/.lingtai-tui/secretary/.lingtai/secretary/
func AgentDir(globalDir string) string {
	return filepath.Join(globalDir, "secretary", ".lingtai", "secretary")
}

// RecipeDir returns a temporary directory populated with the embedded recipe
// assets. The caller should use this path as the recipe directory when
// setting up the secretary agent. The returned directory is inside the
// secretary's working directory so it persists across launches.
func RecipeDir(globalDir string) (string, error) {
	recipeDir := filepath.Join(ProjectDir(globalDir), "recipe")
	if err := populateAssets(recipeDir); err != nil {
		return "", err
	}
	return recipeDir, nil
}

// GreetContent returns the raw greet.md content for the secretary.
func GreetContent() string {
	data, err := assetsFS.ReadFile("assets/greet.md")
	if err != nil {
		return ""
	}
	return string(data)
}

// CommentPath returns the path to the comment.md file after populating assets.
func CommentPath(globalDir string) string {
	return filepath.Join(ProjectDir(globalDir), "recipe", "comment.md")
}

// CovenantPath returns the path to the covenant.md file after populating assets.
func CovenantPath(globalDir string) string {
	return filepath.Join(ProjectDir(globalDir), "recipe", "covenant.md")
}

// SkillDir returns the path to the briefing skill directory after populating assets.
func SkillDir(globalDir string) string {
	return filepath.Join(ProjectDir(globalDir), "recipe", "skills", "briefing")
}

// populateAssets writes the embedded recipe files to disk so the kernel can
// read them via file paths. Files are always overwritten to ensure the latest
// version is on disk after a TUI upgrade.
func populateAssets(targetDir string) error {
	return fs.WalkDir(assetsFS, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the "assets/" prefix to get the relative path
		rel, err := filepath.Rel("assets", path)
		if err != nil {
			return err
		}
		target := filepath.Join(targetDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
