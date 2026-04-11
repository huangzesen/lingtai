package migrate

// migrateSessionResort is a no-op. Session.jsonl is now rebuilt from
// sources on every TUI startup (RebuildFromSources), so the one-time
// migration is unnecessary.
func migrateSessionResort(_ string) error {
	return nil
}
