package timemachine

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type commitInfo struct {
	hash string
	time time.Time
}

const maxSnapshots = 100

// selectKeepers decides which commits to retain based on retention buckets.
// Input commits must be sorted oldest-first. Returns kept hashes oldest-first.
//
// Retention buckets:
//   - 0–2 hours:  keep all (every 5 min)
//   - 2–24 hours: keep 1 per 30 min
//   - 1–7 days:   keep 1 per 6 hours
//   - 7+ days:    keep 1 per day
//
// Hard cap: 100 snapshots. If over, drop oldest.
func selectKeepers(commits []commitInfo, now time.Time) []string {
	if len(commits) == 0 {
		return nil
	}

	kept := make(map[string]bool)
	windows := make(map[string]bool) // track which time windows are already filled

	// Always keep newest and oldest
	kept[commits[0].hash] = true
	kept[commits[len(commits)-1].hash] = true

	// Iterate newest-first to keep the newest commit per window
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		age := now.Sub(c.time)
		var interval time.Duration

		switch {
		case age <= 2*time.Hour:
			interval = 0 // keep all
		case age <= 24*time.Hour:
			interval = 30 * time.Minute
		case age <= 7*24*time.Hour:
			interval = 6 * time.Hour
		default:
			interval = 24 * time.Hour
		}

		if interval == 0 {
			kept[c.hash] = true
			continue
		}

		// Quantize to interval window — keep newest commit per window
		window := c.time.Truncate(interval)
		windowKey := fmt.Sprintf("%d-%s", interval, window.Format(time.RFC3339))
		if !windows[windowKey] {
			windows[windowKey] = true
			kept[c.hash] = true
		}
	}

	// Collect kept hashes in original order (oldest-first)
	var result []string
	for _, c := range commits {
		if kept[c.hash] {
			result = append(result, c.hash)
		}
	}

	// Enforce hard cap — drop oldest if over 100
	if len(result) > maxSnapshots {
		result = result[len(result)-maxSnapshots:]
	}

	return result
}

// listCommits returns all commits oldest-first.
func listCommits(lingtaiDir string) ([]commitInfo, error) {
	cmd := exec.Command("git", "log", "--format=%H %aI", "--reverse")
	cmd.Dir = lingtaiDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []commitInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		t, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			continue
		}
		commits = append(commits, commitInfo{hash: parts[0], time: t})
	}
	return commits, nil
}

// thinHistory removes commits not in the keepers list by rebuilding the
// commit chain. Only runs if there are commits to remove.
func thinHistory(lingtaiDir string, keepers []string) error {
	if len(keepers) == 0 {
		return nil
	}

	keepSet := make(map[string]bool, len(keepers))
	for _, h := range keepers {
		keepSet[h] = true
	}

	// Get all commits to check if thinning is needed
	all, err := listCommits(lingtaiDir)
	if err != nil {
		return err
	}

	// Count how many to remove
	removeCount := 0
	for _, c := range all {
		if !keepSet[c.hash] {
			removeCount++
		}
	}
	if removeCount == 0 {
		return nil
	}

	// Detect current branch name before rebuilding
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = lingtaiDir
	branchOut, err := branchCmd.Output()
	if err != nil {
		return fmt.Errorf("detect branch: %w", err)
	}
	origBranch := strings.TrimSpace(string(branchOut))
	if origBranch == "" || origBranch == "HEAD" {
		origBranch = "main" // fallback
	}

	// Create an orphan branch, replay kept commits, replace original branch.
	// On any error, recover by checking out the original branch.
	if err := git(lingtaiDir, "checkout", "--orphan", "tm-rebuild"); err != nil {
		return fmt.Errorf("checkout orphan: %w", err)
	}

	// Remove all staged files from the orphan
	git(lingtaiDir, "rm", "-rf", "--cached", ".")

	// Replay each kept commit
	replayErr := func() error {
		for _, hash := range keepers {
			if err := git(lingtaiDir, "checkout", hash, "--", "."); err != nil {
				return fmt.Errorf("checkout tree %s: %w", hash, err)
			}
			if err := git(lingtaiDir, "add", "-A"); err != nil {
				return fmt.Errorf("add: %w", err)
			}

			// Get original commit message and author date
			cmd := exec.Command("git", "log", "-1", "--format=%s%n%aI", hash)
			cmd.Dir = lingtaiDir
			msgOut, err := cmd.Output()
			if err != nil {
				return fmt.Errorf("get message %s: %w", hash, err)
			}
			lines := strings.SplitN(strings.TrimSpace(string(msgOut)), "\n", 2)
			msg := lines[0]
			if msg == "" {
				msg = "snapshot"
			}

			// Preserve original author date
			commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", msg)
			commitCmd.Dir = lingtaiDir
			if len(lines) > 1 {
				commitCmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+lines[1])
			}
			if err := commitCmd.Run(); err != nil {
				return fmt.Errorf("commit replay %s: %w", hash, err)
			}
		}
		return nil
	}()

	if replayErr != nil {
		// Recovery: go back to original branch, delete the failed rebuild
		git(lingtaiDir, "checkout", origBranch)
		git(lingtaiDir, "branch", "-D", "tm-rebuild")
		return replayErr
	}

	// Replace original branch with the rebuilt one
	git(lingtaiDir, "branch", "-D", origBranch)
	git(lingtaiDir, "branch", "-M", "tm-rebuild", origBranch)

	// GC to reclaim space
	git(lingtaiDir, "gc", "--prune=now")

	return nil
}

// repoSizeBytes returns the size of .git/ in bytes using git count-objects.
func repoSizeBytes(lingtaiDir string) (int64, error) {
	cmd := exec.Command("git", "count-objects", "-v")
	cmd.Dir = lingtaiDir
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("git count-objects: %w", err)
	}

	var total int64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "size:") || strings.HasPrefix(line, "size-pack:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
				if err == nil {
					total += val * 1024 // KiB → bytes
				}
			}
		}
	}
	return total, nil
}

// enforceSizeCap aggressively thins if the repo exceeds maxSize bytes.
// Tries at most 5 rounds to avoid blocking the goroutine for too long.
func enforceSizeCap(lingtaiDir string, maxSize int64) {
	for round := 0; round < 5; round++ {
		git(lingtaiDir, "gc", "--aggressive", "--prune=now")

		size, err := repoSizeBytes(lingtaiDir)
		if err != nil || size <= maxSize {
			return
		}

		// Still over — halve the snapshots
		commits, err := listCommits(lingtaiDir)
		if err != nil || len(commits) <= 10 {
			return
		}

		var keepers []string
		for i, c := range commits {
			if i == 0 || i == len(commits)-1 || i%2 == 0 {
				keepers = append(keepers, c.hash)
			}
		}

		if err := thinHistory(lingtaiDir, keepers); err != nil {
			return
		}
	}
}
