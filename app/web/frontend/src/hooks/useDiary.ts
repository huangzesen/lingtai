import { useEffect, useRef, useState } from "react";
import type { AgentInfo, DiaryEvent } from "../types";

const POLL_MS = 1500;

export function useDiary(agents: AgentInfo[]) {
  const [entries, setEntries] = useState<DiaryEvent[]>([]);
  const sinceRef = useRef(0);
  const agentsRef = useRef(agents);
  agentsRef.current = agents;

  // Stable effect — only starts once, reads agents from ref
  useEffect(() => {
    const poll = async () => {
      const currentAgents = agentsRef.current;
      if (currentAgents.length === 0) return;

      try {
        const since = sinceRef.current;
        const resp = await fetch(`/api/diary?since=${since}`);
        const data = await resp.json();

        const allNew: DiaryEvent[] = [];
        for (const [key, agentEntries] of Object.entries(data)) {
          const agent = currentAgents.find((a) => a.key === key);
          const name = agent?.name || key;
          for (const e of agentEntries as DiaryEvent[]) {
            allNew.push({
              ...e,
              agent_key: key,
              agent_name: name,
            });
          }
        }

        if (allNew.length > 0) {
          const maxTs = Math.max(...allNew.map((e) => e.time));
          sinceRef.current = maxTs;
          setEntries((prev) => {
            const combined = [...prev, ...allNew];
            combined.sort((a, b) => a.time - b.time);
            return combined;
          });
        }
      } catch {
        /* ignore */
      }
    };

    const id = setInterval(poll, POLL_MS);
    poll();
    return () => clearInterval(id);
  }, []); // stable — never re-runs

  return entries;
}
