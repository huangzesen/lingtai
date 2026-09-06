# Releasing lingtai-tui and lingtai-portal

## Release Process

### 1. Commit and push all changes

Bump [`migration/migration.md`](migration/migration.md)'s frontmatter
(`release_version`, `release_tag`, `kernel_tag`) to the release being tagged
and [`kernel-release.json`](kernel-release.json)'s pinned kernel — the exact
tagged copy of that file is the migration record the update contract reads.

```bash
git push origin main
```

### 2. Tag the release

```bash
git tag v0.X.Y
git push origin v0.X.Y
```

Pushing a `v*` tag triggers the root GitHub Actions workflow at
`.github/workflows/release.yml`, which has three jobs:

- **`source-release`** — verifies the pushed tag and creates the public GitHub
  Release when it does not already exist. GitHub supplies the tag source archives;
  this job does not build or upload prebuilt binaries, checksums, or bundles.
- **`update-homebrew`** — computes the GitHub source-tarball checksum and updates
  the source-build formula in `Lingtai-AI/homebrew-lingtai`. It **fails closed**
  before the tap checkout unless the pushed tag is an exact `vX.Y.Z` — the same
  shape `windows-release` already requires — so a tag name can never reach the
  formula writer or the cross-repo tap token as unvalidated data. Prerelease and
  other non-exact `v*` tags still create a GitHub source release, but are not
  published to Homebrew.
- **`windows-release`** (`needs: source-release`) — builds both
  `lingtai-tui.exe` and `lingtai-portal.exe` for `windows/amd64`; the portal web
  build is mandatory. It packages the dual-binary
  `lingtai-<tag>-windows-amd64.zip` plus its `.sha256` sidecar, generates
  `lingtai-bundle-manifest.json` (schema `lingtai.tui.bundle/v1`) binding the tag's
  exact commit to the archive digest and to [`kernel-release.json`](kernel-release.json)'s
  pinned kernel tag, and uploads all three to the release. It **fails closed**
  before building anything unless `kernel-release.json`'s pinned kernel release
  already exists and publishes a `cp311`/`cp312`/`cp313` `win_amd64` wheel with a
  verified digest — it never resolves "latest kernel."

### Kernel compatibility metadata

[`kernel-release.json`](kernel-release.json) is the repo-owned pin the
`windows-release` job reads to bind a TUI release to one exact kernel release.
Bump it deliberately, in the same PR/commit that intends to ship a new kernel
version with the next TUI release; the workflow never resolves "latest kernel"
on its own.

### Gitee publication

The tag workflow does not synchronize to Gitee and does not publish TUI
binary/bundle assets there. The existing
[`scripts/sync_gitee_mirror.sh`](scripts/sync_gitee_mirror.sh) and
[`scripts/publish_bundle_to_gitee.sh`](scripts/publish_bundle_to_gitee.sh)
remain explicit maintainer tools; running them requires separate release authority
and is not part of the automatic `v*` workflow.

### Installing on Windows (PowerShell)

```powershell
irm https://lingtai.ai/install.ps1 | iex
# or an exact version (parameters require the scriptblock form, not | iex):
&([scriptblock]::Create((irm https://lingtai.ai/install.ps1))) -Version v0.X.Y
```

`install.ps1`'s public (default) mode resolves one exact release tag, downloads
and strictly validates `lingtai-bundle-manifest.json`, downloads and SHA-256
verifies the Windows archive, confirms the staged `lingtai-tui.exe` reports
exactly that tag, and — unless `-SkipVenv` is passed — provisions
`%USERPROFILE%\.lingtai-tui\runtime\venv` from the bundle's pinned kernel
release: it selects the `cp311`/`cp312`/`cp313` `win_amd64` wheel matching the
venv's actual interpreter, verifies its digest, and installs it by explicit
local file path. LingTai is never installed by package name from any index —
the same "no PyPI fallback" contract `install.sh` holds itself to. `-SkipVenv`
skips only the kernel venv and still installs both required TUI/portal binaries;
`-DryRun` performs the same resolution/validation reads but writes nothing. The
Windows Installer Smoke workflow covers the contract suite under PowerShell 5.1
and PowerShell 7 on PR/push; its tag-only exact-tag smoke waits for the published
asset and verifies both installed binaries. See
[`scripts/test-install-ps1.ps1`](scripts/test-install-ps1.ps1) for the full
contract and [`.github/workflows/windows-installer-smoke.yml`](.github/workflows/windows-installer-smoke.yml)
for its Windows PowerShell 5.1 / PowerShell 7 CI coverage.

