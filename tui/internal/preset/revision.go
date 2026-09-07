package preset

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Exit codes are part of the headless revision contract. They intentionally
// remain independent of the older presets-listing error codes.
const (
	ExitOK         = 0
	ExitChanged    = 1
	ExitMalformed  = 2
	ExitConflict   = 3
	ExitFilesystem = 4
)

const (
	ChangeReplace = "replace"
	ChangeRemove  = "remove"
)

type EvidenceScope string

const (
	EvidenceScopePublicAPI          EvidenceScope = "public_api"
	EvidenceScopeProviderEndpoint   EvidenceScope = "provider_endpoint"
	EvidenceScopeNativeSDK          EvidenceScope = "native_sdk"
	EvidenceScopeChatGPTOAuthCodex  EvidenceScope = "chatgpt_oauth_codex"
	EvidenceScopeAccountObservation EvidenceScope = "account_observation"
)

type EvidenceStatus string

const (
	EvidencePositive    EvidenceStatus = "positive"
	EvidenceNegative    EvidenceStatus = "negative"
	EvidenceUnknown     EvidenceStatus = "unknown"
	EvidenceObserved    EvidenceStatus = "observed"
	EvidenceConflicting EvidenceStatus = "conflicting"
)

type CapabilityState string

const (
	CapabilitySupported   CapabilityState = "supported"
	CapabilityUnsupported CapabilityState = "unsupported"
	CapabilityUnknown     CapabilityState = "unknown"
)

type TargetState string

const (
	TargetStateRevise      TargetState = "revise"
	TargetStateUnsupported TargetState = "unsupported"
	TargetStateNoOp        TargetState = "no-op"
)

type RouteBindingMode string

const (
	RouteBindingDirect        RouteBindingMode = "direct"
	RouteBindingProviderChild RouteBindingMode = "provider_child"
)

// Manifest is the operation-specific, content-addressed revision input. It
// is deliberately not the kernel init.json schema and is never loaded from
// global configuration.
type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	Policy        RevisionPolicy   `json:"policy"`
	Evidence      []Evidence       `json:"evidence"`
	Targets       []RevisionTarget `json:"targets"`
	InputContract InputContract    `json:"input_contract"`
}

type RevisionPolicy struct {
	RetainGenerations int    `json:"retain_generations"`
	ParserVersion     string `json:"parser_version"`
}

type Evidence struct {
	ID            string         `json:"id"`
	URL           string         `json:"url"`
	SourceType    string         `json:"source_type"`
	Scope         EvidenceScope  `json:"scope"`
	RetrievedAt   string         `json:"retrieved_at"`
	FreshUntil    string         `json:"fresh_until"`
	Content       string         `json:"content"`
	ContentSHA256 string         `json:"content_sha256"`
	ParserVersion string         `json:"parser_version"`
	Status        EvidenceStatus `json:"status"`
	Claims        []string       `json:"claims"`
}

type InputContract struct {
	Document        string   `json:"document"`
	InputSHA256     string   `json:"input_sha256"`
	PostImageSHA256 string   `json:"post_image_sha256"`
	NamePointer     string   `json:"name_pointer"`
	ProviderPointer string   `json:"provider_pointer"`
	OwnedPaths      []string `json:"owned_paths"`
}

type RouteContract struct {
	API       string        `json:"api"`
	Transport string        `json:"transport"`
	Scope     EvidenceScope `json:"scope"`
	Binding   RouteBinding  `json:"binding"`
}

type RouteBinding struct {
	Mode      RouteBindingMode   `json:"mode"`
	API       *RouteFieldBinding `json:"api,omitempty"`
	Transport *RouteFieldBinding `json:"transport,omitempty"`
	Scope     *RouteFieldBinding `json:"scope,omitempty"`
	Provider  *RouteFieldBinding `json:"provider,omitempty"`
}

type RouteFieldBinding struct {
	Pointer  string `json:"pointer"`
	Expected string `json:"expected"`
}

type RevisionTarget struct {
	CanonicalName string              `json:"canonical_name"`
	Aliases       []string            `json:"aliases"`
	ProviderID    string              `json:"provider_id"`
	State         TargetState         `json:"state"`
	Reason        string              `json:"reason,omitempty"`
	Route         RouteContract       `json:"route"`
	Responses     *ResponsesSemantics `json:"responses,omitempty"`
	Models        []ModelRecord       `json:"models"`
	Changes       []RevisionChange    `json:"changes"`
}

type ModelRecord struct {
	ID           string          `json:"id"`
	Family       string          `json:"family"`
	Generation   int             `json:"generation"`
	Variant      string          `json:"variant"`
	ProviderID   string          `json:"provider_id"`
	EvidenceRefs []string        `json:"evidence_refs"`
	Capabilities CapabilityFacts `json:"capabilities"`
}

type CapabilityFacts struct {
	TextInput  CapabilityState `json:"text_input"`
	ImageInput CapabilityState `json:"image_input"`
	FileInput  CapabilityState `json:"file_input"`
	TUIWiring  CapabilityState `json:"tui_wiring"`
	Runtime    CapabilityState `json:"runtime"`
}

type ResponsesSemantics struct {
	InputModalities               []string `json:"input_modalities"`
	ReasoningVocabulary           []string `json:"reasoning_vocabulary"`
	ReasoningDefault              string   `json:"reasoning_default"`
	ReasoningOmissionMeansDefault bool     `json:"reasoning_omission_means_default"`
	ServiceTierRequested          []string `json:"service_tier_requested"`
	ServiceTierObserved           []string `json:"service_tier_observed"`
}

type RevisionChange struct {
	Path        string          `json:"path"`
	Kind        string          `json:"kind"`
	ExpectedOld json.RawMessage `json:"expected_old"`
	NewValue    json.RawMessage `json:"new_value,omitempty"`
	EvidenceRef string          `json:"evidence_ref"`
	Reason      string          `json:"reason"`
	ModelRefs   []string        `json:"model_refs,omitempty"`
}

type RevisionOptions struct {
	Now         time.Time
	LogicalPath string
}

type RevisionPlan struct {
	TargetName      string              `json:"-"`
	TargetState     TargetState         `json:"-"`
	TargetReason    string              `json:"-"`
	LogicalPath     string              `json:"-"`
	InputSHA256     string              `json:"-"`
	PostImageSHA256 string              `json:"-"`
	Changed         bool                `json:"-"`
	Entries         []RevisionPlanEntry `json:"-"`
	RetainedModels  []string            `json:"-"`
}

type RevisionPlanEntry struct {
	PresetName  string          `json:"preset_name"`
	LogicalPath string          `json:"logical_path"`
	Path        string          `json:"path"`
	Kind        string          `json:"kind"`
	OldValue    json.RawMessage `json:"old_value"`
	NewValue    json.RawMessage `json:"new_value,omitempty"`
	EvidenceRef string          `json:"evidence_ref"`
	Reason      string          `json:"reason"`
	ModelRefs   []string        `json:"model_refs,omitempty"`
	start       int
	end         int
	removeInfo  *removeSpan
}

