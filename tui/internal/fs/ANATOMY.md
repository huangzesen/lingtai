---
related_files:
  - tui/ANATOMY.md
  - tui/internal/tui/ANATOMY.md
  - tui/internal/inventory/ANATOMY.md
  - tui/internal/preset/ANATOMY.md
  - tui/internal/migrate/ANATOMY.md
  - portal/internal/fs/ANATOMY.md
  - tui/internal/fs/types.go
  - tui/internal/fs/agent.go
  - tui/internal/fs/agent_test.go
  - tui/internal/fs/agent_record.go
  - tui/internal/fs/agent_record_test.go
  - tui/internal/fs/activity.go
  - tui/internal/fs/activity_test.go
  - tui/internal/fs/daemon_ledger.go
  - tui/internal/fs/daemon_ledger_test.go
  - tui/internal/fs/kanban_bounded.go
  - tui/internal/fs/kanban_bounded_test.go
  - tui/internal/fs/heartbeat.go
  - tui/internal/fs/heartbeat_test.go
  - tui/internal/fs/atomic_write.go
  - tui/internal/fs/atomic_write_permissions_unix_test.go
  - tui/internal/fs/exclusive_lock.go
  - tui/internal/fs/file_ops_unix.go
  - tui/internal/fs/file_ops_windows.go
  - tui/internal/fs/mail.go
  - tui/internal/fs/mail_test.go
  - tui/internal/fs/mail_recent_window_test.go
  - tui/internal/fs/mail_mixed_legacy_window_repro_test.go
  - tui/internal/fs/message_limit_env_test.go
  - tui/internal/fs/direct_mail.go
  - tui/internal/fs/direct_mail_test.go
  - tui/internal/fs/direct_publication.go
  - tui/internal/fs/direct_unread.go
  - tui/internal/fs/direct_unread_test.go
  - tui/internal/fs/direct_unread_transactionality_test.go
  - tui/internal/fs/direct_unread_durability_test.go
  - tui/internal/fs/network.go
  - tui/internal/fs/network_test.go
  - tui/internal/fs/session.go
  - tui/internal/fs/session_durability_test.go
  - tui/internal/fs/session_persistence_role_test.go
  - tui/internal/fs/session_rebuild_test.go
  - tui/internal/fs/session_rebuild_offsets_test.go
  - tui/internal/fs/session_tail_test.go
  - tui/internal/fs/session_window_test.go
  - tui/internal/sqlitelog/event.go
  - tui/internal/sqlitelog/query_test.go
  - tui/internal/fs/signal.go
  - tui/internal/fs/signal_test.go
  - tui/internal/fs/resolve.go
  - tui/internal/fs/resolve_test.go
  - tui/internal/fs/ledger.go
  - tui/internal/fs/location.go
  - tui/internal/fs/project_hash.go
  - tui/internal/fs/contacts.go
  - tui/internal/fs/jsonl.go
  - tui/internal/fs/location_test.go
  - tui/internal/fs/rebuild_marker_test.go
  - tui/internal/fs/refresh_marker_test.go
  - tui/internal/fs/session_race_test.go
  - tui/internal/fs/tool_call_render_test.go
  - tui/internal/fs/discovery_test.go
maintenance: |
  Keep related_files as repo-relative paths to real files. Include neighboring
  ANATOMY.md files so the anatomy graph stays connected rather than isolated;
  anatomy links must be bidirectional. If you create a new ANATOMY.md, copy this
  maintenance field. If you notice drift between this anatomy and the code,
  report it. See lingtai-dev-guide for details.
---

# fs

> **Maintenance:** see the `lingtai-tui-anatomy` skill at `tui/internal/preset/skills/lingtai-tui-anatomy/SKILL.md`. Coding agents update this file in same-commit as code changes.

## What this is

The TUI's filesystem window into an agent working directory (`<project>/.lingtai/<agent>/`). Agent state — manifest, heartbeat, mail, token ledger, location, network topology, chat history — is read through this package. The kernel owns agent state; the TUI's narrow writes are signal files, human outbox/location, its derived human `logs/session.jsonl` replay cache, and the separately owned project-local direct-unread cursor file `<project>/.lingtai/.tui-asset/direct-unread.json`.

## Components

