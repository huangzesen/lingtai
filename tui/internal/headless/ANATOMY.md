---
related_files:
  - tui/ANATOMY.md
  - tui/internal/preset/ANATOMY.md
  - tui/internal/config/ANATOMY.md
  - tui/internal/fs/ANATOMY.md
  - tui/internal/headless/headless.go
  - tui/internal/headless/headless_test.go
  - tui/internal/headless/presets.go
  - tui/internal/headless/presets_test.go
  - tui/internal/headless/preset_revision.go
  - tui/internal/headless/preset_revision_test.go
  - tui/internal/headless/spawn.go
  - tui/internal/headless/spawn_test.go
  - tui/internal/headless/spawn_serialization_test.go
  - tui/main.go
maintenance: |
  Keep related_files as repo-relative paths to real files. Include neighboring
  ANATOMY.md files so the anatomy graph stays connected rather than isolated;
  anatomy links must be bidirectional. If you create a new ANATOMY.md, copy this
  maintenance field. If you notice drift between this anatomy and the code,
  report it. See lingtai-dev-guide for details.
---

# headless

> **Maintenance:** see the `lingtai-tui-anatomy` skill at `tui/internal/preset/skills/lingtai-tui-anatomy/SKILL.md`. Coding agents update this file in the same commit as code changes.

`headless` is the TUI's non-interactive, JSON-emitting CLI surface: the machine-readable half of `lingtai-tui`. It backs the `bootstrap`, `presets`, and `spawn` subcommands wired from `tui/main.go` so another agent (or a script) can inventory presets and create/launch an agent without a terminal UI. Everything it prints is one JSON document on stdout, and every failure is one JSON error object plus a nonzero exit code — never a Bubble Tea screen.

## Components

| Symbol | Citation | Purpose |
|---|---|---|
| `WriteJSON` | `tui/internal/headless/headless.go:10` | encodes one indented JSON document to the supplied writer — the single output primitive every headless command uses |
| `WriteError` / `WriteErrorDetail` | `tui/internal/headless/headless.go:17,25` | the structured failure shape (`{"error": ..., "code": ...}`, plus optional `details`) so callers can branch on `code` rather than parse prose |
| `ExitError` | `tui/internal/headless/headless.go:37` | writes the error document to stderr and exits nonzero; used directly by `tui/main.go` argument parsing before a subcommand is entered |
| `PresetEntry` / `PresetsOutput` | `tui/internal/headless/presets.go:10,19` | the `presets` output schema — name, source (template/saved), description, and the resolved ref an `init.json` can cite |
| `RunPresets` | `tui/internal/headless/presets.go:25` | lists presets through `internal/preset`, honoring the mutually exclusive `--saved-only` / `--templates-only` filters |
| `RunPresetRevision` | `tui/internal/headless/preset_revision.go:40` | reads only explicit manifest/input paths, invokes the pure revision plan, exposes typed target state/reason in its stable JSON result, and maps dry-run/check/apply plus staged new-output publication to stable exit codes |
| `SpawnOpts` / `SpawnOutput` | `tui/internal/headless/spawn.go:42,53` | the existing `spawn` input options (target dir, preset, agent name, language) and output document (agent dir, name, preset, PID, readiness) |
| `RunSpawn` | `tui/internal/headless/spawn.go:78` | the whole non-interactive create-and-launch path: after argument validation, a per-resolved-root stable sibling `<root>.lingtai-spawn.lock` blocks cooperating creators before the `.lingtai` precheck and remains held through global setup, trusted kernel create, required post-create policy, and registration; it releases before launch/readiness. The rest resolves global state, runs `globalmigrate.Run`, uses a TUI-owned covenant temp input, performs complete trusted kernel create, shared-library/language compatibility writes, deliberately best-effort plain recipe bundle/apply work, required recipe-state persistence, registration, launch, then the existing bounded readiness wait (`defaultReadyTimeout`, `tui/internal/headless/spawn.go:21`) before emitting JSON |

## Connections

- **Called by `tui/main.go`** — `bootstrapMain`, `presetsMain`, `presets revise`, and `spawnMain` are thin argv parsers over this package. `presets revise` is dispatched before `presetsMain` can call `preset.Bootstrap`; it never resolves global state. The adjacent `doctorMain` and `selfUpdateMain` subcommands deliberately do NOT route through here: they repair the local install via `internal/config` rather than emitting headless JSON.
- **Calls `tui/internal/preset/`** for preset enumeration, selected-preset resolution, the TUI-owned covenant input, bundled utilities, and the retained plain-recipe state.
- **Calls `tui/internal/globalmigrate/`** (`tui/internal/headless/spawn.go:136`) so a headless spawn advances per-machine state exactly like an interactive start.
- **Calls `tui/internal/process/`** to seed through `CreateProject` and, only after its complete success protocol, to retain the existing guarded agent launch; it calls `tui/internal/fs/` to observe the new agent's manifest/heartbeat while waiting for readiness.

## Composition

- **Parent:** `tui/` (`tui/ANATOMY.md`)
- **Subfolders:** none — flat package
- **Siblings:** `tui/internal/tui/` is the interactive counterpart; both drive the same `preset`/`process`/`fs` layers.

## Notes

- **stdout is a protocol.** Anything printed on stdout by a headless command must be part of the documented JSON document. Kernel-create stdout/stderr are captured, never forwarded as a second document; handler exit-1 JSON is rendered as one headless stderr error object. Adding a stray `fmt.Println` breaks every scripted caller.
- **Create is conservative.** Registration and launch require exit 0 plus exactly one `{"status":"created"}` result for the requested agent with a nonempty canonical `preset_ref`. Exit 2, timeout/cancel/signal, startup failure, or malformed output is reported through the existing `init_failed` code with a structured reason; it neither registers/launches nor deletes a possibly partial tree. After trusted success, the caller restores only `.lingtai/.library_shared`; a later library/language/recipe-state persistence/registration failure emits `recovery_state:"created_unregistered"` with the project and agent identities instead of launching. The plain recipe bundle/apply paths remain deliberately best-effort legacy side effects: persisted `plain` state records the selected recipe, not proof that every optional recipe artifact was copied or applied. The temporary covenant source is TUI-owned and removed only after the child is terminal.
- **Readiness is bounded, not guaranteed.** `RunSpawn` waits up to `defaultReadyTimeout` for the agent to publish a heartbeat and reports what it observed; it does not block forever, and a not-yet-ready agent is reported, not treated as a failure to launch.
- **Error codes are the stable surface.** `invalid_args`, `init_failed`, `bootstrap_failed`, and the spawn-specific codes are consumed by callers. Rename one only with the callers in the same change.
- **Revision is explicit and staged.** `RunPresetRevision` uses only the manifest/input paths supplied by the caller. `dry-run` and `check` never write; `apply` validates once, stages a document/bundle in a hidden sibling, refuses an output path observed to exist, and attempts one rename into the requested path. A concurrent creator can race that inspection/publication window, with platform-dependent rename behavior; provider, OAuth, MCP, network, auth, Bootstrap, saved-preset, and current-runtime reads are outside this path.
