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

/** Describes a single email event for particle emission. */
export interface EmailEvent {
  from: string;  // node id
  to: string;    // node id
  color: string; // sender color
}

export function useNetwork(
  agents: AgentInfo[],
  entries: DiaryEvent[],
  sentMessages: SentMessage[],
  userPort: number
) {
  const lastSeenTsRef = useRef(0);
  const activityRef = useRef<Map<string, NodeActivity[]>>(new Map());
  const [nodeActivity, setNodeActivity] = useState<Map<string, NodeActivity[]>>(new Map());
  const [pendingEmails, setPendingEmails] = useState<EmailEvent[]>([]);

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

  // Node color map for particle coloring
  const nodeColorMap: Record<string, string> = {};
  for (const n of nodes) nodeColorMap[n.id] = n.color;

  // Build undirected edges from diary entries (one line per pair)
  const edgeMap = new Map<string, number>();
  const addEdge = (from: string, to: string) => {
    const key = [from, to].sort().join("--");
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
    const [source, target] = key.split("--");
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

  // Track node activity + emit email particles for new events
  useEffect(() => {
    const newEvents = entries.filter(
      (e) => e.time > lastSeenTsRef.current
    );

    if (newEvents.length === 0) return;
    lastSeenTsRef.current = Math.max(...newEvents.map((e) => e.time));

    const now = Date.now();
    const activity = activityRef.current;
    const newEmails: EmailEvent[] = [];

    for (const e of newEvents) {
      // Activity tracking
      let activityType: NodeActivity["type"] | null = null;
      if (e.type === "thinking") activityType = "thinking";
      else if (e.type === "tool_call") activityType = "tool";
      else if (e.type === "diary") activityType = "diary";

      if (activityType && e.agent_key) {
        const nodeEvents = activity.get(e.agent_key) || [];
        nodeEvents.push({ type: activityType, time: now });
        if (nodeEvents.length > ACTIVITY_MAX_EVENTS) {
          nodeEvents.splice(0, nodeEvents.length - ACTIVITY_MAX_EVENTS);
        }
        activity.set(e.agent_key, nodeEvents);
      }

      // Email particle emission (only for email_out to avoid duplicates)
      if (e.type === "email_out" && e.to) {
        const fromId = e.agent_key;
        for (const addr of e.to.split(", ")) {
          const toId = addrToNodeId[addr.trim()];
          if (toId && toId !== fromId) {
            newEmails.push({
              from: fromId,
              to: toId,
              color: nodeColorMap[fromId] || "#e94560",
            });
          }
        }
      }
    }

    // Prune old activity entries
    const cutoff = now - ACTIVITY_MAX_AGE_MS;
    for (const [, events] of activity) {
      const fresh = events.filter((ev) => ev.time > cutoff);
      if (fresh.length === 0) {
        activity.delete(events === activity.get("") ? "" : "");
      }
    }
    for (const [nodeId, events] of activity) {
      const fresh = events.filter((ev) => ev.time > cutoff);
      if (fresh.length === 0) {
        activity.delete(nodeId);
      } else {
        activity.set(nodeId, fresh);
      }
    }

    setNodeActivity(new Map(activity));
    if (newEmails.length > 0) {
      setPendingEmails(newEmails);
    }
  }, [entries]);

  // Memoize graphData — only change when agent set or link topology changes.
  // Use a stable key based on node IDs + link keys (not counts) to avoid
  // re-rendering the whole graph when email counts increment.
  const nodeKey = agents.map((a) => a.key).sort().join(",");
  const linkKey = links.map((l) => {
    const src = typeof l.source === "string" ? l.source : l.source.id;
    const tgt = typeof l.target === "string" ? l.target : l.target.id;
    return `${src}--${tgt}`;
  }).sort().join(",");

  const graphData = useMemo(
    () => ({ nodes, links }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [nodeKey, linkKey]
  );

  return { graphData, nodeActivity, pendingEmails };
}
