# 3D Network Page — Design Spec

## Summary

Replace the current D3 SVG-based `NetworkPage` with a 3D force-directed graph using `react-force-graph-3d` (ThreeJS/WebGL). Dark sci-fi command center aesthetic. Built-in pan/zoom/orbit/drag, automatic force-directed layout, directional particles along edges for email visualization, and custom glowing nodes for agent activity.

## Motivation

Current problems:
1. No pan/zoom — can't move the canvas
2. No auto-layout — nodes placed in a fixed circle, no meaningful organization
3. Breaks on re-render — `svg.selectAll("*").remove()` clears everything on data change
4. Doesn't scale — SVG DOM nodes become expensive beyond ~30 agents
5. No depth — flat 2D makes dense networks hard to read

## Dependencies

- `react-force-graph-3d` (npm, ThreeJS/WebGL, actively maintained)
- `three` (peer dependency of react-force-graph-3d)
- `three-spritetext` (npm, for always-visible floating node labels)

## Architecture

### Data Flow

```
useNetwork hook → { graphData: { nodes, links }, nodeActivity } → ForceGraph3D
                                                                 → linkDirectionalParticles (emails)
                                                                 → custom nodeThreeObject (sci-fi nodes)
```

### Files Changed

| File | Change |
|------|--------|
| `package.json` | Add `react-force-graph-3d`, `three`, `three-spritetext` |
| `types.ts` | Remove `Particle`, `Pulse`, `PULSE_STYLES`. Add `GraphNode`, `GraphLink`, `NodeActivity` types. `GraphNode` extends the library's `NodeObject` with our custom fields. |
| `useNetwork.ts` | Remove particle/pulse state management. Output `graphData` with **directed** links. Add `nodeActivity` tracking. Preserve `sentMessages` edge contribution. |
| `NetworkPage.tsx` | Full rewrite. Render `<ForceGraph3D>` with all customizations. No more D3 imports. |
| `App.tsx` | Update `useNetwork` destructuring and props passed to NetworkPage. |

### Type Definitions

```ts
import type { NodeObject, LinkObject } from 'react-force-graph-3d';

// GraphNode extends the library's NodeObject (which injects x,y,z,vx,vy,vz at runtime)
export interface GraphNode extends NodeObject {
  id: string;
  name: string;
  color: string;
  status: "active" | "sleeping";
  type: "admin" | "agent" | "user";
}

// Directed link — one per direction (Alice→Bob and Bob→Alice are separate links)
export interface GraphLink extends LinkObject {
  source: string;  // node id (before force simulation resolves to object)
  target: string;  // node id
  count: number;   // message count in this direction
}

// Tracks recent activity events per node for glow animation
export interface NodeActivity {
  type: "thinking" | "tool" | "diary";  // simplified from DiaryEventType
  time: number;  // timestamp
}
```

### Activity Type Mapping

Only 3 `DiaryEventType` values map to node glow effects:

| DiaryEventType | NodeActivity.type | Glow color |
|---------------|-------------------|------------|
| `thinking` | `thinking` | `#f0a500` (yellow) |
| `tool_call` | `tool` | `#6b9bcb` (blue) |
| `diary` | `diary` | `#6bcb77` (green) |

All other event types (`email_out`, `email_in`, `reasoning`, `tool_result`, `cancel_received`, `cancel_diary`, `unknown`) do NOT produce glow effects. The glow color constants live in `NetworkPage.tsx` as a local `GLOW_COLORS` map.

### react-force-graph-3d Mapping

| Current concept | New implementation |
|----------------|-------------------|
| `NetworkNode[]` | `graphData.nodes` — `GraphNode[]` |
| `NetworkEdge[]` (undirected) | `graphData.links` — `GraphLink[]` (**directed**, one per direction) |
| `Particle[]` (hand-rolled SVG animation) | `linkDirectionalParticles` prop — built-in, count based on link activity |
| `Pulse[]` (hand-rolled SVG rings) | Custom `nodeThreeObject` — animated emissive glow intensity on Three.js mesh |
| Circle layout (manual x/y) | Automatic 3D force-directed layout (d3-force-3d) |
| No pan/zoom | Built-in orbit camera controls |
| `d3.drag()` on individual nodes | Built-in node drag (re-heats simulation) |

