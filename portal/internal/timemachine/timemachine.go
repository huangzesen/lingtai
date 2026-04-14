package timemachine

import (
	"fmt"
	"os/exec"
	"time"
)

const (
	snapshotInterval = 5 * time.Minute
	thinInterval     = 1 * time.Hour
	maxFileSize  int64 = 10 * 1024 * 1024       // 10MB
	maxRepoSize  int64 = 2 * 1024 * 1024 * 1024  // 2GB
)

// Start launches the time machine background goroutine. It initializes
// the git repo if needed and takes snapshots every 5 minutes. Returns
// a stop function that blocks until the goroutine exits.
//
// If git is not installed, prints a warning and returns a no-op stop.
func Start(lingtaiDir string) func() {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Println("  time machine: git not found, skipping")
		return func() {}
	}

	if err := initGit(lingtaiDir); err != nil {
		fmt.Printf("  time machine: init failed: %v\n", err)
		return func() {}
	}

	done := make(chan struct{})
	quit := make(chan struct{})

	go func() {
		defer close(done)

		snapshotTicker := time.NewTicker(snapshotInterval)
		defer snapshotTicker.Stop()

		lastThin := time.Now()

		for {
			select {
			case <-quit:
				// Final snapshot before exit
				scanLargeFiles(lingtaiDir, maxFileSize)
				snapshot(lingtaiDir)
				return
			case <-snapshotTicker.C:
				scanLargeFiles(lingtaiDir, maxFileSize)
				committed, _ := snapshot(lingtaiDir)

				if committed && time.Since(lastThin) >= thinInterval {
					commits, err := listCommits(lingtaiDir)
					if err == nil && len(commits) > 0 {
						now := time.Now()
						keepers := selectKeepers(commits, now)
						thinHistory(lingtaiDir, keepers)
						enforceSizeCap(lingtaiDir, maxRepoSize)
						lastThin = now
					}
				}
			}
		}
	}()

	return func() {
		close(quit)
		<-done
	}
}
