import type { NetworkStats } from './types';
import type { Theme } from './theme';
import { t } from './i18n';

const items: { key: keyof Omit<NetworkStats, 'total_mails'>; stateKey: string }[] = [
  { key: 'active', stateKey: 'ACTIVE' },
  { key: 'idle', stateKey: 'IDLE' },
  { key: 'stuck', stateKey: 'STUCK' },
  { key: 'asleep', stateKey: 'ASLEEP' },
  { key: 'suspended', stateKey: 'SUSPENDED' },
];

export function Stats({ stats, totalAgents, lang, theme }: { stats: NetworkStats; totalAgents: number; lang: string; theme: Theme }) {
  return (
    <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexShrink: 0 }}>
      <div style={{ textAlign: 'center' }}>
        <div style={{ fontSize: 18, fontWeight: 'bold', color: theme.textDim }}>{totalAgents}</div>
        <div style={{ fontSize: 9, color: theme.textDim }}>{t(lang, 'agents')}</div>
      </div>
      <div style={{ width: 1, height: 28, background: theme.divider, margin: '0 4px' }} />
      {items.map(({ key, stateKey }) => {
        const color = theme.stateColors[stateKey];
        return (
          <div key={key} style={{ textAlign: 'center' }}>
            <div style={{ fontSize: 18, fontWeight: 'bold', color }}>{stats[key]}</div>
            <div style={{ fontSize: 9, color }}>{t(lang, `state.${key}`)}</div>
          </div>
        );
      })}
      <div style={{ width: 1, height: 28, background: theme.divider, margin: '0 4px' }} />
      <div style={{ textAlign: 'center' }}>
        <div style={{ fontSize: 18, fontWeight: 'bold', color: theme.edgeColors.mail }}>{stats.total_mails}</div>
        <div style={{ fontSize: 9, color: theme.edgeColors.mail }}>{t(lang, 'mails')}</div>
      </div>
    </div>
  );
}
