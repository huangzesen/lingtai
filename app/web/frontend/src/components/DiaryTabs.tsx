import type { AgentInfo, DiaryEventType } from "../types";
import { AGENT_COLORS, ALL_DIARY_EVENT_TYPES, TAG_COLORS, TAG_LABELS } from "../types";

interface DiaryTabsProps {
  agents: AgentInfo[];
  activeTab: string;
  onTabChange: (tab: string) => void;
  activeTypes: Set<DiaryEventType>;
  onToggleType: (type: DiaryEventType) => void;
}

export function DiaryTabs({ agents, activeTab, onTabChange, activeTypes, onToggleType }: DiaryTabsProps) {
  const activeAgent = agents.find((a) => a.key === activeTab);
  const activeIdx = agents.findIndex((a) => a.key === activeTab);
  const activeColor =
    activeTab === "all"
      ? undefined
      : AGENT_COLORS[activeIdx % AGENT_COLORS.length];

  return (
    <div className="flex flex-col border-b border-border">
      <div className="flex items-center gap-2 px-3 py-1.5">
        <span className="text-[10px] text-text-dim uppercase tracking-wider">
          Diary
        </span>
        <select
          value={activeTab}
          onChange={(e) => onTabChange(e.target.value)}
          className="px-2 py-1 text-xs border border-border rounded bg-bg text-text cursor-pointer"
          style={activeColor ? { color: activeColor } : undefined}
        >
          <option value="all">All agents</option>
          {agents.map((a) => (
            <option key={a.key} value={a.key}>
              {a.name}
            </option>
          ))}
        </select>
        {activeAgent && (
          <span
            className="text-[10px]"
            style={{ color: activeColor }}
          >
            :{activeAgent.port}
          </span>
        )}
      </div>
      <div className="flex flex-wrap gap-1 px-3 pb-1.5">
        {ALL_DIARY_EVENT_TYPES.map(type => (
          <button
            key={type}
            onClick={() => onToggleType(type)}
            className={`text-xs px-2 py-0.5 rounded-full cursor-pointer transition-opacity ${
              activeTypes.has(type) ? '' : 'opacity-30'
            }`}
            style={{
              backgroundColor: activeTypes.has(type) ? TAG_COLORS[type][0] : '#2a2a2a',
              color: activeTypes.has(type) ? TAG_COLORS[type][1] : '#666',
            }}
          >
            {TAG_LABELS[type]}
          </button>
        ))}
      </div>
    </div>
  );
}
