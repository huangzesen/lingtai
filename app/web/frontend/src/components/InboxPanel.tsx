import { useEffect, useRef, useMemo } from "react";
import type { AgentInfo, Email, SentMessage } from "../types";
import { EmailBubble } from "./EmailBubble";
import { InputBar } from "./InputBar";

interface InboxPanelProps {
  agents: AgentInfo[];
  keyToName: Record<string, string>;
  addressToName: Record<string, string>;
  receivedEmails: Email[];
  sentMessages: SentMessage[];
  onSent: (msg: SentMessage) => void;
}

export function InboxPanel({
  agents,
  keyToName,
  addressToName,
  receivedEmails,
  sentMessages,
  onSent,
}: InboxPanelProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const allMessages = useMemo(() => {
    const items: Array<{
      type: "sent" | "received";
      time: string;
      msg: SentMessage | Email;
    }> = [];
    for (const s of sentMessages) {
      items.push({ type: "sent", time: s.time, msg: s });
    }
    for (const e of receivedEmails) {
      items.push({ type: "received", time: e.time, msg: e });
    }
    items.sort((a, b) => a.time.localeCompare(b.time));
    return items;
  }, [sentMessages, receivedEmails]);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [allMessages]);

  return (
    <div className="flex-[2] flex flex-col border-r border-border">
      <div className="px-4 py-2 text-xs text-accent uppercase tracking-widest border-b border-border">
        Inbox
      </div>
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto p-4 flex flex-col gap-2"
      >
        {allMessages.map((item, i) => {
          if (item.type === "sent") {
            const s = item.msg as SentMessage;
            const ccNames = s.cc.map((k) => keyToName[k] || k);
            return (
              <EmailBubble
                key={`sent-${i}`}
                direction="sent"
                toName={keyToName[s.to] || s.to}
                cc={ccNames.length > 0 ? ccNames : undefined}
                message={s.text}
                time={s.time}
              />
            );
          } else {
            const e = item.msg as Email;
            const fromName = addressToName[e.from] || e.from;
            const ccNames = (e.cc || []).map(
              (addr) => addressToName[addr] || addr
            );
            return (
              <EmailBubble
                key={e.id}
                direction="received"
                fromName={fromName}
                subject={e.subject}
                cc={ccNames.length > 0 ? ccNames : undefined}
                message={e.message}
                time={e.time}
              />
            );
          }
        })}
      </div>
      <InputBar agents={agents} keyToName={keyToName} onSent={onSent} />
    </div>
  );
}
