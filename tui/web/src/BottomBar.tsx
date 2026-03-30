import type { Network } from './types';
import type { EdgeMode } from './Graph';
import { Stats } from './Stats';
import { Kanban } from './Kanban';
import { inkBorder, inkEdgeColors } from './theme';
import { t } from './i18n';

export function BottomBar({ network, edgeMode, lang, onToggle }: {
  network: Network;
  edgeMode: EdgeMode;
  lang: string;
  onToggle: () => void;
}) {
  return (
    <div style={{
      background: 'rgba(13,13,15,0.95)',
      borderTop: `1px solid ${inkBorder}`,
      padding: '10px 16px',
      display: 'flex',
      alignItems: 'flex-start',
      gap: 24,
      maxHeight: '40vh',
      overflowY: 'auto',
      flexShrink: 0,
    }}>
      <Stats stats={network.stats} lang={lang} />
      <div style={{ display: 'flex', flexShrink: 0, alignSelf: 'center', borderRadius: 4, overflow: 'hidden', border: `1px solid ${inkBorder}` }}>
        {(['avatar', 'email'] as EdgeMode[]).map(mode => {
          const active = edgeMode === mode;
          const color = mode === 'avatar' ? inkEdgeColors.avatar : inkEdgeColors.mail;
          return (
            <button
              key={mode}
              onClick={active ? undefined : onToggle}
              style={{
                background: active ? color + '30' : 'transparent',
                border: 'none',
                borderRight: mode === 'avatar' ? `1px solid ${inkBorder}` : 'none',
                padding: '3px 10px',
                cursor: active ? 'default' : 'pointer',
                color: active ? color : color + '66',
                fontSize: 10,
                letterSpacing: 0.5,
              }}
            >
              {t(lang, `edge.${mode}`)}
            </button>
          );
        })}
      </div>
      <Kanban nodes={network.nodes} lang={lang} />
    </div>
  );
}
