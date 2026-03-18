# Implementation Plan: 3D Network Page

**Spec:** `docs/superpowers/specs/2026-03-17-3d-network-page-design.md`

## Prerequisites

Read the spec file first. All design decisions are documented there.

## Steps

### Step 1: Install dependencies

```bash
cd app/web/frontend
npm install react-force-graph-3d three three-spritetext
```

Verify the packages installed correctly and the dev server still starts (`npm run dev`).

### Step 2: Update `types.ts`

1. **Remove**: `Particle`, `Pulse`, `PulseType`, `PULSE_STYLES` types/constants
2. **Add** new types:

```ts
import type { NodeObject, LinkObject } from 'react-force-graph-3d';

export interface GraphNode extends NodeObject {
  id: string;
  name: string;
  color: string;
  status: "active" | "sleeping";
  type: "admin" | "agent" | "user";
}

export interface GraphLink extends LinkObject {
  source: string;
  target: string;
  count: number;
}

export interface NodeActivity {
  type: "thinking" | "tool" | "diary";
  time: number;
}
```

3. **Keep**: `NetworkNode`, `NetworkEdge` can be removed since they're replaced by `GraphNode`/`GraphLink`. Check if anything else imports them first.

**Smoke test**: `npm run dev` should still compile (there will be import errors in files that reference removed types — that's expected, we fix them in subsequent steps).

### Step 3: Rewrite `useNetwork.ts`

Replace the hook to output the new format. Key changes:

1. **Output type** changes to `{ graphData: { nodes: GraphNode[], links: GraphLink[] }, nodeActivity: Map<string, NodeActivity[]> }`
1b. After building links, compute per-node email volume and attach as `_volume`:
   ```ts
   const volumeMap: Record<string, number> = {};
   for (const link of graphLinks) {
     const src = typeof link.source === 'string' ? link.source : link.source.id;
     const tgt = typeof link.target === 'string' ? link.target : link.target.id;
     volumeMap[src] = (volumeMap[src] || 0) + link.count;
     volumeMap[tgt] = (volumeMap[tgt] || 0) + link.count;
   }
   for (const node of graphNodes) {
     node._volume = volumeMap[node.id] || 0;
   }
   ```
2. **Directed edges**: Remove the `[from, to].sort().join("-")` pattern. Instead use `${from}->${to}` as the key (directed). This means Alice→Bob and Bob→Alice are separate links.
3. **Remove** all `Particle`/`Pulse` state, `useState`, `setTimeout` cleanup
4. **Add** `nodeActivity` tracking: maintain a `Map<string, NodeActivity[]>` that tracks last 5 events per node. Only store events where `type` is `thinking`, `tool_call`, or `diary`. Map `tool_call` → `"tool"` in the activity type. Prune entries older than 10 seconds.
5. **Keep** the `sentMessages` contribution — user→agent directed edges
6. **Keep** the node-building logic (agents + "You" user node)

The `nodeActivity` should use a ref + state pattern: accumulate in a ref, update state periodically or on new entries.

**Smoke test**: Import the hook somewhere, verify it returns the correct shape. The NetworkPage won't work yet — that's next.

### Step 4: Rewrite `NetworkPage.tsx` — Basic 3D graph

Full rewrite. Start with a minimal working version:

```tsx
import ForceGraph3D from 'react-force-graph-3d';
import type { GraphNode, GraphLink } from '../types';

interface NetworkPageProps {
  graphData: { nodes: GraphNode[]; links: GraphLink[] };
  nodeActivity: Map<string, NodeActivity[]>;
  lightMode: boolean;
}

export function NetworkPage({ graphData, nodeActivity, lightMode }: NetworkPageProps) {
  return (
    <div className="flex-1 bg-bg overflow-hidden" style={{ position: 'relative' }}>
      <ForceGraph3D
        graphData={graphData}
        backgroundColor={lightMode ? '#faf6f0' : '#0a0a1a'}
        nodeId="id"
        nodeLabel={(node: GraphNode) => `${node.name} (${node.status}) [${node.type}]`}
        nodeColor={(node: GraphNode) => node.color}
        linkSource="source"
        linkTarget="target"
        linkColor={() => '#1a3a5c'}
        linkWidth={(link: GraphLink) => Math.min(1 + link.count * 0.3, 4)}
        linkCurvature={0.15}
        linkDirectionalParticles={(link: GraphLink) => link.count > 0 ? 2 : 0}
        linkDirectionalParticleWidth={3}
        linkDirectionalParticleSpeed={0.015}
        linkDirectionalArrowLength={3.5}
        linkDirectionalArrowRelPos={1}
      />
    </div>
  );
}
```

**Verify**: The graph renders, nodes are visible, links connect them, particles flow, you can orbit/zoom/pan.

### Step 5: Custom `nodeThreeObject` — Sci-fi nodes

Add custom Three.js node rendering:

1. Import `THREE` from `three` and `SpriteText` from `three-spritetext`
2. Create `nodeThreeObject` callback that returns a Three.js Group containing:
   - A sphere (`SphereGeometry`, radius ~5) with `MeshStandardMaterial` using the node's color as `emissive`
   - For admin nodes: an outer wireframe `IcosahedronGeometry` (radius ~8, wireframe material, low opacity)
   - For user node: `OctahedronGeometry` instead of sphere
   - A `SpriteText` label above the node (node name, small font size, white or node color)
   - Active nodes: full emissive intensity. Sleeping nodes: reduced emissive intensity (0.2)
3. Set `nodeThreeObjectExtend={false}` to replace default spheres entirely

**Verify**: Nodes render as glowing spheres, admin nodes have the wireframe shell, user node is an octahedron, labels are visible and face the camera.

### Step 6: Node activity glow animation

Add animated glow based on `nodeActivity`:

1. In the `nodeThreeObject` callback, store a reference to the mesh material
2. Use `useRef` to track materials by node ID
3. In a `useEffect` + `requestAnimationFrame` loop:
   - For each node with recent activity (within last 3-5 seconds), increase emissive intensity
   - Decay intensity over time (lerp toward base value)
   - Color the emissive based on activity type: thinking=yellow, tool=blue, diary=green
4. Define `GLOW_COLORS` locally in NetworkPage:
   ```ts
   const GLOW_COLORS = { thinking: '#f0a500', tool: '#6b9bcb', diary: '#6bcb77' };
   ```

**Verify**: When agents are thinking/calling tools/writing diary, their nodes glow with the appropriate color.

### Step 7: View toggle (Communication vs Activity)

Add floating control buttons over the 3D canvas:

1. Add `useState<'comm' | 'activity'>('comm')` for view mode
2. Render control buttons absolutely positioned top-right over the canvas, in two rows:
   - Row 1 (View): Communication | Activity
   - Row 2 (Layout): Default | Email Volume | Contact Cluster
3. In Communication mode: normal link opacity, particles visible
4. In Activity mode: `linkOpacity={0.05}`, `linkDirectionalParticles={0}`, enhanced node glow
5. Pass mode to the glow animation loop to intensify glow in activity mode

**Verify**: View toggle switches between modes. Camera position preserved across toggles.

### Step 7b: Layout toggle (Default vs Email Volume vs Contact Cluster)

Add layout reorganization via d3-force manipulation:

1. Add `useState<'default' | 'volume' | 'cluster'>('default')` for layout mode
2. Store a `useRef` to the ForceGraph3D instance (via `ref` prop)
3. Precompute per-node email volume in `useNetwork` hook — add `_volume: number` to each `GraphNode` (sum of all link counts where node is source or target)
4. Implement `setLayout` function that swaps d3 forces at runtime:

   **Email Volume mode:**
   ```ts
   graphRef.current.d3Force('radial', d3.forceRadial(
     (node: GraphNode) => maxRadius * (1 - (node._volume || 0) / maxVolume)
   ).strength(0.4));
   graphRef.current.d3Force('charge').strength(-30);
   graphRef.current.d3ReheatSimulation();
   ```

   **Contact Cluster mode:**
   ```ts
   graphRef.current.d3Force('radial', null);
   graphRef.current.d3Force('link').strength(
     (link: GraphLink) => 0.3 + 0.7 * (link.count / maxCount)
   );
   graphRef.current.d3Force('charge').strength(-80);
   graphRef.current.d3ReheatSimulation();
   ```

   **Default mode:**
   ```ts
   graphRef.current.d3Force('radial', null);
   graphRef.current.d3Force('charge').strength(-30);
   graphRef.current.d3Force('link').strength(null);
   graphRef.current.d3ReheatSimulation();
   ```

5. Wire layout buttons to `setLayout` calls

**Verify**: Click each layout button. Nodes smoothly animate into the new arrangement. Email Volume pulls high-traffic nodes to center. Contact Cluster groups frequent communicators. Default restores balanced spacing.

### Step 8: Update `App.tsx`

1. Change `useNetwork` destructuring: `const { graphData, nodeActivity } = useNetwork(...)`
2. Update NetworkPage props: `<NetworkPage graphData={graphData} nodeActivity={nodeActivity} lightMode={lightMode} />`
3. Remove any imports of `Particle`, `Pulse`, etc. that are no longer needed

**Verify**: Full app compiles and works end-to-end. Network page shows 3D graph. Inbox page unaffected.

### Step 9: Handle container sizing

Ensure the 3D graph fills its container correctly:

1. Use a `useRef` on the container div + `ResizeObserver` to track `width` and `height`
2. Pass `width={width}` `height={height}` to `ForceGraph3D`
3. Or let the library auto-detect from its parent (test this first — may work without manual sizing)

**Verify**: Resize the browser window. The 3D graph adjusts correctly.

### Step 10: Final polish

1. Tune force simulation params if needed (`d3AlphaDecay`, `d3VelocityDecay`, charge strength)
2. Adjust particle speed, link curvature, node sizes for visual balance
3. Test with 0, 1, 5, 10+ agents
4. Test light mode (lower priority — basic color swap)
5. Remove unused D3 dependency from `package.json` if no other component uses it

**Verify**: Run the full app. Network page is functional, visually coherent, and handles all edge cases.
