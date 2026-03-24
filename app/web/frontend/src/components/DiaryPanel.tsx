import { useEffect, useMemo, useRef, useState } from "react";
import type { AgentInfo, DiaryEvent, DiaryEventType } from "../types";
import { ALL_DIARY_EVENT_TYPES } from "../types";
import { DiaryEntry } from "./DiaryEntry";
import { DiaryTabs } from "./DiaryTabs";

interface DiaryPanelProps {
  agents: AgentInfo[];
  entries: DiaryEvent[];
  addressToName: Record<string, string>;
}

export function DiaryPanel({ agents, entries, addressToName }: DiaryPanelProps) {
  const [activeTab, setActiveTab] = useState("all");
  const [activeTypes, setActiveTypes] = useState<Set<DiaryEventType>>(
    new Set(ALL_DIARY_EVENT_TYPES)
  );
  const scrollRef = useRef<HTMLDivElement>(null);

  const handleToggleType = (type: DiaryEventType) => {
    setActiveTypes(prev => {
      const next = new Set(prev);
      if (next.has(type)) next.delete(type);
      else next.add(type);
      return next;
    });
  };

  const filtered = useMemo(
    () =>
      entries
        .filter(e => activeTab === "all" || e.agent_key === activeTab)
        .filter(e => activeTypes.has(e.type)),
    [entries, activeTab, activeTypes]
  );

  const lastEntryTimeRef = useRef(0);
  useEffect(() => {
    if (entries.length === 0) return;
    const lastTime = entries[entries.length - 1].time;
    if (lastTime > lastEntryTimeRef.current) {
      lastEntryTimeRef.current = lastTime;
      if (scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    }
  }, [entries]);

  return (
    <div className="flex-1 flex flex-col bg-panel-dark">
      <DiaryTabs
        agents={agents}
        activeTab={activeTab}
        onTabChange={setActiveTab}
        activeTypes={activeTypes}
        onToggleType={handleToggleType}
      />
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-3 text-xs text-text-muted"
      >
        {filtered.map((e, i) => (
          <DiaryEntry
            key={`${e.agent_key}-${e.time}-${i}`}
            event={e}
            agents={agents}
            addressToName={addressToName}
          />
        ))}
        {filtered.length === 0 && (
          <div className="text-center text-text-dim text-xs py-4">
            No events match current filters
          </div>
        )}
      </div>
    </div>
  );
}
