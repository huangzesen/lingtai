---
name: lingtai-update-mainland
description: Use when building or fetching TUI/portal releases from mainland China.
version: 1.0.0
last_changed_at: "2026-09-06T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Mainland-China routing

Nested `lingtai-update` reference. Troubleshooting guidance, not a promise that
any mirror is reachable. Before selecting the install flow, `install.sh` probes
`https://proxy.golang.org/github.com/golang/go/@latest` for three seconds when
`curl` is available and `GOPROXY` is unset; if that probe fails it exports all
three variables below, overwriting existing `GOSUMDB` or `NPM_CONFIG_REGISTRY`
values:

```bash
export GOPROXY="https://goproxy.cn,direct"
export GOSUMDB="sum.golang.google.cn"
export NPM_CONFIG_REGISTRY="https://registry.npmmirror.com"
```

## Release provider and the Python dependency index

`install.sh --source auto|github|mirror` (or `LINGTAI_SOURCE`) selects where the
release archives, the bundle manifest, and the pinned kernel release come from.
`auto` runs a bounded, fail-open public-IP country lookup and prefers the
lingtai.ai download-acceleration mirror for mainland China; any detection or
reachability failure falls back to GitHub for the SAME resolved tag/bundle.
GitHub remains the sole release/version authority either way — the mirror only
re-serves bytes GitHub already published, never its own "latest". (`--source
gitee` is retired: the mirror replaces the earlier Gitee route.) The TUNA
dependency-index default is retained from the earlier Gitee path, where an
Aliyun-host upgrade failed at third-party dependency resolution through
`pypi.org`. That historical observation is not a live acceptance result for
the new lingtai.ai mirror; its production configuration and mainland
reachability still require verification.

`install.sh` now picks exactly one dependency index
(`python_dependency_index_url`):

| Situation | Index used for third-party dependencies |
|---|---|
| non-empty `LINGTAI_PYPI_INDEX_URL` | that value, on either provider |
| final bundle provider is the mirror | `https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple` |
| final bundle provider is GitHub | `https://pypi.org/simple` |

"Final" means the provider that actually served the bundle manifest, after any
same-tag fallback moved the selection. There is no `--extra-index-url`: exactly
one index is consulted. LingTai itself is never affected — it is always
installed from the checksum-verified local artifact by explicit file path and is
never requested by package name from any index. Tsinghua TUNA is a
cloud-neutral domestic default, not a promise that it is reachable from your
host; override it with `LINGTAI_PYPI_INDEX_URL` when your organization runs its
own index.

Scope: this applies to the POSIX verified-bundle path in `install.sh`.
`install.ps1` (Windows) and `--latest` (main-from-source) are unchanged. So is
the bootstrap that `/update-tui` and the Homebrew migration use: they fetch the
version-pinned GitHub raw `install.sh`
(`tui/internal/config/tui_updater.go:442-465`) before any provider selection
happens, so that first fetch still needs a working GitHub raw route regardless
of which index the installer later chooses.

Homebrew has separate knobs because its superenv can scrub ordinary variables:
`HOMEBREW_GOPROXY` maps to `GOPROXY` and `HOMEBREW_NPM_CONFIG_REGISTRY` to
`NPM_CONFIG_REGISTRY`. The formula probes the npm registry with `npm ping`; a
failed probe leaves npm's registry alone. Verify TLS and the actual client
(`go`, `npm`, or `curl`) independently, then retry the smallest failing phase.

The Go/npm mirrors above and the dependency-index selection above are separate
routes with separate failure modes — diagnose the failing one, not both. Kernel
runtime update/nudge behavior after install still belongs to the kernel's
`system-manual`, not to this TUI/portal route.