type removeSpan struct {
	start int
	end   int
}

type revisionError struct {
	code int
	msg  string
}

func (e *revisionError) Error() string { return e.msg }

// RevisionErrorCode maps a revision error to the stable process code.
func RevisionErrorCode(err error) int {
	if err == nil {
		return ExitOK
	}
	if typed, ok := err.(*revisionError); ok {
		return typed.code
	}
	return ExitMalformed
}

func malformedf(format string, args ...interface{}) error {
	return &revisionError{code: ExitMalformed, msg: fmt.Sprintf(format, args...)}
}

func conflictf(format string, args ...interface{}) error {
	return &revisionError{code: ExitConflict, msg: fmt.Sprintf(format, args...)}
}

// DecodeRevisionManifest decodes exactly one JSON manifest document. The
// adapter supplies bytes; this function has no filesystem or runtime access.
func DecodeRevisionManifest(data []byte) (Manifest, error) {
	if _, err := parseJSONDocument(data); err != nil {
		return Manifest{}, malformedf("decode revision manifest: %v", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, malformedf("decode revision manifest: %v", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, malformedf("revision manifest contains multiple JSON documents")
	}
	return manifest, nil
}

// PlanRevision validates the typed manifest, validates the explicit input,
// and produces the one deterministic plan used by dry-run, check, and apply.
func PlanRevision(manifest Manifest, input []byte, opts RevisionOptions) (RevisionPlan, error) {
	if opts.LogicalPath == "" {
		return RevisionPlan{}, malformedf("logical input path is required")
	}
	if opts.Now.IsZero() {
		return RevisionPlan{}, malformedf("explicit validation time is required")
	}
	if err := validateManifest(manifest, opts.Now); err != nil {
		return RevisionPlan{}, err
	}
	inputSHA := sha256Hex(input)
	doc, err := parseJSONDocument(input)
	if err != nil {
		return RevisionPlan{}, malformedf("decode revision input: %v", err)
	}
	logicalPath := filepath.ToSlash(filepath.Clean(opts.LogicalPath))
	if logicalPath != filepath.ToSlash(filepath.Clean(manifest.InputContract.Document)) {
		return RevisionPlan{}, malformedf("input document %q does not match manifest document %q", logicalPath, manifest.InputContract.Document)
	}
	name, err := stringAt(doc, manifest.InputContract.NamePointer)
	if err != nil {
		return RevisionPlan{}, malformedf("read input name: %v", err)
	}
	provider, err := stringAt(doc, manifest.InputContract.ProviderPointer)
	if err != nil {
		return RevisionPlan{}, malformedf("read input provider: %v", err)
	}
	target, err := findTarget(manifest.Targets, name)
	if err != nil {
		return RevisionPlan{}, err
	}
	if provider != target.ProviderID {
		return RevisionPlan{}, conflictf("input provider %q does not match target provider %q", provider, target.ProviderID)
	}
	if err := validateInputRoute(doc, target, manifest.InputContract); err != nil {
		return RevisionPlan{}, err
	}
	if target.State != TargetStateRevise && manifest.InputContract.PostImageSHA256 != manifest.InputContract.InputSHA256 {
		return RevisionPlan{}, malformedf("non-revision target %q must declare the pinned input as its post-image", target.CanonicalName)
	}
	modelRevision := target.State == TargetStateRevise && hasModelRevision(target.Changes)
	retained := []string{}
	if modelRevision {
		retained, err = retainedModels(manifest, *target)
		if err != nil {
			return RevisionPlan{}, err
		}
	}
	retainedSet := make(map[string]bool, len(retained))
	for _, model := range retained {
		retainedSet[model] = true
	}
	if modelRevision {
		if err := validateExplicitRetirements(*target, retainedSet); err != nil {
			return RevisionPlan{}, err
		}
	}
	for _, change := range target.Changes {
		if change.Kind == ChangeReplace {
			if err := validateChangeSemantics(*target, change, retainedSet); err != nil {
				return RevisionPlan{}, err
			}
		}
	}
	if inputSHA != manifest.InputContract.InputSHA256 {
		if inputSHA != manifest.InputContract.PostImageSHA256 {
			return RevisionPlan{}, malformedf("input sha256 mismatch: got %s, want pinned input %s or declared post-image %s", inputSHA, manifest.InputContract.InputSHA256, manifest.InputContract.PostImageSHA256)
		}
		return RevisionPlan{
			TargetName:      target.CanonicalName,
			TargetState:     target.State,
			TargetReason:    target.Reason,
			LogicalPath:     logicalPath,
			InputSHA256:     inputSHA,
			PostImageSHA256: manifest.InputContract.PostImageSHA256,
			Changed:         false,
			Entries:         []RevisionPlanEntry{},
			RetainedModels:  retained,
		}, nil
	}

	entries := make([]RevisionPlanEntry, 0, len(target.Changes))
	for _, change := range target.Changes {
		entry, err := planChange(doc, change, target.CanonicalName, logicalPath)
		if err != nil {
			return RevisionPlan{}, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PresetName != entries[j].PresetName {
			return entries[i].PresetName < entries[j].PresetName
		}
		if entries[i].LogicalPath != entries[j].LogicalPath {
			return entries[i].LogicalPath < entries[j].LogicalPath
		}
		if entries[i].Path != entries[j].Path {
			return entries[i].Path < entries[j].Path
		}
		return entries[i].Kind < entries[j].Kind
	})
	plan := RevisionPlan{
		TargetName:      target.CanonicalName,
		TargetState:     target.State,
		TargetReason:    target.Reason,
		LogicalPath:     logicalPath,
		InputSHA256:     inputSHA,
		PostImageSHA256: manifest.InputContract.PostImageSHA256,
		Changed:         len(entries) > 0,
		Entries:         entries,
		RetainedModels:  retained,
	}
	postImage, err := ApplyRevision(input, plan)
	if err != nil {
		return RevisionPlan{}, err
	}
	if got := sha256Hex(postImage); got != manifest.InputContract.PostImageSHA256 {
		return RevisionPlan{}, malformedf("post_image_sha256 does not match planned output: got %s, want %s", got, manifest.InputContract.PostImageSHA256)
	}
	return plan, nil
}

// ApplyRevision applies only a previously validated plan. It splices the
// original bytes, so unowned fields, ordering, whitespace, and unknown values
// remain untouched.
func ApplyRevision(input []byte, plan RevisionPlan) ([]byte, error) {
	if sha256Hex(input) != plan.InputSHA256 {
		return nil, conflictf("input changed after planning")
	}
	if !validSHA256(plan.PostImageSHA256) {
		return nil, malformedf("revision plan requires a valid post_image_sha256")
	}
	splices := make([]RevisionPlanEntry, len(plan.Entries))
	copy(splices, plan.Entries)
	for i := range splices {
		for j := 0; j < i; j++ {
			overlap, err := pointerPathsOverlap(splices[i].Path, splices[j].Path)
			if err != nil {
				return nil, conflictf("invalid revision path overlap at %q: %v", splices[i].Path, err)
			}
			if overlap {
				return nil, conflictf("overlapping revision changes at %q and %q", splices[j].Path, splices[i].Path)
			}
		}
	}
	for i := range splices {
		start, end := splices[i].start, splices[i].end
		if splices[i].removeInfo != nil {
			start, end = splices[i].removeInfo.start, splices[i].removeInfo.end
		}
		if start < 0 || end < start || end > len(input) {
			return nil, conflictf("invalid revision span at %q", splices[i].Path)
		}
	}
	sort.Slice(splices, func(i, j int) bool {
		left, right := splices[i].start, splices[j].start
		if splices[i].removeInfo != nil {
			left = splices[i].removeInfo.start
		}
		if splices[j].removeInfo != nil {
			right = splices[j].removeInfo.start
		}
		return left > right
	})
	for i := 1; i < len(splices); i++ {
		previousStart := splices[i-1].start
		currentEnd := splices[i].end
		if splices[i-1].removeInfo != nil {
			previousStart = splices[i-1].removeInfo.start
		}
		if splices[i].removeInfo != nil {
			currentEnd = splices[i].removeInfo.end
		}
		if currentEnd > previousStart {
			return nil, conflictf("overlapping revision changes at %q and %q", splices[i-1].Path, splices[i].Path)
		}
	}
	output := append([]byte(nil), input...)
	for _, entry := range splices {
		start, end := entry.start, entry.end
		if entry.removeInfo != nil {
			start, end = entry.removeInfo.start, entry.removeInfo.end
		}
		replacement := []byte{}
		if entry.Kind == ChangeReplace {
			replacement = compactJSON(entry.NewValue)
		}
		output = append(append(append([]byte{}, output[:start]...), replacement...), output[end:]...)
	}
	if _, err := parseJSONDocument(output); err != nil {
		return nil, fmt.Errorf("materialized revision is invalid JSON: %w", err)
	}
	if got := sha256Hex(output); got != plan.PostImageSHA256 {
		return nil, conflictf("materialized revision sha256 mismatch: got %s, want %s", got, plan.PostImageSHA256)
	}
	return output, nil
}

func validateManifest(manifest Manifest, now time.Time) error {
	if manifest.SchemaVersion != "lingtai.preset.revision/v1" {
		return malformedf("unsupported or missing revision schema_version %q", manifest.SchemaVersion)
	}
	if manifest.Policy.RetainGenerations != 2 {
		return malformedf("policy.retain_generations must be exactly 2")
	}
	if manifest.Policy.ParserVersion == "" {
		return malformedf("policy.parser_version is required")
	}
	if len(manifest.Evidence) == 0 || len(manifest.Targets) == 0 {
		return malformedf("evidence and targets are required")
	}
	documentPath := filepath.Clean(filepath.FromSlash(manifest.InputContract.Document))
	if !validSHA256(manifest.InputContract.InputSHA256) || !validSHA256(manifest.InputContract.PostImageSHA256) || manifest.InputContract.Document == "" || filepath.IsAbs(documentPath) || documentPath == "." || documentPath == ".." || strings.HasPrefix(documentPath, ".."+string(filepath.Separator)) {
		return malformedf("input_contract requires document, input_sha256, and post_image_sha256")
	}
	if err := validatePointer(manifest.InputContract.NamePointer); err != nil {
		return malformedf("invalid input name pointer: %v", err)
	}
	if err := validatePointer(manifest.InputContract.ProviderPointer); err != nil {
		return malformedf("invalid input provider pointer: %v", err)
	}
	if len(manifest.InputContract.OwnedPaths) == 0 {
		return malformedf("input_contract.owned_paths is required")
	}
	owned := map[string]bool{}
	for _, path := range manifest.InputContract.OwnedPaths {
		if err := validatePointer(path); err != nil {
			return malformedf("invalid owned path %q: %v", path, err)
		}
		if owned[path] {
			return malformedf("duplicate owned path %q", path)
		}
		owned[path] = true
	}
	if err := validateDisjointPaths("owned", manifest.InputContract.OwnedPaths); err != nil {
		return err
	}
	evidenceByID := map[string]Evidence{}
	claimStatus := map[string]EvidenceStatus{}
	for _, evidence := range manifest.Evidence {
		if evidence.ID == "" || evidence.URL == "" || evidence.SourceType == "" || !validScope(evidence.Scope) || evidence.Content == "" || len(evidence.Claims) == 0 {
			return conflictf("incomplete evidence record %q", evidence.ID)
		}
		if evidence.ParserVersion != manifest.Policy.ParserVersion {
			return conflictf("evidence %q uses parser %q, want %q", evidence.ID, evidence.ParserVersion, manifest.Policy.ParserVersion)
		}
		if !validSHA256(evidence.ContentSHA256) || evidence.ContentSHA256 != sha256Hex([]byte(evidence.Content)) {
			return malformedf("evidence %q content_sha256 mismatch", evidence.ID)
		}
		retrieved, err := time.Parse(time.RFC3339, evidence.RetrievedAt)
		if err != nil {
			return malformedf("evidence %q retrieved_at: %v", evidence.ID, err)
		}
		freshUntil, err := time.Parse(time.RFC3339, evidence.FreshUntil)
		if err != nil || !freshUntil.After(retrieved) {
			return malformedf("evidence %q has invalid fresh_until", evidence.ID)
		}
		if now.After(freshUntil) {
			return conflictf("evidence %q is stale", evidence.ID)
		}
		if evidence.Status != EvidencePositive && evidence.Status != EvidenceNegative && evidence.Status != EvidenceUnknown && evidence.Status != EvidenceObserved && evidence.Status != EvidenceConflicting {
			return malformedf("evidence %q has invalid status %q", evidence.ID, evidence.Status)
		}
		if evidence.Status == EvidenceConflicting {
			return conflictf("evidence %q is explicitly conflicting", evidence.ID)
		}
		if _, exists := evidenceByID[evidence.ID]; exists {
			return malformedf("duplicate evidence id %q", evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
		for _, claim := range evidence.Claims {
			if prior, exists := claimStatus[claim]; exists && prior != evidence.Status {
				return conflictf("conflicting evidence claim %q", claim)
			}
			claimStatus[claim] = evidence.Status
		}
	}

	seenNames := map[string]bool{}
	for i := range manifest.Targets {
		target := &manifest.Targets[i]
		if target.CanonicalName == "" || strings.ContainsAny(target.CanonicalName, `/\\`) || target.ProviderID == "" {
			return conflictf("target %q has invalid canonical name or provider", target.CanonicalName)
		}
		if seenNames[target.CanonicalName] {
			return malformedf("duplicate canonical target %q", target.CanonicalName)
		}
		seenNames[target.CanonicalName] = true
		for _, alias := range append([]string(nil), target.Aliases...) {
			if alias == "" || seenNames[alias] {
				return malformedf("alias %q collides with a canonical target or alias", alias)
			}
			seenNames[alias] = true
		}
		if target.State != TargetStateRevise && target.State != TargetStateUnsupported && target.State != TargetStateNoOp {
			return malformedf("target %q has invalid state %q", target.CanonicalName, target.State)
		}
		if target.State != TargetStateRevise {
			if strings.TrimSpace(target.Reason) == "" {
				return malformedf("non-revision target %q requires a deterministic reason", target.CanonicalName)
			}
			if len(target.Models) != 0 || len(target.Changes) != 0 {
				return malformedf("non-revision target %q must not declare models or changes", target.CanonicalName)
			}
		} else if len(target.Changes) == 0 {
			return malformedf("revision target %q requires changes", target.CanonicalName)
		}
		if err := validateRoute(*target, manifest.InputContract); err != nil {
			return err
		}
		if target.Route.API == "responses" {
			if target.Responses == nil {
				return conflictf("target %q is Responses-routed but has no Responses semantics", target.CanonicalName)
			}
			if err := validateResponses(target.ProviderID, *target.Responses); err != nil {
				return err
			}
		}
		modelRevision := target.State == TargetStateRevise && hasModelRevision(target.Changes)
		if len(target.Models) != 0 {
			if err := validateModels(*target, evidenceByID); err != nil {
				return err
			}
		}
		if modelRevision && len(target.Models) == 0 {
			return conflictf("model revision target %q requires evidenced models", target.CanonicalName)
		}
		changePaths := make([]string, 0, len(target.Changes))
		for _, change := range target.Changes {
			if change.Path == "" || !owned[change.Path] {
				return malformedf("change path %q is not an explicit owned path", change.Path)
			}
			if change.Kind != ChangeReplace && change.Kind != ChangeRemove {
				return malformedf("unsupported change kind %q", change.Kind)
			}
			if len(change.ExpectedOld) == 0 || !validJSONValue(change.ExpectedOld) {
				return malformedf("change %q requires a valid expected_old value", change.Path)
			}
			if change.EvidenceRef == "" || strings.TrimSpace(change.Reason) == "" {
				return malformedf("change %q requires evidence_ref and reason", change.Path)
			}
			if change.Kind == ChangeRemove && len(change.NewValue) != 0 {
				return malformedf("remove change %q must not have new_value", change.Path)
			}
			if change.Kind == ChangeReplace && (len(change.NewValue) == 0 || !validJSONValue(change.NewValue)) {
				return malformedf("replace %q requires a valid new_value", change.Path)
			}
			if len(uniqueStrings(change.ModelRefs)) != len(change.ModelRefs) {
				return malformedf("change %q has duplicate model_refs", change.Path)
			}
			changePaths = append(changePaths, change.Path)
			evidence, ok := evidenceByID[change.EvidenceRef]
			if !ok {
				return conflictf("change %q references missing evidence %q", change.Path, change.EvidenceRef)
			}
			if evidence.Status != EvidencePositive && evidence.Status != EvidenceObserved {
				return conflictf("change %q lacks positive evidence", change.Path)
			}
			if evidence.Scope != target.Route.Scope {
				return conflictf("change %q evidence scope %q is out of scope for route %q", change.Path, evidence.Scope, target.Route.Scope)
			}
		}
		if err := validateDisjointPaths("change", changePaths); err != nil {
			return err
		}
	}
	return nil
}

func validateRoute(target RevisionTarget, input InputContract) error {
	route := target.Route
	if route.API != "responses" && route.API != "chat_completions" && route.API != "native" {
		return conflictf("unsupported route api %q", route.API)
	}
	if (route.Transport != "http" && route.Transport != "websocket" && route.Transport != "native") || !validScope(route.Scope) {
		return conflictf("route transport and evidence scope are required")
	}
	if route.API == "native" && route.Transport != "native" {
		return conflictf("native route has non-native transport %q", route.Transport)
	}
	binding := route.Binding
	if binding.Mode == "" {
		if binding.API != nil || binding.Transport != nil || binding.Scope != nil || binding.Provider != nil {
			return malformedf("route binding fields require an explicit mode")
		}
		if target.State == TargetStateRevise {
			return conflictf("revision target %q requires an explicit route input binding", target.CanonicalName)
		}
		return nil
	}
	switch binding.Mode {
	case RouteBindingDirect:
		if binding.API == nil || binding.Transport == nil || binding.Scope == nil || binding.Provider != nil {
			return malformedf("direct route binding requires exactly api, transport, and scope fields")
		}
		fields := []struct {
			name    string
			binding *RouteFieldBinding
			want    string
		}{
			{name: "api", binding: binding.API, want: route.API},
			{name: "transport", binding: binding.Transport, want: route.Transport},
			{name: "scope", binding: binding.Scope, want: string(route.Scope)},
		}
		seenPointers := map[string]bool{}
		for _, field := range fields {
			if err := validateRouteFieldBinding(field.name, field.binding, field.want); err != nil {
				return err
			}
			if seenPointers[field.binding.Pointer] {
				return malformedf("direct route binding fields must use distinct input pointers")
			}
			seenPointers[field.binding.Pointer] = true
		}
	case RouteBindingProviderChild:
		if binding.Provider == nil || binding.API != nil || binding.Transport != nil || binding.Scope != nil {
			return malformedf("provider_child route binding requires exactly the provider field")
		}
		if err := validateRouteFieldBinding("provider", binding.Provider, target.ProviderID); err != nil {
			return err
		}
		if binding.Provider.Pointer != input.ProviderPointer {
			return malformedf("provider_child route binding must use input_contract.provider_pointer")
		}
	default:
		return malformedf("unsupported route binding mode %q", binding.Mode)
	}
	return nil
}

func validateRouteFieldBinding(name string, binding *RouteFieldBinding, want string) error {
	if strings.TrimSpace(binding.Expected) == "" {
		return malformedf("route %s binding requires an expected value", name)
	}
	if err := validatePointer(binding.Pointer); err != nil {
		return malformedf("invalid route %s pointer %q: %v", name, binding.Pointer, err)
	}
	if binding.Expected != want {
		return conflictf("route %s binding %q does not match route %s %q", name, binding.Expected, name, want)
	}
	return nil
}

func validateResponses(provider string, semantics ResponsesSemantics) error {
	for _, modality := range semantics.InputModalities {
		if modality != "text" && modality != "image" && modality != "file" {
			return conflictf("unsupported Responses input modality %q", modality)
		}
	}
	if len(uniqueStrings(semantics.InputModalities)) != len(semantics.InputModalities) || len(uniqueStrings(semantics.ReasoningVocabulary)) != len(semantics.ReasoningVocabulary) || len(uniqueStrings(semantics.ServiceTierRequested)) != len(semantics.ServiceTierRequested) || len(uniqueStrings(semantics.ServiceTierObserved)) != len(semantics.ServiceTierObserved) {
		return malformedf("Responses vocabularies must not contain duplicates")
	}
	if len(semantics.InputModalities) == 0 || len(semantics.ReasoningVocabulary) == 0 || len(semantics.ServiceTierRequested) == 0 || len(semantics.ServiceTierObserved) == 0 {
		return conflictf("incomplete Responses semantics")
	}
	if semantics.ReasoningDefault == "" || !contains(semantics.ReasoningVocabulary, semantics.ReasoningDefault) {
		return conflictf("Responses reasoning_default must be in the declared vocabulary")
	}
	if provider == "codex" || provider == "codex-pool" {
		if !sameStringSet(semantics.ReasoningVocabulary, []string{"low", "medium", "high", "xhigh"}) {
			return conflictf("Codex reasoning vocabulary must remain low, medium, high, xhigh")
		}
		if semantics.ReasoningOmissionMeansDefault {
			return conflictf("Codex reasoning omission cannot use generic default semantics")
		}
	} else {
		for _, level := range []string{"default", "none", "minimal", "low", "medium", "high", "xhigh"} {
			if !contains(semantics.ReasoningVocabulary, level) {
				return conflictf("generic Responses reasoning vocabulary is missing %q", level)
			}
		}
		if !semantics.ReasoningOmissionMeansDefault {
			return conflictf("generic Responses reasoning must preserve omission as default")
		}
	}
	return nil
}

func validateModels(target RevisionTarget, evidenceByID map[string]Evidence) error {
	seen := map[string]bool{}
	seenClassifications := map[string]bool{}
	for _, model := range target.Models {
		if model.ID == "" || model.Family == "" || model.Generation <= 0 || model.Variant == "" || model.ProviderID != target.ProviderID {
			return conflictf("target %q has incomplete model record %q", target.CanonicalName, model.ID)
		}
		if seen[model.ID] {
			return malformedf("target %q has duplicate model %q", target.CanonicalName, model.ID)
		}
		seen[model.ID] = true
		classification := fmt.Sprintf("%s\x00%d\x00%s", model.Family, model.Generation, model.Variant)
		if seenClassifications[classification] {
			return conflictf("target %q has ambiguous generation/variant evidence for %s", target.CanonicalName, classification)
		}
		seenClassifications[classification] = true
		if len(model.EvidenceRefs) == 0 {
			return conflictf("model %q has no evidence", model.ID)
		}
		for _, ref := range model.EvidenceRefs {
			evidence, ok := evidenceByID[ref]
			if !ok {
				return conflictf("model %q references missing evidence %q", model.ID, ref)
			}
			if evidence.Status != EvidencePositive && evidence.Status != EvidenceObserved {
				return conflictf("model %q lacks positive evidence", model.ID)
			}
			if evidence.Scope != target.Route.Scope {
				return conflictf("model %q evidence scope %q is out of scope for route %q", model.ID, evidence.Scope, target.Route.Scope)
			}
		}
		if !validCapability(model.Capabilities.TextInput) || !validCapability(model.Capabilities.ImageInput) || !validCapability(model.Capabilities.FileInput) || !validCapability(model.Capabilities.TUIWiring) || !validCapability(model.Capabilities.Runtime) {
			return malformedf("model %q has invalid tri-state capability", model.ID)
		}
	}
	if len(target.Models) == 0 {
		return conflictf("target %q has no evidenced models", target.CanonicalName)
	}
	return nil
}

func retainedModels(manifest Manifest, target RevisionTarget) ([]string, error) {
	byFamily := map[string]map[int]bool{}
	for _, model := range target.Models {
		if byFamily[model.Family] == nil {
			byFamily[model.Family] = map[int]bool{}
		}
		byFamily[model.Family][model.Generation] = true
	}
	var retained []string
	for family, ranks := range byFamily {
		if len(ranks) < manifest.Policy.RetainGenerations {
			return nil, conflictf("family %q has fewer than two explicit generations", family)
		}
		ordered := make([]int, 0, len(ranks))
		for rank := range ranks {
			ordered = append(ordered, rank)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(ordered)))
		for _, rank := range ordered[:manifest.Policy.RetainGenerations] {
			for _, model := range target.Models {
				if model.Family == family && model.Generation == rank {
					retained = append(retained, model.ID)
				}
			}
		}
	}
	sort.Strings(retained)
	return retained, nil
}

func validateExplicitRetirements(target RevisionTarget, retained map[string]bool) error {
	for _, model := range target.Models {
		if retained[model.ID] {
			continue
		}
		explicit := false
		for _, change := range target.Changes {
			if change.Kind == ChangeRemove && rawContainsModelID(change.ExpectedOld, model.ID) {
				explicit = true
				break
			}
		}
		if !explicit {
			return conflictf("retired model %q must have an explicit remove change", model.ID)
		}
	}
	return nil
}

func rawContainsModelID(raw json.RawMessage, id string) bool {
	var scalar string
	if json.Unmarshal(raw, &scalar) == nil {
		return scalar == id
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return false
	}
	return json.Unmarshal(object["id"], &scalar) == nil && scalar == id
}

func planChange(doc *jsonDocument, change RevisionChange, presetName, logicalPath string) (*RevisionPlanEntry, error) {
	node, parent, index, err := doc.lookup(change.Path)
	if err != nil {
		return nil, conflictf("change %q cannot resolve: %v", change.Path, err)
	}
	if !jsonEquivalent(doc.raw(node), change.ExpectedOld) {
		return nil, conflictf("expected-old conflict at %q", change.Path)
	}
	entry := &RevisionPlanEntry{
		PresetName:  presetName,
		LogicalPath: logicalPath,
		Path:        change.Path,
		Kind:        change.Kind,
		OldValue:    append(json.RawMessage(nil), doc.raw(node)...),
		EvidenceRef: change.EvidenceRef,
		Reason:      change.Reason,
		ModelRefs:   append([]string(nil), change.ModelRefs...),
		start:       node.start,
		end:         node.end,
	}
	sort.Strings(entry.ModelRefs)
	if change.Kind == ChangeReplace {
		entry.NewValue = compactJSON(change.NewValue)
		if jsonEquivalent(entry.OldValue, entry.NewValue) {
			return nil, nil
		}
		return entry, nil
	}
	if parent == nil {
		return nil, conflictf("cannot remove the JSON document root")
	}
	entry.removeInfo = removalSpan(parent, index, node)
	return entry, nil
}

func validateChangeSemantics(target RevisionTarget, change RevisionChange, retained map[string]bool) error {
	path := change.Path
	newValue := change.NewValue
	lower := strings.ToLower(path)
	if hasModelRevision([]RevisionChange{{Path: path}}) && strings.HasSuffix(lower, "/model") {
		var model string
		if err := json.Unmarshal(newValue, &model); err != nil || !retained[model] {
			return conflictf("model change %q is not one of the latest two explicitly evidenced models", path)
		}
	}
	capability, capabilityChange := capabilityForPath(lower)
	if capabilityChange {
		if len(change.ModelRefs) == 0 {
			return conflictf("capability change %q requires one or more evidenced model_refs", path)
		}
		modelsByID := make(map[string]ModelRecord, len(target.Models))
		for _, model := range target.Models {
			modelsByID[model.ID] = model
		}
		modelRevision := hasModelRevision(target.Changes)
		for _, ref := range change.ModelRefs {
			model, ok := modelsByID[ref]
			if !ok {
				return conflictf("capability change %q references unevidenced model %q", path, ref)
			}
			if modelRevision && !retained[ref] {
				return conflictf("capability change %q references retired model %q", path, ref)
			}
			if capabilityPromotion(newValue) && capabilityState(model.Capabilities, capability) != CapabilitySupported {
				return conflictf("capability change %q would promote a non-supported %s fact for model %q", path, capability, ref)
			}
		}
	} else if len(change.ModelRefs) != 0 {
		return malformedf("non-capability change %q must not declare model_refs", path)
	}
	if strings.Contains(lower, "wire_api") || strings.Contains(lower, "responses_transport") {
		if target.ProviderID != "custom" || target.Route.API != "responses" || target.Route.Binding.Mode != RouteBindingDirect || (target.Route.Transport != "http" && target.Route.Transport != "websocket") {
			return conflictf("wire_api/responses_transport is only eligible for custom Responses targets")
		}
	}
	if isReasoningPath(path) {
		if target.Responses == nil {
			return conflictf("reasoning change %q requires Responses semantics", path)
		}
		level, ok := strictJSONString(newValue)
		if !ok {
			return conflictf("reasoning change %q requires a string replacement", path)
		}
		if !contains(target.Responses.ReasoningVocabulary, level) {
			return conflictf("reasoning value %q is outside the evidenced vocabulary", level)
		}
	}
	serviceTierSide, serviceTierChange, err := classifyServiceTierPath(path)
	if err != nil {
		return err
	}
	if serviceTierChange {
		if target.Responses == nil {
			return conflictf("service-tier change %q requires Responses semantics", path)
		}
		tier, ok := strictJSONString(newValue)
		if !ok {
			return conflictf("service-tier change %q requires a string replacement", path)
		}
		vocabulary := target.Responses.ServiceTierRequested
		if serviceTierSide == serviceTierObserved {
			vocabulary = target.Responses.ServiceTierObserved
		}
		if !contains(vocabulary, tier) {
			return conflictf("service tier %q is outside the evidenced %s vocabulary", tier, serviceTierSide)
		}
	}
	return nil
}

type serviceTierSide string

const (
	serviceTierRequested serviceTierSide = "request"
	serviceTierObserved  serviceTierSide = "observation"
)

func classifyServiceTierPath(path string) (serviceTierSide, bool, error) {
	segments, err := splitPointer(path)
	if err != nil {
		return "", false, malformedf("invalid service-tier path %q: %v", path, err)
	}
	found := false
	requested := false
	observed := false
	for i, segment := range segments {
		segment = strings.ToLower(segment)
		switch segment {
		case "service_tier.requested", "service_tier_requested":
			found = true
			requested = true
		case "service_tier.observed", "service_tier_observed":
			found = true
			observed = true
		case "service_tier":
			found = true
			previous := ""
			next := ""
			if i > 0 {
				previous = strings.ToLower(segments[i-1])
			}
			if i+1 < len(segments) {
				next = strings.ToLower(segments[i+1])
			}
			if previous == "observed" || next == "observed" {
				observed = true
			} else {
				requested = true
			}
		}
	}
	if !found {
		return "", false, nil
	}
	if requested && observed {
		return "", false, conflictf("service-tier path %q is ambiguous between request and observation", path)
	}
	if observed {
		return serviceTierObserved, true, nil
	}
	return serviceTierRequested, true, nil
}

func isReasoningPath(path string) bool {
	segments, err := splitPointer(path)
	if err != nil {
		return false
	}
	for _, segment := range segments {
		lower := strings.ToLower(segment)
		if strings.Contains(lower, "reasoning") || strings.Contains(lower, "thinking") {
			return true
		}
	}
	return false
}

func strictJSONString(value json.RawMessage) (string, bool) {
	var decoded interface{}
	if err := json.Unmarshal(value, &decoded); err != nil {
		return "", false
	}
	result, ok := decoded.(string)
	return result, ok
}

func hasModelRevision(changes []RevisionChange) bool {
	for _, change := range changes {
		segments, err := splitPointer(change.Path)
		if err != nil {
			continue
		}
		for _, segment := range segments {
			if segment == "model" || segment == "models" || segment == "model_list" {
				return true
			}
		}
	}
	return false
}

func capabilityForPath(path string) (string, bool) {
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case "text", "text_input":
			return "text", true
		case "image", "images", "image_input", "vision":
			return "image", true
		case "file", "files", "file_input":
			return "file", true
		case "tui_wiring", "wiring":
			return "tui_wiring", true
		case "runtime", "account", "account_observation", "oauth":
			return "runtime", true
		}
	}
	return "", false
}

func capabilityPromotion(value []byte) bool {
	var enabled bool
	if json.Unmarshal(value, &enabled) == nil {
		return enabled
	}
	var state CapabilityState
	return json.Unmarshal(value, &state) == nil && state == CapabilitySupported
}

func capabilityState(capabilities CapabilityFacts, capability string) CapabilityState {
	switch capability {
	case "text":
		return capabilities.TextInput
	case "image":
		return capabilities.ImageInput
	case "file":
		return capabilities.FileInput
	case "tui_wiring":
		return capabilities.TUIWiring
	case "runtime":
		return capabilities.Runtime
	default:
		return CapabilityUnknown
	}
}

func validateInputRoute(doc *jsonDocument, target *RevisionTarget, contract InputContract) error {
	binding := target.Route.Binding
	if binding.Mode == "" {
		return nil
	}
	switch binding.Mode {
	case RouteBindingDirect:
		for _, field := range []struct {
			name    string
			binding *RouteFieldBinding
		}{
			{name: "api", binding: binding.API},
			{name: "transport", binding: binding.Transport},
			{name: "scope", binding: binding.Scope},
		} {
			if err := validateInputRouteField(doc, field.name, field.binding); err != nil {
				return err
			}
		}
	case RouteBindingProviderChild:
		return validateInputRouteField(doc, "provider", binding.Provider)
	default:
		return malformedf("unsupported route binding mode %q", binding.Mode)
	}
	return nil
}

func validateInputRouteField(doc *jsonDocument, name string, binding *RouteFieldBinding) error {
	raw, err := rawAt(doc, binding.Pointer)
	if err != nil {
		return conflictf("route field %q cannot be read: %v", name, err)
	}
	var got string
	if err := json.Unmarshal(raw, &got); err != nil || got != binding.Expected {
		return conflictf("route field %q does not match target", name)
	}
	return nil
}

func validateDisjointPaths(kind string, paths []string) error {
	for i := range paths {
		for j := 0; j < i; j++ {
			overlap, err := pointerPathsOverlap(paths[i], paths[j])
			if err != nil {
				return malformedf("invalid %s path %q: %v", kind, paths[i], err)
			}
			if overlap {
				return malformedf("overlapping %s paths %q and %q", kind, paths[j], paths[i])
			}
		}
	}
	return nil
}

func pointerPathsOverlap(left, right string) (bool, error) {
	leftSegments, err := splitPointer(left)
	if err != nil {
		return false, err
	}
	rightSegments, err := splitPointer(right)
	if err != nil {
		return false, err
	}
	if len(leftSegments) > len(rightSegments) {
		leftSegments, rightSegments = rightSegments, leftSegments
	}
	for i := range leftSegments {
		if leftSegments[i] != rightSegments[i] {
			return false, nil
		}
	}
	return true, nil
}

func findTarget(targets []RevisionTarget, name string) (*RevisionTarget, error) {
	var found *RevisionTarget
	for i := range targets {
		target := &targets[i]
		if target.CanonicalName == name || contains(target.Aliases, name) {
			if found != nil {
				return nil, conflictf("input name %q matches multiple targets", name)
			}
			found = target
		}
	}
	if found == nil {
		return nil, conflictf("undeclared/nonmatching target %q", name)
	}
	return found, nil
}

func validCapability(value CapabilityState) bool {
	return value == CapabilitySupported || value == CapabilityUnsupported || value == CapabilityUnknown
}

func validScope(scope EvidenceScope) bool {
	switch scope {
	case EvidenceScopePublicAPI, EvidenceScopeProviderEndpoint, EvidenceScopeNativeSDK, EvidenceScopeChatGPTOAuthCodex, EvidenceScopeAccountObservation:
		return true
	default:
		return false
	}
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	left, right := map[string]bool{}, map[string]bool{}
	for _, value := range got {
		left[value] = true
	}
	for _, value := range want {
		right[value] = true
	}
	return reflect.DeepEqual(left, right)
}

func compactJSON(data []byte) []byte {
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return append([]byte(nil), data...)
	}
	return compact.Bytes()
}

