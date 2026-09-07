---
related_files:
  - CONTRACT.md
  - dev-guide-skill/SKILL.md
  - tui/architecture_documents_test.go
  - tui/ANATOMY.md
  - portal/ANATOMY.md
  - docs/ANATOMY.md
  - tui/internal/inventory/ANATOMY.md
  - README.md
  - README.zh.md
  - README.wen.md
  - RELEASING.md
  - CLAUDE.md
  - .github/workflows/release.yml
  - .github/workflows/windows-installer-smoke.yml
  - .github/workflows/delete-merged-branch.yml
  - .github/rulesets/main.json
  - .github/rulesets/release-tags.json
  - install.sh
  - install.ps1
  - scripts/update.sh
  - scripts/fix.sh
  - scripts/verify.sh
  - scripts/dev.sh
  - scripts/remove.sh
  - scripts/update.ps1
  - scripts/fix.ps1
  - scripts/verify.ps1
  - scripts/dev.ps1
  - scripts/remove.ps1
  - kernel-release.json
  - scripts/publish_bundle_to_gitee.sh
  - scripts/sync_gitee_mirror.sh
  - scripts/test-install-ps1.ps1
  - scripts/test-remove-ps1.ps1
  - scripts/test-lifecycle-assets.sh
  - scripts/test-install-sh-hardening.sh
  - scripts/test-install-sh-desktop.sh
  - tui/main.go
  - tui/go.mod
  - tui/Makefile
  - tui/internal/preset/skills/lingtai-tui-help/SKILL.md
  - tui/internal/preset/skills/lingtai-tui-anatomy/SKILL.md
  - portal/main.go
  - portal/embed.go
  - portal/go.mod
  - portal/Makefile
  - .github/workflows/sync-hf.yml
  - .gitignore
  - LICENSE
  - NOTICE
  - assets/braille/22610_source.png
  - assets/braille/22610_source.svg
  - assets/braille/22610_source_alt.svg
  - assets/braille/22610_w29_tui.txt
  - assets/braille/22610_w44.txt
  - assets/braille/22610_w66.txt
  - discussions/cascade-skill-and-sentinel-ordering-patch.md
  - discussions/codex-credential-redesign-patch.md
  - discussions/covenant-distillation-and-per-agent-profile.md
  - discussions/firstrun-step2-builtin-default-patch.md
  - discussions/intrinsics-strict-schema-scan.md
  - discussions/lingtai-preset-swap-silent-revert-patch.md
  - discussions/lingtai-vision-capability-fallback-patch.md
  - discussions/preset-editor-codex-oauth-patch.md
  - examples/bash_policy.json
  - examples/imap.jsonc
  - examples/init.jsonc
  - examples/telegram.jsonc
  - migration/migration.md
  - prompt/archive/base_prompt.md
  - prompt/archive/base_prompt_wen.md
  - prompt/archive/base_prompt_zh.md
  - prompt/archive/covenant_base.md
  - prompt/archive/covenant_base_lzh.md
  - prompt/archive/covenant_base_zh.md
  - prompt/archive/molt_prompt_default.md
  - reports/ANATOMY.md
  - scripts/dump_tool_descriptions.py
  - scripts/img2blocks.py
  - scripts/img2braille.py
  - scripts/rename.py
  - scripts/star_tracker.py
  - scripts/test-install-sh-mirror-bundle.sh
  - scripts/test-install-sh.sh
  - scripts/test-publish-bundle-to-gitee.sh
  - scripts/test-release-workflow-publish-gating.py
  - scripts/test-sync-gitee-mirror.sh
  - scripts/test_dump_tool_descriptions.py
  - scripts/test_star_tracker.py
maintenance: |
  This file is both the repository-root anatomy and the normative
  anatomy-of-anatomy for the distributed code navigation system across the two
  binaries (lingtai-tui, lingtai-portal) and the install pipeline. Keep
  related_files repo-relative, duplicate-free, and linked to real files. Keep
  the root CONTRACT.md reciprocal and update the paired conventions together
  when their boundary changes. Code is the structural source of truth: repair
  stale navigation in the same change that moves files, symbols, connections,
  composition, or state. Preserve the child template and its maintenance rule;
  validate the distributed graph before merge. Capability mentions in any
  document require explicit navigation mapping to the implementing code: a
  related_files entry in the owning ANATOMY.md (or a markdown link to that node
  when the document lives in the anatomy graph itself), bidirectional between
  document and owner. A capability with no mapping is drift; fix the mapping in
  the same change. See dev-guide-skill/SKILL.md for the workflow.
---

# lingtai

> **Maintenance:** this file and its `## Maintenance` section below are the
> normative convention. **Coding agents** update the relevant anatomy in the
> same commit as code changes. **LingTai agents** report drift as issues (mail
> or `discussions/<name>-patch.md`); do not silently fix.

## Purpose

**ANATOMY is the distributed code navigation system**, and this root file is
both its top-level map and its normative anatomy-of-anatomy. Each architectural
layer keeps an `ANATOMY.md` beside the code it maps, and those local maps link
into a graph an agent descends from this repository root to the exact code that
answers a structural question. Anatomy owns structure (code is the source of
truth); [`CONTRACT.md`](CONTRACT.md) is the paired system defining what each
layer promises. `## Components` below is the repository map — **start there**;
`## Anatomy convention` after it owns the schema and link rules.

This repo is the Go side of LingTai: `lingtai-tui`, `lingtai-portal`, and the
install pipeline. The Python kernel (`lingtai` on PyPI) lives in the sibling
`lingtai-kernel`. Only the TUI launches Python agents (as subprocesses); both
binaries observe them via the filesystem, and neither has a runtime Python
dependency.

> **What is an `ANATOMY.md`?** This root file defines the convention (see
> `## Anatomy convention`). The bundled `lingtai-tui-anatomy` skill
> (`tui/internal/preset/skills/lingtai-tui-anatomy/SKILL.md`) is the
> discoverable navigation aid into this distributed graph; this root remains
> normative, and the skill routes readers here rather than duplicating the
> convention.

## Components

The repo root holds two binary trees plus shared infrastructure. Each binary is a self-contained Go module; they communicate with running agents purely through the agent's working directory (`.lingtai/<agent>/`).