| Symbol | Citation | Purpose |
|--------|----------|---------|
| **agent.go** | | |
| `ReadAgent(dir)` | `tui/internal/fs/agent.go:33` | reads `.agent.json` → `AgentNode` (durable agent_id, current address, name, state, is_human, capabilities, location) |
| `ParseCapabilities(raw)` | `tui/internal/fs/agent.go:63` | handles `[]string` and `[["name", {}], ...]` tuple formats |
| `CapabilitiesForDisplay(manifest)` | `tui/internal/fs/agent.go:100` | prepends intrinsic caps (`system, soul, email, psyche`) to manifest caps, deduped, for operator display (kanban/props) |
| `ReadInitManifest(dir)` | `tui/internal/fs/agent.go:124` | prefers `system/manifest.resolved.json`, falls back to `init.json`, and flattens `llm.*` + `soul.delay` |
| `WritePrompt` | `tui/internal/fs/agent.go:213` | writes `.prompt` signal file (TUI→agent injection) |
| `WriteInquiry` | `tui/internal/fs/agent.go:220` | writes `.inquiry` signal file; no-op if `.inquiry` or `.inquiry.taken` exists |
| `IsOrchestratorManifest(manifest)` | `tui/internal/fs/agent.go:249` | lower-level orchestrator role detector shared by TUI display logic and running-agent inventory |
| `DiscoverAgents(baseDir)` | `tui/internal/fs/agent.go:267` | scans for all subdirectories with `.agent.json` |
| `ReadStatus(dir)` | `tui/internal/fs/agent.go:395` | reads `.status.json` → `AgentStatus` with whole-file semantics retained for unrelated callers |
| **agent_record.go** | | |
| `ReadAgentRecord(dir)` | `tui/internal/fs/agent_record.go:65` | reads `system/agent_record.json` (kernel-published, schema `lingtai.agent_record/v1`, see lingtai-kernel `src/lingtai/kernel/session_stats/CONTRACT.md`) → `(AgentRecord, ok)`; missing file, malformed JSON, or an unrecognized `schema` all return `ok=false` (all-or-nothing — never a partial parse), matching that module's direct-migration contract. `tui/internal/tui/home_telemetry.go`'s Home telemetry row is the sole current consumer |
| `ReadContextStats(dir)` | `tui/internal/fs/agent.go:416` | summarizes retained `history/chat_history.jsonl`: entries, role counts, text input/output, tool calls/results, and per-tool distribution |
| `AggregateTokens(dirs)` | `tui/internal/fs/agent.go:543` | sums `TokenTotals` across multiple agent ledgers |
| `SumTokenLedger(path)` | `tui/internal/fs/agent.go:560` | sums a single main-agent `token_ledger.jsonl` → `TokenTotals`, skipping historical daemon-mirrored rows (`source=daemon`, `em_id`, or `run_id`) |
| `SumTokenLedgerByProvider` / `SumMoltSessionTokenLedger` / `SumMoltSessionToolCalls` | `tui/internal/fs/agent.go:676-950` | whole-ledger/session helpers retained for unrelated callers; explicitly forbidden from the Kanban path |
| `SumSessionTokenLedgerBetween` | `tui/internal/fs/agent.go:958` | reusable `[since, before)` ledger-window summation helper used by molt-session stats and since-cutoff callers |
| **rebuild_marker.go** | | |
| `RecentRebuildTimes` / `RecentRefreshCompleteTimes` | `tui/internal/fs/rebuild_marker.go` | legacy marker helpers retained for unrelated callers; Kanban derives both marker kinds from `ReadKanbanEventWindow` |
| **jsonl.go** | | |
| `forEachJSONLLine(path, fn)` | `tui/internal/fs/jsonl.go:16` | streams JSONL files one line at a time without `ReadFile`/`strings.Split`, avoiding duplicate buffers and Scanner token limits for ledger/history hot paths |
| **kanban_bounded.go** | | |
| `KanbanReadLimit` / `readBoundedJSONLTail` | `tui/internal/fs/kanban_bounded.go:19-259` | Kanban-only 1000-line/record profile and private backward-seek JSONL tail; hard 8 MiB ceiling, fixed 64 KiB blocks, incomplete EOF and byte-boundary fragments dropped, with complete/recent/partial source truth retained |
| `ReadKanbanTokenWindow` / `ReadKanbanEventWindow` / `KanbanSessionTokenStats` / `ReadKanbanContextStats` | `tui/internal/fs/kanban_bounded.go:272-543` | bounded token/provider/recent rows and retained-context structure; current/last-session partitions publish only when timestamped event-boundary coverage spans the independently bounded token window and includes a real molt boundary |
| `ReadKanbanAgentRaw` / `ReadKanbanInitManifest` / `ReadKanbanStatus` | `tui/internal/fs/kanban_bounded.go:559-603` | bounded fixed-state presentation reads (2 MiB each); no preset/auth/key files are opened |
| `readDirEntriesBounded` / `resolveContainedKanbanChild` | `tui/internal/fs/kanban_bounded.go:621-746` | agent discovery reads at most 1001 entries to prove the 1000-entry window complete or truncated; topology children must be existing relative paths whose symlink-resolved target remains physically below the network base |
| `ReadKanbanDaemonDetailSnapshot` | `tui/internal/fs/kanban_bounded.go:804-1001` | tails only `daemons/.dispatch-ledger.jsonl`, deduplicates valid run IDs by latest append occurrence in newest-first order, and opens each selected run/card/ledger at most once after proportional `Lstat` no-symlink child checks; every file/ledger stays byte/line bounded and there is no `ReadDir` fallback |
| `BoundedReadStats` / `KanbanReadState` | `tui/internal/fs/kanban_bounded.go:38-109` | existing guarded-probe metadata now also retains completeness, truncation, and malformed-record counts; Kanban renders only its derived source truth while raw paths/IO counts remain diagnostic-only |
| **daemon_ledger.go** | | |
| `DaemonLedgerSummary(agentDir, recentN)` | `tui/internal/fs/daemon_ledger.go:70` | single traversal returning both provider/backend totals (`map[string]TokenTotals`) and most-recent tagged per-call rows (`[]DaemonLedgerEntry`); one daemon.json read per run (typed `daemonCard` includes `backend` plus `cli_tokens`/`tokens` sub-structs), valid ledger rows retain backend in memory, CLI/legacy snapshots remain totals-only and use `daemonFallbackProvider` attribution |
| `DaemonRecentLedger(agentDir, recentN)` | `tui/internal/fs/daemon_ledger.go:165` | convenience wrapper — returns only the recent-rows half of `DaemonLedgerSummary` |
| `ReadRecentDaemonActivity` / `DaemonRecentLedgerSummary` / `RecentLingtaiDaemonModels` | `tui/internal/fs/activity.go`, `tui/internal/fs/daemon_ledger.go` | tails only the bounded append-only dispatch ledger and checks exact selected run directories inside the 10-minute live window; Home derives counts/token totals/exact LingTai model names from one shared traversal, while the model-only compatibility helper reads selected cards without token tails; per-run token reads are line/byte bounded, and no recurring helper has a lifetime `ReadDir` fallback |
| `daemonFallbackProvider` | `tui/internal/fs/daemon_ledger.go:207` | derives a provider/backend label for runs with no per-call ledger: preset_provider → non-lingtai backend → model derivation → raw backend/model → "daemon" |
| `DeriveLedgerProvider` | `tui/internal/fs/agent.go:1064` | maps endpoint host / model prefix → canonical provider name |
| **heartbeat.go** | | |
| `IsAlive(dir, thresholdSec)` | `tui/internal/fs/heartbeat.go:11` | reads `.agent.heartbeat` unix timestamp, returns `age < threshold` |
| `IsAliveHuman()` | `tui/internal/fs/heartbeat.go:24` | always `true` |
| **atomic_write.go / file_ops_*.go** | | |
| `writeAtomicReplacement` / `createAtomicReplacementTemp` / `writeAtomicBytes` | `tui/internal/fs/atomic_write.go:15-106` | writes through a random exclusive same-directory temp, preserves an existing target's permission bits or applies the caller fallback through the process umask for a new target, flushes and closes before atomic replacement, cleans every unpublished temp, and best-effort flushes the parent directory |
| `replaceFile` / `lockFileExclusive` / `unlockFile` | `tui/internal/fs/file_ops_unix.go:10-20`, `tui/internal/fs/file_ops_windows.go:11-31` | supplies platform replacement and advisory-lock operations: rename/flock on non-Windows and `MoveFileEx` with replacement/write-through plus `LockFileEx`/`UnlockFileEx` on Windows |
| `AcquireExclusiveFileLock` / `ExclusiveFileLock.Release` | `tui/internal/fs/exclusive_lock.go` | reusable stable-file advisory-exclusive lock: canonical path mutex, ensured lock parent, blocking OS lock, then idempotent unlock/close/mutex release; the sidecar is never removed |
| **mail.go** | | |
| `newMailboxID()` | `tui/internal/fs/mail.go:33` | builds `YYYYMMDDTHHMMSS-xxxx` short id matching the kernel's `_new_mailbox_id` |
| `prepareMailDirs` | `tui/internal/fs/mail.go:55` | allocates a short id and creates every mailbox leaf the send will write, retrying on collisions in any target folder |
| `ReadInbox(dir)` | `tui/internal/fs/mail.go:93` | reads `mailbox/inbox/` → `[]MailMessage` |
| `ReadSent(dir)` | `tui/internal/fs/mail.go:97` | reads `mailbox/sent/` → `[]MailMessage` |
| `MailCache` | `tui/internal/fs/mail.go:166` | incremental refresh cache: outbox + inbox + sent merged |
| `NewMailCache(humanDir)` | `tui/internal/fs/mail.go:176` | creates cache; `Refresh()` returns updated copy (receiver not mutated) |
| `MessageLimitEnv` / `DefaultRecentMessageLimit` | `tui/internal/fs/mail.go:107`, `tui/internal/fs/mail.go:112` | the environment variable `LINGTAI_TUI_MESSAGE_LIMIT` and the window an untouched environment resolves to (1000) |
| `RecentMessageLimit()` | `tui/internal/fs/mail.go:138` | resolves the cap on newest message entries any page loads or displays, and is the ONLY parser of `MessageLimitEnv`: unset/non-integer/negative → the default, positive N → N, `0` → unlimited (the full historical load, an explicit performance opt-in). Read live per call rather than latched, so the window is testable without process-global state; a typo resolves to the default rather than blocking the page |
| `ClampToRecentMessageLimit(n)` | `tui/internal/fs/mail.go:155` | bounds a caller's desired entry count by the resolved window and is where "0 means unlimited" is decided once — a consumer writing its own `n > limit` would clamp an unlimited window to zero |
| `MailCache.RefreshRecent(limit)` | `tui/internal/fs/mail.go:338` | bounded refresh: directory listings only, a per-id-shape candidate window (newest `limit` canonical ids by name plus newest `limit` legacy ids by leaf mtime, at most `2*limit` bodies) selected before any `message.json` is read, retained entries reused, candidates provably older than a full retained window skipped, result trimmed to the newest `limit` by timestamp; `limit <= 0` delegates to `Refresh()`, which is how the unlimited opt-in reaches the disk |
| `mailboxCandidate` / `parseCanonicalMailboxID` / `retainedWindowCutoff` | `tui/internal/fs/mail.go:494`, `tui/internal/fs/mail.go:510`, `tui/internal/fs/mail.go:532` | the shape gate and cost gates behind the bound: a candidate carries the instant its listing implies (name stamp for canonical `YYYYMMDDTHHMMSS-xxxx` ids, mtime for everything else), and once the retained window is full the oldest retained `ReceivedAt` minus one second of proxy slack decides which candidates are not worth opening |
| `isMailboxLeafName` / `isMailboxLeafEntry` | `tui/internal/fs/mail.go:555`, `tui/internal/fs/mail.go:561` | the staging filter shared by every mailbox reader here: a dot-prefixed directory (for example kernel `.<id>.staging` or TUI `.<name>.tmp-<hex>`) is an in-flight or abandoned leaf, never a message, and is excluded by name without a stat |
| `mailboxIDsIn` / `readMailboxMessage` | `tui/internal/fs/mail.go:569`, `tui/internal/fs/mail.go:612` | the listing/body split that makes the bound possible: names are enumerated without opening a single body; `readMailboxFile` (`tui/internal/fs/mail.go:608`) is the swappable read seam tests count bodies through |
| `RecentMailboxIDs(folder, limit)` | `tui/internal/fs/mail.go:596` | the listing half as a standalone primitive for readers outside this package (the `/mailbox` page): lexically newest `limit` leaf ids, oldest-first, staging names excluded, chosen from the directory listing alone; exact only for canonical ids, see Notes; `limit <= 0` returns every id |
| `MailCache.Clone()` | `tui/internal/fs/mail.go:189` | deep-clones seen sets, message slices, recipients, attachments, and identity while preserving nil versus non-nil-empty shapes for accepted-snapshot publication |
| `writeJSONAtomic` | `tui/internal/fs/mail.go:708-710` | delegates mailbox JSON publication to the shared unique-temp atomic replacement primitive without changing routing or mailbox semantics |
| `WriteMail` | `tui/internal/fs/mail.go:712-775` | writes local mail to recipient inbox + sender sent (or human outbox for pseudo-agent); returns `ErrRemoteMailUnsupported` before mailbox allocation for remote addresses |
| **direct_mail.go** | | |
| `DirectTarget` / `DirectThreadKey` / `AddressFingerprint` | `tui/internal/fs/direct_mail.go:9-38` | target carries canonical project + target directories, durable manifest AgentID, and current route; thread identity hashes `(project, agent_id)`, while the address fingerprint is route-only |
| `NormalizeMailEndpoints` / `IsDirectMail` | `tui/internal/fs/direct_mail.go:40-149` | keeps lenient deduplication for topology, but direct membership requires one valid raw recipient, empty CC, distinct endpoints, exact current addresses, and matching supplied inbound `identity.agent_id` |
| **direct_publication.go** | | |
| `DirectMailPublication` / `NewDirectMailPublication` | `tui/internal/fs/direct_publication.go:15-84` | immutable, indexed accepted-direct snapshot: one pass over accepted mail selects same-strict-peer-address candidates (`directMailPeerAddress`, `tui/internal/fs/direct_publication.go:89-106`), keeps `IsDirectMail` as the final predicate, and retains per-thread detached chronological messages plus pre-resolved incoming unread summaries and the latest monotonic boundary |
| `DirectPage` | `tui/internal/fs/direct_publication.go:145-163` | newest-horizon chronological page plus `hasOlder` for exactly one validated stable route; O(page) work/allocation independent of unrelated accepted mail |
| publication unread accessors | `tui/internal/fs/direct_publication.go:179-214` | fail-closed per-thread incoming summaries, latest boundary, and human-address/target-set validation consumed by the publication-aware unread store APIs |
| **direct_unread.go** | | |
| `withDirectUnreadTransaction` / `refreshedDirectUnreadState` | `tui/internal/fs/direct_unread.go:59-100` | serializes by canonical state path in-process, holds a stable sibling `.lock` with an OS-exclusive advisory lock, and rereads a valid durable baseline; lock order is path mutex → OS lock → store mutex, released in reverse |
| `DirectUnreadStore` / `OpenDirectUnreadStore` / `OpenDirectUnreadStorePublication` | `tui/internal/fs/direct_unread.go:17-23`, `tui/internal/fs/direct_unread.go:106-168` | stores project-local version-1 direct-thread cursors and performs open/read/baseline/save as one stable-path transaction; the accepted-slice entry point delegates all routing/boundary resolution to a `DirectMailPublication`, and the publication variant baselines without rescanning accepted history |
| `SyncTargets` / `UnreadCount` / `MarkSeen` + `*Publication` variants | `tui/internal/fs/direct_unread.go:170-342` | adds but never prunes stable keys, keeps cached unread reads, and transactionally refreshes then copy-on-write saves monotonic cursor changes before publishing memory; each legacy accepted-slice API is a thin wrapper over its publication-aware variant, which consumes the publication's pre-resolved summaries/boundary instead of walking accepted mail |
| `Clone` | `tui/internal/fs/direct_unread.go:344-360` | detached in-memory cursor store retaining the same durable path, for async lane commands that mutate/persist a clone and install it only after exact acceptance coordinates still match |
| `saveDirectUnreadState` / direct cursor resolver | `tui/internal/fs/direct_unread.go:335-470` | serializes indented version-1 JSON plus newline through the shared atomic replacement helper and accepts only strict incoming direct mail with RFC3339Nano timestamps and exact nonblank stable IDs |
| **ledger.go** | | |
| `ReadLedger(dir)` | `tui/internal/fs/ledger.go:17` | reads `delegates/ledger.jsonl` → `[]AvatarEdge` + child dirs |
| **location.go** | | |
| `humanLocationManifestMutex` / `RemoveHumanManifestForReset` | `tui/internal/fs/location.go:23-49` | gives one lexical-canonical manifest path a process-lifetime mutex; the reset helper crosses that exact writer barrier, removes only `human/.agent.json` while locked, and releases before recursive tree removal |
| `ResolveLocation()` | `tui/internal/fs/location.go:51-77` | queries `ipinfo.io/json` → `Location` |
| `LocationStale(loc, maxAge)` | `tui/internal/fs/location.go:79-90` | true if `ResolvedAt` exceeds `maxAge` |
| `UpdateHumanLocation` / `StoreResolvedHumanLocation` / `storeResolvedHumanLocationLocked` | `tui/internal/fs/location.go:92-145` | coalesces stale same-manifest lookups; the resolved-value entry point synchronously reuses an existing lookup through the identical transaction, latest-manifest reread, merge-only `location` commit, and shared unique-temp atomic replacement |
| **network.go** | | |
| `NetworkOptions` / `KanbanNetworkOptions` / `BuildNetworkWithOptions` | `tui/internal/fs/network.go:9-128` | zero values preserve full legacy behavior; the Kanban profile sets `SkipMailEdges` and 1000-entry/record limits for agents, topology, contacts, and dispatch activity plus byte ceilings, and propagates agent-directory truncation for bounded-subset rendering |
| **activity.go** | | |
| `ComputeNetworkActivity(baseDir)` | `tui/internal/fs/activity.go:46` | lightweight non-human project activity badge: folds agent state, heartbeat liveness, `.status.json` activity evidence, and running daemons into active, daemon-active, idle, asleep, suspend. Heartbeat liveness (`Alive`) gates every non-daemon bucket but never rewrites the per-agent manifest `State` — a stale/missing heartbeat surfaces only through `AgentNode.Alive=false`, and a genuine manifest `SUSPENDED` is the only path to a `SUSPENDED` node |
| `hasStatusActivity(agentDir, now)` | `tui/internal/fs/activity.go:174` | treats heartbeat-live agents as active when status-snapshot evidence is fresh: `active_turn` via mtime/started_at/last_progress_at within 600s, or `last_progress_at` within 90s |
| `CountDaemons(agentDir)` | `tui/internal/fs/activity.go` | classifies only exact recent run cards selected from the bounded dispatch-ledger tail; it never enumerates `daemons/`, has no historical-directory fallback, and its cost is independent of lifetime run count |
| **resolve.go** | | |
| `ParseAddress(addr)` | `tui/internal/fs/resolve.go:16` | `"localhost:/path"` or `"[ipv6]:/path"` → `(host, path, ok)` |
| `IsRemoteAddress(addr)` | `tui/internal/fs/resolve.go:62` | true if non-localhost host prefix |
| `ResolveAddress(addr, baseDir)` | `tui/internal/fs/resolve.go:81` | relative name → absolute path; host:path → as-is |
| `RelativizeAddress(addr, baseDir)` | `tui/internal/fs/resolve.go:94` | absolute → relative by stripping `baseDir/` prefix |
| **signal.go** | | |
| `Signal` type | `tui/internal/fs/signal.go:9` | `SignalSleep`, `SignalSuspend`, `SignalInterrupt` |
| `TouchSignal`, `HasSignal` | `tui/internal/fs/signal.go:17,21` | write/check `.sleep` / `.suspend` / `.interrupt` |
| `CleanSignals(dir)` | `tui/internal/fs/signal.go:32` | remove all signal + refresh handshake files |
| `SuspendAndWait` | `tui/internal/fs/signal.go:43` | touch `.suspend`, poll heartbeat until dead or timeout |
| **session.go** | | |
| `SessionPersistenceRole` / `SessionCache` | `tui/internal/fs/session.go:125-165` | separates the sole `MainAggregateWriter` from zero-safe `NoPersist`, independently of mutex-protected replay-window completeness and offsets |
| `NewSessionCache` | `tui/internal/fs/session.go:176-190` | pure in-memory construction with an explicit persistence role; creates no file or directory |
| `RebuildFromSources` / `RebuildFromSourcesInMemory` | `tui/internal/fs/session.go:195-213` | authoritative full ingest; write-through requests still pass through the cache's persistence role |
| `RebuildFromSourcesWindowedInMemory` / `Complete` / `ExactHistoryStats` | `tui/internal/fs/session.go:219-230`, `tui/internal/fs/session.go:425-436` | bounded newest-content ingest; completeness prevents partial-file truncation but does not grant write authority. `ExactHistoryStats` (the separately invoked full-history metadata count) is retained and tested but has no production caller — see the note below |
| `Persist` / `PersistErr` / `rewriteFile` / `append` | `tui/internal/fs/session.go:313-389` | complete `MainAggregateWriter` snapshots use unique-temp atomic replacement; `PersistErr` reports replacement failures while compatibility `Persist` and internal rebuild deliberately remain best-effort, and append behavior is unchanged |
| `Refresh` | `tui/internal/fs/session.go:2251-2260` | incremental poll from each source's last complete consumed record; `NoPersist` caches update memory without appending the shared aggregate |
| **project_hash.go** | | |
| `ProjectHash(projectPath)` | `tui/internal/fs/project_hash.go:9` | SHA-256 first 12 hex chars — used as the registry key for each project |
| **contacts.go** | | |
| `ReadContacts(dir)` | `tui/internal/fs/contacts.go:15` | reads `mailbox/contacts.json` → `[]ContactEdge` |
| **types.go** | | |
| `AgentNode` | `tui/internal/fs/types.go:15` | durable agent_id, current address, agent_name, nickname, state, alive, is_human, capabilities, location |
| `AvatarEdge`, `ContactEdge`, `MailEdge` | `tui/internal/fs/types.go:29-47` | graph edge types |
| `Network`, `NetworkStats` | `tui/internal/fs/types.go:50-67` | full topology + aggregate counts |
| `MailMessage` | `tui/internal/fs/types.go:70` | mailbox message schema; `Delivered` is transient (`json:"-"`) |
| `Location` | `tui/internal/fs/types.go:5` | city, region, country, timezone, loc, resolved_at |

