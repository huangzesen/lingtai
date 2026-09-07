package preset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func revisionEvidence(content, scope string) Evidence {
	sum := sha256.Sum256([]byte(content))
	return Evidence{
		ID:            "evidence-" + strings.ReplaceAll(scope, "_", "-"),
		URL:           "https://example.test/" + scope,
		SourceType:    "synthetic",
		Scope:         EvidenceScope(scope),
		RetrievedAt:   "2026-09-01T00:00:00Z",
		FreshUntil:    "2026-12-01T00:00:00Z",
		Content:       content,
		ContentSHA256: hex.EncodeToString(sum[:]),
		ParserVersion: "parser-v1",
		Status:        EvidencePositive,
		Claims:        []string{"model metadata"},
	}
}

func revisionManifest(input []byte) Manifest {
	return Manifest{
		SchemaVersion: "lingtai.preset.revision/v1",
		Policy: RevisionPolicy{
			RetainGenerations: 2,
			ParserVersion:     "parser-v1",
		},
		Evidence: []Evidence{revisionEvidence("synthetic evidence", string(EvidenceScopePublicAPI))},
		Targets: []RevisionTarget{{
			CanonicalName: "custom",
			Aliases:       []string{"custom-alias"},
			ProviderID:    "custom",
			State:         TargetStateRevise,
			Route: RouteContract{
				API:       "responses",
				Transport: "http",
				Scope:     EvidenceScopePublicAPI,
				Binding: RouteBinding{
					Mode:     RouteBindingProviderChild,
					Provider: &RouteFieldBinding{Pointer: "/manifest/llm/provider", Expected: "custom"},
				},
			},
			Responses: revisionGenericResponses(),
			Models: []ModelRecord{
				{ID: "model-old", Family: "family", Generation: 1, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilityUnknown)},
				{ID: "model-current", Family: "family", Generation: 2, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilitySupported)},
				{ID: "model-current-mini", Family: "family", Generation: 2, Variant: "mini", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilityUnsupported)},
			},
			Changes: []RevisionChange{{
				Path:        "/manifest/llm/model",
				Kind:        ChangeReplace,
				ExpectedOld: json.RawMessage(`"model-old"`),
				NewValue:    json.RawMessage(`"model-current"`),
				EvidenceRef: "evidence-public-api",
				Reason:      "select the explicitly evidenced current generation",
			}},
		}},
		InputContract: InputContract{
			Document:        "preset.json",
			InputSHA256:     sha256Hex(input),
			PostImageSHA256: sha256Hex(bytes.Replace(input, []byte(`"model":"model-old"`), []byte(`"model":"model-current"`), 1)),
			NamePointer:     "/name",
			ProviderPointer: "/manifest/llm/provider",
			OwnedPaths:      []string{"/manifest/llm/model"},
		},
	}
}

func revisionCapabilities(image CapabilityState) CapabilityFacts {
	return CapabilityFacts{TextInput: CapabilitySupported, ImageInput: image, FileInput: CapabilityUnknown, TUIWiring: CapabilityUnknown, Runtime: CapabilityUnknown}
}

func revisionGenericResponses() *ResponsesSemantics {
	return &ResponsesSemantics{
		InputModalities:               []string{"text", "image", "file"},
		ReasoningVocabulary:           []string{"default", "none", "minimal", "low", "medium", "high", "xhigh"},
		ReasoningDefault:              "default",
		ReasoningOmissionMeansDefault: true,
		ServiceTierRequested:          []string{"normal", "fast"},
		ServiceTierObserved:           []string{"normal", "fast"},
	}
}

