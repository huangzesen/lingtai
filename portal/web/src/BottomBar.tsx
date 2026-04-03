import type { Network } from './types';
import type { EdgeMode } from './Graph';
import type { Theme } from './theme';
import { Stats } from './Stats';
import { t } from './i18n';

export function BottomBar({ network, edgeMode, showNames, showFilter, lang, theme, onToggle, onToggleNames, onToggleFilter }: {
  network: Network;
  edgeMode: EdgeMode;
  showNames: boolean;
  showFilter: boolean;
  lang: string;
  theme: Theme;
  onToggle: () => void;
  onToggleNames: () => void;
  onToggleFilter: () => void;
}) {
  const btnStyle = (active: boolean): React.CSSProperties => ({
    background: active ? theme.textDim + '20' : 'transparent',
    border: `1px solid ${theme.border}`,
    borderRadius: 4,
    padding: '3px 10px',
    cursor: 'pointer',
    color: active ? theme.textDim : theme.textDim + '66',
    fontSize: 10,
    letterSpacing: 0.5,
    flexShrink: 0,
  });

  return (
    <div style={{
      background: theme.barBg,
      borderTop: `1px solid ${theme.border}`,
      padding: '10px 16px',
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      flexShrink: 0,
    }}>
      <Stats stats={network.stats} lang={lang} theme={theme} />
      <div style={{ display: 'flex', flexShrink: 0, alignSelf: 'center', borderRadius: 4, overflow: 'hidden', border: `1px solid ${theme.border}` }}>
        {(['avatar', 'email'] as EdgeMode[]).map(mode => {
          const active = edgeMode === mode;
          const color = mode === 'avatar' ? theme.edgeColors.avatar : theme.edgeColors.mail;
          return (
            <button
              key={mode}
              onClick={active ? undefined : onToggle}
              style={{
                background: active ? color + '30' : 'transparent',
                border: 'none',
                borderRight: mode === 'avatar' ? `1px solid ${theme.border}` : 'none',
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
      <button onClick={onToggleNames} style={btnStyle(showNames)} title={showNames ? 'Hide names' : 'Show names'}>
        {showNames ? 'name ✓' : 'name'}
      </button>
      <button onClick={onToggleFilter} style={btnStyle(showFilter)} title="Filter nodes and mail types">
        filter
      </button>
    </div>
  );
}