## Connections

- **Called by `tui/internal/tui/`** — every Bubble Tea screen reads agent state through this package (network home, agent detail, mail viewer, kanban, session log).
- **Called by `tui/internal/inventory/`** — running-agent inventory enriches process rows with `.agent.json`, heartbeat, status PID, lock, admin, IM identity, and orchestrator-role metadata.
- **Reads from agent working directories** — `.agent.json`, `.agent.heartbeat`, `.status.json`, `mailbox/*/`, token/event/history JSONL, topology/contact state, `daemons/.dispatch-ledger.jsonl`, and exact dispatched run cards/ledgers. Kanban uses only the bounded variants above.
- **Writes signal files** (the only agent-owned files the TUI writes): `.sleep`, `.suspend`, `.interrupt`, `.prompt`, `.inquiry`, `.refresh`/`.refresh.taken`.
- **Writes human-owned/derived state** — local `WriteMail` writes recipient inbox + sender sent, or `human/mailbox/outbox/<mailbox-id>/` for pseudo-agent sends; remote addresses fail before any mailbox write. Only a complete `MainAggregateWriter` changes `human/logs/session.jsonl` (`tui/internal/fs/session.go:313-350`). Separately, `DirectUnreadStore` reads/writes project-local `<project>/.lingtai/.tui-asset/direct-unread.json` cursor state under the stable sibling `direct-unread.json.lock` (`tui/internal/fs/direct_unread.go:59-100,103-153,335-348`); it is not agent, migration, or session state.
- **Calls `ipinfo.io`** — `ResolveLocation` makes an HTTP call; `UpdateHumanLocation` owns the serialized stale lookup, while `StoreResolvedHumanLocation` lets recipe rendering synchronously cache the value it already resolved without a second request.

