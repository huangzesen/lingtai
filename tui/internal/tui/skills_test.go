package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkills_FollowsSymlinks(t *testing.T) {
	targetDir := t.TempDir()
	os.WriteFile(filepath.Join(targetDir, "SKILL.md"), []byte("---\nname: symlinked-skill\ndescription: A symlinked skill\nversion: 1.0.0\n---\nBody here.\n"), 0o644)

	skillsDir := filepath.Join(t.TempDir(), ".skills")
	os.MkdirAll(skillsDir, 0o755)
	os.Symlink(targetDir, filepath.Join(skillsDir, "test-skill-en"))

	regularDir := filepath.Join(skillsDir, "regular-skill")
	os.MkdirAll(regularDir, 0o755)
	os.WriteFile(filepath.Join(regularDir, "SKILL.md"), []byte("---\nname: regular-skill\ndescription: A regular skill\nversion: 1.0.0\n---\nBody.\n"), 0o644)

	skills, problems := scanSkills(skillsDir)
	if len(problems) != 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	names := []string{skills[0].Name, skills[1].Name}
	if names[0] != "regular-skill" || names[1] != "symlinked-skill" {
		t.Errorf("skill names = %v, want [regular-skill, symlinked-skill]", names)
	}
}

func TestScanSkills_SkipsBrokenSymlinks(t *testing.T) {
	skillsDir := filepath.Join(t.TempDir(), ".skills")
	os.MkdirAll(skillsDir, 0o755)

	os.Symlink("/nonexistent", filepath.Join(skillsDir, "broken-skill"))

	skills, problems := scanSkills(skillsDir)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
	if len(problems) != 0 {
		t.Errorf("expected 0 problems, got %d", len(problems))
	}
}