func TestPlanRevisionRejectsMissingSchemaVersion(t *testing.T) {
	manifest := revisionManifest([]byte(`{"name":"custom"}`))
	manifest.SchemaVersion = ""
	_, err := PlanRevision(manifest, []byte(`{"name":"custom"}`), RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err == nil {
		t.Fatal("expected missing schema version to fail")
	}
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d: %v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsInputHashMismatch(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.InputContract.InputSHA256 = sha256Hex([]byte("different input"))
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d: %v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsStaleEvidence(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Evidence[0].FreshUntil = "2026-09-03T00:00:00Z"
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
}

func TestPlanRevisionRequiresExplicitRetirement(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = append(manifest.Targets[0].Models, ModelRecord{
		ID: "model-retired", Family: "family", Generation: 3, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilityUnknown),
	})
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
}

func TestPlanRevisionPreservesOrderAndUnownedFields(t *testing.T) {
	input := []byte(`{"name":"custom-alias","unowned":{"z":1,"a":2},"manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old","unknown":{"keep":true}},"capabilities":{"vision":false}}}`)
	manifest := revisionManifest(input)
	plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err != nil {
		t.Fatalf("PlanRevision: %v", err)
	}
	if !plan.Changed || len(plan.Entries) != 1 {
		t.Fatalf("plan changed=%v entries=%d, want changed with one entry", plan.Changed, len(plan.Entries))
	}
	if got := plan.TargetName; got != "custom" {
		t.Fatalf("target name = %q, want canonical custom", got)
	}
	if got := plan.Entries[0].PresetName; got != "custom" {
		t.Fatalf("plan preset name = %q, want canonical custom", got)
	}
	if got := len(plan.RetainedModels); got != 3 {
		t.Fatalf("retained model count = %d, want all variants across latest two generations", got)
	}
	output, err := ApplyRevision(input, plan)
	if err != nil {
		t.Fatalf("ApplyRevision: %v", err)
	}
	if !bytes.Contains(output, []byte(`"unowned":{"z":1,"a":2}`)) || !bytes.Contains(output, []byte(`"unknown":{"keep":true}`)) {
		t.Fatalf("unowned fields/order were not preserved: %s", output)
	}
	if strings.Index(string(output), `"name"`) > strings.Index(string(output), `"unowned"`) {
		t.Fatalf("top-level field order changed: %s", output)
	}
	if !bytes.Contains(output, []byte(`"model":"model-current"`)) {
		t.Fatalf("owned model was not replaced: %s", output)
	}
	clean, err := PlanRevision(manifest, output, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err != nil {
		t.Fatalf("idempotent PlanRevision: %v", err)
	}
	if clean.Changed || len(clean.Entries) != 0 {
		t.Fatalf("rechecking applied output produced a change: changed=%v entries=%d", clean.Changed, len(clean.Entries))
	}
	if clean.InputSHA256 != manifest.InputContract.PostImageSHA256 {
		t.Fatalf("materialized input hash = %s, want declared post-image %s", clean.InputSHA256, manifest.InputContract.PostImageSHA256)
	}
}

func TestPlanRevisionRejectsHashMismatchedChangedUnownedDocument(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	variant := bytes.Replace(input, []byte(`{"name":"custom",`), []byte(`{"name":"custom","endpoint":"https://unreviewed.example",`), 1)
	variant = bytes.Replace(variant, []byte(`"model":"model-old"`), []byte(`"model":"model-current"`), 1)
	_, err := PlanRevision(revisionManifest(input), variant, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsEmptyRouteBinding(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Route.Binding = RouteBinding{}
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitConflict, err)
	}
}

func TestPlanRevisionRejectsArbitraryMarkerAsRouteBinding(t *testing.T) {
	input := []byte(`{"name":"custom","marker":"route-ok","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Route.Binding = RouteBinding{
		Mode:     RouteBindingMode("marker"),
		Provider: &RouteFieldBinding{Pointer: "/marker", Expected: "route-ok"},
	}
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsProviderChildBindingFromNonProviderField(t *testing.T) {
	input := []byte(`{"name":"custom","marker":"custom","manifest":{"llm":{"provider":"custom","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Route.Binding.Provider.Pointer = "/marker"
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionBindsTypedRouteClaimsToInput(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","api":"responses","transport":"http","scope":"public_api","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Route.Binding = RouteBinding{
		Mode:      RouteBindingDirect,
		API:       &RouteFieldBinding{Pointer: "/manifest/llm/api", Expected: "responses"},
		Transport: &RouteFieldBinding{Pointer: "/manifest/llm/transport", Expected: "http"},
		Scope:     &RouteFieldBinding{Pointer: "/manifest/llm/scope", Expected: "public_api"},
	}
	plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err != nil {
		t.Fatalf("PlanRevision: %v", err)
	}
	if !plan.Changed {
		t.Fatal("typed route claims should not suppress the model revision")
	}
}

func TestPlanRevisionNonRevisionTargetsProduceExplicitUnchangedPlans(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	for _, state := range []TargetState{TargetStateUnsupported, TargetStateNoOp} {
		t.Run(string(state), func(t *testing.T) {
			manifest := revisionManifest(input)
			manifest.Targets[0].State = state
			manifest.Targets[0].Reason = "this named target is intentionally unchanged"
			manifest.Targets[0].Models = nil
			manifest.Targets[0].Changes = nil
			manifest.Targets[0].Route.Binding = RouteBinding{}
			manifest.InputContract.PostImageSHA256 = sha256Hex(input)
			plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
			if err != nil {
				t.Fatalf("PlanRevision: %v", err)
			}
			if plan.Changed || plan.TargetState != state || plan.TargetReason == "" || len(plan.Entries) != 0 {
				t.Fatalf("plan = %#v, want explicit unchanged %q result", plan, state)
			}
		})
	}
}

func TestPlanRevisionRejectsNonRevisionArbitraryPostImage(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].State = TargetStateNoOp
	manifest.Targets[0].Reason = "this named target is intentionally unchanged"
	manifest.Targets[0].Models = nil
	manifest.Targets[0].Changes = nil
	manifest.InputContract.PostImageSHA256 = sha256Hex([]byte("unrelated materialized document"))
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionAllowsSingleGenerationWhenNoModelRevisionIsInScope(t *testing.T) {
	input := []byte(`{"name":"custom","marker":false,"manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = manifest.Targets[0].Models[:1]
	manifest.Targets[0].Changes = []RevisionChange{{Path: "/marker", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`false`), NewValue: json.RawMessage(`true`), EvidenceRef: "evidence-public-api", Reason: "enable the explicitly scoped marker"}}
	manifest.InputContract.OwnedPaths = []string{"/marker"}
	manifest.InputContract.PostImageSHA256 = sha256Hex(bytes.Replace(input, []byte(`"marker":false`), []byte(`"marker":true`), 1))
	if plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"}); err != nil {
		t.Fatalf("PlanRevision rejected non-model single-generation revision: %v", err)
	} else if !plan.Changed {
		t.Fatal("single-generation non-model revision should produce a change")
	}
}

func TestPlanRevisionRejectsOverlappingOwnedPathsBeforePlanning(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.InputContract.OwnedPaths = []string{"/manifest/llm", "/manifest/llm/model"}
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsDuplicateChangePathsBeforePlanning(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Changes = append(manifest.Targets[0].Changes, manifest.Targets[0].Changes[0])
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestPlanRevisionRejectsTargetAliasCollision(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets = append(manifest.Targets, RevisionTarget{CanonicalName: "other", Aliases: []string{"custom-alias"}, ProviderID: "other", State: TargetStateNoOp, Reason: "not represented"})
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitMalformed {
		t.Fatalf("error code = %d, want %d; err=%v", code, ExitMalformed, err)
	}
}

func TestApplyRevisionRejectsOverlappingPlanEntries(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	manifest := revisionManifest(input)
	plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err != nil {
		t.Fatalf("PlanRevision: %v", err)
	}
	plan.Entries = append(plan.Entries, plan.Entries[0])
	if _, err := ApplyRevision(input, plan); RevisionErrorCode(err) != ExitConflict {
		t.Fatalf("ApplyRevision error = %v, want conflict", err)
	}
}

func TestRevisionParserRejectsDuplicateAndTrailingDocuments(t *testing.T) {
	for name, input := range map[string][]byte{
		"duplicate key":          []byte(`{"schema_version":"lingtai.preset.revision/v1","schema_version":"lingtai.preset.revision/v1"}`),
		"trailing document":      []byte(`{"schema_version":"lingtai.preset.revision/v1"}{}`),
		"arbitrary route marker": []byte(`{"schema_version":"lingtai.preset.revision/v1","targets":[{"route":{"binding":{"mode":"direct","marker":"route-ok"}}}]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeRevisionManifest(input); RevisionErrorCode(err) != ExitMalformed {
				t.Fatalf("DecodeRevisionManifest error = %v, want malformed", err)
			}
		})
	}
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"}}}`)
	if _, err := PlanRevision(revisionManifest(input), append(input, []byte(` {}`)...), RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"}); RevisionErrorCode(err) != ExitMalformed {
		t.Fatalf("trailing input error = %v, want malformed", err)
	}
}

func TestApplyRevisionRemoveSpliceEdges(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		path     string
		old      string
		expected string
	}{
		{"object first", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"a":1,"b":2,"c":3}}}`, "/manifest/object/a", "1", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"b":2,"c":3}}}`},
		{"object middle", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"a":1,"b":2,"c":3}}}`, "/manifest/object/b", "2", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"a":1,"c":3}}}`},
		{"object last", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"a":1,"b":2,"c":3}}}`, "/manifest/object/c", "3", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"a":1,"b":2}}}`},
		{"object only", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{"only":1}}}`, "/manifest/object/only", "1", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"object":{}}}`},
		{"array first", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1,2,3]}}`, "/manifest/array/0", "1", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[2,3]}}`},
		{"array middle", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1,2,3]}}`, "/manifest/array/1", "2", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1,3]}}`},
		{"array last", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1,2,3]}}`, "/manifest/array/2", "3", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1,2]}}`},
		{"array only", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[1]}}`, "/manifest/array/0", "1", `{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"array":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			manifest := revisionManifest(input)
			manifest.Targets[0].Changes = []RevisionChange{{Path: tc.path, Kind: ChangeRemove, ExpectedOld: json.RawMessage(tc.old), EvidenceRef: "evidence-public-api", Reason: "remove the explicitly retired value"}}
			manifest.InputContract.OwnedPaths = []string{tc.path}
			manifest.InputContract.PostImageSHA256 = sha256Hex([]byte(tc.expected))
			plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
			if err != nil {
				t.Fatalf("PlanRevision: %v", err)
			}
			got, err := ApplyRevision(input, plan)
			if err != nil {
				t.Fatalf("ApplyRevision: %v", err)
			}
			if string(got) != tc.expected {
				t.Fatalf("output = %s, want %s", got, tc.expected)
			}
		})
	}
}

