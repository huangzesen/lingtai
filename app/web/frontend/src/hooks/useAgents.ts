import { useEffect, useState } from "react";
import type { AgentInfo } from "../types";

const POLL_MS = 3000;

export function useAgents() {
  const [agents, setAgents] = useState<AgentInfo[]>([]);

  useEffect(() => {
    const poll = async () => {
      try {
        const resp = await fetch("/api/agents");
        const data: AgentInfo[] = await resp.json();
        setAgents((prev) => {
          // Only update if agent list actually changed
          if (JSON.stringify(prev) === JSON.stringify(data)) return prev;
          return data;
        });
      } catch {
        /* ignore */
      }
    };
    poll();
    const id = setInterval(poll, POLL_MS);
    return () => clearInterval(id);
  }, []);

  const keyToName: Record<string, string> = {};
  const addressToName: Record<string, string> = {};
  for (const a of agents) {
    keyToName[a.key] = a.name;
    addressToName[a.address] = a.name;
  }

  return { agents, keyToName, addressToName };
}
