import { useState } from "react";
import { useAgents } from "./hooks/useAgents";
import { useInbox } from "./hooks/useInbox";
import { useDiary } from "./hooks/useDiary";
import { useNetwork } from "./hooks/useNetwork";
import { Header } from "./components/Header";
import { InboxPanel } from "./components/InboxPanel";
import { DiaryPanel } from "./components/DiaryPanel";
import { NetworkPage } from "./components/NetworkPage";

const USER_PORT = 8300;

export default function App() {
  const [activePage, setActivePage] = useState<"inbox" | "network">("inbox");
  const { agents, keyToName, addressToName } = useAgents();
  const { receivedEmails, sentMessages, addSent } = useInbox();
  const entries = useDiary(agents);
  const { nodes, edges, particles } = useNetwork(
    agents,
    entries,
    sentMessages,
    USER_PORT
  );

  return (
    <div className="h-screen flex flex-col bg-bg text-text font-sans">
      <Header
        agents={agents}
        userPort={USER_PORT}
        activePage={activePage}
        onPageChange={setActivePage}
      />
      <div className="flex-1 flex overflow-hidden">
        {activePage === "inbox" ? (
          <>
            <InboxPanel
              agents={agents}
              keyToName={keyToName}
              addressToName={addressToName}
              receivedEmails={receivedEmails}
              sentMessages={sentMessages}
              onSent={addSent}
            />
            <DiaryPanel
              agents={agents}
              entries={entries}
              addressToName={addressToName}
            />
          </>
        ) : (
          <NetworkPage nodes={nodes} edges={edges} particles={particles} />
        )}
      </div>
    </div>
  );
}
