interface EmailBubbleProps {
  direction: "sent" | "received";
  fromName?: string;
  toName?: string;
  cc?: string[];
  subject?: string;
  message: string;
  time: string;
}

export function EmailBubble({
  direction,
  fromName,
  toName,
  cc,
  subject,
  message,
  time,
}: EmailBubbleProps) {
  const isSent = direction === "sent";
  return (
    <div
      className={`px-3.5 py-2.5 rounded-lg text-sm leading-relaxed whitespace-pre-wrap break-words max-w-[80%] ${
        isSent
          ? "self-end bg-border"
          : "self-start bg-panel border border-border"
      }`}
    >
      <div className="text-[11px] text-text-dim mb-1">
        <span className="text-text-faint">{new Date(time).toLocaleTimeString()}</span>{" "}
        {isSent ? `To: ${toName}` : `From: ${fromName}`}
        {subject && subject !== "(no subject)" && ` — ${subject}`}
        {cc && cc.length > 0 && ` · CC: ${cc.join(", ")}`}
      </div>
      {escapeAndRender(message)}
    </div>
  );
}

function escapeAndRender(text: string) {
  return <span>{text}</span>;
}
