import { useEffect, useState } from 'react';
import type { Network } from './types';
import { fetchNetwork } from './api';
import { Graph } from './Graph';
import { BottomBar } from './BottomBar';
import { inkBg, ColorTextDim } from './theme';

export default function App() {
  const [network, setNetwork] = useState<Network | null>(null);

  useEffect(() => {
    const poll = () => fetchNetwork().then(setNetwork).catch(console.error);
    poll();
    const id = setInterval(poll, 3000);
    return () => clearInterval(id);
  }, []);

  if (!network) {
    return (
      <div style={{ background: inkBg, color: ColorTextDim, height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        Connecting...
      </div>
    );
  }

  return (
    <div style={{ background: inkBg, height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <div style={{ flex: 1 }}>
        <Graph network={network} />
      </div>
      <BottomBar network={network} />
    </div>
  );
}
