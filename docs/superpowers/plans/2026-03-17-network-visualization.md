# Network Visualization Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a force-directed network graph page to the web dashboard showing agents as draggable nodes with animated email particles along edges.

**Architecture:** New React page component with D3 force simulation. Minor backend change to expose agent `type` field. No new API endpoints — network data derived from existing diary events client-side.

**Tech Stack:** D3.js (d3-force, d3-selection, d3-drag), React 18, TypeScript, SVG

**Spec:** `docs/superpowers/specs/2026-03-17-network-visualization-design.md`

---

## File Structure

| Action | Path | Responsibility |
|--------|------|---------------|
| Modify | `app/web/server/routes.py` | Add `type` field to `/api/agents` response |
| Modify | `app/web/frontend/src/types.ts` | Add `type` to `AgentInfo`, add network types |
| Modify | `app/web/frontend/src/components/Header.tsx` | Add page tabs |
| Modify | `app/web/frontend/src/App.tsx` | Page switching logic |
| Create | `app/web/frontend/src/hooks/useNetwork.ts` | Derive edges + particles from diary |
| Create | `app/web/frontend/src/components/NetworkPage.tsx` | D3 force graph with SVG rendering |

---

## Chunk 1: Backend — Agent Type

### Task 1: Add `type` field to agents endpoint

**Files:**
- Modify: `app/web/server/routes.py`

- [ ] **Step 1: Add `type` to agents response**

In `routes.py`, modify the `list_agents` function to include the agent type:

```python
@router.get("/agents")
def list_agents(request: Request):
    state = _get_state(request)
    agents = []
    for entry in state.agents.values():
        agents.append({
            "id": entry.agent_id,
            "name": entry.name,
            "key": entry.key,
            "address": entry.address,
            "port": entry.port,
            "status": entry.agent.state.value,
            "type": "admin" if entry.agent._admin else "agent",
        })
    return agents
```

- [ ] **Step 2: Smoke-test**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai && source venv/bin/activate
python -c "from app.web.server.routes import router; print('OK')"
```

- [ ] **Step 3: Commit**

```bash
git add app/web/server/routes.py
git commit -m "feat(web): add type field to agents API response"
```

---

## Chunk 2: Frontend — Types & Dependencies

### Task 2: Install D3 and update types

**Files:**
- Modify: `app/web/frontend/src/types.ts`

- [ ] **Step 1: Install D3**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai/app/web/frontend
npm install d3
npm install -D @types/d3
```

- [ ] **Step 2: Update `AgentInfo` and add network types in `types.ts`**

Add `type` to `AgentInfo`:

```typescript
export interface AgentInfo {
  id: string;
  name: string;
  key: string;
  address: string;
  port: number;
  status: "active" | "sleeping";
  type: "admin" | "agent";
}
```

Add network types at the end of the file:

```typescript
/** Network visualization types */

export interface NetworkNode {
  id: string;
  name: string;
  color: string;
  status: "active" | "sleeping";
  type: "admin" | "agent" | "user";
  x?: number;
  y?: number;
  vx?: number;
  vy?: number;
  fx?: number | null;
  fy?: number | null;
}

export interface NetworkEdge {
  source: string | NetworkNode;
  target: string | NetworkNode;
  count: number;
}

export interface Particle {
  id: string;
  source: string;
  target: string;
  color: string;
  startTime: number;
  duration: number;
}
```

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai
git add app/web/frontend/src/types.ts app/web/frontend/package.json app/web/frontend/package-lock.json
git commit -m "feat(web): install D3, add network types and agent type field"
```

---

## Chunk 3: Network Hook

### Task 3: Create useNetwork hook

**Files:**
- Create: `app/web/frontend/src/hooks/useNetwork.ts`

- [ ] **Step 1: Write `useNetwork.ts`**

```typescript
import { useCallback, useEffect, useRef, useState } from "react";
import type {
  AgentInfo,
  DiaryEvent,
  NetworkNode,
  NetworkEdge,
  Particle,
  SentMessage,
} from "../types";
import { AGENT_COLORS } from "../types";

