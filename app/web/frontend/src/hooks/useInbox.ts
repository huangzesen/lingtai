import { useCallback, useEffect, useRef, useState } from "react";
import type { Email, SentMessage } from "../types";

const POLL_MS = 1500;

export function useInbox() {
  const [receivedEmails, setReceivedEmails] = useState<Email[]>([]);
  const [sentMessages, setSentMessages] = useState<SentMessage[]>([]);
  const lastCountRef = useRef(0);

  useEffect(() => {
    const poll = async () => {
      try {
        const resp = await fetch("/api/inbox");
        const data = await resp.json();
        if (data.emails.length > lastCountRef.current) {
          lastCountRef.current = data.emails.length;
          setReceivedEmails(data.emails);
        }
      } catch {
        /* ignore */
      }
    };
    const id = setInterval(poll, POLL_MS);
    poll();
    return () => clearInterval(id);
  }, []);

  const addSent = useCallback((msg: SentMessage) => {
    setSentMessages((prev) => [...prev, msg]);
  }, []);

  return { receivedEmails, sentMessages, addSent };
}
