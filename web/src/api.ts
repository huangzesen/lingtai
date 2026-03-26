import type { Network } from './types';

export async function fetchNetwork(): Promise<Network> {
  const res = await fetch('/api/network');
  if (!res.ok) throw new Error(`API error: ${res.status}`);
  return res.json();
}
