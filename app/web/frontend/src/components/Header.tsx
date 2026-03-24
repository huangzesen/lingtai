import type { AgentInfo } from "../types";

interface HeaderProps {
  agents: AgentInfo[];
  userPort: number;
  activePage: "inbox" | "network";
  onPageChange: (page: "inbox" | "network") => void;
  lightMode: boolean;
  onToggleTheme: () => void;
}

export function Header({
  agents,
  userPort,
  activePage,
  onPageChange,
  lightMode,
  onToggleTheme,
}: HeaderProps) {
  const activeCount = agents.filter((a) => a.status === "active").length;
  return (
    <div className="flex items-center gap-3 px-5 py-2.5 bg-panel border-b border-border">
      <h1 className="text-base font-bold text-accent">灵台</h1>
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
      <div className="ml-auto flex items-center gap-3">
        {activeCount > 0 && (
          <span className="text-xs text-emerald-400">
            ● {activeCount} active
          </span>
        )}
        <button
          onClick={onToggleTheme}
          className="px-2 py-1 text-xs border border-border rounded cursor-pointer bg-transparent text-text-dim hover:text-text"
          title={lightMode ? "Switch to dark mode" : "Switch to light mode"}
        >
          {lightMode ? "Dark" : "Light"}
        </button>
      </div>
    </div>
  );
}
