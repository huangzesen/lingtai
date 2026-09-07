package headless

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/lingtai-tui/internal/preset"
)

func TestRunPresetRevisionModesAndExplicitOutput(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "preset.json")
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"old"}}}`)
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := revisionCLIManifest(input)
	manifestPath := filepath.Join(tmp, "manifest.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(tmp, "no-global-state"))
	t.Setenv("USERPROFILE", filepath.Join(tmp, "no-global-state"))

	var dryOut, dryErr bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputPath, "--mode", "dry-run"}, &dryOut, &dryErr); code != 0 {
		t.Fatalf("dry-run code=%d stderr=%s", code, dryErr.String())
	}
	if !strings.Contains(dryOut.String(), `"changed": true`) || dryErr.Len() != 0 {
		t.Fatalf("dry-run output=%q stderr=%q", dryOut.String(), dryErr.String())
	}
	if !strings.Contains(dryOut.String(), `"target_state": "revise"`) {
		t.Fatalf("dry-run did not expose target state: %s", dryOut.String())
	}

	var checkOut bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputPath, "--mode", "check"}, &checkOut, &bytes.Buffer{}); code != 1 {
		t.Fatalf("check code=%d output=%s, want 1 for diff", code, checkOut.String())
	}

	outputDir := filepath.Join(tmp, "new-output")
	var applyOut bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputPath, "--mode", "apply", "--output-dir", outputDir}, &applyOut, &bytes.Buffer{}); code != 0 {
		t.Fatalf("apply code=%d output=%s", code, applyOut.String())
	}
	got, err := os.ReadFile(filepath.Join(outputDir, "preset.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"model":"new"`)) || bytes.Contains(input, []byte(`"model":"new"`)) {
		t.Fatalf("apply changed wrong content: output=%s input=%s", got, input)
	}
	var cleanOut bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", outputDir, "--mode", "check"}, &cleanOut, &bytes.Buffer{}); code != 0 || strings.Contains(cleanOut.String(), `"changed": true`) {
		t.Fatalf("checking applied output code=%d output=%s", code, cleanOut.String())
	}
}

