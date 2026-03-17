# Agent Network Visualization

Second page in the web dashboard showing agents and user as draggable nodes in a force-directed graph. Edges represent email connections. Animated light dots travel along edges when emails are sent.

## Location

Same app: `app/web/`. Adds a new page to the existing React frontend. Minor backend change to expose agent type.

## Tech

- **D3.js** (`d3-force`, `d3-selection`, `d3-drag`) for physics simulation and interaction (~90KB)
- No new backend endpoints — derives network data from existing diary events
- Page toggle via header tabs (React state, no router)

## Backend Change

### `/api/agents` response — add `type` field

```python
# In routes.py list_agents()
{
    "id": "alice",
    "name": "Alice",
    "key": "a",
    "address": "127.0.0.1:8301",
    "port": 8301,
    "status": "active",
    "type": "admin"  # or "agent"
}
```

Derived from `entry.agent._admin`: `"admin"` if `True`, `"agent"` otherwise.

### Frontend `AgentInfo` type — add `type`

```typescript
interface AgentInfo {
  // ... existing fields ...
  type: "admin" | "agent";
}
```

## Data Flow

```
useDiary entries (already polling)
  → useNetwork derives edges from email_out / email_in events
  → builds edge map: {source, target, count}
  → tracks recent emails for particle spawning
  → D3 force simulation positions nodes
  → SVG renders nodes + edges + animated particles
```

## Page Navigation

Header gets two tab buttons: "Inbox" and "Network". `App.tsx` tracks `activePage` state and conditionally renders:
- `"inbox"` → InboxPanel + DiaryPanel (existing)
- `"network"` → NetworkPage (new)

## Components

### Modified

**`Header.tsx`** — add page tabs ("Inbox" / "Network"). Active tab highlighted with accent underline.

**`App.tsx`** — add `activePage` state. Render inbox view or network view based on state.

**`types.ts`** — add `type` to `AgentInfo`. Add network-related types.

### New

**`NetworkPage.tsx`** — full-page SVG container. Owns the D3 force simulation. Renders nodes, edges, and animated particles.

**`hooks/useNetwork.ts`** — derives network graph from diary entries. Manages particle animation lifecycle.

## NetworkPage Detail

### Nodes

- **Agent nodes:** Circle with agent color border. Name label centered inside. Small status dot (green = active, gray = sleeping).
- **Admin nodes:** Double-ring (concentric circles) — outer ring slightly larger with same color. Visually distinct from regular agents.
- **User node:** Circle with accent color. Label "You".
- All nodes draggable via D3 drag behavior.

### Edges

- Lines between nodes that have exchanged at least one email.
- Thickness proportional to email count (min 1px, max 4px).
- Color: `#0f3460` (border color) for normal edges.
- Only appear after first email between two nodes.
- Edges are undirected — emails in either direction contribute to the same edge.

### Particles (Animated Dots)

- When a new `email_out` or `email_in` event appears in the diary (timestamp newer than last seen), spawn an animated dot.
- Dot travels from sender node to recipient node over ~1.5s.
- Dot color matches the sender's agent color.
- Dot has a glow effect (SVG filter).
- Multiple dots can be in flight simultaneously.
- Dots fade out at the end of their journey.
- Particle lifecycle managed via `requestAnimationFrame` in the `useNetwork` hook.

### Force Layout

- Nodes repel each other (`d3.forceManyBody()`)
- Edges attract connected nodes (`d3.forceLink()`)
- Everything centers in viewport (`d3.forceCenter()`)
- Collision detection prevents overlap (`d3.forceCollide()`)
- Simulation stabilizes after a few seconds.
- On drag: reheat simulation, fix dragged node position, release to let it settle.

### User Node in Network

- User is a node like any other in the force layout.
- User address is `127.0.0.1:{userPort}`.
- Emails from/to user are derived from:
  - `email_out` events where `to` matches user address → edge from agent to user
  - `email_in` events where `from` matches user address → edge from user to agent
  - Sent messages from `useInbox` → edge from user to agent

## useNetwork Hook

```typescript
interface NetworkNode {
  id: string;          // agent key or "user"
  name: string;
  color: string;
  status: "active" | "sleeping";
  type: "admin" | "agent" | "user";
  // D3 adds: x, y, vx, vy
}

interface NetworkEdge {
  source: string;      // node id
  target: string;      // node id
  count: number;       // total emails exchanged
}

interface Particle {
  id: string;
  source: string;
  target: string;
  color: string;
  startTime: number;   // performance.now()
  duration: number;     // ms (1500)
}
```

**Input:** diary entries, agents list, user port, sent messages.

**Logic:**
1. Scan diary entries for `email_out` and `email_in` events.
2. Map addresses to node IDs (using `addressToName` from `useAgents`).
3. Build edge map: key = sorted pair of node IDs, value = email count.
4. Track `lastSeenTs` per agent. When new email events appear, spawn particles.
5. `requestAnimationFrame` loop updates particle progress (0→1 over duration). Remove particles when progress >= 1.

## What This Does NOT Do

- Does not add new backend endpoints (uses existing diary data)
- Does not add React Router (simple state toggle)
- Does not show email content in the network view (that's what the inbox page is for)
- Does not persist node positions (layout resets on page load)