func TestPlanRevisionRejectsExpectedOldConflict(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"different"}}}`)
	manifest := revisionManifest(input)
	manifest.InputContract.InputSHA256 = sha256Hex(input)
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
}

func TestPlanRevisionRejectsPublicEvidenceForCodexOAuth(t *testing.T) {
	input := []byte(`{"name":"codex","manifest":{"llm":{"provider":"codex","model":"old"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].CanonicalName = "codex"
	manifest.Targets[0].Aliases = nil
	manifest.Targets[0].ProviderID = "codex"
	manifest.Targets[0].Route = RouteContract{
		API:       "responses",
		Transport: "http",
		Scope:     EvidenceScopeChatGPTOAuthCodex,
		Binding: RouteBinding{
			Mode:     RouteBindingProviderChild,
			Provider: &RouteFieldBinding{Pointer: "/manifest/llm/provider", Expected: "codex"},
		},
	}
	manifest.Targets[0].Responses = &ResponsesSemantics{
		InputModalities:      []string{"text", "image", "file"},
		ReasoningVocabulary:  []string{"low", "medium", "high", "xhigh"},
		ReasoningDefault:     "xhigh",
		ServiceTierRequested: []string{"fast"},
		ServiceTierObserved:  []string{"fast", "normal"},
	}
	for i := range manifest.Targets[0].Models {
		manifest.Targets[0].Models[i].ProviderID = "codex"
	}
	manifest.InputContract.OwnedPaths = []string{"/manifest/llm/model"}
	manifest.InputContract.InputSHA256 = sha256Hex(input)
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
	if !strings.Contains(err.Error(), `evidence scope "public_api" is out of scope for route "chatgpt_oauth_codex"`) {
		t.Fatalf("error = %v, want the public-vs-OAuth scope branch", err)
	}
}

func TestPlanRevisionRejectsUnknownCapabilityPromotion(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","api_compat":"openai","model":"model-old"},"capabilities":{"vision":false}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Changes[0].Path = "/manifest/capabilities/vision"
	manifest.Targets[0].Changes[0].ExpectedOld = json.RawMessage(`false`)
	manifest.Targets[0].Changes[0].NewValue = json.RawMessage(`true`)
	manifest.Targets[0].Changes[0].Reason = "promote image input"
	manifest.Targets[0].Changes[0].ModelRefs = []string{"model-old"}
	manifest.InputContract.OwnedPaths = []string{"/manifest/capabilities/vision"}
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
}

func TestPlanRevisionAllowsSupportedSelectedModelWhenUnsupportedGenerationIsRetired(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","model":"old","models":[{"id":"old"},{"id":"current"},{"id":"next"}]},"capabilities":{"vision":false}}}`)
	expected := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","model":"next","models":[{"id":"current"},{"id":"next"}]},"capabilities":{"vision":true}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = []ModelRecord{
		{ID: "old", Family: "family", Generation: 1, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilityUnsupported)},
		{ID: "current", Family: "family", Generation: 2, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilitySupported)},
		{ID: "next", Family: "family", Generation: 3, Variant: "base", ProviderID: "custom", EvidenceRefs: []string{"evidence-public-api"}, Capabilities: revisionCapabilities(CapabilitySupported)},
	}
	manifest.Targets[0].Changes = []RevisionChange{
		{Path: "/manifest/llm/model", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`"old"`), NewValue: json.RawMessage(`"next"`), EvidenceRef: "evidence-public-api", Reason: "select the newest supported generation"},
		{Path: "/manifest/llm/models/0", Kind: ChangeRemove, ExpectedOld: json.RawMessage(`{"id":"old"}`), EvidenceRef: "evidence-public-api", Reason: "retire the unsupported generation"},
		{Path: "/manifest/capabilities/vision", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`false`), NewValue: json.RawMessage(`true`), EvidenceRef: "evidence-public-api", Reason: "enable vision for the selected supported model", ModelRefs: []string{"next"}},
	}
	manifest.InputContract.OwnedPaths = []string{"/manifest/llm/model", "/manifest/llm/models/0", "/manifest/capabilities/vision"}
	manifest.InputContract.PostImageSHA256 = sha256Hex(expected)
	plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if err != nil {
		t.Fatalf("PlanRevision: %v", err)
	}
	foundCapabilityRef := false
	for _, entry := range plan.Entries {
		if entry.Path == "/manifest/capabilities/vision" && len(entry.ModelRefs) == 1 && entry.ModelRefs[0] == "next" {
			foundCapabilityRef = true
		}
	}
	if !foundCapabilityRef {
		t.Fatalf("plan did not retain the capability model association: %#v", plan.Entries)
	}
	got, err := ApplyRevision(input, plan)
	if err != nil {
		t.Fatalf("ApplyRevision: %v", err)
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("output = %s, want %s", got, expected)
	}
}

