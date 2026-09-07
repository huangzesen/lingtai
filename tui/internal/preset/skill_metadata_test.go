package preset

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"
)

var lastChangedAtRE = regexp.MustCompile(`(?m)^last_changed_at:\s*"?([^"\n]+)"?\s*$`)

func TestBundledSkillsHaveLastChangedAt(t *testing.T) {
	count := 0
	err := fs.WalkDir(skillsFS, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "/SKILL.md") {
			return nil
		}
		count++
		data, err := fs.ReadFile(skillsFS, path)
		if err != nil {
			return err
		}
		body := string(data)
		if !strings.HasPrefix(body, "---\n") {
			t.Errorf("%s missing YAML frontmatter", path)
			return nil
		}
		end := strings.Index(body[4:], "\n---")
		if end < 0 {
			t.Errorf("%s missing closing YAML frontmatter delimiter", path)
			return nil
		}
		frontmatter := body[:4+end]
		match := lastChangedAtRE.FindStringSubmatch(frontmatter)
		if match == nil {
			t.Errorf("%s missing last_changed_at frontmatter", path)
			return nil
		}
		value := strings.TrimSpace(match[1])
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Errorf("%s has invalid last_changed_at %q: %v", path, value, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 73 {
		t.Fatalf("checked %d bundled skill SKILL.md files, want 73", count)
	}
}