func validJSONValue(data []byte) bool {
	_, err := parseJSONDocument(data)
	return err == nil
}

func jsonEquivalent(a, b []byte) bool {
	var left, right interface{}
	leftDecoder := json.NewDecoder(bytes.NewReader(a))
	rightDecoder := json.NewDecoder(bytes.NewReader(b))
	leftDecoder.UseNumber()
	rightDecoder.UseNumber()
	if leftDecoder.Decode(&left) != nil || rightDecoder.Decode(&right) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

// The following small ordered JSON reader is intentionally private to the
// revision engine. encoding/json is used for manifest/schema values, while
// input values retain source offsets to avoid reserializing unowned content.
type jsonDocument struct {
	data []byte
	root *jsonNode
}

type jsonNode struct {
	kind   byte
	start  int
	end    int
	key    string
	object []jsonMember
	array  []*jsonNode
}

type jsonMember struct {
	keyStart int
	keyEnd   int
	value    *jsonNode
}

type jsonParser struct {
	data []byte
	pos  int
}

func parseJSONDocument(data []byte) (*jsonDocument, error) {
	parser := &jsonParser{data: data}
	parser.space()
	root, err := parser.value()
	if err != nil {
		return nil, err
	}
	parser.space()
	if parser.pos != len(data) {
		return nil, fmt.Errorf("trailing data")
	}
	return &jsonDocument{data: data, root: root}, nil
}

func (p *jsonParser) space() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) value() (*jsonNode, error) {
	p.space()
	if p.pos >= len(p.data) {
		return nil, fmt.Errorf("unexpected end")
	}
	start := p.pos
	switch p.data[p.pos] {
	case '{':
		p.pos++
		node := &jsonNode{kind: '{', start: start}
		p.space()
		if p.pos < len(p.data) && p.data[p.pos] == '}' {
			p.pos++
			node.end = p.pos
			return node, nil
		}
		keys := map[string]bool{}
		for {
			p.space()
			keyStart := p.pos
			key, err := p.string()
			if err != nil {
				return nil, err
			}
			if keys[key] {
				return nil, fmt.Errorf("duplicate object key %q", key)
			}
			keys[key] = true
			keyEnd := p.pos
			p.space()
			if p.pos >= len(p.data) || p.data[p.pos] != ':' {
				return nil, fmt.Errorf("missing colon")
			}
			p.pos++
			child, err := p.value()
			if err != nil {
				return nil, err
			}
			child.key = key
			node.object = append(node.object, jsonMember{keyStart: keyStart, keyEnd: keyEnd, value: child})
			p.space()
			if p.pos >= len(p.data) {
				return nil, fmt.Errorf("unterminated object")
			}
			if p.data[p.pos] == '}' {
				p.pos++
				node.end = p.pos
				return node, nil
			}
			if p.data[p.pos] != ',' {
				return nil, fmt.Errorf("missing object comma")
			}
			p.pos++
		}
	case '[':
		p.pos++
		node := &jsonNode{kind: '[', start: start}
		p.space()
		if p.pos < len(p.data) && p.data[p.pos] == ']' {
			p.pos++
			node.end = p.pos
			return node, nil
		}
		for {
			child, err := p.value()
			if err != nil {
				return nil, err
			}
			node.array = append(node.array, child)
			p.space()
			if p.pos >= len(p.data) {
				return nil, fmt.Errorf("unterminated array")
			}
			if p.data[p.pos] == ']' {
				p.pos++
				node.end = p.pos
				return node, nil
			}
			if p.data[p.pos] != ',' {
				return nil, fmt.Errorf("missing array comma")
			}
			p.pos++
		}
	case '"':
		if _, err := p.string(); err != nil {
			return nil, err
		}
		return &jsonNode{kind: 's', start: start, end: p.pos}, nil
	default:
		for p.pos < len(p.data) && !strings.ContainsRune(" \t\r\n,]}", rune(p.data[p.pos])) {
			p.pos++
		}
		if p.pos == start {
			return nil, fmt.Errorf("invalid value")
		}
		var value interface{}
		if err := json.Unmarshal(p.data[start:p.pos], &value); err != nil {
			return nil, err
		}
		return &jsonNode{kind: 'v', start: start, end: p.pos}, nil
	}
}

