# Gatekeeper Capability Design

Draft for issue #864.

## Current State

LingTai's Go repository owns the TUI, Portal, installer, user-facing examples,
and documentation. The actual agent tool dispatch loop lives in the kernel
package outside this repository, so a complete gatekeeper implementation must
land where shell, file, and future tool calls are dispatched.

Today this repository documents adjacent boundaries:

- `examples/bash_policy.json` shows shell policy configuration, but it is not a
  durable cross-tool approval system.
- `CONTRACT.md` defines the target Ports and Adapters direction and the
  filesystem-only interoperability rule.
- `ANATOMY.md` documents that TUI and Portal are control surfaces; they do not
  intercept kernel tool execution.

## Proposed Capability

Add a first-class `gatekeeper` kernel capability that intercepts risky external
side effects before dispatch. It should be opt-in per agent and should preserve
zero behavior change for agents without a gatekeeper config.

Initial scope:

- file writes and edits outside allowed local roots
- shell commands that are destructive, ambiguous, or write outside allowed roots
- a durable pending-request record for blocked calls
- dual-channel human approval before exact replay of the recorded operation

Out of scope for the first slice:

- full static proof for arbitrary shell programs
- package-manager policy
- UI automation for approval
- Portal/TUI acting as the enforcement point
- retroactive enforcement for tool calls already in progress

## Configuration

Per-agent opt-in:

```jsonc
{
  "local_write_roots": ["/path/to/approved/work"],
  "remote_write_roots": [],
  "ssh_hosts": [],
  "trusted_scripts": [],
  "pending_dir": ".security/pending",
  "ttl_seconds": 86400
}
```

Suggested merge behavior:

- No config file means gatekeeper disabled.
- A shared network config may contribute list-valued grants.
- Per-agent config and shared config are unioned for allowlists.
- Scalar values such as `pending_dir` and `ttl_seconds` stay agent-local unless
  explicitly adopted by the implementation contract.

## Enforcement Point

The gatekeeper belongs in the kernel dispatch path, before the concrete file or
shell adapter runs:

1. Tool call enters the dispatch loop.
2. Gatekeeper classifies the operation.
3. Safe operation continues to the existing adapter.
4. Risky operation writes a pending record and returns a blocked result.
5. A separate approval worker replays the exact recorded operation only after
   both configured approval channels approve it.

The gatekeeper module should not send notifications and should not execute
blocked operations itself. Notification and replay are separate auditable
components.

## Pending Record

```jsonc
{
  "id": "uuid",
  "created_at": "2026-08-15T00:00:00Z",
  "expires_at": "2026-08-16T00:00:00Z",
  "status": "pending",
  "operation": {
    "kind": "shell_command",
    "command": "rm -rf build",
    "cwd": "/project",
    "tool_call_id": "optional-runtime-id",
    "reason": "destructive verb outside allowlist"
  },
  "approvals": {
    "telegram": null,
    "wechat": null
  }
}
```

Required invariants:

- The recorded command, arguments, and working directory are replayed exactly.
- Approval never asks an LLM to reconstruct the operation.
- Missing, expired, or partial approval is deny by default.
- Pending records must not contain secrets or full tool output.

## Classification

File operations are path-based:

- Resolve the target path.
- Resolve each allowed root.
- Permit only exact root matches or descendants.
- Symlink resolution must happen before comparison.

Shell operations are best-effort and fail-closed:

- destructive verbs such as `rm`, `rmdir`, `unlink`, `shred`, and `truncate`
  are risky unless every target is provably allowed
- destination verbs such as `cp`, `mv`, and `rsync` check destination paths
- redirects check their destination path
- ambiguous tokens containing globs, variables, command substitution, or shell
  character classes are risky
- opaque inline execution such as `sh -c`, `python -c`, `node -e`, `eval`, and
  `xargs` is risky until a narrower parser is implemented

Python inline code may later get a stdlib AST pass, but that should be a second
slice with its own tests.

## First Vertical Slice

Recommended first PR in the kernel:

1. Add config loading and path allowlist helpers.
2. Gate explicit file write/edit targets.
3. Write pending records for denied file operations.
4. Return a structured blocked result with the pending id.
5. Add tests for allowed root, sibling path rejection, symlink escape, missing
   config as disabled, and pending record contents.

Shell gating and dual-channel replay should follow as separate PRs after the
file-operation contract is proven.

## Open Questions

- Which two approval channels are mandatory for the default product profile?
- Should TUI expose pending gatekeeper requests, or should the first release be
  notification-channel only?
- Where should shared network grants live in the current filesystem layout?
- What exact blocked-result schema should tool callers receive?
- Should approval replay run inside the kernel process, as a sidecar command,
  or as a separate supervised worker?
