# Releasing lingtai-tui

## Prerequisites

- Go toolchain installed
- `gh` CLI authenticated
- Push access to `huangzesen/lingtai` and `huangzesen/homebrew-lingtai`
- Node.js + npm (for portal web frontend build)

## Release Process

### 1. Commit and push all changes

```bash
git push origin main
```

### 2. Tag the release

```bash
git tag v0.X.Y
git push origin v0.X.Y
```

### 3. Cross-compile both binaries

```bash
cd tui && make cross-compile
cd ../portal && make cross-compile
```

This produces:
- `tui/bin/lingtai-{darwin-arm64,darwin-x64,linux-x64,linux-arm64}`
- `portal/bin/lingtai-portal-{darwin-arm64,darwin-x64,linux-x64,linux-arm64}`

### 4. Package tarballs

**Each tarball MUST contain both `lingtai-tui` and `lingtai-portal`** — the Homebrew formula's `bin.install` expects both names exactly.

```bash
cd /path/to/repo
for arch in darwin-arm64 darwin-x64 linux-x64 linux-arm64; do
  cp "tui/bin/lingtai-${arch}" lingtai-tui
  cp "portal/bin/lingtai-portal-${arch}" lingtai-portal
  tar czf "tui/bin/lingtai-${arch}.tar.gz" lingtai-tui lingtai-portal
done
rm -f lingtai-tui lingtai-portal
```

### 5. Create the GitHub release

```bash
cd tui/bin
gh release create v0.X.Y --title "v0.X.Y" --notes "release notes here..." \
  lingtai-darwin-arm64.tar.gz lingtai-darwin-x64.tar.gz \
  lingtai-linux-x64.tar.gz lingtai-linux-arm64.tar.gz
```

### 6. Update the Homebrew tap

Get the new checksums:

```bash
shasum -a 256 tui/bin/lingtai-*.tar.gz
```

Edit the formula in the tap repo:

```bash
cd $(brew --repository)/Library/Taps/huangzesen/homebrew-lingtai
# Edit lingtai-tui.rb:
#   - Update `version` to the new version
#   - Update all `url` lines to point to the new tag
#   - Update all `sha256` lines with the new checksums
git add lingtai-tui.rb
git commit -m "bump lingtai-tui to v0.X.Y"
git push
```

### 7. Verify

```bash
brew update && brew reinstall huangzesen/lingtai/lingtai-tui
lingtai-tui version  # should show v0.X.Y
```

## Common Mistakes

| Mistake | Consequence | Prevention |
|---------|------------|------------|
| Uploading stale tar.gz from `tui/bin/` | Users install old version | Always re-tar after `make cross-compile` |
| Binary inside tar named `lingtai-darwin-arm64` instead of `lingtai-tui` | Homebrew install fails with "No such file" | The tar loop above renames via `cp` |
| Missing `lingtai-portal` in tar | Homebrew install fails with "No such file - lingtai-portal" | Always include both binaries |
| Forgetting to update Homebrew checksums | `brew install` fails with checksum mismatch | Run `shasum -a 256` after uploading |
| Not rebuilding portal | Portal binary is from an old version | Always `cd portal && make cross-compile` |

## Future: GitHub Actions

This process should eventually be automated with a GitHub Actions workflow triggered by tag push. Until then, follow the manual steps above exactly.