## Composition

- **Parent:** `tui/internal/` (no own anatomy)
- **Subfolders:** none — flat package
- **Siblings:** `tui/internal/preset/ANATOMY.md`, `tui/internal/migrate/ANATOMY.md` — fs is a data layer, preset and migrate are logic layers

## State

- **Reads**: `.agent.json`, `.agent.heartbeat`, `.status.json`, `mailbox/inbox/*`, `mailbox/sent/*`, `logs/log.sqlite` (additive index), `logs/token_ledger.jsonl` (main rows only for agent totals/detail), `logs/events.jsonl`, `logs/soul_inquiry.jsonl`, `logs/soul_flow.jsonl`, `delegates/ledger.jsonl`, `mailbox/contacts.json`, `daemons/*/daemon.json`, `daemons/*/logs/token_ledger.jsonl`, and TUI-owned `<project>/.lingtai/.tui-asset/direct-unread.json`.
- **Writes**: signal files (`.sleep`, `.suspend`, `.interrupt`, `.prompt`, `.inquiry`), human `mailbox/outbox/*`, human `.agent.json` location field, the TUI-derived human `logs/session.jsonl` replay cache only from `MainAggregateWriter` persist/append paths, and project-local `<project>/.lingtai/.tui-asset/direct-unread.json` plus its stable `.lock`; replacement writes use cleaned unique sibling temps rather than fixed `.tmp` names.