## Visual Design — Dark Sci-Fi Command Center

### Background
- Deep space black/dark navy (`#0a0a1a` or `#050510`)

### Nodes
- **Geometry**: Glowing spheres (`SphereGeometry`) with `MeshStandardMaterial`
- **Color**: From `AGENT_COLORS`, applied as emissive color
- **Admin nodes**: Outer wireframe icosahedron shell (low-opacity wireframe) — like a shield/forcefield
- **User node ("You")**: Distinct geometry — octahedron or diamond shape
- **Active status**: Bright emissive glow, full opacity
- **Sleeping status**: Dim emissive, reduced opacity, desaturated
- **Activity glow**: When node is thinking/using tools/writing diary, pulse the emissive intensity up then decay — driven by `nodeActivity` map. Use `requestAnimationFrame` in the `nodeThreeObject` setup to animate emissive intensity.
- **Labels**: `SpriteText` (from `three-spritetext`) floating above each sphere — agent name, always facing camera

### Edges (Links)
- Thin lines, slightly glowing, base color `#1a3a5c` or similar dark blue
- Width/opacity scales with `count` (message volume)
- **Directed links**: Each direction is a separate link. Use `linkCurvature: 0.15` so bidirectional links between the same pair form visible arcs rather than overlapping
- `linkDirectionalArrowLength`: small arrow (3-4) at target end to show direction

### Particles (Email Animation)
- Use `linkDirectionalParticles` — set count > 0 on links with recent email activity
- `linkDirectionalParticleColor`: accessor returning the sender's agent color (from `source` node's color)
- `linkDirectionalParticleWidth`: 3-4
- `linkDirectionalParticleSpeed`: 0.01-0.02
- Particles glow naturally in WebGL against dark background

### Node Labels
- `nodeLabel` prop for hover tooltips: "agent_name (status) [type]"
- `SpriteText` from `three-spritetext` for always-visible name label above node

### Camera
- Default: orbit controls
- Initial position: looking at graph center from a moderate distance
- Camera position preserved across re-renders (the library handles this internally)

## View Toggle: Communication vs Activity

Overlay controls (top-right corner of the 3D view, floating HTML div positioned absolute over the canvas).

Two rows of controls:
1. **View mode** — Communication vs Activity (visual appearance)
2. **Layout mode** — Default vs Email Volume vs Contact Cluster (node positioning)

All toggle state stored as local `useState` in `NetworkPage`.

### View: Communication Mode (default)
- Edges weighted by message count (width + opacity)
- Directional particles visible
- `linkDirectionalParticles` active

### View: Activity Mode
- Node glow intensity driven by recent event frequency
- Edges dimmed (`linkOpacity: 0.05`)
- `linkDirectionalParticles` set to 0 (hidden)
- Glow colors: thinking=yellow (`#f0a500`), tool=blue (`#6b9bcb`), diary=green (`#6bcb77`)

Switching view modes preserves camera position (only visual props change, no `graphData` mutation).

## Layout Toggle: Default vs Email Volume vs Contact Cluster

### Layout: Default
- Standard d3-force-3d force-directed layout
- Default charge strength (`-30`), default link strength
- Balanced spacing, no bias

### Layout: Email Volume (centripetal)
- Adds a `d3.forceRadial` force: nodes with high total email volume are pulled toward the center, low-traffic nodes drift to the periphery
- Radial distance = `maxRadius * (1 - normalizedVolume)` — high volume → small radius (center), low volume → large radius (outside)
- Radial force strength: `0.4`
- Charge weakened to `-30` so radial force dominates
- Requires precomputing per-node total email volume (sum of all link counts involving that node)

### Layout: Contact Cluster
- No radial force
- Link force strength proportional to message count: `0.3 + 0.7 * (count / maxCount)` — heavy communicators pull tight, sparse communicators stay loose
- Charge increased to `-80` so unconnected nodes push far away
- Result: natural clusters of frequently-communicating agents, with sparse cross-cluster links stretched

