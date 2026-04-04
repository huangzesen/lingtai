import type { Network } from './types';
import type { Theme } from './theme';
import type { EdgeMode } from './Graph';
import { Stats } from './Stats';
import { t } from './i18n';

function EdgeToggle({ edgeMode, lang, theme, onToggle }: {
  edgeMode: EdgeMode;
  lang: string;
  theme: Theme;
  onToggle: () => void;
}) {
  return (
    <div style={{
      display: 'flex',
      borderRadius: 4,
      overflow: 'hidden',
      border: `1px solid ${theme.border}`,
      flexShrink: 0,
    }}>
      {(['avatar', 'email'] as EdgeMode[]).map(mode => {
        const active = edgeMode === mode;
        const color = mode === 'avatar' ? theme.edgeColors.avatar : theme.edgeColors.mail;
        return (
          <button
            key={mode}
            onClick={active ? undefined : onToggle}
            style={{
              background: active ? color + '25' : 'transparent',
              border: 'none',
              borderRight: mode === 'avatar' ? `1px solid ${theme.border}` : 'none',
              padding: '2px 10px',
              cursor: active ? 'default' : 'pointer',
              color: active ? color : color + '55',
              fontSize: 10,
              letterSpacing: 0.5,
              transition: 'all 0.15s',
            }}
          >
            {t(lang, `edge.${mode}`)}
          </button>
        );
      })}
    </div>
  );
}

export function BottomBar({ network, lang, theme, edgeMode, onToggleEdgeMode }: {
  network: Network;
  lang: string;
  theme: Theme;
  edgeMode: EdgeMode;
  onToggleEdgeMode: () => void;
}) {
  return (
    <div style={{
      background: theme.barBg,
      borderTop: `1px solid ${theme.border}`,
      padding: '8px 16px',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: 16,
      flexShrink: 0,
    }}>
      <EdgeToggle edgeMode={edgeMode} lang={lang} theme={theme} onToggle={onToggleEdgeMode} />
      <div style={{ width: 1, height: 28, background: theme.border }} />
      <Stats stats={network.stats} totalAgents={network.nodes.length} lang={lang} theme={theme} />
    </div>
  );
}
