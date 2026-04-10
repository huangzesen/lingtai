package migrate

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateLegacyAddonsWarn prints a one-time warning if the deprecated
// ~/.lingtai-tui/addons/ directory still exists. Addon configs now live
// at .lingtai/.addons/ inside each project. This migration does not
// delete anything — it just tells the user to clean up and re-setup.
func migrateLegacyAddonsWarn(_ string) error {
	globalDir := globalTUIDir()
	if globalDir == "" {
		return nil
	}
	legacyAddons := filepath.Join(globalDir, "addons")
	fi, err := os.Stat(legacyAddons)
	if err != nil || !fi.IsDir() {
		return nil
	}

	fmt.Println()
	fmt.Println("⚠ Legacy addon folder detected: ~/.lingtai-tui/addons/")
	fmt.Println()
	fmt.Println("  Addon configs have moved to .lingtai/.addons/ inside each project.")
	fmt.Println("  Please remove the old folder:")
	fmt.Println()
	fmt.Printf("    rm -rf %s\n", legacyAddons)
	fmt.Println()
	fmt.Println("  Then ask your agent to re-setup any addons (IMAP, Telegram,")
	fmt.Println("  Feishu) — it knows the setup skills and will create the")
	fmt.Println("  config in the right place.")
	fmt.Println()

	return nil
}
