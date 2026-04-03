import type { Network } from './types';
import type { Theme } from './theme';
import { Stats } from './Stats';

export function BottomBar({ network, lang, theme }: {
  network: Network;
  lang: string;
  theme: Theme;
}) {
  return (
    <div style={{
      background: theme.barBg,
      borderTop: `1px solid ${theme.border}`,
      padding: '8px 16px',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      flexShrink: 0,
    }}>
      <Stats stats={network.stats} lang={lang} theme={theme} />
    </div>
  );
}
