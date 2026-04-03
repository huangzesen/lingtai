export interface AgentNode {
  address: string;
  agent_name: string;
  nickname: string;
  state: 'ACTIVE' | 'IDLE' | 'STUCK' | 'ASLEEP' | 'SUSPENDED' | '';
  alive: boolean;
  is_human: boolean;
  capabilities: string[];
}

export interface AvatarEdge {
  parent: string;
  child: string;
  child_name: string;
}

export interface ContactEdge {
  owner: string;
  target: string;
  name: string;
}

export interface MailEdge {
  sender: string;
  recipient: string;
  count: number;
  direct: number;
  cc: number;
  bcc: number;
}

export interface NetworkStats {
  active: number;
  idle: number;
  stuck: number;
  asleep: number;
  suspended: number;
  total_mails: number;
}

export interface Network {
  nodes: AgentNode[];
  avatar_edges: AvatarEdge[];
  contact_edges: ContactEdge[];
  mail_edges: MailEdge[];
  stats: NetworkStats;
  lang: string;
}