## Notes

- **Read-only for agent state.** This package is the TUI's window — it never writes agent-owned files except signal files. The kernel owns `.agent.json`, heartbeats, mailboxes, ledgers, logs. Do not add write paths for kernel-owned state.
- **Mailbox id shape.** `WriteMail` allocates short, human-scannable ids of the form `YYYYMMDDTHHMMSS-xxxx` (20 chars, UTC, 4 hex chars of UUID4 entropy) via `newMailboxID`. This matches the kernel's `_new_mailbox_id` in `lingtai-kernel/src/lingtai/kernel/intrinsics/email/primitives.py` and the portal's mirror in `portal/internal/fs/mail.go`, so directory names, `id`, and `_mailbox_id` look identical regardless of which side wrote the message. The directory name IS the id — `prepareMailDirs` uses `os.Mkdir` (not `MkdirAll`) on each leaf so collisions in any target folder surface as `fs.ErrExist` and trigger up to 8 regenerations without overwriting existing mail.
- **`Delivered` is transient.** `MailMessage.Delivered` is `json:"-"` — set by `MailCache.Refresh()` / `RefreshRecent()` based on which folder the message was found in. Outbox → false; inbox/sent → true.
- **`MailCache` refresh and snapshot boundaries differ.** `Refresh()` returns a new cache without mutating the receiver, while `Clone()` is the explicit deep-copy boundary for accepted publication: nested recipient, attachment, and identity graphs cannot alias the live producer, and nil shapes are preserved.
- **Bounded mailbox window.** `RefreshRecent(limit)` is the refresh the TUI page uses, and `RecentMessageLimit()` resolves the configured cap (default 1000); `LINGTAI_TUI_MESSAGE_LIMIT=0` is the explicit unlimited value. With `limit=0`, `RefreshRecent` skips candidate narrowing and loads the full mailbox. It lists mailbox directories (names only), narrows the on-disk candidates **before** reading any `message.json`, reuses already-loaded entries without re-reading them, then sorts by parsed instant and keeps the newest `limit`. Candidate selection is per id shape, because a real mailbox is not uniformly canonical: current producers (TUI, portal, kernel) write sortable `YYYYMMDDTHHMMSS-xxxx` ids, but older producers left `hb-*` and hash-style ids whose names carry no chronology, and dot-prefixed staging leaves such as `.<id>.staging` sit beside them. Canonical ids are windowed by name (exact); every other id is windowed by leaf mtime (one lstat per legacy leaf per refresh, no body read; a copied or restored mailbox may have flattened mtimes, in which case the legacy window is arbitrary but still bounded); dot-prefixed names are skipped outright. The two windows are kept separate so legacy names that sort after `2026…` can never crowd canonical ids out — a single lexical window did exactly that on a mailbox holding more than `limit` `hb-*` leaves and hid the newest delivered message. A cold refresh therefore opens at most `2*limit` bodies; once the retained window is full, candidates whose proxy instant is more than a second older than the oldest retained `ReceivedAt` are not opened, so a steady-state tick reads only what arrived. The final trim is by timestamp so anything already in hand is windowed by real recency rather than name shape. Retention otherwise matches `Refresh()`: an entry stays loaded even if its folder stops listing it. Consequence for callers: a bounded cache is not the whole mailbox, so anything built from it — including `DirectMailPublication` — indexes only the retained window. `RefreshRecent(0)` and `Refresh()` are the unbounded paths for callers that need every message. `RecentMailboxIDs(folder, limit)` exposes just the listing half of that bound, because the `/mailbox` page decodes a different message shape (the internal/IMAP union) and needs to choose its window without inheriting `MailMessage`; it still windows lexically, so on a folder mixing legacy ids the `/mailbox` page's window is by name shape rather than age — a known limitation, not a promise.
- **`BuildNetwork`'s mail edges are the unbounded path.** `buildMailEdges` reads every message in every agent's inbox to aggregate per-edge counts, so its cost grows with the network's whole mail history. Every TUI caller passes `SkipMailEdges: true` (a `tui` package test enforces this structurally); the full builder stays for the portal API, which serves `mail_edges`/`total_mails` over HTTP. Do not add a TUI caller of the default `BuildNetwork`.
- **Direct mail identity boundary.** `DirectTarget` separates stable identity (canonical project directory + manifest `agent_id`) from current routing (target directory + address); `DirectThreadKey` hashes only the stable pair, and `AddressFingerprint` is route-only. `NormalizeMailEndpoints` remains deliberately lenient for topology edges. `IsDirectMail` instead validates one raw recipient, rejects any CC, malformed/multi-entry envelope, empty/equal endpoints, or cross-address record, and on incoming mail requires any supplied nonblank `identity.agent_id` to match literally while allowing exact-address fallback for legacy mail without that field.
- **Durable direct unread boundary.** `<project>/.lingtai/.tui-asset/direct-unread.json` remains schema version 1 and does not read or migrate historical rail state. Mutations serialize on one path-keyed process mutex and the stable sibling `.lock`, reread durable state while locked, prefer a valid disk snapshot, retain valid open memory when disk is missing/malformed/unsupported, and fail closed on other read errors. Each `DirectThreadKey` entry retains exact `agent_id` and a monotonic parsed-time cursor of sorted unique effective IDs at that instant, so route/directory changes do not reset it, same-timestamp IDs union, and absent inventory entries are not pruned (`tui/internal/fs/direct_unread.go:17-34,59-100,103-203,243-291,369-470`). `UnreadCount` remains an in-memory read. This state is separate from `SessionCache`/`session.jsonl` and has no migration behavior. The immutable `DirectMailPublication` (`tui/internal/fs/direct_publication.go`) is pure in-memory index state built once per accepted refresh by its TUI caller; it owns no file, lock, or durable bytes, and the durable store's F-era path-locked transaction semantics are consumed unchanged by the publication-aware APIs.
- **Session persistence role.** `MainAggregateWriter` is the only role authorized to mutate the compatibility aggregate `human/logs/session.jsonl`; zero-safe `NoPersist` is enforced inside both rewrite and append primitives. Complete rewrites encode, flush, close, and atomically replace through a unique sibling temp, so concurrent final state may be either complete snapshot without exposing a torn canonical file. `PersistErr` exposes replacement failure; compatibility `Persist` intentionally discards it. `complete` describes whether the in-memory history window can safely replace or extend a complete derived file—it is not write authorization (`tui/internal/fs/session.go:313-350`).
- **Session cache reconstruction.** `RebuildFromSources` is idempotent — it re-ingests all mail + events + inquiries from offset 0, sorts by timestamp, and requests a role-gated `session.jsonl` rewrite; `RebuildFromSourcesInMemory` performs the same read/merge without filesystem writes for detached generation-gated work. Canonical `logs/events.jsonl` owns session content and completeness: the additive SQLite log's source identity and endpoint offsets do not prove interior continuity, so they are not used to declare a replay complete. Every path retains the last complete-record boundary it actually consumed, so trailing partial records and concurrent appends are retried by `Refresh` rather than leaked, duplicated, or skipped.
- **Windowed reconstruction.** `RebuildFromSourcesWindowedInMemory` retains only the newest requested parser-produced session-event content window while loading the caller's mail/inquiries. For a positive limit, `mail_page_size` owns that bounded window after `RecentMessageLimit()` clamps it; `LINGTAI_TUI_MESSAGE_LIMIT=0` instead passes the full-history sentinel to the session rebuild, while `mail_page_size` remains the initial display and Ctrl+U reveal batch. There is no disk-backed older-page increment. Empty/missing/wrong-type text rows do not spend content slots, while hidden `llm_call` and zero-token `llm_response` grouping carriers still do. JSONL content is read backward from EOF. A cut legacy group retains only its nearest hidden `llm_response` marker. Parser-proven offsets, stable sort, and the shared completeness gate on both persistence and incremental disk append keep a windowed cache from ever claiming to be the full history.
- **Exact-count metadata is retained but unused.** `ExactHistoryStats` / `HistoryStats` / `SetHistoryStats` and the bounded metadata scanner behind them still exist and are still tested here, but **no production caller invokes them**: the Main page deliberately computes no total-message count, so nothing schedules that full-history scan. Do not reintroduce a caller — the page's positive/default contract is "load and display the newest `RecentMessageLimit()` entries"; with `LINGTAI_TUI_MESSAGE_LIMIT=0`, it may load complete history but still displays in `mail_page_size` batches, and it never counts history merely to report a total.
- **`parseEvent` event-type allow-list.** Only certain `events.jsonl` / `log.sqlite` types become `SessionEntry`s: `thinking`, `diary`, `text_input`, `text_output`, `tool_call`, `tool_result`, `insight`, `soul_flow`, `notification`, `aed`, and `apriori_summary`. Four kernel-side rename/promotion rules at ingest: `consultation_fire → soul_flow` (carries `fire_id` for voice-index inflation against `logs/soul_flow.jsonl`); `notification_pair_injected → notification` (carries `sources []string` and prefers the kernel-logged `summary` string for body, **plus an optional `meta *NotificationMeta`** with `current_time`, `context.{system_tokens,history_tokens,usage}`, and `injection_seq` — the kernel's `build_meta` snapshot at injection time, rendered as a faint footer line by `mail.go`; nil for events written before issue #40); `aed_attempt`/`aed_exhausted`/`aed_timeout → aed` (subtype written to `Source`, body recovered from raw `type` plus per-subtype fields — `attempt`/`error`, `attempts`/`error`, `seconds`); and `apriori_summary_generated`/`apriori_summary_cap_refused`/`apriori_summary_failed`/`apriori_summary_empty`/`apriori_summary_no_summarizer → apriori_summary` (summary metadata and generated text preserved for Ctrl+O rendering). To surface a new event type in the chat replay: extend the rename map (if needed), the allow-list in `parseEventMap` (the `switch eventType` in `tui/internal/fs/session.go`) and the `sqlitelog` session-event filter (`sessionEventFilterSQL` in `tui/internal/sqlitelog/event.go`), `extractSessionEventText`, and the renderer in `tui/internal/tui/mail.go`.
- **Provider derivation.** `DeriveLedgerProvider` uses endpoint host substring matching first, then model prefix fallback. Unknown endpoints surface the hostname so the UI still shows a breakdown.
- **Location is cached for 1 hour and merged into the latest manifest.** `UpdateHumanLocation` holds one `Abs(Clean(manifestPath))` process mutex from the stale check through the ≤5-second resolve and final commit. The commit rereads a valid `.agent.json`, retains any newer fresh location, otherwise changes only the top-level `location` object (whose timestamp is nested `resolved_at`), and publishes through the package's cleaned unique sibling atomic replacement. `StoreResolvedHumanLocation` uses the same transaction without network I/O; missing or malformed manifests remain best-effort no-ops. Nirvana's final filesystem boundary calls `RemoveHumanManifestForReset` only after durable Mail drain: it waits for this same mutex, removes the location writer's commit target, releases the mutex, and only then recursively removes `.lingtai`, preventing late partial resurrection without holding the manifest lock across discovery, suspension, or `RemoveAll`.
- **`related_files` is this package's full inventory.** The repo-wide no-orphan rule (root `ANATOMY.md`, `## Anatomy convention`) requires every tracked file here to appear in the frontmatter above, and `TestArchitectureDocumentsCoverEveryTrackedFile` (`tui/architecture_documents_test.go`) fails when one is missing. The body stays the curated architectural map: adding a file does not oblige a new row above, but adding its `related_files` entry in the same commit is mandatory — and deleting a file means deleting its entry.