func TestPlanRevisionRejectsCapabilityPromotionWithoutModelEvidence(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom"},"capabilities":{"vision":false}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = nil
	manifest.Targets[0].Changes = []RevisionChange{{
		Path: "/manifest/capabilities/vision", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`false`), NewValue: json.RawMessage(`true`), EvidenceRef: "evidence-public-api", Reason: "promote vision without model facts",
	}}
	manifest.InputContract.OwnedPaths = []string{"/manifest/capabilities/vision"}
	manifest.InputContract.PostImageSHA256 = sha256Hex(bytes.Replace(input, []byte(`"vision":false`), []byte(`"vision":true`), 1))
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
	if !strings.Contains(err.Error(), "requires one or more evidenced model_refs") {
		t.Fatalf("error = %v, want missing model_refs rejection", err)
	}
}

func TestPlanRevisionRejectsObservedOnlyServiceTierOnRequestPath(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","service_tier":"fast"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = nil
	manifest.Targets[0].Responses.ServiceTierRequested = []string{"fast"}
	manifest.Targets[0].Responses.ServiceTierObserved = []string{"fast", "normal"}
	manifest.Targets[0].Changes = []RevisionChange{{
		Path: "/manifest/llm/service_tier", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`"fast"`), NewValue: json.RawMessage(`"normal"`), EvidenceRef: "evidence-public-api", Reason: "request an observation-only tier",
	}}
	manifest.InputContract.OwnedPaths = []string{"/manifest/llm/service_tier"}
	manifest.InputContract.PostImageSHA256 = sha256Hex(bytes.Replace(input, []byte(`"service_tier":"fast"`), []byte(`"service_tier":"normal"`), 1))
	_, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
	if code := RevisionErrorCode(err); code != ExitConflict {
		t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
	}
	if !strings.Contains(err.Error(), "request vocabulary") {
		t.Fatalf("error = %v, want request-side service-tier rejection", err)
	}
}

