import { useEffect, useState } from 'react';
import type { Network } from './types';
import { fetchNetwork } from './api';
import { Graph, type EdgeMode } from './Graph';
import { BottomBar } from './BottomBar';
import { inkBg, ColorTextDim } from './theme';
import { t } from './i18n';

export default function App() {
  const [network, setNetwork] = useState<Network | null>(null);
  const [edgeMode, setEdgeMode] = useState<EdgeMode>('avatar');

  useEffect(() => {
    const poll = () => fetchNetwork().then(setNetwork).catch(console.error);
    poll();
    const id = setInterval(poll, 3000);
    return () => clearInterval(id);
  }, []);

  const lang = network?.lang ?? 'en';

  if (!network) {
    return (
      <div style={{ background: inkBg, color: ColorTextDim, height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {t(lang, 'connecting')}
      </div>
    );
  }

  return (
    <div style={{ background: inkBg, height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <div style={{ flex: 1, minHeight: 0 }}>
        <Graph network={network} edgeMode={edgeMode} />
      </div>
      <BottomBar
        network={network}
        edgeMode={edgeMode}
        lang={lang}
        onToggle={() => setEdgeMode(m => m === 'avatar' ? 'email' : 'avatar')}
      />
    </div>
  );
}
