import type { Network } from './types';
import { Stats } from './Stats';
import { Kanban } from './Kanban';

export function BottomBar({ network }: { network: Network }) {
  return (
    <div style={{
      background: 'rgba(10,10,26,0.95)',
      borderTop: '1px solid rgba(255,255,255,0.08)',
      padding: '10px 16px',
      display: 'flex',
      alignItems: 'flex-start',
      gap: 24,
    }}>
      <Stats stats={network.stats} />
      <Kanban nodes={network.nodes} />
    </div>
  );
}