export function useNetwork(
  agents: AgentInfo[],
  entries: DiaryEvent[],
  sentMessages: SentMessage[],
  addressToName: Record<string, string>,
  userPort: number
) {
  const [particles, setParticles] = useState<Particle[]>([]);
  const lastSeenTsRef = useRef(0);
  const animFrameRef = useRef<number>(0);

  // Build node list: agents + user
  const nodes: NetworkNode[] = agents.map((a, i) => ({
    id: a.key,
    name: a.name,
    color: AGENT_COLORS[i % AGENT_COLORS.length],
    status: a.status,
    type: a.type,
  }));
  nodes.push({
    id: "user",
    name: "You",
    color: "#e94560",
    status: "active" as const,
    type: "user",
  });

  // Build address → node id map
  const addrToNodeId: Record<string, string> = {};
  for (const a of agents) {
    addrToNodeId[a.address] = a.key;
  }
  addrToNodeId[`127.0.0.1:${userPort}`] = "user";

  // Build edges from diary entries
  const edgeMap = new Map<string, number>();
  const addEdge = (from: string, to: string) => {
    const key = [from, to].sort().join("-");
    edgeMap.set(key, (edgeMap.get(key) || 0) + 1);
  };

  for (const e of entries) {
    if (e.type === "email_out" && e.to) {
      const fromId = e.agent_key;
      // e.to might be an address or comma-separated addresses
      for (const addr of e.to.split(", ")) {
        const toId = addrToNodeId[addr.trim()];
        if (toId && toId !== fromId) addEdge(fromId, toId);
      }
    }
    if (e.type === "email_in" && e.from) {
      const toId = e.agent_key;
      const fromId = addrToNodeId[e.from];
      if (fromId && fromId !== toId) addEdge(fromId, toId);
    }
  }

  // Also count sent messages from user
  for (const s of sentMessages) {
    addEdge("user", s.to);
  }

  const edges: NetworkEdge[] = [];
  for (const [key, count] of edgeMap) {
    const [source, target] = key.split("-");
    edges.push({ source, target, count });
  }

  // Spawn particles for new email events
  useEffect(() => {
    const newEvents = entries.filter(
      (e) =>
        e.time > lastSeenTsRef.current &&
        (e.type === "email_out" || e.type === "email_in")
    );

    if (newEvents.length > 0) {
      lastSeenTsRef.current = Math.max(...newEvents.map((e) => e.time));

      const newParticles: Particle[] = [];
      for (const e of newEvents) {
        if (e.type === "email_out" && e.to) {
          const fromId = e.agent_key;
          const agentIdx = agents.findIndex((a) => a.key === fromId);
          const color =
            AGENT_COLORS[agentIdx % AGENT_COLORS.length] || "#e94560";
          for (const addr of e.to.split(", ")) {
            const toId = addrToNodeId[addr.trim()];
            if (toId && toId !== fromId) {
              newParticles.push({
                id: `${e.time}-${fromId}-${toId}-${Math.random()}`,
                source: fromId,
                target: toId,
                color,
                startTime: performance.now(),
                duration: 1500,
              });
            }
          }
        }
      }

      if (newParticles.length > 0) {
        setParticles((prev) => [...prev, ...newParticles]);
      }
    }
  }, [entries, agents, addrToNodeId]);

  // Animation loop — remove expired particles
  useEffect(() => {
    const tick = () => {
      const now = performance.now();
      setParticles((prev) => {
        const alive = prev.filter(
          (p) => now - p.startTime < p.duration
        );
        return alive.length !== prev.length ? alive : prev;
      });
      animFrameRef.current = requestAnimationFrame(tick);
    };
    animFrameRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(animFrameRef.current);
  }, []);

  return { nodes, edges, particles };
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai
git add app/web/frontend/src/hooks/useNetwork.ts
git commit -m "feat(web): add useNetwork hook for graph data and particles"
```

---

## Chunk 4: Network Page Component

### Task 4: Create NetworkPage with D3 force simulation

**Files:**
- Create: `app/web/frontend/src/components/NetworkPage.tsx`

- [ ] **Step 1: Write `NetworkPage.tsx`**

```tsx
import { useEffect, useRef } from "react";
import * as d3 from "d3";
import type { NetworkNode, NetworkEdge, Particle } from "../types";

interface NetworkPageProps {
  nodes: NetworkNode[];
  edges: NetworkEdge[];
  particles: Particle[];
}

export function NetworkPage({ nodes, edges, particles }: NetworkPageProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const simRef = useRef<d3.Simulation<NetworkNode, NetworkEdge> | null>(null);
  const nodesRef = useRef<NetworkNode[]>([]);
  const edgesRef = useRef<NetworkEdge[]>([]);

  // Initialize and update D3 simulation
  useEffect(() => {
    const svg = d3.select(svgRef.current);
    const width = svgRef.current?.clientWidth || 800;
    const height = svgRef.current?.clientHeight || 600;

    // Copy nodes to preserve D3 positions across re-renders
    const nodeMap = new Map(nodesRef.current.map((n) => [n.id, n]));
    const simNodes: NetworkNode[] = nodes.map((n) => {
      const existing = nodeMap.get(n.id);
      return existing
        ? { ...n, x: existing.x, y: existing.y, vx: existing.vx, vy: existing.vy, fx: existing.fx, fy: existing.fy }
        : { ...n };
    });
    nodesRef.current = simNodes;

    // Resolve edges to node references
    const simEdges: NetworkEdge[] = edges.map((e) => ({
      source: simNodes.find((n) => n.id === (typeof e.source === "string" ? e.source : e.source.id)) || e.source,
      target: simNodes.find((n) => n.id === (typeof e.target === "string" ? e.target : e.target.id)) || e.target,
      count: e.count,
    }));
    edgesRef.current = simEdges;

    // Clear previous
    svg.selectAll("*").remove();

    // Defs — glow filter
    const defs = svg.append("defs");
    const filter = defs.append("filter").attr("id", "particle-glow");
    filter
      .append("feGaussianBlur")
      .attr("stdDeviation", "3")
      .attr("result", "blur");
    const merge = filter.append("feMerge");
    merge.append("feMergeNode").attr("in", "blur");
    merge.append("feMergeNode").attr("in", "SourceGraphic");

    // Edge group
    const edgeGroup = svg.append("g").attr("class", "edges");
    // Node group (on top of edges)
    const nodeGroup = svg.append("g").attr("class", "nodes");
    // Particle group (on top of everything)
    const particleGroup = svg.append("g").attr("class", "particles");

    // Draw edges
    const edgeLines = edgeGroup
      .selectAll("line")
      .data(simEdges)
      .join("line")
      .attr("stroke", "#0f3460")
      .attr("stroke-width", (d: NetworkEdge) =>
        Math.min(1 + d.count * 0.5, 4)
      );

    // Draw nodes
    const nodeGs = nodeGroup
      .selectAll("g")
      .data(simNodes)
      .join("g")
      .attr("cursor", "grab")
      .call(
        d3
          .drag<SVGGElement, NetworkNode>()
          .on("start", (event, d) => {
            if (!event.active) simRef.current?.alphaTarget(0.3).restart();
            d.fx = d.x;
            d.fy = d.y;
          })
          .on("drag", (event, d) => {
            d.fx = event.x;
            d.fy = event.y;
          })
          .on("end", (event, d) => {
            if (!event.active) simRef.current?.alphaTarget(0);
            d.fx = null;
            d.fy = null;
          })
      );

    // Admin nodes: outer ring
    nodeGs
      .filter((d: NetworkNode) => d.type === "admin")
      .append("circle")
      .attr("r", 28)
      .attr("fill", "none")
      .attr("stroke", (d: NetworkNode) => d.color)
      .attr("stroke-width", 1.5)
      .attr("stroke-dasharray", "4 2");

    // Main circle
    nodeGs
      .append("circle")
      .attr("r", 22)
      .attr("fill", "#16213e")
      .attr("stroke", (d: NetworkNode) => d.color)
      .attr("stroke-width", 2);

    // Name label
    nodeGs
      .append("text")
      .text((d: NetworkNode) => d.name)
      .attr("text-anchor", "middle")
      .attr("dy", "0.35em")
      .attr("fill", (d: NetworkNode) => d.color)
      .attr("font-size", "10px")
      .attr("font-weight", "bold")
      .attr("pointer-events", "none");

    // Status dot
    nodeGs
      .filter((d: NetworkNode) => d.type !== "user")
      .append("circle")
      .attr("cx", 16)
      .attr("cy", -16)
      .attr("r", 4)
      .attr("fill", (d: NetworkNode) =>
        d.status === "active" ? "#4ecdc4" : "#666"
      );

    // Force simulation
    const simulation = d3
      .forceSimulation<NetworkNode>(simNodes)
      .force(
        "link",
        d3
          .forceLink<NetworkNode, NetworkEdge>(simEdges)
          .id((d) => d.id)
          .distance(120)
      )
      .force("charge", d3.forceManyBody().strength(-300))
      .force("center", d3.forceCenter(width / 2, height / 2))
      .force("collide", d3.forceCollide(40))
      .on("tick", () => {
        edgeLines
          .attr("x1", (d: any) => d.source.x)
          .attr("y1", (d: any) => d.source.y)
          .attr("x2", (d: any) => d.target.x)
          .attr("y2", (d: any) => d.target.y);

        nodeGs.attr("transform", (d: NetworkNode) => `translate(${d.x},${d.y})`);
      });

    simRef.current = simulation;

    return () => {
      simulation.stop();
    };
  }, [nodes, edges]);

  // Render particles (separate effect — runs on every frame via React state)
  useEffect(() => {
    if (!svgRef.current) return;
    const svg = d3.select(svgRef.current);
    const particleGroup = svg.select("g.particles");
    if (particleGroup.empty()) return;

    const now = performance.now();

    particleGroup.selectAll("circle").remove();

    for (const p of particles) {
      const progress = (now - p.startTime) / p.duration;
      if (progress < 0 || progress > 1) continue;

      const sourceNode = nodesRef.current.find((n) => n.id === p.source);
      const targetNode = nodesRef.current.find((n) => n.id === p.target);
      if (!sourceNode?.x || !targetNode?.x) continue;

      const x = sourceNode.x + (targetNode.x - sourceNode.x) * progress;
      const y = sourceNode.y! + (targetNode.y! - sourceNode.y!) * progress;
      const opacity = progress < 0.8 ? 1 : 1 - (progress - 0.8) / 0.2;

      particleGroup
        .append("circle")
        .attr("cx", x)
        .attr("cy", y)
        .attr("r", 5)
        .attr("fill", p.color)
        .attr("opacity", opacity)
        .attr("filter", "url(#particle-glow)");
    }
  }, [particles]);

  return (
    <div className="flex-1 bg-bg flex items-center justify-center overflow-hidden">
      <svg
        ref={svgRef}
        className="w-full h-full"
        style={{ minHeight: "100%" }}
      />
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai
git add app/web/frontend/src/components/NetworkPage.tsx
git commit -m "feat(web): add NetworkPage with D3 force graph"
```

---

## Chunk 5: Wire Up — Header Tabs & App

### Task 5: Add page tabs to Header

**Files:**
- Modify: `app/web/frontend/src/components/Header.tsx`

- [ ] **Step 1: Update `Header.tsx`**

```tsx
import type { AgentInfo } from "../types";

interface HeaderProps {
  agents: AgentInfo[];
  userPort: number;
  activePage: "inbox" | "network";
  onPageChange: (page: "inbox" | "network") => void;
}

export function Header({
  agents,
  userPort,
  activePage,
  onPageChange,
}: HeaderProps) {
  const activeCount = agents.filter((a) => a.status === "active").length;
  return (
    <div className="flex items-center gap-3 px-5 py-2.5 bg-panel border-b border-border">
      <h1 className="text-base font-bold text-accent">StoAI</h1>
      <span className="text-xs text-text-dim">
        {agents.length} agent{agents.length !== 1 ? "s" : ""} · User
        mailbox :{userPort}
      </span>
      <div className="flex gap-0 ml-4">
        <button
          onClick={() => onPageChange("inbox")}
          className={`px-3 py-1 text-xs uppercase tracking-widest border-b-2 cursor-pointer bg-transparent ${
            activePage === "inbox"
              ? "text-accent border-accent"
              : "text-text-dim border-transparent hover:text-text"
          }`}
        >
          Inbox
        </button>
        <button
          onClick={() => onPageChange("network")}
          className={`px-3 py-1 text-xs uppercase tracking-widest border-b-2 cursor-pointer bg-transparent ${
            activePage === "network"
              ? "text-accent border-accent"
              : "text-text-dim border-transparent hover:text-text"
          }`}
        >
          Network
        </button>
      </div>
      {activeCount > 0 && (
        <span className="text-xs text-emerald-400 ml-auto">
          ● {activeCount} active
        </span>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add app/web/frontend/src/components/Header.tsx
git commit -m "feat(web): add page tabs to Header"
```

---

### Task 6: Update App.tsx with page switching and network

**Files:**
- Modify: `app/web/frontend/src/App.tsx`

- [ ] **Step 1: Replace `App.tsx`**

```tsx
import { useState } from "react";
import { useAgents } from "./hooks/useAgents";
import { useInbox } from "./hooks/useInbox";
import { useDiary } from "./hooks/useDiary";
import { useNetwork } from "./hooks/useNetwork";
import { Header } from "./components/Header";
import { InboxPanel } from "./components/InboxPanel";
import { DiaryPanel } from "./components/DiaryPanel";
import { NetworkPage } from "./components/NetworkPage";

const USER_PORT = 8300;

export default function App() {
  const [activePage, setActivePage] = useState<"inbox" | "network">("inbox");
  const { agents, keyToName, addressToName } = useAgents();
  const { receivedEmails, sentMessages, addSent } = useInbox();
  const entries = useDiary(agents);
  const { nodes, edges, particles } = useNetwork(
    agents,
    entries,
    sentMessages,
    addressToName,
    USER_PORT
  );

  return (
    <div className="h-screen flex flex-col bg-bg text-text font-sans">
      <Header
        agents={agents}
        userPort={USER_PORT}
        activePage={activePage}
        onPageChange={setActivePage}
      />
      <div className="flex-1 flex overflow-hidden">
        {activePage === "inbox" ? (
          <>
            <InboxPanel
              agents={agents}
              keyToName={keyToName}
              addressToName={addressToName}
              receivedEmails={receivedEmails}
              sentMessages={sentMessages}
              onSent={addSent}
            />
            <DiaryPanel
              agents={agents}
              entries={entries}
              addressToName={addressToName}
            />
          </>
        ) : (
          <NetworkPage nodes={nodes} edges={edges} particles={particles} />
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Build and verify**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai/app/web/frontend
npm run build
```

Expected: build succeeds with no TypeScript errors.

- [ ] **Step 3: Commit**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai
git add app/web/frontend/src/App.tsx
git commit -m "feat(web): wire up network page with page switching"
```

---

## Chunk 6: Smoke Test

### Task 7: End-to-end verification

- [ ] **Step 1: Build frontend**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai/app/web/frontend
npm run build
```

- [ ] **Step 2: Start server**

```bash
cd /Users/huangzesen/Documents/GitHub/stoai
python -m app.web
```

- [ ] **Step 3: Verify**

- Open http://localhost:8080
- Header should show "Inbox" and "Network" tabs
- Click "Network" — should show force-directed graph with 3 agent nodes + 1 user node
- Nodes should be draggable
- Admin nodes (if any) should have double-ring
- Send a message from inbox → switch to network → should see particle animation
- `curl http://localhost:8080/api/agents` should include `"type": "agent"` on each entry
