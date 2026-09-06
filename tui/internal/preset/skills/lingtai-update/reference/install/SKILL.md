---
name: lingtai-update-install
description: Use when installing or building the lingtai-tui and lingtai-portal binaries.
version: 1.0.0
last_changed_at: "2026-09-06T00:00:00Z"
maintenance: "If you find stale or incorrect information here, use the lingtai-issue-report skill to assemble evidence and obtain per-issue human consent before filing an issue. Never include secrets, credentials, tokens, or private paths."
---

# Install and build

Nested `lingtai-update` reference. The supported public first-install path is:

```bash
curl -fsSL https://lingtai.ai/install.sh | bash
```

`install.sh` resolves the latest release from GitHub; the source option selects
the download route, not version authority
(`--source auto|github|mirror`, default `auto`, or `LINGTAI_SOURCE`;
`--source gitee` is retired), prefers a matching `lingtai-<tag>-<os>-<arch>.tar.gz`
asset, and falls back to building the release source when no asset is
available. It installs both binaries into a selected bin directory and
prepares the TUI-managed runtime; that runtime is not a second TUI update path.

On the verified-bundle path the Python `lingtai` runtime is installed from the
bundle's pinned kernel artifact by explicit local file path after a SHA256
check — never by requesting the package name `lingtai` from an index. Only that
artifact's third-party dependencies use a package index, and exactly one:
a non-empty `LINGTAI_PYPI_INDEX_URL` always wins, otherwise the provider that
actually served the bundle manifest decides (the lingtai.ai mirror →
`https://mirrors.tuna.tsinghua.edu.cn/pypi/web/simple`, GitHub →
`https://pypi.org/simple`). There is no `--extra-index-url`. See
`reference/mainland-china/SKILL.md` for the reachability reasoning; Windows
(`install.ps1`) and `--latest` are outside this behavior.

For a deliberate source build, the current repository layout is:

```bash
cd tui && make build
cd ../portal && make build
```

The portal build first needs its checked-in web dependencies and frontend build
(`cd portal/web && npm ci && npm run build`); `portal/embed.go` then embeds the
result into `lingtai-portal`. Go and Node are build prerequisites, not runtime
dependencies of the portal binary.

The installer accepts `--version <tag>`, `--ref <ref>`, `--bin-dir <dir>` or
`--prefix <dir>`; its `--update --prefix <prefix> --version <tag>
--non-interactive` form is the source updater contract. Prefer the public
`lingtai.ai` URL for new installs. Homebrew remains a supported migration path;
bare `pip install/upgrade lingtai` is kernel development/diagnosis guidance, not
normal TUI update guidance.