### 3. Create the GitHub release

The `source-release` job creates the GitHub release, and `windows-release` adds
the dual-binary ZIP, checksum sidecar, and bundle manifest. To create a release
manually (or to add richer notes), run:

```bash
gh release create v0.X.Y --title "v0.X.Y" --notes "release notes here..."
```

Binary assets are attached by the workflow. If the workflow could not run, the
release still installs — `install.sh` falls back to building from the release
source tarball.

Immediately after the release above is published, `windows-release`'s
"Notify lingtai-web download mirror" step sends one `repository_dispatch`
(`release-asset-published`) to `huangzesen/lingtai-web` naming this release's
tag and the three uploaded assets (the Windows zip, its `.sha256` sidecar,
and `lingtai-bundle-manifest.json`) with sha256/size recomputed fresh from
the still-on-disk bytes. This exists solely so `lingtai.ai` can mirror the
same bytes for mainland-China download acceleration; GitHub remains the sole
official release authority, and a missing or failed dispatch never edits,
retries, or undoes the GitHub release itself. Requires the
`LINGTAI_WEB_DISPATCH_TOKEN` repository secret (a token with
`repository_dispatch` write access on `huangzesen/lingtai-web`) as a
deployment prerequisite; without it the step prints a `::warning::` and exits
0, so its absence cannot fail a release. See `huangzesen/lingtai-web`'s
`docs/release-mirror/CONTRACT.md` for the receiving side's contract.

### 4. Verify the automated Homebrew tap update

Check the `Release` workflow run for the tag and confirm it pushed a formula
update to `Lingtai-AI/homebrew-lingtai`.

```bash
gh run list --workflow Release --event push --limit 5
gh run watch <run-id>
```

Then verify the installed version:

```bash
brew update && brew upgrade lingtai-ai/lingtai/lingtai-tui
lingtai-tui version  # should show v0.X.Y
```

### 5. Fallback: update the Homebrew tap manually

Use this only when the root release workflow failed or cannot run. Do not race a
successful workflow with a hand edit.

```bash
# Get the source tarball checksum
curl -sL "https://github.com/Lingtai-AI/lingtai/archive/refs/tags/v0.X.Y.tar.gz" | shasum -a 256

# Edit the formula
cd $(brew --repository)/Library/Taps/lingtai-ai/homebrew-lingtai
# In lingtai-tui.rb: update the url tag and sha256
git add lingtai-tui.rb
git commit -m "bump lingtai-tui to v0.X.Y"
git push
```

The inactive `tui/.github/workflows/release.yml` path is intentionally not part
of the release process; GitHub only runs workflows from the repository-root
`.github/workflows/` directory. Existing npm package files are outside this
release checklist and are not decided here.

## Installing without Homebrew

The tag workflow publishes both GitHub source archives and the verified Windows
bundle assets described above. Homebrew and the manual commands below still
build `lingtai-tui` and `lingtai-portal` from the tagged source when a source
build is preferred:

```bash
curl -fsSL https://lingtai.ai/install.sh | bash
# or, direct from the repo:
curl -fsSL https://raw.githubusercontent.com/Lingtai-AI/lingtai/main/install.sh | bash
```