func (p *jsonParser) string() (string, error) {
	if p.pos >= len(p.data) || p.data[p.pos] != '"' {
		return "", fmt.Errorf("expected string")
	}
	start := p.pos
	p.pos++
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case '\\':
			p.pos += 2
		case '"':
			p.pos++
			var value string
			if err := json.Unmarshal(p.data[start:p.pos], &value); err != nil {
				return "", err
			}
			return value, nil
		default:
			if p.data[p.pos] < 0x20 {
				return "", fmt.Errorf("control character in string")
			}
			p.pos++
		}
	}
	return "", fmt.Errorf("unterminated string")
}

func (d *jsonDocument) raw(node *jsonNode) []byte { return d.data[node.start:node.end] }

func stringAt(d *jsonDocument, pointer string) (string, error) {
	raw, err := rawAt(d, pointer)
	if err != nil {
		return "", err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("not a string")
	}
	return value, nil
}

func rawAt(d *jsonDocument, pointer string) ([]byte, error) {
	node, _, _, err := d.lookup(pointer)
	if err != nil {
		return nil, err
	}
	return d.raw(node), nil
}

func (d *jsonDocument) lookup(pointer string) (*jsonNode, *jsonNode, int, error) {
	segments, err := splitPointer(pointer)
	if err != nil {
		return nil, nil, 0, err
	}
	node := d.root
	var parent *jsonNode
	index := 0
	for _, segment := range segments {
		parent = node
		switch node.kind {
		case '{':
			found := false
			for i, member := range node.object {
				if member.value.key == segment {
					node = member.value
					index = i
					found = true
					break
				}
			}
			if !found {
				return nil, nil, 0, fmt.Errorf("object key %q not found", segment)
			}
		case '[':
			i, err := strconv.Atoi(segment)
			if err != nil || i < 0 || i >= len(node.array) {
				return nil, nil, 0, fmt.Errorf("array index %q not found", segment)
			}
			index = i
			node = node.array[i]
		default:
			return nil, nil, 0, fmt.Errorf("cannot descend into scalar")
		}
	}
	return node, parent, index, nil
}

func splitPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("must start with /")
	}
	parts := strings.Split(pointer[1:], "/")
	for i, part := range parts {
		var builder strings.Builder
		for j := 0; j < len(part); j++ {
			if part[j] != '~' {
				builder.WriteByte(part[j])
				continue
			}
			if j+1 >= len(part) || (part[j+1] != '0' && part[j+1] != '1') {
				return nil, fmt.Errorf("invalid escape in segment %q", part)
			}
			if part[j+1] == '0' {
				builder.WriteByte('~')
			} else {
				builder.WriteByte('/')
			}
			j++
		}
		parts[i] = builder.String()
	}
	return parts, nil
}

func validatePointer(pointer string) error {
	_, err := splitPointer(pointer)
	return err
}

func removalSpan(parent *jsonNode, index int, node *jsonNode) *removeSpan {
	if parent.kind == '{' {
		member := parent.object[index]
		if len(parent.object) == 1 {
			return &removeSpan{start: member.keyStart, end: node.end}
		}
		if index < len(parent.object)-1 {
			return &removeSpan{start: member.keyStart, end: parent.object[index+1].keyStart}
		}
		return &removeSpan{start: parent.object[index-1].value.end, end: node.end}
	}
	if parent.kind == '[' {
		if len(parent.array) == 1 {
			return &removeSpan{start: node.start, end: node.end}
		}
		if index < len(parent.array)-1 {
			return &removeSpan{start: node.start, end: parent.array[index+1].start}
		}
		return &removeSpan{start: parent.array[index-1].end, end: node.end}
	}
	return &removeSpan{start: node.start, end: node.end}
}

