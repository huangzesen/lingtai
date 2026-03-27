import type { Network } from './types';
import { Stats } from './Stats';
import { Kanban } from './Kanban';
import { inkBg, inkBorder } from './theme';

export function BottomBar({ network }: { network: Network }) {
  return (
    <div style={{
      background: 'rgba(13,13,15,0.95)',
      borderTop: `1px solid ${inkBorder}`,
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