On the ordinary stable macOS path with no existing command, `install.sh`
registers a self-contained lazy `lingtai-desktop` command and stops. A stable,
version-pinned `--update` does the same when the command is absent, and may
atomically refresh only an executable, regular, non-symlink command carrying
this installer's lazy-bootstrap marker. Registration in either mode does not
contact Desktop, invoke its installer, or create any Desktop
App/current/receipt/cache/version state. The command embeds Desktop `0.1.10`
plus SHA-256 pins audited from its commit
`fd39dd61e4d123b2835064c1c148566d8b36ceb0`. On its first execution only, it
downloads the matching `install-macos-app.py`, `desktop_user_cli.py`, and
independent `verify-app-archive.py`, rejects any byte mismatch, invokes the
Desktop installer, and then continues the user's requested command. Archive,
manifest, managed-support generation/update, smoke, atomic-publication, and
later-update policy stay entirely in the Desktop code. Desktop v0.1.10 is the
verified N→N+1 managed-support generation, `0.1.10-b4575bccdedd`; its
self-update contract advances from the v0.1.9 public generation without
inferring or migrating private pre-generation layouts. The public LingTai
entry pins only the raw bootstrap trust set rather than duplicating Desktop's App/support
manifest digests. Later command executions delegate to the installed current
CLI without reinstalling. `--skip-desktop` opts out of registration.

LingTai's `scripts/remove.sh` deliberately does not delete Desktop's App or
command state because neither is owned by the TUI receipt. A later main install
therefore treats a regular, executable, non-symlink command target as already satisfied only
when it is this installer's marked lazy bootstrap, or when it carries Desktop's
official launcher marker and the managed current App executable is complete.
An ordinary stable install preserves the marked lazy command's bytes and mode;
both ordinary install and stable update preserve a complete official launcher
and App executable byte-for-byte and mode-for-mode. A symlink, arbitrary file,
or official-marker launcher without its regular executable App remains a loud
no-overwrite failure.

