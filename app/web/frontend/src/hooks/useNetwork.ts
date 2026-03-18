import { useEffect, useMemo, useRef, useState } from "react";
import type {
  AgentInfo,
  DiaryEvent,
  GraphNode,
  GraphLink,
  NodeActivity,
  SentMessage,
} from "../types";
import { AGENT_COLORS } from "../types";

const ACTIVITY_MAX_EVENTS = 5;
const ACTIVITY_MAX_AGE_MS = 10_000;

export function useNetwork(
  agents: AgentInfo[],
  entries: DiaryEvent[],
  sentMessages: SentMessage[],
  userPort: number
) {
  const lastSeenTsRef = useRef(0);
  const activityRef = useRef<Map<string, NodeActivity[]>>(new Map());
  const [nodeActivity, setNodeActivity] = useState<Map<string, NodeActivity[]>>(new Map());

  // Build node list: agents + user
  const nodes: GraphNode[] = agents.map((a, i) => ({
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

  // Build directed edges from diary entries
  const edgeMap = new Map<string, number>();
  const addEdge = (from: string, to: string) => {
    const key = `${from}->${to}`;
    edgeMap.set(key, (edgeMap.get(key) || 0) + 1);
  };

  for (const e of entries) {
    if (e.type === "email_out" && e.to) {
      const fromId = e.agent_key;
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

  for (const s of sentMessages) {
    addEdge("user", s.to);
  }

  const links: GraphLink[] = [];
  for (const [key, count] of edgeMap) {
    const [source, target] = key.split("->");
    links.push({ source, target, count });
  }

  // Compute per-node email volume for layout modes
  const volumeMap: Record<string, number> = {};
  for (const link of links) {
    const src = typeof link.source === "string" ? link.source : link.source.id;
    const tgt = typeof link.target === "string" ? link.target : link.target.id;
    volumeMap[src] = (volumeMap[src] || 0) + link.count;
    volumeMap[tgt] = (volumeMap[tgt] || 0) + link.count;
  }
  for (const node of nodes) {
    node._volume = volumeMap[node.id] || 0;
  }

  // Track node activity for glow animation
  useEffect(() => {
    const newEvents = entries.filter(
      (e) => e.time > lastSeenTsRef.current
    );

    if (newEvents.length === 0) return;
    lastSeenTsRef.current = Math.max(...newEvents.map((e) => e.time));

    const now = Date.now();
    const activity = activityRef.current;

    for (const e of newEvents) {
      let activityType: NodeActivity["type"] | null = null;
      if (e.type === "thinking") activityType = "thinking";
      else if (e.type === "tool_call") activityType = "tool";
      else if (e.type === "diary") activityType = "diary";

      if (activityType && e.agent_key) {
        const nodeEvents = activity.get(e.agent_key) || [];
        nodeEvents.push({ type: activityType, time: now });
        // Keep only last N events
        if (nodeEvents.length > ACTIVITY_MAX_EVENTS) {
          nodeEvents.splice(0, nodeEvents.length - ACTIVITY_MAX_EVENTS);
        }
        activity.set(e.agent_key, nodeEvents);
      }
    }

    // Prune old entries
    const cutoff = now - ACTIVITY_MAX_AGE_MS;
    for (const [nodeId, events] of activity) {
      const fresh = events.filter((ev) => ev.time > cutoff);
      if (fresh.length === 0) {
        activity.delete(nodeId);
      } else {
        activity.set(nodeId, fresh);
      }
    }

    // Update state with a new Map (trigger re-render)
    setNodeActivity(new Map(activity));
  }, [entries]);

  // Memoize graphData to avoid reinitializing the force simulation on every render.
  // react-force-graph-3d tears down and recreates the simulation when graphData identity changes.
  const graphData = useMemo(
    () => ({ nodes, links }),
    // Stable key: agent count + link count + total message volume
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [agents.length, links.length, links.reduce((s, l) => s + l.count, 0)]
  );

  return { graphData, nodeActivity };
}
