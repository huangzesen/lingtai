import type { NetworkStats } from './types';
import { stateColors } from './theme';

const items: { key: keyof Omit<NetworkStats, 'total_mails'>; label: string }[] = [
  { key: 'active', label: 'ACTIVE' },
  { key: 'idle', label: 'IDLE' },
  { key: 'stuck', label: 'STUCK' },
  { key: 'asleep', label: 'ASLEEP' },
  { key: 'suspended', label: 'SUSPENDED' },
];

export function Stats({ stats }: { stats: NetworkStats }) {
  return (
    <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexShrink: 0 }}>
      {items.map(({ key, label }) => (
        <div key={key} style={{ textAlign: 'center' }}>
          <div style={{ fontSize: 18, fontWeight: 'bold', color: stateColors[label] }}>{stats[key]}</div>
          <div style={{ fontSize: 9, color: stateColors[label] }}>{label}</div>
        </div>
      ))}
      <div style={{ width: 1, height: 28, background: 'rgba(255,255,255,0.1)', margin: '0 4px' }} />
      <div style={{ textAlign: 'center' }}>
        <div style={{ fontSize: 18, fontWeight: 'bold', color: '#63b3ed' }}>{stats.total_mails}</div>
        <div style={{ fontSize: 9, color: '#63b3ed' }}>MAILS</div>
      </div>
    </div>
  );
}