Linux/WSL, an ordinary existing-install re-run, `--latest`, arbitrary `--ref`,
and `--skip-desktop` installs do not register or refresh the command; the sole
update exception is stable, version-pinned `--update`. Windows is platform-N/A
because LingTai Desktop itself is macOS-only. The exact
[`v0.1.10` public release](https://github.com/Lingtai-AI/lingtai-desktop/releases/tag/v0.1.10)
provides the tag and assets read by the first `lingtai-desktop` execution.
Temporary public-support, release, or transport unavailability fails clearly,
leaves the command retryable, and publishes no partial Desktop state. Version
and support-file hashes are one fixed trust set; there is no free-form Desktop
version override.

Manual source build (if you prefer to build the binaries yourself):

```bash
git clone https://github.com/Lingtai-AI/lingtai.git
cd lingtai/tui && make build
# Binary at tui/bin/lingtai-tui

cd ../portal && make build
# Binary at portal/bin/lingtai-portal
```

Requires Go toolchain and Node.js (for portal web frontend).

### Source selection (GitHub vs the lingtai.ai mirror) and the Python runtime

The POSIX installer has one explicit non-release mode:

```bash
curl -fsSL https://lingtai.ai/install.sh | bash -s -- --latest
```

`--latest` resolves `refs/heads/main` independently in the TUI repository and
`lingtai-kernel`, verifies each shallow checkout against its resolved full SHA,
builds the TUI from source, and installs the kernel from the checked-out local
source tree. It prints both SHAs and records them in `~/.lingtai-tui/install.json`
under `source_mode: "latest-main"`, `tui_commit`, and `kernel_commit`. This mode
is deliberately separate from the no-argument/latest-release, `--version`,
`--ref`, and `--update` paths; conflicts fail before network access, and a
failed main checkout or kernel install never falls back to a stable release or
package-index install. It is POSIX-only; `install.ps1` is unchanged.

The behavior below applies to the bundle assets published by the tag workflow
and to compatible bundle releases published separately. After the Windows
release assets above are uploaded and the release is public, this job's final
step dispatches a `repository_dispatch` to `huangzesen/lingtai-web` so it can
mirror those same bytes for download acceleration — see the post-publication
dispatch described above; that mirror never publishes an
independent release, so it has no "latest" of its own. The legacy Gitee
synchronization/publish maintainer tools remain unrelated to this path; the
tag workflow does not invoke them (see "Gitee publication" above).

`install.sh --source auto|github|mirror` (or `LINGTAI_SOURCE` env var; `gitee`
is retired) controls where the TUI/portal archives, the bundle manifest, and
the pinned kernel release come from. `auto` (the default) runs a bounded,
fail-open public-IP country lookup and prefers the lingtai.ai mirror for
mainland-China installs; any lookup or provider-reachability failure falls
back to GitHub. A fallback always re-fetches the SAME resolved tag/bundle from
the other provider — it never independently resolves "latest" a second time
(the mirror has no listing/"latest" capability of its own; version identity
always comes from GitHub), so a TUI archive from one release can never be
paired with a kernel artifact from a different one.

The Python `lingtai` runtime installs from the bundle's pinned kernel release
artifact (a platform wheel matched to the venv's actual interpreter, or the
pinned sdist as a fallback) by **explicit local file path**. LingTai is
**never** installed by requesting the package name `lingtai` from any package
index — there is no PyPI fallback for LingTai itself. SHA256 is verified
before install. A package index is used only to resolve `lingtai`'s
third-party dependencies once the local artifact is being installed, and
`install.sh` consults exactly one: a non-empty `LINGTAI_PYPI_INDEX_URL` always
wins, otherwise the provider that actually served the bundle manifest (after
any same-tag fallback) picks a default it can reach — the mirror →
`https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple`, GitHub →
`https://pypi.org/simple`. There is no `--extra-index-url`.

That default exists because `pypi.org` is not reliably reachable from
mainland-China hosts: on the earlier Gitee path, sending third-party
dependencies to `pypi.org` left an Aliyun-host install failing at dependency
resolution. The new mirror retains that index default; this historical
observation is not production/mainland acceptance of lingtai.ai downloads.
Tsinghua TUNA is a cloud-neutral domestic default, not a reachability
guarantee; `LINGTAI_PYPI_INDEX_URL` is the explicit escape hatch. This applies
to the POSIX verified-bundle path only — `install.ps1` and `--latest` are
unchanged, and the `/update-tui`/Homebrew migration bootstrap still fetches the
version-pinned GitHub raw `install.sh`
([`tui/internal/config/tui_updater.go`](tui/internal/config/tui_updater.go))
before any provider selection happens.

On the default one-command path (no `--ref`, not `--update`) a resolved
bundle + a successful kernel-artifact install are **mandatory**: if no bundle
manifest can be resolved on either provider (same-tag fallback attempted), or
the resolved bundle's kernel artifact fails to verify or install, `install.sh`
**fails loud** with the provider/tag/error rather than degrading to any other
install source. `--ref`/source-ref builds have no bundle to pin against and
fail loud the same way. `--skip-python` (alias `--skip-venv`) is the explicit,
honest opt-out for a TUI/portal-only install — you then provision the Python
runtime yourself (for example an editable install against a local
`lingtai-kernel` checkout). When a Homebrew-to-native migration finds a legacy
`~/.lingtai-tui/runtime` but no native receipt, this mode deliberately preserves
that real runtime root and lets the native TUI/portal binaries and receipt be
installed beside it; a later explicit `fix.sh` repair can provision a parallel
runtime without adopting or overwriting the legacy one.

`install.json`'s `kernel_source` field is written only on a verified bundle
install (`kernel_source: "bundle"`, plus `kernel_bundle_id`/`kernel_version`/
`kernel_provider`); it is omitted otherwise. The TUI's own runtime updater
(`tui/internal/config/venv.go`) reads this field and skips **both routine and
forced** PyPI queries/installs for a bundle-provisioned runtime — `force=true`
(`doctor`/`/update --force`) reports that the kernel is pinned to the
compatible bundle and directs the user to the one-command installer rather
than reinterpreting "force" as "discard the pin and install latest PyPI."
Legacy runtimes with no `kernel_source` metadata are unaffected: the updater's
existing PyPI-compare/upgrade behavior is unchanged for them, since retracting
that established capability is out of scope here — but the CLI no longer
*introduces* a new PyPI install source for LingTai on a fresh install.
