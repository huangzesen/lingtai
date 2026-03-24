import { useEffect, useState, useRef } from "react";
import type { AgentInfo, SentMessage } from "../types";

interface InputBarProps {
  agents: AgentInfo[];
  keyToName: Record<string, string>;
  onSent: (msg: SentMessage) => void;
}

export function InputBar({ agents, onSent }: InputBarProps) {
  const [target, setTarget] = useState("");

  // Set default target when agents load
  useEffect(() => {
    if (!target && agents.length > 0) {
      setTarget(agents[0].key);
    }
  }, [agents, target]);
  const [ccVisible, setCcVisible] = useState(false);
  const [bccVisible, setBccVisible] = useState(false);
  const [ccChecked, setCcChecked] = useState<Record<string, boolean>>({});
  const [bccChecked, setBccChecked] = useState<Record<string, boolean>>({});
  const inputRef = useRef<HTMLInputElement>(null);

  const sendEmail = async () => {
    const text = inputRef.current?.value.trim();
    if (!text || !target) return;
    inputRef.current!.value = "";

    const cc = Object.keys(ccChecked).filter((k) => ccChecked[k] && k !== target);
    const bcc = Object.keys(bccChecked).filter((k) => bccChecked[k] && k !== target);
    // BCC takes precedence — remove from CC if in both
    const finalCC = cc.filter((k) => !bcc.includes(k));

    onSent({ to: target, cc: finalCC, text, time: new Date().toISOString() });

    await fetch("/api/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agent: target, message: text, cc: finalCC, bcc }),
    });

    setCcChecked({});
    setBccChecked({});
    inputRef.current?.focus();
  };

  const otherAgents = agents.filter((a) => a.key !== target);

  return (
    <div>
      {ccVisible && (
        <div className="px-4 py-1.5 bg-panel text-xs text-text-muted border-t border-border flex flex-wrap gap-x-3 gap-y-1">
          <span className="text-text-dim">CC:</span>
          {otherAgents.map((a) => (
            <label key={a.key} className="cursor-pointer whitespace-nowrap">
              <input
                type="checkbox"
                className="mr-1"
                checked={!!ccChecked[a.key]}
                onChange={(e) =>
                  setCcChecked((p) => ({ ...p, [a.key]: e.target.checked }))
                }
              />
              {a.name}
            </label>
          ))}
        </div>
      )}
      {bccVisible && (
        <div className="px-4 py-1.5 bg-panel text-xs text-text-muted border-t border-border flex flex-wrap gap-x-3 gap-y-1">
          <span className="text-text-dim">BCC:</span>
          {otherAgents.map((a) => (
            <label key={a.key} className="cursor-pointer whitespace-nowrap">
              <input
                type="checkbox"
                className="mr-1"
                checked={!!bccChecked[a.key]}
                onChange={(e) =>
                  setBccChecked((p) => ({ ...p, [a.key]: e.target.checked }))
                }
              />
              {a.name}
            </label>
          ))}
        </div>
      )}
      <div className="flex items-center gap-2 px-4 py-2.5 bg-panel border-t border-border">
        <select
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          className="px-2 py-1.5 border border-border rounded-md bg-bg text-text text-sm"
        >
          {agents.map((a) => (
            <option key={a.key} value={a.key}>
              To: {a.name} (:{a.port})
            </option>
          ))}
        </select>
        <button
          onClick={() => setCcVisible((v) => !v)}
          className={`px-2.5 py-1.5 text-xs border rounded-md cursor-pointer ${
            ccVisible
              ? "text-text border-accent"
              : "text-text-muted border-border bg-bg"
          }`}
        >
          CC
        </button>
        <button
          onClick={() => setBccVisible((v) => !v)}
          className={`px-2.5 py-1.5 text-xs border rounded-md cursor-pointer ${
            bccVisible
              ? "text-text border-accent"
              : "text-text-muted border-border bg-bg"
          }`}
        >
          BCC
        </button>
        <input
          ref={inputRef}
          placeholder="Type a message..."
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              sendEmail();
            }
          }}
          className="flex-1 px-3 py-1.5 border border-border rounded-md bg-bg text-text text-sm outline-none focus:border-accent"
          autoFocus
        />
        <button
          onClick={sendEmail}
          className="px-4 py-1.5 bg-accent text-white border-none rounded-md cursor-pointer text-sm hover:bg-accent-hover"
        >
          Send
        </button>
      </div>
    </div>
  );
}
