import type { AgentInfo, DiaryEvent } from "../types";
import { AGENT_COLORS, TAG_COLORS, TAG_LABELS } from "../types";

interface DiaryEntryProps {
  event: DiaryEvent;
  agents: AgentInfo[];
  addressToName: Record<string, string>;
}

export function DiaryEntry({ event, agents, addressToName }: DiaryEntryProps) {
  const agentIndex = agents.findIndex((a) => a.key === event.agent_key);
  const agentColor = AGENT_COLORS[agentIndex % AGENT_COLORS.length] || "#888";
  const [tagBg, tagFg] = TAG_COLORS[event.type] || TAG_COLORS.unknown;
  const tagLabel = TAG_LABELS[event.type] || event.type;

  const ts = new Date(event.time * 1000).toLocaleTimeString();

  let content: React.ReactNode = null;

  switch (event.type) {
    case "diary":
    case "thinking":
    case "unknown":
      content = event.text || "";
      break;
    case "tool_call": {
      const args = JSON.stringify(event.args || {}).slice(0, 80);
      content = `${event.tool}(${args})`;
      break;
    }
    case "reasoning":
      content = `${event.tool}: ${event.text || ""}`;
      break;
    case "tool_result":
      content = `${event.tool} → ${event.status || ""}`;
      break;
    case "email_out": {
      const toName = addressToName[event.to || ""] || event.to || "";
      const subj = event.subject ? ` — ${event.subject}` : "";
      content = (
        <>
          to {toName}
          {subj}
          {event.message && (
            <div className="mt-1 p-1.5 bg-white/[0.03] rounded text-[11px] text-text-muted max-h-[200px] overflow-y-auto whitespace-pre-wrap">
              {event.message}
            </div>
          )}
        </>
      );
      break;
    }
    case "email_in": {
      const fromName = addressToName[event.from || ""] || event.from || "";
      const subj = event.subject ? ` — ${event.subject}` : "";
      content = (
        <>
          from {fromName}
          {subj}
          {event.message && (
            <div className="mt-1 p-1.5 bg-white/[0.03] rounded text-[11px] text-text-muted max-h-[200px] overflow-y-auto whitespace-pre-wrap">
              {event.message}
            </div>
          )}
        </>
      );
      break;
    }
    case "silence_received": {
      const fromName = addressToName[event.from || ""] || event.from || "";
      content = `by ${fromName}`;
      break;
    }
    case "kill_received": {
      const fromName = addressToName[event.from || ""] || event.from || "";
      content = `by ${fromName}`;
      break;
    }
  }

  return (
    <div className="py-1 border-b border-bg leading-relaxed text-xs">
      <span className="text-text-faint">{ts}</span>{" "}
      <span className="font-bold" style={{ color: agentColor }}>
        [{event.agent_name}]
      </span>{" "}
      <span
        className="text-[10px] px-1 rounded mr-1"
        style={{ backgroundColor: tagBg, color: tagFg }}
      >
        {tagLabel}
      </span>
      {content}
    </div>
  );
}
