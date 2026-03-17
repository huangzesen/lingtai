export interface AgentInfo {
  id: string;
  name: string;
  key: string;
  address: string;
  port: number;
  status: "active" | "sleeping";
  type: "admin" | "agent";
}

export interface Email {
  id: string;
  from: string;
  to?: string[];
  cc?: string[];
  subject: string;
  message: string;
  time: string;
}

export type DiaryEventType =
  | "diary"
  | "thinking"
  | "tool_call"
  | "reasoning"
  | "tool_result"
  | "email_out"
  | "email_in"
  | "cancel_received"
  | "cancel_diary"
  | "unknown";

export interface DiaryEvent {
  type: DiaryEventType;
  time: number;
  agent_key: string;
  agent_name: string;
  text?: string;
  tool?: string;
  args?: Record<string, unknown>;
  status?: string;
  to?: string;
  from?: string;
  subject?: string;
  message?: string;
}

export interface SentMessage {
  to: string;
  cc: string[];
  text: string;
  time: string;
}

/** Network visualization types */

export interface NetworkNode {
  id: string;
  name: string;
  color: string;
  status: "active" | "sleeping";
  type: "admin" | "agent" | "user";
  x?: number;
  y?: number;
  vx?: number;
  vy?: number;
  fx?: number | null;
  fy?: number | null;
}

export interface NetworkEdge {
  source: string | NetworkNode;
  target: string | NetworkNode;
  count: number;
}

export interface Particle {
  id: string;
  source: string;
  target: string;
  color: string;
  startTime: number;
  duration: number;
}

/** Agent accent colors — indexed by agent order. */
export const AGENT_COLORS = [
  "#e94560",
  "#4ecdc4",
  "#f0a500",
  "#6bcb77",
  "#b06bcb",
  "#6b9bcb",
  "#cb6bb5",
  "#cbc76b",
];

/** Diary event tag colors: [background, text]. */
export const TAG_COLORS: Record<DiaryEventType, [string, string]> = {
  diary: ["#1a3a1a", "#6bcb77"],
  thinking: ["#3a3a1a", "#cbc76b"],
  tool_call: ["#1a1a3a", "#6b9bcb"],
  reasoning: ["#2a1a3a", "#b06bcb"],
  tool_result: ["#1a2a2a", "#6bcbbb"],
  email_out: ["#1a2a3a", "#6bb5cb"],
  email_in: ["#2a1a2a", "#cb6bb5"],
  cancel_received: ["#3a1a1a", "#e94560"],
  cancel_diary: ["#3a2a1a", "#f0a500"],
  unknown: ["#2a2a2a", "#888888"],
};

/** Tag display labels. */
export const TAG_LABELS: Record<DiaryEventType, string> = {
  diary: "diary",
  thinking: "thinking",
  tool_call: "tool",
  reasoning: "why",
  tool_result: "result",
  email_out: "sent",
  email_in: "received",
  cancel_received: "CANCELLED",
  cancel_diary: "cancel diary",
  unknown: "event",
};
