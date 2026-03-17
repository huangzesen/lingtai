import { useEffect, useRef, useState } from "react";
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