// MaterializeRevisionBundle stages and publishes a new output directory. The
// input path may be a document or a directory bundle; only the contract's
// explicit document is changed. Existing output paths are refused.
func MaterializeRevisionBundle(inputPath, outputDir string, plan RevisionPlan) error {
	if outputDir == "" || !filepath.IsAbs(outputDir) {
		return &revisionError{code: ExitMalformed, msg: "apply output directory must be an explicit absolute path"}
	}
	if _, err := os.Lstat(outputDir); err == nil {
		return &revisionError{code: ExitFilesystem, msg: "apply output directory already exists"}
	} else if !os.IsNotExist(err) {
		return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("inspect apply output directory: %v", err)}
	}
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("stat revision input: %v", err)}
	}
	if inputInfo.IsDir() {
		rel, relErr := filepath.Rel(inputPath, outputDir)
		if relErr != nil || rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return &revisionError{code: ExitFilesystem, msg: "apply output directory must be outside the input bundle"}
		}
	}
	parent := filepath.Dir(outputDir)
	if _, err := os.Stat(parent); err != nil {
		return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("stat output parent: %v", err)}
	}
	stage, err := os.MkdirTemp(parent, ".preset-revision-staging-")
	if err != nil {
		return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("create staging directory: %v", err)}
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	if inputInfo.IsDir() {
		if err := copyRevisionTree(inputPath, stage, plan); err != nil {
			return &revisionError{code: ExitFilesystem, msg: err.Error()}
		}
		if err := os.Chmod(stage, inputInfo.Mode().Perm()); err != nil {
			return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("preserve input bundle mode: %v", err)}
		}
	} else {
		input, err := os.ReadFile(inputPath)
		if err != nil {
			return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("read revision input: %v", err)}
		}
		output, err := ApplyRevision(input, plan)
		if err != nil {
			return err
		}
		path := filepath.Join(stage, filepath.Base(inputPath))
		if err := os.WriteFile(path, output, inputInfo.Mode().Perm()); err != nil {
			return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("write staged revision: %v", err)}
		}
	}
	if err := os.Rename(stage, outputDir); err != nil {
		return &revisionError{code: ExitFilesystem, msg: fmt.Sprintf("publish staged revision: %v", err)}
	}
	cleanup = false
	return nil
}

func copyRevisionTree(inputRoot, stage string, plan RevisionPlan) error {
	return filepath.Walk(inputRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(inputRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(stage, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("revision bundles may contain regular files only: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == plan.LogicalPath {
			data, err = ApplyRevision(data, plan)
			if err != nil {
				return err
			}
		}
		if err := os.WriteFile(dest, data, info.Mode().Perm()); err != nil {
			return err
		}
		return nil
	})
}