func TestRunPresetRevisionReportsUnsupportedTargetWithoutDisappearing(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "preset.json")
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"old"}}}`)
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := revisionCLIManifest(input)
	manifest.Targets[0].State = preset.TargetStateUnsupported
	manifest.Targets[0].Reason = "provider-resolved model is not represented by this revision contract"
	manifest.Targets[0].Models = nil
	manifest.Targets[0].Changes = nil
	manifest.InputContract.PostImageSHA256 = sha256Hex(input)
	manifestPath := filepath.Join(tmp, "manifest.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputPath, "--mode", "check"}, &stdout, &stderr); code != preset.ExitOK {
		t.Fatalf("unsupported check code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"target_state": "unsupported"`, `"target_reason": "provider-resolved model is not represented by this revision contract"`, `"changed": false`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("unsupported result missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunPresetRevisionRejectsOutputForReadOnlyMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", "manifest.json", "--input", "input.json", "--mode", "dry-run", "--output-dir", "out"}, &stdout, &stderr); code != preset.ExitMalformed {
		t.Fatalf("code=%d, want %d; stderr=%s", code, preset.ExitMalformed, stderr.String())
	}
	if stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunPresetRevisionAppliesDirectoryBundleWithoutChangingInput(t *testing.T) {
	inputRoot := t.TempDir()
	docDir := filepath.Join(inputRoot, "nested")
	if err := os.Mkdir(docDir, 0o755); err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"old"}}}`)
	if err := os.WriteFile(filepath.Join(docDir, "preset.json"), input, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputRoot, "notes.txt"), []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := revisionCLIManifest(input)
	manifest.InputContract.Document = "nested/preset.json"
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	outputRoot := filepath.Join(t.TempDir(), "bundle-output")
	var stdout, stderr bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputRoot, "--mode", "apply", "--output-dir", outputRoot}, &stdout, &stderr); code != preset.ExitOK {
		t.Fatalf("apply code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(outputRoot, "nested", "preset.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"model":"new"`)) {
		t.Fatalf("bundle document was not revised: %s", got)
	}
	notes, err := os.ReadFile(filepath.Join(outputRoot, "notes.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(notes) != "untouched\n" {
		t.Fatalf("bundle sibling changed: %q", notes)
	}
	original, err := os.ReadFile(filepath.Join(docDir, "preset.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, input) {
		t.Fatalf("input bundle was modified: %s", original)
	}
}

func TestRunPresetRevisionRefusesPreExistingOutput(t *testing.T) {
	tmp := t.TempDir()
	inputPath := filepath.Join(tmp, "preset.json")
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"old"}}}`)
	if err := os.WriteFile(inputPath, input, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(tmp, "manifest.json")
	manifestBytes, err := json.Marshal(revisionCLIManifest(input))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(tmp, "existing-output")
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outputDir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := RunPresetRevision([]string{"--manifest", manifestPath, "--input", inputPath, "--mode", "apply", "--output-dir", outputDir}, &stdout, &stderr); code != preset.ExitFilesystem {
		t.Fatalf("apply code=%d, want %d; stdout=%s stderr=%s", code, preset.ExitFilesystem, stdout.String(), stderr.String())
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("pre-existing output changed: %q", got)
	}
}

func revisionCLIManifest(input []byte) preset.Manifest {
	sum := sha256.Sum256([]byte("synthetic evidence"))
	inputSum := sha256.Sum256(input)
	return preset.Manifest{
		SchemaVersion: "lingtai.preset.revision/v1",
		Policy:        preset.RevisionPolicy{RetainGenerations: 2, ParserVersion: "parser-v1"},
		Evidence: []preset.Evidence{{
			ID: "evidence-public-api", URL: "https://example.test/public-api", SourceType: "synthetic", Scope: preset.EvidenceScopePublicAPI,
			RetrievedAt: "2026-09-01T00:00:00Z", FreshUntil: "2026-12-01T00:00:00Z", Content: "synthetic evidence", ContentSHA256: hex.EncodeToString(sum[:]), ParserVersion: "parser-v1", Status: preset.EvidencePositive, Claims: []string{"model metadata"},
		}},
		Targets: []preset.RevisionTarget{{
			CanonicalName: "custom", ProviderID: "custom", State: preset.TargetStateRevise, Route: preset.RouteContract{API: "responses", Transport: "http", Scope: preset.EvidenceScopePublicAPI, Binding: preset.RouteBinding{Mode: preset.RouteBindingProviderChild, Provider: &preset.RouteFieldBinding{Pointer: "/manifest/llm/provider", Expected: "custom"}}},
			Responses: &preset.ResponsesSemantics{
				InputModalities: []string{"text", "image", "file"}, ReasoningVocabulary: []string{"default", "none", "minimal", "low", "medium", "high", "xhigh"}, ReasoningDefault: "default", ReasoningOmissionMeansDefault: true,
				ServiceTierRequested: []string{"normal", "fast"}, ServiceTierObserved: []string{"normal", "fast"},
			},
			Models: []preset.ModelRecord{
				{ID: "old", Family: "family", Generation: 1, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: preset.CapabilityFacts{TextInput: preset.CapabilitySupported, ImageInput: preset.CapabilityUnknown, FileInput: preset.CapabilityUnknown, TUIWiring: preset.CapabilityUnknown, Runtime: preset.CapabilityUnknown}},
				{ID: "new", Family: "family", Generation: 2, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: preset.CapabilityFacts{TextInput: preset.CapabilitySupported, ImageInput: preset.CapabilityUnknown, FileInput: preset.CapabilityUnknown, TUIWiring: preset.CapabilityUnknown, Runtime: preset.CapabilityUnknown}},
			},
			Changes: []preset.RevisionChange{{Path: "/manifest/llm/model", Kind: preset.ChangeReplace, ExpectedOld: json.RawMessage(`"old"`), NewValue: json.RawMessage(`"new"`), EvidenceRef: "evidence-public-api", Reason: "synthetic revision"}},
		}},
		InputContract: preset.InputContract{Document: "preset.json", InputSHA256: hex.EncodeToString(inputSum[:]), PostImageSHA256: sha256Hex(bytes.Replace(input, []byte(`"model":"old"`), []byte(`"model":"new"`), 1)), NamePointer: "/name", ProviderPointer: "/manifest/llm/provider", OwnedPaths: []string{"/manifest/llm/model"}},
	}
}
