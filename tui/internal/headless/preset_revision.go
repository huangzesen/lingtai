package headless

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/lingtai-tui/internal/preset"
)

type presetRevisionOptions struct {
	Manifest  string
	Input     string
	Mode      string
	OutputDir string
}

type presetRevisionOutput struct {
	SchemaVersion  string                     `json:"schema_version"`
	Mode           string                     `json:"mode"`
	InputSHA256    string                     `json:"input_sha256"`
	ManifestSHA256 string                     `json:"manifest_sha256"`
	TargetState    preset.TargetState         `json:"target_state"`
	TargetReason   string                     `json:"target_reason,omitempty"`
	Status         string                     `json:"status"`
	Changed        bool                       `json:"changed"`
	Plan           []preset.RevisionPlanEntry `json:"plan"`
	Diagnostics    []string                   `json:"diagnostics"`
}

// RunPresetRevision is the thin, headless adapter for the pure preset
// revision engine. It reads only the two explicit input paths and never calls
// preset.Bootstrap, global config, authentication, or a provider.
func RunPresetRevision(args []string, stdout, stderr io.Writer) int {
	opts, err := parsePresetRevisionOptions(args, stderr)
	if err != nil {
		return writeRevisionError(stderr, preset.ExitMalformed, err)
	}
	manifestBytes, err := os.ReadFile(opts.Manifest)
	if err != nil {
		return writeRevisionError(stderr, preset.ExitFilesystem, fmt.Errorf("read revision manifest: %w", err))
	}
	manifest, err := preset.DecodeRevisionManifest(manifestBytes)
	if err != nil {
		return writeRevisionError(stderr, preset.RevisionErrorCode(err), err)
	}
	inputBytes, logicalPath, err := readRevisionInput(opts.Input, manifest.InputContract.Document)
	if err != nil {
		if _, ok := err.(revisionInputContractError); ok {
			return writeRevisionError(stderr, preset.ExitMalformed, err)
		}
		return writeRevisionError(stderr, preset.ExitFilesystem, err)
	}
	plan, err := preset.PlanRevision(manifest, inputBytes, preset.RevisionOptions{
		Now:         time.Now().UTC(),
		LogicalPath: logicalPath,
	})
	if err != nil {
		return writeRevisionError(stderr, preset.RevisionErrorCode(err), err)
	}
	result := presetRevisionOutput{
		SchemaVersion:  "lingtai.preset.revision/result/v1",
		Mode:           opts.Mode,
		InputSHA256:    plan.InputSHA256,
		ManifestSHA256: sha256Hex(manifestBytes),
		TargetState:    plan.TargetState,
		TargetReason:   plan.TargetReason,
		Status:         "unchanged",
		Changed:        plan.Changed,
		Plan:           plan.Entries,
		Diagnostics:    []string{},
	}
	if plan.Changed {
		result.Status = "changed"
	}
	if opts.Mode == "apply" {
		if err := preset.MaterializeRevisionBundle(opts.Input, opts.OutputDir, plan); err != nil {
			return writeRevisionError(stderr, preset.RevisionErrorCode(err), err)
		}
		result.Status = "applied"
	}
	if err := WriteJSON(stdout, result); err != nil {
		return writeRevisionError(stderr, preset.ExitFilesystem, fmt.Errorf("write revision result: %w", err))
	}
	if opts.Mode == "check" && plan.Changed {
		return preset.ExitChanged
	}
	return preset.ExitOK
}

func parsePresetRevisionOptions(args []string, stderr io.Writer) (presetRevisionOptions, error) {
	var opts presetRevisionOptions
	flags := flag.NewFlagSet("presets revise", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.Manifest, "manifest", "", "")
	flags.StringVar(&opts.Input, "input", "", "")
	flags.StringVar(&opts.Mode, "mode", "", "")
	flags.StringVar(&opts.OutputDir, "output-dir", "", "")
	if err := flags.Parse(args); err != nil {
		return presetRevisionOptions{}, err
	}
	if len(flags.Args()) != 0 {
		return presetRevisionOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}
	if opts.Manifest == "" || opts.Input == "" || opts.Mode == "" {
		return presetRevisionOptions{}, fmt.Errorf("--manifest, --input, and --mode are required")
	}
	if opts.Mode != "dry-run" && opts.Mode != "check" && opts.Mode != "apply" {
		return presetRevisionOptions{}, fmt.Errorf("--mode must be dry-run, check, or apply")
	}
	if opts.Mode == "apply" && opts.OutputDir == "" {
		return presetRevisionOptions{}, fmt.Errorf("--output-dir is required for apply")
	}
	if opts.Mode != "apply" && opts.OutputDir != "" {
		return presetRevisionOptions{}, fmt.Errorf("--output-dir is only valid for apply")
	}
	return opts, nil
}

func readRevisionInput(path, document string) ([]byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("stat revision input: %w", err)
	}
	if !info.IsDir() {
		logical := filepath.ToSlash(filepath.Base(path))
		if document != logical {
			return nil, "", revisionInputContractError{fmt.Errorf("manifest document %q does not match input file %q", document, logical)}
		}
		data, err := os.ReadFile(path)
		return data, logical, err
	}
	if document == "" || filepath.IsAbs(document) {
		return nil, "", revisionInputContractError{fmt.Errorf("bundle document must be a relative path")}
	}
	clean := filepath.Clean(filepath.FromSlash(document))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, "", revisionInputContractError{fmt.Errorf("bundle document escapes input directory")}
	}
	data, err := os.ReadFile(filepath.Join(path, clean))
	return data, filepath.ToSlash(clean), err
}

type revisionInputContractError struct{ err error }

func (e revisionInputContractError) Error() string { return e.err.Error() }

func writeRevisionError(stderr io.Writer, code int, err error) int {
	name := "revision_error"
	switch code {
	case preset.ExitMalformed:
		name = "malformed_revision"
	case preset.ExitConflict:
		name = "revision_conflict"
	case preset.ExitFilesystem:
		name = "revision_filesystem"
	}
	if writeErr := WriteError(stderr, err.Error(), name); writeErr != nil {
		return preset.ExitFilesystem
	}
	return code
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