func TestPlanRevisionAllowsObservedServiceTierOnObservedPath(t *testing.T) {
	input := []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom"},"observed":{"service_tier":"fast"}}}`)
	manifest := revisionManifest(input)
	manifest.Targets[0].Models = nil
	manifest.Targets[0].Responses.ServiceTierRequested = []string{"fast"}
	manifest.Targets[0].Responses.ServiceTierObserved = []string{"fast", "normal"}
	manifest.Targets[0].Changes = []RevisionChange{{
		Path: "/manifest/observed/service_tier", Kind: ChangeReplace, ExpectedOld: json.RawMessage(`"fast"`), NewValue: json.RawMessage(`"normal"`), EvidenceRef: "evidence-public-api", Reason: "record an evidenced observed tier",
	}}
	manifest.InputContract.OwnedPaths = []string{"/manifest/observed/service_tier"}
	manifest.InputContract.PostImageSHA256 = sha256Hex(bytes.Replace(input, []byte(`"service_tier":"fast"`), []byte(`"service_tier":"normal"`), 1))
	if plan, err := PlanRevision(manifest, input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"}); err != nil {
		t.Fatalf("PlanRevision: %v", err)
	} else if !plan.Changed {
		t.Fatal("observed service-tier revision should produce a change")
	}
}

