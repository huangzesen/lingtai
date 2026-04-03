package migrate

// migrateTapeNormalize is now a no-op.
// Tape normalization (null → [], backfill direct/cc/bcc) is handled by
// full reconstruction via ReconstructTape at portal startup when the tape
// is missing or uses the old format.
func migrateTapeNormalize(_ string) error {
	return nil
}