### Implementation via `d3Force()`

`react-force-graph-3d` exposes the d3-force simulation via `.d3Force(name, force)`. Layout changes are applied by swapping forces at runtime and calling `.d3ReheatSimulation()` to animate the transition:

```ts
// Email Volume mode
graphRef.current.d3Force('radial', d3.forceRadial(
  (node: GraphNode) => maxRadius * (1 - node._volume / maxVolume)
).strength(0.4));
graphRef.current.d3Force('charge').strength(-30);
graphRef.current.d3ReheatSimulation();

// Contact Cluster mode
graphRef.current.d3Force('radial', null);
graphRef.current.d3Force('link').strength(
  (link: GraphLink) => 0.3 + 0.7 * (link.count / maxCount)
);
graphRef.current.d3Force('charge').strength(-80);
graphRef.current.d3ReheatSimulation();

// Default mode
graphRef.current.d3Force('radial', null);
graphRef.current.d3Force('charge').strength(-30);
graphRef.current.d3Force('link').strength(null);  // reset to d3 default
graphRef.current.d3ReheatSimulation();
```

### Data Requirements

`useNetwork` hook must compute and attach `_volume` to each `GraphNode`:
```ts
// Total email volume per node (sum of all link counts where node is source or target)
node._volume = links
  .filter(l => l.source === node.id || l.target === node.id)
  .reduce((sum, l) => sum + l.count, 0);
```

Add `_volume: number` to `GraphNode` interface.

## useNetwork Hook Changes

### Current Output
```ts
{ nodes: NetworkNode[], edges: NetworkEdge[], particles: Particle[], pulses: Pulse[] }
```

### New Output
```ts
{
  graphData: {
    nodes: GraphNode[],
    links: GraphLink[],
  },
  nodeActivity: Map<string, NodeActivity[]>,  // last N events per node
}
```

### Key Changes
- **Directed edges**: Replace the sorted-key undirected edge aggregation with directed edges. `addEdge(from, to)` no longer sorts keys — `from→to` and `to→from` are separate links.
- **Remove** all `useState` for particles/pulses — the library handles particle animation
- **Remove** `setTimeout` cleanup logic
- **Preserve** `sentMessages` contribution to edges (user→agent links)
- **`nodeActivity`**: Track last 5 events per node for glow decay. Only store events with `type` in `["thinking", "tool_call", "diary"]`. Prune old entries (older than 10s) on each update.

### App.tsx Changes

Current destructuring:
```ts
const { nodes, edges, particles, pulses } = useNetwork(agents, entries, sentMessages, userPort);
```

New destructuring:
```ts
const { graphData, nodeActivity } = useNetwork(agents, entries, sentMessages, userPort);
```

NetworkPage props change from `{ nodes, edges, particles, pulses, lightMode }` to `{ graphData, nodeActivity, lightMode }`.

## What We Lose vs Gain

### Lose
- SVG pulse rings → replaced by 3D glow animation (better)
- Manual particle animation code → replaced by built-in (simpler, smoother)
- Pixel-perfect 2D positioning → 3D space (different, but more expressive)

### Gain
- Pan, zoom, orbit, drag — all built-in
- Automatic force-directed layout — no manual positioning
- Scales to 100+ nodes on WebGL
- 3D depth for dense networks
- Built-in particle system for edge animation
- Dark sci-fi aesthetic natural in WebGL/3D

## Edge Cases

- **No agents connected**: Show only the "You" node floating in space
- **Single agent**: Two nodes, one link, particles flowing
- **Agent disconnects**: Node dims, edges fade but remain (historical data persists in diary)
- **Window resize**: `react-force-graph-3d` handles resize via `width`/`height` props — bind to container size with a `ResizeObserver` or by passing `width={containerWidth}` `height={containerHeight}`
- **Light mode**: Adjust background to light (`#faf6f0`), node materials to non-emissive with solid fill, edge colors to dark (`#d4c9b8`). Less sci-fi, more clinical. (Low priority — dark mode is primary)