func TestPlanRevisionRejectsNonStringReasoningAndServiceTierReplacements(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       []byte
		path        string
		expectedOld json.RawMessage
	}{
		{name: "reasoning", input: []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","reasoning":"high"}}}`), path: "/manifest/llm/reasoning", expectedOld: json.RawMessage(`"high"`)},
		{name: "service tier", input: []byte(`{"name":"custom","manifest":{"llm":{"provider":"custom","service_tier":"fast"}}}`), path: "/manifest/llm/service_tier", expectedOld: json.RawMessage(`"fast"`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := revisionManifest(tc.input)
			manifest.Targets[0].Models = nil
			manifest.Targets[0].Changes = []RevisionChange{{Path: tc.path, Kind: ChangeReplace, ExpectedOld: tc.expectedOld, NewValue: json.RawMessage(`true`), EvidenceRef: "evidence-public-api", Reason: "attempt a non-string semantic value"}}
			manifest.InputContract.OwnedPaths = []string{tc.path}
			manifest.InputContract.PostImageSHA256 = sha256Hex(bytes.Replace(tc.input, tc.expectedOld, []byte(`true`), 1))
			_, err := PlanRevision(manifest, tc.input, RevisionOptions{Now: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), LogicalPath: "preset.json"})
			if code := RevisionErrorCode(err); code != ExitConflict {
				t.Fatalf("error code = %d, want %d: %v", code, ExitConflict, err)
			}
			if !strings.Contains(err.Error(), "requires a string replacement") {
				t.Fatalf("error = %v, want type rejection", err)
			}
		})
	}
}
