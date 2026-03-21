# Rename 灵台 → 灵台 (Língtái)

**Date:** 2026-03-20
**Status:** Approved

## Motivation

灵台 (Língtái) — 灵台方寸山 is where 孙悟空 learned his 72 transformations.
The framework already uses avatar (分身) to spawn agent clones (器灵).
Each avatar ventures into 三千世界 and returns with experiences — the self-growing
network of avatars IS the agent itself. The name captures this perfectly.

## Naming Map

| Before | After | Context |
|--------|-------|---------|
| `lingtai` | `lingtai` | Python package, PyPI name, CLI command |
| `lingtai-kernel` | `lingtai-kernel` | Kernel PyPI package |
| `lingtai_kernel` | `lingtai_kernel` | Kernel Python module (import name) |
| `灵台` | `灵台` | Brand/display name in docs and UI |
| `~/.lingtai/` | `~/.lingtai/` | Default agent working directory |
| `lingtai = "app:main"` | `lingtai = "app:main"` | Console script entry point |
| `_lingtai` | `_lingtai` | TCP wire protocol marker key |
| `agent@lingtai` | `agent@lingtai` | Git user.email in agent workdirs |
| `灵台 Agent` | `灵台 Agent` | Git user.name in agent workdirs |
| `lingtai/1.0` | `lingtai/1.0` | User-Agent HTTP header |
| `logger_name: "lingtai"` | `logger_name: "lingtai"` | Python logger name |

## Scope — Two Repos

### 1. `lingtai` repo (`/Users/huangzesen/Documents/GitHub/lingtai`)

- **Directory rename:** `src/lingtai/` → `src/lingtai/`
- **Source files:** ~57 files under `src/lingtai/`
- **Test files:** ~54 files under `tests/`
- **App files:** `app/` (entry point, config, setup wizard, web, email)
- **Examples:** `examples/` (chat agents, orchestration, etc.)
- **Config:** `pyproject.toml` (package name, dependency, console script)
- **Docs:** `docs/*.md`, `docs/superpowers/**/*.md`
- **Default dir:** `~/.lingtai` → `~/.lingtai` in config/app code

### 2. `lingtai-kernel` repo (`/Users/huangzesen/Documents/GitHub/lingtai-kernel`)

- **Directory rename:** `src/lingtai_kernel/` → `src/lingtai_kernel/`
- **Source files:** ~30 files under `src/lingtai_kernel/`
- **Test files:** ~19 files under `tests/`
- **Config:** `pyproject.toml` (package name)
- **Docs:** README.md

## Execution Strategy — Script-Driven Bulk Rename

A Python script that:

### Step 1: Dry run

Print all files and matches grouped by replacement rule. Operator reviews before proceeding.

### Step 2: Rename directories (via `git mv`)

- `src/lingtai/` → `src/lingtai/`
- `src/lingtai_kernel/` → `src/lingtai_kernel/`
- Delete `src/lingtai.egg-info/` and `src/lingtai_kernel.egg-info/` (regenerated on install)

### Step 3: Find-and-replace in file contents

Ordered to avoid partial matches (longest first):

1. `lingtai_kernel` → `lingtai_kernel`
2. `lingtai-kernel` → `lingtai-kernel`
3. `灵台` → `灵台`
4. `lingtai` → `lingtai` (catches remaining: imports, package refs, paths, protocol keys)

### Step 4: Manual post-processing

- Rewrite CLAUDE.md description organically (replace Stoa etymology with 灵台方寸山 story)
- Leave standalone "Stoa" / "Stoa Poikile" references in historical docs (design.md, discussion-log.md, example-role.md) untouched

### Step 5: Reinstall and verify

- `pip install -e .` in both repos
- `python -m pytest tests/` in both repos
- Smoke-test: `python -c "import lingtai"` and `python -c "import lingtai_kernel"`

## Exclusions

- `.git/` directories — never touch
- `venv/` — will be refreshed by reinstall
- Binary/image files — skip
- This spec file itself — skip (historical record)
- Email addresses (e.g., `stoaiagent@gmail.com`) — user will handle separately
- Standalone "Stoa" / "Stoa Poikile" etymology text in docs — leave as historical context

## Verification

- All tests pass in both repos
- `import lingtai` and `import lingtai_kernel` work
- `from lingtai import Agent, BaseAgent, AgentConfig` works
- No remaining references to `lingtai` in source code (except excluded items above)