- **`ANATOMY.md` / `CONTRACT.md`** — the two normative distributed-system roots. This file is the code-navigation map and anatomy-of-anatomy; `CONTRACT.md` is the code-interface/Behavior definition root and contract-of-contract. They list each other in `related_files`.
- **`dev-guide-skill/`** — the repository-local agent dev kit. Its `SKILL.md` routes agents into the Anatomy and Contract systems and the change/validation workflow, and may grow focused scripts, references, templates, or assets as real workflows recur. Distinct from the bundled `lingtai-dev-guide` skill under `tui/internal/preset/skills/`, which ships to agents and owns deeper per-topic procedures.
- **`tui/architecture_documents_test.go`** — the real-repository architecture check in the existing TUI module (`cd tui && go test ./...`). It covers three things: the root Anatomy/Contract/dev-guide routing plus the links from the three READMEs and `CLAUDE.md`; the runtime/control-surface route anchors; and — `TestArchitectureDocumentsCoverEveryTrackedFile` — the graph-coverage rule, walking `related_files` from this root against `git ls-files` so an orphan tracked file, an unreachable `ANATOMY.md`, an empty/duplicated list, a self-link, or an entry that no longer resolves fails the build. It deliberately reads frontmatter with a narrow line-based reader rather than pulling in a YAML dependency; prose accuracy and semantic misdescription stay in review. The root documents belong to neither binary, so the check lives in the TUI module rather than a third module.
- **`tui/`** — Terminal UI binary (`lingtai-tui`). Bubble Tea v2 + lipgloss v2. Single-binary launcher, agent monitor, first-run wizard, mail viewer, preset editor. Builds to `tui/bin/lingtai-tui`. The flat `tui/main.go` wires subcommands (`purge`, `list`, `clean`, `suspend`, `bootstrap`, `presets`, `spawn`, `self-update`, `doctor`) and the interactive entry; everything substantive is under `tui/internal/`. See the per-package summary below.
- **`portal/`** — Web portal binary (`lingtai-portal`). Go HTTP server with an embedded React frontend served from a single binary via `embed.FS`. Reads the same `.lingtai/` filesystem the TUI does, surfaces a network visualisation, mail/replay UI, and topology recorder. Builds to `portal/bin/lingtai-portal`. Per-package layout under `portal/internal/`.
- **`install.sh`** — One-shot installer (`curl -fsSL https://lingtai.ai/install.sh | bash`), Homebrew-free. `--source auto|github|mirror` (default `auto`, or `LINGTAI_SOURCE`) selects the release provider: `auto` runs a bounded, fail-open public-IP country lookup (`detect_country_cn`) and prefers the lingtai.ai download-acceleration mirror for mainland China, falling back to GitHub on any detection/reachability failure — always for the SAME resolved tag/bundle, never by re-querying "latest" a second time (`resolve_source_provider`, `mirror_release_asset_url`, `fetch_bundle_manifest`, `fetch_kernel_manifest`). `--source gitee` is explicitly retired (rejected with a pointer to `--source mirror`); the mirror re-serves bytes GitHub has already published (Lingtai-AI/lingtai-web `docs/release-mirror/CONTRACT.md`) rather than hosting an independent release, so it has no "latest" of its own and version identity always comes from GitHub. Every tagged release exposes `lingtai-bundle-manifest.json` (schema `lingtai.tui.bundle/v1`, published by `.github/workflows/release.yml`'s `windows-release` job), binding one exact TUI tag/commit to one exact pinned kernel tag/version/artifacts/checksums; its strict parser (`parse_bundle_manifest`) also accepts a `lingtai-<tag>-windows-amd64.zip` archive entry for `install.ps1`'s use without selecting or downloading it itself — see `RELEASING.md`. Downloads a prebuilt per-platform tarball (`lingtai-<tag>-<os>-<arch>.tar.gz`) when the release exposes one, **verifying its `.sha256` sidecar before extraction**, otherwise falls back to building the release source tarball with Go/npm. Installs into `--bin-dir`/`--prefix`, else a writable `/usr/local/bin`, else `~/.local/bin` (never prefers Homebrew). Ordinary (non-`--update`) install is first-install-only: `validate_fresh_install_state`/`validate_install_target` refuse to run over an existing receipt or managed binary at the selected target, and refuse an existing runtime root unless explicit `--skip-python` requests a TUI/portal-only install that preserves that real legacy directory unchanged — pointing to `fix.sh`/`update.sh` instead of silently adopting or overwriting state — and the top of `main()` refuses a symlinked `$HOME/.lingtai-tui`. Then one-shot-creates/updates the Python runtime venv at `~/.lingtai-tui/runtime/venv`: `canonical_runtime_venv` requires that venv path to be physically (not merely lexically) contained under the owned runtime root, rejecting a symlink-escaped venv or ancestor, and `runtime_venv_state` refuses to silently reuse an already-occupied healthy/broken runtime on this same non-update path. LingTai is installed ONLY from a verified pinned kernel source, never from a package index by name: on the default release-asset path a resolved bundle manifest is mandatory, or — for a source-only TUI release with no dual bundle manifest — the exact-tag `kernel-release.json` pin (`fetch_kernel_pin`, schema `lingtai.tui.kernel-pin/v1`, same-tag-only provider fallback, never re-resolving "latest") is tried before failing loud; `kernel_tag_for_install`/`kernel_source_for_install` prefer an existing bundle and fall back to the pin. Either way `install_kernel_from_bundle` selects a compatible platform wheel for the venv's actual interpreter (`select_kernel_wheel`, via `packaging.tags.sys_tags()`) or the pinned sdist fallback, verifies its SHA256, and installs it by **explicit local file path**; the kernel release manifest itself is validated by a strict Python parser (`update_validate_manifest`) checking every required key, artifact shape, digest format, and — for wheels — that the filename's own version/tag triple agrees with its declared metadata, not a substring schema check. Only that artifact's third-party dependencies touch a package index, and exactly one: `python_dependency_index_url` returns a non-empty `LINGTAI_PYPI_INDEX_URL` if set, else the default of the provider that actually served the final bundle manifest (mirror → `https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple`, GitHub → `https://pypi.org/simple`) — one `--index-url` argv pair, never `--extra-index-url`, because `pypi.org` is not reliably reachable from the mainland-China hosts the mirror route exists to serve. This index selection covers the POSIX verified-bundle/pin path only; `install.ps1` and `--latest` (`install_kernel_from_main`) are unaffected, as is the version-pinned GitHub raw `install.sh` that `tui/internal/config/tui_updater.go`'s Homebrew migration / `/update-tui` bootstrap fetches before any provider selection happens — see "Lifecycle source ownership" below for why that flag's existing behavior is untouched by this integration. If no bundle manifest or release pin can be resolved (either provider, same-tag fallback attempted), or the resolved kernel artifact fails to verify/install, `ensure_runtime_venv` **fails loud** — the overall install exits nonzero rather than silently reaching for PyPI. `--ref`/source-ref builds have no bundle to pin against and fail loud the same way. `--skip-python` (alias `--skip-venv`) is the explicit opt-out for a TUI/portal-only install. The install postcondition (`runtime_health_check`) requires both `lingtai` and `lingtai.kernel` to import with `sys.prefix` equal to the selected venv AND both modules' `__file__` physically inside that same prefix — not merely importable — so a same-version package reachable through an external `.pth` entry or system site-packages can never be mistaken for a healthy owned install; stamps the env marker, symlinks `lingtai-agent`. Stamps exact `vX.Y.Z` release installs as that tag and writes `install.json` via exclusive-create, no-clobber, mode-`0600` publication on a fresh install (`install_method: "source"`, additive `install_kind: release-asset|source-build`, additive `runtime_venv` pointer, and — only on a verified kernel install — additive `kernel_source: "bundle"|"release-pin"` + the matching bundle/pin provenance fields) so both the TUI source updater and `tui/internal/config/venv.go`'s bundle-provenance gate can read it; `--update` legitimately republishes over its own existing receipt in place. On WSL/Debian/Ubuntu it can `apt-get install` missing Go/Python/git when interactive with sudo; non-interactive mode prints the exact command instead. Independently of source policy, still auto-detects CN-restricted Go-proxy reachability for source builds and falls back to mirrors for Go modules / `npm` / Go checksum DB. After that receipt is valid, an ordinary stable macOS install (including a release-tag `--from-source` fallback) or a version-pinned internal stable `--update` registers a self-contained lazy `lingtai-desktop` command pinned to Desktop `0.1.10` plus exact SHA-256 values audited from Desktop v0.1.10 commit `fd39dd61e4d123b2835064c1c148566d8b36ceb0`; `--skip-desktop` opts out. Registration performs zero Desktop network reads and creates no Desktop App/current/receipt/cache/version state. On the command's first execution only, it downloads and hash-verifies the Desktop project's four-file support trust set: `install-macos-app.py`, `desktop_user_cli.py`, the independent `verify-app-archive.py`, and stable `support_bootstrap.py`; it delegates release API/archive/manifest verification and atomic App publication to that code, then continues the requested command; later executions delegate to the installed current CLI without reinstalling. Linux/WSL, `--latest`, arbitrary `--ref`, ordinary existing-receipt reinstall, and native Windows do not enter this macOS-only boundary. The exact `v0.1.10` tag and release assets are public for the first Desktop command execution; a transient support, release, or transport read failure remains retryable and publishes no Desktop state. Helper functions are unit-tested via `scripts/test-install-sh.sh`, `scripts/test-install-sh-mirror-bundle.sh`, and `scripts/test-install-sh-hardening.sh`; `scripts/test-install-sh-desktop.sh` runs the offline fake-HOME orchestration journey (the kernel-release-pin, strict-manifest-validation, runtime-containment, existing-state, receipt-publication, and Desktop capabilities integrated from the public web fork — see "Lifecycle source ownership" below).
  Registration preserves a complete official Desktop launcher byte-for-byte when its managed current App executable is also a regular executable. An ordinary stable install likewise preserves a regular executable non-symlink target bearing this installer's lazy-bootstrap marker; an eligible stable update atomically refreshes that owned lazy command from the current generator and trust pins. Non-executable files, symlinks, foreign targets, and incomplete official launchers remain loud no-overwrite failures. This covers Desktop-first installs and Desktop/lazy state deliberately left behind by `scripts/remove.sh`.
  An ordinary re-run that detects an existing TUI receipt also retains its prior update/reinstall behavior and does not re-register the lazy Desktop command.
- **`install.sh --latest`** — Explicit POSIX development mode: resolves `refs/heads/main` in both `Lingtai-AI/lingtai` and `Lingtai-AI/lingtai-kernel` to full SHAs before shallow checkouts, verifies both checkouts against those pins, builds the TUI from the existing source path, installs the kernel from the checked-out local source (never by package name), and writes/shows both commits. It is explicit and conflicts with release/ref/update/source/python-skip modes; the no-argument path remains the latest official stable bundle flow.
- **`install.ps1`** — The native-Windows (PowerShell 5.1 Desktop and PowerShell 7+ Core) counterpart to `install.sh`, extending the same bundle/kernel-manifest contracts rather than a parallel protocol. Public (default, no `-ArchivePath`) mode resolves one exact `vX.Y.Z` tag from GitHub (`-Version`, or latest resolved once via the release API), downloads and strictly validates `lingtai-bundle-manifest.json` (`Confirm-BundleManifest`, the same field/shape/digest rules as `install.sh`'s `parse_bundle_manifest`), downloads `lingtai-<tag>-windows-amd64.zip` plus its `.sha256` sidecar, verifies the sidecar agrees with the manifest digest before trusting either, and confirms the staged `lingtai-tui.exe` reports exactly that tag before any `-BinDir` write. Unless `-SkipVenv`, it then provisions `%USERPROFILE%\.lingtai-tui\runtime\venv` from an already-available supported CPython 3.11/3.12/3.13 (`py` launcher or `python`/`python3` on PATH — never an unpinned Python/uv bootstrap), removes any of LingTai's OWN `lingtai-<version>.dist-info` directories whose metadata pip cannot parse and therefore refuses to uninstall (`Remove-OrphanedKernelDistInfo` — narrow by design, never a venv rebuild: the normalized `lingtai_kernel-*` sibling can never match and the resolved dependency tree is preserved), fetches the bundle's pinned `lingtai-kernel-release-manifest.json` (`Confirm-KernelManifest`), selects the wheel matching the venv's actual `cpXY-cpXY-win_amd64` tag (`Select-KernelWheel`), verifies its SHA-256, and installs it by **explicit local file path** (`Install-KernelWheel`) — LingTai is never requested from a package index by name and the kernel tag is never changed from the bundle's pin. Verifies `import lingtai`/version/non-editable provenance (`Confirm-KernelImport`, which resolves the distribution that actually **provides the imported module** — matching metadata `Name` *and* `RECORD` ownership of `lingtai.__file__`, never `importlib.metadata.distribution('lingtai')`, whose `.dist-info`-directory-name lookup lets a leftover `lingtai-<version>.dist-info` shadow the real provenance record; genuine ambiguity fails loud as `DIST_AMBIGUOUS`/`DIST_NOT_FOUND_FOR_MODULE`) and writes an additive `kernel-provenance.json` beside the venv before writing `install.json` (`install_kind: powershell-release-asset|powershell-local-artifact`, plus the same `kernel_source`/`kernel_bundle_id`/`kernel_version`/`kernel_provider` fields `install.sh` writes). `-ArchivePath`+`-ChecksumPath` is the local-artifact mode (offline binary install; the default runtime step still resolves the bundle over the network for `-Version`, since the kernel pin is not shipped inside the archive). `-SkipVenv` is the explicit binary-only mode; `-SkipPortal` is the TUI-only binary opt-out mirroring `install.sh --skip-portal` (portal is neither required nor installed; in `-Latest` mode the portal web frontend and portal Go build are skipped entirely). `-DryRun` performs the same resolution/validation reads but writes nothing. Contract-tested by `scripts/test-install-ps1.ps1` (public-mode resolution/validation against a local fake-GitHub-API `HttpListener`, plus the pre-existing local-artifact contracts) on both PowerShell hosts via `.github/workflows/windows-installer-smoke.yml`.
- **`install.ps1 -Latest`** — Explicit native-Windows current-main development mode: amd64-only, with WSL2 guidance for ARM64; first inventories Git, Go, Node.js/npm (Node 20.19+, 22.12+, or a newer major; Node 21 and Node 22.<12 are unsupported), and supported 64-bit CPython 3.11–3.13. Missing or invalid prerequisites are repaired only through exact winget packages (`Git.Git`, `GoLang.Go`, `OpenJS.NodeJS.LTS`, `Python.Python.3.13`) using `winget install --id <ID> --exact --source winget --accept-source-agreements --accept-package-agreements --disable-interactivity --silent`; package/policy/elevation failure fails loud with exact commands, successful prerequisite installs are external winget changes, and release/local-artifact modes never enter this bootstrap path. After bootstrap it refreshes this process PATH, revalidates every prerequisite (`git --version`, `go version`, `node --version`, `npm --version`, and supported 64-bit Python), resolves full `refs/heads/main` SHAs for both repositories before shallow checkout, verifies both exact checkouts, builds `lingtai-tui.exe` and `lingtai-portal.exe` (unless `-SkipPortal`, which skips the portal web frontend and portal Go build for a TUI-only dev install), installs the checked-out kernel as a non-editable local build, verifies that `lingtai.__file__` is inside the managed venv and PEP 610 `direct_url.json` identifies that exact checkout, and records cross-platform current-main provenance (`source_mode: latest-main`, `kernel_source: main`, full `tui_commit`/`kernel_commit`) in `install.json`. `-Latest -DryRun` reports the exact prerequisite repair plan, invokes no winget, and makes no destination, PATH, or config writes.
- **Lifecycle source ownership.** `install.sh` and `install.ps1` are the two stable, agent/user-facing entrypoints; each has its own `-h`/`--help`. `scripts/update.sh`, `scripts/fix.sh`, `scripts/verify.sh`, `scripts/dev.sh`, `scripts/remove.sh`, and the Windows counterparts `scripts/update.ps1`, `scripts/fix.ps1`, `scripts/verify.ps1`, `scripts/dev.ps1`, `scripts/remove.ps1` are standalone maintenance children for an *existing* installation — this repository is their sole byte-source, and `install.sh`'s own help text points here rather than re-documenting them. None of the ten sources `install.sh`/`install.ps1` or each other; each validates its own preconditions independently and reads/writes only the `lingtai.tui.install/v1` receipt (`install.json`) that `install.sh`/`install.ps1` already write via `write_install_metadata`/`Write-InstallMetadata`, so they operate unmodified against either entrypoint's installs. `install.sh --update` (see above) is a distinct, narrower, pre-existing contract: an internal non-interactive re-run used only by `tui/internal/config/tui_updater.go` for the TUI's own Homebrew-migration/self-update backends (`nativeMigrationInstallCommand`, `sourceTUIUpdater`) against a version-pinned raw-GitHub `install.sh`; it is not the same operation as `update.sh` and is out of scope for this lifecycle-source change.
  - **`scripts/update.sh`** — Explicit exact-artifact ordinary update after `--yes`: `--bin-dir DIR --runtime-python PATH --tui-archive ... --tui-sha256 HEX --kernel-artifact ... --kernel-sha256 HEX --tui-tag vX.Y.Z --kernel-version VERSION --yes`. Requires a strict healthy `release-asset`/`source-build` receipt (rejects `dev-source`); verifies every checksum/archive/identity before mutating; reinstalls the exact kernel wheel, atomically replaces `lingtai-tui`, then atomically revalidates and updates the receipt. Does not select "latest" itself (the caller resolves and supplies exact artifacts) and does not touch Portal.
  - **`scripts/fix.sh`** — Bounded runtime repair: read-only diagnosis by default; `--apply --yes` with `--kernel-artifact`/`--kernel-sha256` and one explicit, unoccupied, normalized direct child of `$HOME/.lingtai-tui/runtime` creates a new runtime venv, installs the exact kernel wheel, proves import/version provenance against the prior receipt's `kernel_version`, then atomically repoints only the receipt's `runtime_venv`. Never executes, deletes, or reuses the old runtime.
  - **`scripts/verify.sh`** — Strictly read-only proof of one exact target/runtime/receipt: `--bin-dir DIR --runtime-python PATH [--metadata PATH]`. Validates receipt schema/ownership, exact TUI identity, `sys.prefix`, import provenance (release or `dev-source`), and `lingtai.__version__ == kernel_version`; prints `PASS` with the observed values or fails loud. Never mutates.
  - **`scripts/dev.sh`** — Explicit editable development install after `--yes`: `--tui-source DIR --kernel-source DIR --bin-dir DIR [--runtime-python PATH] --yes [--skip-portal]`. Builds the supplied TUI/kernel Git checkouts, editable-installs the kernel checkout into the selected venv, installs the built binaries, and only after all postconditions pass writes one complete `dev-source` receipt with canonical source paths/commits — never a partial one. An existing target requires a valid v1 receipt owning the same target/runtime.
  - **`scripts/update.ps1`** — The native-Windows counterpart to `update.sh`, mirroring its exact-artifact ordinary-update contract: `-BinDir DIR -RuntimePython PATH -TuiArchive ... -TuiSha256 HEX -KernelArtifact ... -KernelSha256 HEX -TuiTag vX.Y.Z -KernelVersion VERSION -Yes`. The TUI archive is the Windows release `.zip` (`lingtai-<tag>-windows-amd64.zip`, expanded with System.IO.Compression after an unsafe-path scan); the kernel artifact is a `.whl`. Requires a strict healthy ordinary receipt (rejects dev/source-ref/latest-main provenance), verifies every checksum/archive/identity before mutating, reinstalls the exact kernel wheel, atomically replaces `lingtai-tui.exe`, then atomically revalidates and updates the receipt. Does not select "latest" itself and does not touch Portal.
  - **`scripts/fix.ps1`** — The native-Windows counterpart to `fix.sh`: bounded runtime repair, read-only diagnosis by default; `-Apply -Yes` with `-KernelArtifact`/`-KernelSha256` and one explicit, unoccupied, normalized direct child of `%USERPROFILE%\.lingtai-tui\runtime` creates a new runtime venv (bootstrap `python` parses only the old receipt and creates the venv; the old runtime is never executed), installs the exact kernel wheel, proves import/version provenance against the prior receipt's `kernel_version`, then atomically repoints only the receipt's `runtime_venv`. Never executes, deletes, or reuses the old runtime.
  - **`scripts/verify.ps1`** — The native-Windows counterpart to `verify.sh`: strictly read-only proof of one exact target/runtime/receipt: `-BinDir DIR -RuntimePython PATH [-Metadata PATH]`. Validates receipt schema/ownership, exact TUI identity, `sys.prefix`, import provenance (ordinary or `powershell-dev-source`), and `lingtai.__version__ == kernel_version`; prints `PASS` with the observed values or fails loud. Never mutates. `powershell-latest-main`/`powershell-source-ref` receipts stamp a `main-<sha>`/ref identity this verifier cannot prove and fail closed, mirroring `verify.sh`'s rejection of non-vX.Y.Z/dev stamps.
  - **`scripts/dev.ps1`** — The native-Windows counterpart to `dev.sh`: explicit editable development install after `-Yes`: `-TuiSource DIR -KernelSource DIR -BinDir DIR [-RuntimePython PATH] -Yes [-SkipPortal]`. Builds the supplied TUI/kernel Git checkouts, editable-installs the kernel checkout into the selected venv, installs the built binaries, and only after all postconditions pass writes one complete `powershell-dev-source` receipt with canonical source paths/commits — never a partial one. An existing target requires a valid v1 receipt owning the same target/runtime.
  - **`scripts/remove.sh`** / **`scripts/remove.ps1`** — The first lifecycle assets whose purpose is deletion: `--bin-dir DIR --yes` (POSIX) / `-BinDir DIR -Yes` (PowerShell). Deletes ONLY the exact artifact set the receipt proves `--bin-dir` owns — the managed `lingtai-tui`/`lingtai-portal` binaries, the POSIX-only `lingtai`/`lingtai-agent` symlinks (only if they are exactly the owned symlink shape, never an unrelated pre-existing file at that name), and the receipt-pointed runtime venv (physically re-validated as contained under `$HOME/.lingtai-tui/runtime` immediately before deletion, not from a stale precondition read). **The receipt is the only deletion oracle; there is no filename-pattern sweep of any kind.** In particular, `install.sh`'s repair-retry loop (`ensure_runtime_venv`) *retains* the old `runtime/venv` on an unhealthy-runtime retry and provisions a *new* path named `runtime/venv-repair-$$-1`, which then becomes the live receipt-pointed `runtime_venv` (`install.sh:1726-1728,1785-1790,1815-1820,1842-1847,1859,1194-1200`) — so a `venv-repair-*`-named directory, when it exists, is ordinarily the *live* venv the receipt names, not an orphan, and remove.sh deletes it (once, as the receipt's own `runtime_venv`) exactly like any other receipt-pointed venv; the retained *old* `runtime/venv` is the actual leftover, and remove.sh reports it as a survivor rather than guessing at its ownership. `fix.sh` does not create `venv-repair-*` names at all — its repair directory is a caller-supplied `--runtime-dir`, arbitrarily named (`scripts/fix.sh:57,101-103`), and `fix.sh` never deletes it either. Refuses (does not partially remove) a target whose resolved binary path is Homebrew-shaped (`HOMEBREW_PREFIX`/`HOMEBREW_CELLAR`/`HOMEBREW_REPOSITORY` env prefixes, a `/Cellar/` path component, or a well-known Homebrew root — the same path-based check as `tui/internal/config/venv.go`'s `detectHomebrewTUIInstall`), pointing at `brew uninstall` instead. Deletes the receipt LAST, only after every other owned artifact's deletion succeeds, so a partial failure (proven by real fault injection in the test suite) always leaves a receipt that accurately describes the still-partially-present install rather than a false-clean state; failure output names exactly which artifacts were and were not removed. On a `--bin-dir` mismatch, the error names the receipt's own `bin_dir` and an actionable re-run command rather than forcing the user to read `install.json` by hand. Never a recursive removal of `$HOME/.lingtai-tui` itself — only `rmdir`-style removal of the runtime root and state root, and only once empty, so NOT-owned state (`config.json`, `.env`, `tui_config.json`, `presets/saved/`, `codex-auth*.json`, per-project `.lingtai/` state, and any receipt-unproven directory such as a retained old `runtime/venv` or an unrelated `venv-repair-*`-named directory) is always preserved and reported as a survivor, never silently swept. Running either script twice is idempotent: a second run with no receipt present reports "nothing to remove" and exits 0. Accepts `release-asset`, `source-build`, `dev-source` and the Windows-native `powershell-*` kinds alike (remove.ps1's validKinds is kept in lockstep with install.ps1's Write-InstallMetadata install_kind values by the installer parity contract) — a dev install is still a real install a user may want to fully remove. Exit codes are intentionally asymmetric across platforms: `remove.sh` distinguishes a usage error (missing `--bin-dir`, exit 2) from a runtime/ownership refusal (exit 1) because it parses argv in a manual loop; `remove.ps1` relies on PowerShell's own `param()` binding plus one `Fail` helper for every refusal, so a missing `-BinDir` and every other refusal both exit 1 — documented in `remove.ps1`'s own `.NOTES`, not a defect to reconcile. Non-goals for v1: no selective-removal flags (the owned/not-owned boundary in this description is already unconditional and total), no automated legacy-Homebrew uninstall (explicit refusal only), no recovery of a broken/tampered receipt (removal refuses rather than guesses), no filename-pattern-based cleanup of any unreferenced runtime directory (receipt-proven ownership only; a future PR could add a `retained_runtimes`-style receipt field if a proven cleanup story is wanted).
  - Desktop App and lazy-command state are outside the TUI receipt and deliberately survive `remove.sh`; the next ordinary stable macOS install recognizes and keeps only a complete official launcher or this installer's marked lazy bootstrap, while the next eligible stable update keeps a complete official launcher but refreshes the marked lazy bootstrap. Neither path accepts a symlink, foreign file, or incomplete official launcher.
  - Contract-tested by `scripts/test-lifecycle-assets.sh`, which invokes each script as a real subprocess (none of the six is written as a guarded function library, so they are exercised by argv, not sourced) against a real venv, a real installed `lingtai` distribution, and a real `lingtai.tui.install/v1` receipt: `verify.sh` proves a full PASS and a dev-source rejection; `update.sh` proves a genuine artifact swap (new `lingtai-tui`, reinstalled kernel wheel, atomically revalidated receipt) plus its `--yes`/dev-source guards; `fix.sh` proves a real read-only diagnosis then a real `--apply --yes` repair venv; `dev.sh`'s precondition/argument validation is covered the same way, but its own Go/npm build is out of scope here — see its maintenance header, which reserves that acceptance run for an isolated environment with a real TUI/kernel checkout and toolchain; `remove.sh` proves a real happy-path removal (binaries/symlinks/venv/receipt actually gone, NOT-owned `.env` sentinel actually preserved), idempotent second run, `--yes`/`--bin-dir` usage errors, a `bin_dir` mismatch refusal whose error names the receipt's actual `bin_dir`, Homebrew-shape refusal, an unrelated pre-existing file at the `lingtai` alias name surviving untouched, a real fault-injected partial failure (an unwritable runtime root) followed by a real retry that completes the interrupted removal, and two dedicated receipt-is-the-only-oracle regression cases: an unrelated real directory that merely matches the `venv-repair-*` naming convention survives untouched, and a receipt whose own `runtime_venv` happens to be named `venv-repair-*` is removed exactly once (never double-listed) while a separately-retained, receipt-unproven old `runtime/venv` sibling survives and is reported as a survivor. `scripts/test-remove-ps1.ps1` is the `remove.ps1` analogue, written to exercise the same real-subprocess/real-fixture/real-fault-injection contracts (including a genuine open-file-handle lock to force a Windows-real partial-removal fault, and a partial/tampered-receipt fixture proving every `Set-StrictMode`-guarded property read fails clean rather than crashing) — **executed as of #796**: the `windows-installer-smoke.yml` suite runs it on both Windows PowerShell 5.1 (Desktop) and PowerShell 7 (Core) with success; keep running it there on every relevant change.
- **`kernel-release.json`** — Repo-owned compatibility pin (schema `lingtai.tui.kernel-pin/v1`). `.github/workflows/release.yml`'s `windows-release` job reads `kernel_tag` from this file to bind each TUI release's bundle manifest to one exact kernel release; it fails closed before building anything if the pinned kernel release or its `win_amd64` wheel manifest entry doesn't exist. Bump it deliberately, in the same PR/commit that intends to ship a new kernel version with the next TUI release — the workflow never resolves "latest kernel."
- **`scripts/`** — The standalone lifecycle maintenance assets (`update.sh`, `fix.sh`, `verify.sh`, `dev.sh`, `remove.sh` and their Windows counterparts `update.ps1`, `fix.ps1`, `verify.ps1`, `dev.ps1`, `remove.ps1` — see "Lifecycle source ownership" above; only `install.sh`/`install.ps1` stay at the repo top level) plus auxiliary Python utilities (image-to-blocks, tool description dumper, file-rename helper) and release/installer test and publish infrastructure: `test-install-sh.sh` / `test-install-sh-mirror-bundle.sh` / `test-install-sh-hardening.sh` / `test-install-sh-desktop.sh` (source `install.sh` with `LINGTAI_INSTALL_SH_SOURCE_ONLY=1` against fake transports and fake homes), `test-publish-bundle-to-gitee.sh` / `test-sync-gitee-mirror.sh` (exercise standalone scripts against fake-curl or real local-git-remote fixtures), `test-install-ps1.ps1` (the `install.ps1` contract suite — see above), `test-lifecycle-assets.sh` (real-subprocess contract suite for `update.sh`/`fix.sh`/`verify.sh`/`dev.sh`/`remove.sh` — see "Lifecycle source ownership" above), `test-remove-ps1.ps1` (the `remove.ps1` real-subprocess/real-fault-injection contract suite, Windows-only — see "Lifecycle source ownership" above), `test-release-workflow-publish-gating.py` (static assertions that `release.yml` is exactly `source-release`+`update-homebrew`+`windows-release`, that only `windows-release` uploads assets, and that it fails closed on the kernel pin), `sync_gitee_mirror.sh` (non-force git push of the exact release commit/tag to the Gitee mirror — fast-forward-only, create-only tag, never `--force`), and `publish_bundle_to_gitee.sh` (the Gitee release asset publisher, `--execute`-gated). The Gitee sync/publish scripts remain explicit maintainer tools and are not invoked by the tag workflow; see `RELEASING.md`. NOT the runtime — these are dev/release tools, not shipped in any TUI/portal binary.
- **`examples/`** — Reference config files (`init.jsonc`, `bash_policy.json`, `imap.jsonc`, `telegram.jsonc`) for users wiring up their own agents.
- **`docs/`** — Repo-native developer and reference docs (specs, plans, daily change log, screenshots, known limitations, graphify). The human-facing beginner guide now lives on the website tutorial (`https://lingtai.ai/{en,zh,wen}/tutorial/`), not in this repo; see `docs/ANATOMY.md`.
- **`reports/`** — Local-only by default. The tracked files here are the deliberate exceptions: one evidence bundle per shipped release plus a few promoted explainers. See `reports/ANATOMY.md` for the tracked/untracked boundary and `CLAUDE.md` for the working rule.
- **`prompt/`** — Localised prompt fragments. Everything tracked here now lives under `prompt/archive/` — the superseded base-prompt, covenant, and molt-prompt originals, kept for provenance after the live copies moved into the kernel and `tui/internal/preset/`. Nothing in either binary reads this directory.
- **`assets/`** — Static images (logos, screenshots) used by README and docs, plus `assets/braille/` — the source art and the pre-rendered 29/44/66-column braille variants of the 𢘐 (U+22610) glyph behind the first-run splash (`tui/internal/tui/firstrun.go:3436`).
- **`discussions/`** — Patch proposals and design discussions written by LingTai agents. This is where the maintenance banner's "report drift as issues, do not silently fix" rule lands: `discussions/<name>-patch.md`.
- **`migration/`** — `migration/migration.md`, the release-scoped migration note (product, release tag, pinned kernel tag) handed to users upgrading across a breaking release.
- **`README.md` / `README.zh.md` / `README.wen.md`** — Tri-lingual project README: concise orientation (what LingTai is, install/start, interfaces, architecture, contributing). Each links to its locale's website tutorial (`https://lingtai.ai/{en,zh,wen}/tutorial/`) for step-by-step beginner learning rather than duplicating it.
- **`RELEASING.md`** — Release process: tag, GitHub release, Windows asset/bundle publication, automated Homebrew tap update, manual tap fallback, and the PowerShell install path.
- **`.github/workflows/release.yml`** — Tag-push workflow (`v*` push only), three jobs. `source-release` verifies the tag and creates the public GitHub Release without building or uploading binaries. `update-homebrew` fails closed unless the pushed tag is an exact `vX.Y.Z` (checked in its first step, before the `Lingtai-AI/homebrew-lingtai` checkout materializes `HOMEBREW_TAP_TOKEN` on the runner), then computes the GitHub tag source-tarball checksum, rewrites `lingtai-tui.rb`, and pushes the source-build formula update to that tap; the tag reaches every step as `env: TAG`, never as a `${{ }}` expression interpolated into a shell script. `windows-release` (`needs: source-release`) fails closed unless `kernel-release.json`'s pinned kernel release exists and publishes a verified `win_amd64` wheel, then cross-compiles `lingtai-tui.exe`/`lingtai-portal.exe` for `windows/amd64`, packages `lingtai-<tag>-windows-amd64.zip`+`.sha256`, generates `lingtai-bundle-manifest.json`, and uploads all three via `gh release upload` — the only job in this workflow that uploads assets — then, once the release is public, its final step dispatches a `repository_dispatch` (`release-asset-published`) to `Lingtai-AI/lingtai-web` naming those same three assets for lingtai.ai's download-acceleration mirror (gated on the `LINGTAI_WEB_DISPATCH_TOKEN` deployment prerequisite; never edits or blocks the GitHub release itself). See `RELEASING.md`.
- **`.github/rulesets/`** — Intended GitHub branch/tag ruleset state as reviewable JSON (`main.json` targets `~DEFAULT_BRANCH`; `release-tags.json` makes `refs/tags/v*` immutable so the ref that triggers `release.yml` cannot be moved or deleted after publication). Rulesets are repository *settings*, so nothing in this tree applies them — an admin POSTs these files. `docs/repository-rulesets.md` owns the evidence for why they exist, the apply commands, and the read-back checks that prove a ruleset actually targets a ref. Not read by any binary.

- **`.github/workflows/windows-installer-smoke.yml`** — Runs `scripts/test-install-ps1.ps1` and `scripts/test-remove-ps1.ps1` under both Windows PowerShell 5.1 and PowerShell 7 on `windows-latest` (PR/push, no live-release dependency), plus a `windows-release-asset-smoke` job gated to `push: tags: v*` that polls the just-published release for its Windows asset and runs a real `-SkipVenv` install against it.
- **`.github/workflows/delete-merged-branch.yml`** — Deletes a pull request's head branch when that PR merges, so merged refs stop accumulating on `origin`. Runs on `pull_request_target: closed` (base-branch definition, writable token, no checkout and no PR-authored code) and acts only when the PR actually merged and its head lives in this repository — fork heads and closed-without-merge branches are left alone, as are the default/`develop`/`gh-pages`/`release/*` branches and any branch that is still the head of another open PR. An already-deleted ref is a success, not a failure, so it composes with the repository's own "automatically delete head branches" setting rather than fighting it.
- **`CLAUDE.md`** — Repo-specific Claude Code instructions (build commands, gotchas, sibling repos).

### `tui/` packages

| Package | LOC | Role |
|---------|-----|------|
| `tui/internal/tui/` | ~22k | Bubble Tea models for every screen — first-run wizard, network home (`app.go`), agent detail, mail composer, preset editor, knowledge/skills, doctor, addon installer. The biggest module by far; the `tui/` package is itself decomposable but the boundaries match Bubble Tea's screen-per-file convention. |
| `tui/internal/preset/` | — | Atomic `{llm, capabilities}` bundle layer. `preset.go` (~1900 lines) handles load/save/list, `recipe_apply.go` handles recipe import, `state.go` tracks user preset state. Embeds the canonical preset templates, covenant text, principles, soul fragments, procedures, skills, and recipe assets via `//go:embed`. |
| `tui/internal/migrate/` | — | Retained m001–m039 historical source/tests and registry API; production startup, project creation, launcher, and diagnostics do not execute it or advance `.lingtai/meta.json`. See `tui/internal/migrate/ANATOMY.md`. |
| `tui/internal/globalmigrate/` | — | Per-machine analogue under `~/.lingtai-tui/`. Same conventions, separate version space (`~/.lingtai-tui/meta.json`). For things like Homebrew tap renames and runtime venv relocations. Currently at v2; v2 (`split-presets-dir`) is a neutralized no-op tombstone — it once moved/deleted flat `presets/*.json` files and caused the preset-loss incident, so its destructive body was removed while the version entry is retained for advancement semantics. See `tui/internal/globalmigrate/ANATOMY.md`. |
| `tui/internal/fs/` | — | Filesystem accessors: agent manifest, heartbeat, mail (read/list/write outbox), token ledger, location, network discovery, signal files, session JSONL load. The TUI's read-only window into a running agent's working directory. |
| `tui/internal/sqlitelog/` | — | Small sqlite3 CLI-backed readers for kernel `logs/log.sqlite`; currently used by `/notification` to page notification events just-in-time instead of relying on stale `.notification/` snapshots. See `tui/internal/sqlitelog/ANATOMY.md`. |
| `tui/internal/config/` | — | Global TUI config under `~/.lingtai-tui/`: `tui_config.json`, runtime venv resolution, addon registry. |
| `tui/internal/process/` | — | Subprocess launcher (`launcher.go`). Spawns `python -m lingtai run <dir>` with the right venv, log redirection, and PID tracking; also the terminate path. See `tui/internal/process/ANATOMY.md`. |
| `tui/internal/inventory/` | — | Typed running-agent inventory shared by `lingtai-tui list` and `/projects`: processscan rows plus `.agent.json`/heartbeat/status/admin/IM enrichment, duplicate collapse, deterministic grouping, and admin-only enterability. |
| `tui/internal/headless/` | — | JSON-emitting non-interactive CLI surface. Backs the `bootstrap`, `presets`, and `spawn` subcommands wired from `tui/main.go` (`bootstrapMain`, `presetsMain`, `spawnMain`). The adjacent `doctorMain` and `selfUpdateMain` subcommands use `config` update routines directly because they repair the local install rather than emitting headless JSON. Exposes `RunPresets`, `RunSpawn`, `ExitError` for structured agent-consumable output. See `tui/internal/headless/ANATOMY.md`. |
| `tui/i18n/` | — | en/zh/wen JSON tables. **Three locales always** — adding a key requires updating all three; a key missing everywhere renders as the raw key string. See `tui/i18n/ANATOMY.md`. |
| `tui/internal/doctorreport/` | — | Writer-only serializer for a finished `/doctor` run: redacts the captured draft and emits the private `report.md`/`metadata.json`/`redaction.json` bundle. Runs no diagnostics of its own. See `tui/internal/doctorreport/ANATOMY.md`. |
| `tui/scripts/` | — | Build helper scripts (cross-compile, asset bundling). |
| `tui/packages/` | — | Vendored or generated dependency artefacts. |
| Per-OS `*_unix.go` / `*_windows.go` | — | Platform-specific shims for `agent_count`, `exec`, `list`, `purge`, `suspend` subcommands. |

### `portal/` packages

| Package | LOC | Role |
|---------|-----|------|
| `portal/internal/api/` | ~1.5k | HTTP server (`server.go`), handlers (`handlers.go`), and replay endpoint (`replay.go` — 784 lines, the largest single API surface). Listens on a randomly-chosen port (or `--port`), writes the bound port to `.portal/port` so the TUI can find it. |
| `portal/internal/fs/` | ~2.2k | Same shape as `tui/internal/fs/` but tailored to portal's needs: agent reading, heartbeat, mail, network/topology reconstruction (`reconstruct.go`, 326 lines), location resolution. |
| `portal/internal/migrate/` | — | Retained m001–m039 historical source/tests and registry API; Portal production startup does not execute it or advance `.lingtai/meta.json`. See `portal/internal/migrate/ANATOMY.md`. |
| `portal/web/` | — | React 19 + TypeScript + Vite frontend. Source under `portal/web/src/` (`App.tsx`, `Graph.tsx`, `BottomBar.tsx`, `FilterPanel.tsx`, etc.). Builds to `portal/web/dist/` then `embed.go` (`//go:embed all:web/dist`) compiles it into the Go binary. |
| `portal/i18n/` | — | en/zh/wen JSON tables. Independent of the TUI's i18n — same three-locale rule. |
| `portal/docs/` | — | Portal-specific docs and screenshots. |

## Connections

- **TUI → kernel.** The TUI launches the kernel as a subprocess: `python -m lingtai run <agent-dir>` via `process/launcher.go`. The kernel is installed into `~/.lingtai-tui/runtime/venv/` (an isolated venv set up on first run via `pip install lingtai`). After spawn, the TUI never talks to the agent process directly — only via the agent's working directory.
- **TUI → filesystem (read).** `internal/fs/` reads `.lingtai/<agent>/.agent.json`, `.agent.heartbeat`, `mailbox/`, `logs/token_ledger.jsonl`, `history/chat_history.jsonl`, `system/*.md`. The kernel writes these; the TUI never writes them.
- **TUI → filesystem (write).** Signal files only: `.lingtai/<agent>/{.sleep, .suspend, .interrupt, .clear, .prompt, .refresh, .inquiry}`. The kernel polls these on each heartbeat tick. `init.json` is also writeable but only via explicit user actions in the wizard / preset editor.
- **TUI → human pseudo-mailbox.** The TUI is the user's MUA: it writes outbound messages into `.lingtai/human/mailbox/outbox/<uuid>/message.json`; agents poll this folder and claim deliveries.
- **Portal → filesystem.** Same read pattern as the TUI; additionally writes `.lingtai/.portal/port`, recordings under `.lingtai/.portal/recordings/`, and topology snapshots that feed the replay timeline.
- **Portal ↔ TUI integration.** `lingtai-tui` discovers an installed `lingtai-portal` to launch on `/viz`; otherwise the binaries are independent.
- **TUI ↔ Homebrew tap.** Pushing a release tag runs `.github/workflows/release.yml`, which updates `Lingtai-AI/homebrew-lingtai/lingtai-tui.rb`, so `brew install`/manual `brew upgrade lingtai-ai/lingtai/lingtai-tui` still pull from there. Manual tap edits are fallback/debug steps only. See `RELEASING.md`. LingTai's own update paths (`/update-tui`, `self-update`, `doctor`) no longer run `brew upgrade` for a detected Homebrew install — they migrate it to the native installer instead (`tui/internal/config/tui_updater.go`'s `homebrewTUIUpdater`), leaving the old formula/keg installed but no longer the update target.
- **Portal embeds web frontend.** `embed.go` at the portal root compiles `portal/web/dist/` into the Go binary so `lingtai-portal` ships with no runtime dependency on Node.

### Cross-repo dependencies

This repo depends on `lingtai-kernel` only at runtime (the Python agent it launches), not at build time. Other sibling repos:

- **`lingtai-kernel`** — Python kernel + `lingtai` PyPI package. Owns the canonical agent runtime.
- **`lingtai-skill`** — Single-source-of-truth for the mailbox-protocol `SKILL.md`. Vendored into plugin repos via `lingtai-claude-code/scripts/sync-from-canonical.sh`.
- **`lingtai-claude-code`** — Claude Code plugin (SessionStart hook, marketplace manifest).
- **`codex-plugin`** — OpenAI Codex CLI plugin.
- **`lingtai-imap` / `lingtai-telegram` / `lingtai-feishu` / `lingtai-wechat`** — MCP server addons. Each ships as a separate PyPI package.
- **`Lingtai-AI/homebrew-lingtai`** — Homebrew tap for `lingtai-tui`.

## Composition

- **Parent:** none — this is a top-level repo.
- **Subfolders:** `tui/`, `portal/`, `docs/`, `examples/`, `prompt/`, `scripts/`, `assets/`. The TUI and portal each have full per-package internal trees with their own `internal/` boundaries.
- **Build outputs:** `tui/bin/lingtai-tui`, `portal/bin/lingtai-portal`. Cross-compile via `make cross-compile` in either directory (darwin/linux/windows × amd64/arm64).
- **Module names:** `github.com/anthropics/lingtai-tui` and `github.com/anthropics/lingtai-portal`. Note the historical naming — these are NOT moving to a `Lingtai-AI/` import path even though the GitHub org renamed.

## State

- **Per-project state** under `<project>/.lingtai/`:
  - `meta.json` — legacy project migration metadata may remain on disk, but TUI and Portal production do not read, write, or advance it.
  - `<agent>/init.json` — the agent's preset manifest (LLM + capabilities + allowed presets list).
  - `<agent>/.agent.json` / `.agent.heartbeat` / `.status.json` — written by the agent, read by the TUI/portal.
  - `<agent>/mailbox/{inbox,outbox,sent,archive}/<uuid>/message.json` — filesystem mailbox.
  - `<agent>/logs/log.sqlite` — kernel event trace; `/notification` reads notification events from this database just-in-time so the view reflects current log history rather than a sidecar snapshot.
  - `<agent>/.notification/<channel>.json` — `.notification/` filesystem-as-protocol sidecar signals (email, soul, system events). The TUI no longer renders these directly in `/notification`; `/goal` remains the narrow write exception that appends a `goal.request` event to `<agent>/.notification/system.json` so the running agent can guide goal setup.
  - `human/` — the user's pseudo-agent (no admin, no heartbeat). Mailbox layout identical to a real agent.
  - `.tui-asset/` — TUI-owned per-project caches (remotes list, etc.).
  - `.portal/port` / `.portal/recordings/` — portal-owned files when running.
- **Per-machine state** under `~/.lingtai-tui/`:
  - `meta.json` — global migration version stamp.
  - `tui_config.json` — global TUI preferences (default language, model selection, etc.).
  - `runtime/venv/` — Python venv with `lingtai` installed; agents launch from here.
  - `presets/templates/` — TUI-owned, rewritten on every Bootstrap from embedded data. Don't hand-edit.
  - `presets/saved/` — User-owned preset clones; the wizard's auto-clone-on-edit lands new presets here as `<template>-<N>.json`.
  - `utilities/` — Optional skills paths surfaced to agents.

## Notes

- **Runtime/control-surface boundary:** TUI and Portal are control/presentation processes; the independently running kernel process owns the agent heartbeat, listeners, and lifecycle. Closing a frontend is not an agent lifecycle operation, and ordinary persistence does not require a second `launchd` supervisor. Use explicit lifecycle commands and inspect current state instead. `tui/ANATOMY.md` carries the same-repo quit/launch/attach/signal/inventory file-and-symbol routes; `portal/ANATOMY.md` carries the Portal shutdown boundary; exact Python runtime semantics remain in the separate `lingtai-kernel-anatomy` graph.
- **Human-facing docs ownership:** the step-by-step beginner guide lives on the website tutorial (`https://lingtai.ai/{en,zh,wen}/tutorial/`), maintained outside this repo. In-repo, the human-facing surfaces are the three READMEs (concise orientation) and the bundled help assets (`tui/internal/preset/skills/lingtai-tui-help/assets/`, the canonical slash-command catalog). Any change that adds/removes/renames user-visible capabilities, slash commands, setup/install flows, channel/addon surfaces, memory/molt behavior, daemon/avatar behavior, or safety boundaries must keep the README orientation and help assets accurate and flag the website tutorial for a matching update (tracked in the separate website repo).
- **Binary naming.** The TUI binary is `lingtai-tui`, never `lingtai`. `lingtai` is the Python agent CLI inside the runtime venv (`~/.lingtai-tui/runtime/venv/bin/lingtai`). Build to `tui/bin/lingtai-tui`; never `tui/bin/lingtai`.
- **Bubble Tea v2 paste delivery.** Bubble Tea v2 splits keys (`tea.KeyPressMsg`) from clipboard pastes (`tea.PasteMsg`). Any `Update` dispatcher gating on `case tea.KeyPressMsg:` must also forward `tea.PasteMsg` to whichever text widget is focused — otherwise paste silently drops. For embedded sub-models hosted inside another model (e.g. `PresetEditorModel` inside `FirstRunModel`), the host's outer `default:` branch must forward paste msgs into the sub-model. Trace top-down to find missing layers; the symptom is "typing works, paste does nothing."
- **`textarea` vs `textinput`.** For any paste-friendly field (API keys, base URLs), use `textarea` even when the content is conceptually one line. `textinput` drops characters on multi-byte / clipboard pastes. Always apply `themedTextareaStyles()` from the `tui` package — bare `textarea.New()` ships dark default cursor/focus colors that render as a black smear against the warm theme.
- **Migration retirement.** TUI and Portal retain the shared migration registries as non-executing history/test APIs. Production does not consult or stamp project migration progress; compatibility diagnosis/repair belongs to the kernel reader and explicit Agent edits.
- **Dev-mode rebuild gotcha.** Rebuild both binaries after code changes as usual; runtime project migration bumps are retired, so a stale migration registry is not a startup compatibility gate.
- **Preset architecture.** Presets are atomic `{llm, capabilities}` bundles. `templates/` is TUI-owned (rewritten every Bootstrap from embedded data, prunes retired entries — never hand-edit). `saved/` is user-owned (Bootstrap never touches it). The directory IS the answer to "is this a template?" — there's no in-band marker. Each loaded `Preset` carries a `Source` field (`SourceTemplate` / `SourceSaved`); prefer `IsTemplate(p)` over the legacy `IsBuiltin(p.Name)`. When writing `manifest.preset.*` paths from Go, always use `preset.RefFor(p)` to pick the right subdirectory based on `Source`.
- **Authorization gate.** `manifest.preset.allowed` is the explicit list of preset paths the agent may swap to at runtime. The kernel refuses any swap not in `allowed`. `default` and `active` MUST both appear in `allowed`; `init_schema.validate_init` enforces this. m029 was the migration that introduced this declarative form.
- **Three-locale rule.** Adding an i18n key means updating en.json, zh.json, AND wen.json in BOTH `tui/i18n/` and (where applicable) `portal/i18n/`. Missing translations show as the raw key on screen — they don't fall back. Procedural / dev-only strings can stay English-only with a comment noting why.
- **Filesystem-only IPC.** The TUI and portal never open a socket or RPC channel to a running agent. All communication is via files: agent manifests, heartbeats, signal files, mailbox folders, `.notification/` sidecars, and read-only `logs/log.sqlite` event traces. This is the same boundary the kernel-side documents in `lingtai-kernel/src/lingtai/kernel/ANATOMY.md` "Notifications". Anything you'd want to add here that needs cross-process communication should follow the same pattern: write a file, let the other side poll or read the persisted event log.

## Anatomy convention

This root is the normative anatomy-of-anatomy; the map above is the payload,
these rules are how the navigation graph is shaped. Governance of *behavior*
lives in [`CONTRACT.md`](CONTRACT.md); the change/validation *workflow* lives in
[`dev-guide-skill/SKILL.md`](dev-guide-skill/SKILL.md). This section owns only
the structural schema and link rules, and does not restate either.

**Coverage (no orphans).** Every tracked file in this repository MUST be
reachable from this root anatomy by descending `related_files` — the whole file
tree climbs the anatomy graph, and no tracked file is an orphan. Each file is
owned by the anatomy of the layer it belongs to (a package's own `ANATOMY.md`
when it has one, otherwise its parent's), and an `ANATOMY.md` counts only once
its parent lists it. This is enforced, not aspirational:
`tui/architecture_documents_test.go`'s
`TestArchitectureDocumentsCoverEveryTrackedFile` walks the graph from this file
against `git ls-files` and fails on any orphan, unreachable anatomy, empty or
duplicated list, self-link, or entry that no longer resolves to a tracked file.
Adding a file to the repo therefore means adding it to exactly the
`related_files` list that owns it, in the same change. `related_files` is the
complete inventory of a layer; the anatomy *body* stays the curated architectural
map of that layer and is deliberately not a per-file listing.

**Navigation model.** Navigation is distributed: the root defines the system and
enumerates the two binary trees; each component's anatomy maps only the layer it
owns; parent/child and `related_files` links connect them. Do not copy local
facts into this root. For a structural question, descend the graph (this file →
the relevant tree or component anatomy → cited code); for enumeration (every
callsite, every matching file), use search. A folder earns an anatomy when an
agent can reason about it as an architectural unit without reading all its
siblings; pure helpers and trivial leaves do not. Legacy per-package anatomies
keep their current shape until they migrate; a component enters the paired
governed system only when its co-located `CONTRACT.md` is linked from the root
contract, and from that point the schema and link rules here apply.

The governed-child frontmatter, body, and link/pairing rules below are the
**normative target** for that first governed child, not machinery the smoke test
runs today. The repository has zero governed children, so there is no mechanical
child gate. A first governed-child PR must justify and add only the focused
validation its concrete graph needs; until then these rules remain review-owned.

**Frontmatter.** A root-governed component anatomy has exactly two YAML keys, in
order: `related_files` (a non-empty, duplicate-free list of repo-relative
regular files — the paired `CONTRACT.md`, the parent and direct-child anatomies,
and the code files it maps) and `maintenance` (a non-empty statement; use the
Template's text, or a root-specific one here because this file also governs the
system). Paths MUST be repo-relative, resolve to files, use `/`, and contain no
`.`/`..` segments.

**Body.** A root-governed component anatomy opens with one paragraph naming the
layer, then uses these five `##` sections once, in order: `## Components` (files,
symbols, or child components with verified `file:line` citations),
`## Connections`, `## Composition`, `## State`, `## Notes`. It SHOULD stay near
80 lines — a larger map suggests smaller components — with no empty stubs. This
root file is the sole exception to that body/size shape: it also carries this
meta-convention and the repository-wide map above.

**Link and pairing.**

1. This root anatomy and root contract list each other in `related_files`.
2. A root-governed component's co-located `ANATOMY.md` and `CONTRACT.md` list
   each other exactly once.
3. Parent/child anatomy links are reciprocal so navigation can descend and
   return. Cross-binary references are narrative, not enumerated call-graph
   edges.
4. The contract owns interface behavior; the anatomy owns structure. Cross-link
   instead of copying a rule into both.
5. Orphans, missing targets, duplicate links, one-way pair links, and unpaired
   governed components are defects and MUST fail validation.

## Maintenance

Maintenance is part of reading:

- If code and anatomy disagree structurally, code is normally the current fact;
  repair the anatomy before leaving the change. If the code move itself is a
  defect, report or fix the code and keep the mismatch visible until resolved.
- If code and contract disagree behaviorally, do **not** rewrite the contract to
  match accidental behavior. Treat the implementation as defective unless an
  authorized contract change updates the Port, adapters, version, and tests.
- Verify every touched citation after moves, renames, splits, or ownership
  changes. The anatomy drift checker catches missing/out-of-range citation
  targets, not semantic misdescription.
- Adding, moving, or deleting a tracked file is an anatomy change. Update the
  owning `related_files` list in the same commit, or
  `TestArchitectureDocumentsCoverEveryTrackedFile` fails with the exact paths to
  add. Deleting a file means deleting its entry; a new package directory means a
  new `ANATOMY.md` linked from its parent.
- **Capability mentions require explicit bidirectional mapping to implementing
  code.** Any document (README, docs, skill, issue/PR body, proposal) that names
  a user-visible or agent-visible capability must resolve to the code that
  implements it: either the owning ANATOMY.md lists that document in its
  `related_files` and the implementing files, or the document links to the
  owning anatomy node. A capability with no mapping is navigation drift and
  must be repaired in the same change, not deferred.
- Keep parent/child and Anatomy/Contract pair links reciprocal, and keep the
  two-binary facts compatible across `tui/ANATOMY.md` and `portal/ANATOMY.md`.
  When this system's convention itself changes, update this root, its smoke test
  (`tui/architecture_documents_test.go`), the repository-local dev guide, and the
  README entries together. The bundled `lingtai-tui-anatomy` skill is a legacy
  citation-navigation aid that predates this convention; aligning it is separate,
  owner-gated work, not part of every change here.

## Template

```markdown
---
related_files:
  - <repo-relative paired CONTRACT.md>
  - <repo-relative parent ANATOMY.md>
  - <repo-relative direct-child ANATOMY.md, when any>
  - <repo-relative mapped code file>
maintenance: |
  Keep related_files repo-relative, duplicate-free, and linked to real files.
  Keep this component's ANATOMY.md and CONTRACT.md reciprocal and keep
  parent/child anatomy links bidirectional. Code is the structural source of
  truth: update this anatomy in the same change that moves files, symbols,
  connections, composition, or state. Verify every changed citation and run the
  architecture-document validation before merge.
---
# <Component Name> Anatomy

<One paragraph defining the architectural layer this folder embodies.>

## Components

- `<symbol>` — purpose (`repo/relative/file.go:line-line`).

## Connections

## Composition

## State

## Notes
```
