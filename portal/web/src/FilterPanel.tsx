import { useState } from 'react';
import type { Network } from './types';
import type { Theme } from './theme';

export interface FilterState {
  hiddenNodes: Set<string>;
  showDirect: boolean;
  showCC: boolean;
  showBCC: boolean;
}

export function defaultFilter(): FilterState {
  return { hiddenNodes: new Set(), showDirect: true, showCC: true, showBCC: true };
}

export function FilterPanel({ network, filter, theme, onChange }: {
  network: Network;
  filter: FilterState;
  theme: Theme;
  onChange: (f: FilterState) => void;
}) {
  const [tab, setTab] = useState<'nodes' | 'mail'>('nodes');

  const agents = (network.nodes || []);

  const tabStyle = (active: boolean): React.CSSProperties => ({
    background: active ? theme.stateColors['ACTIVE'] + '20' : 'transparent',
    border: 'none',
    borderBottom: active ? `2px solid ${theme.stateColors['ACTIVE']}` : '2px solid transparent',
    padding: '6px 12px',
    cursor: 'pointer',
    color: active ? theme.text : theme.textDim,
    fontSize: 11,
    fontWeight: active ? 600 : 400,
    flex: 1,
    textAlign: 'center' as const,
  });

  const toggleNode = (addr: string) => {
    const next = new Set(filter.hiddenNodes);
    if (next.has(addr)) next.delete(addr); else next.add(addr);
    onChange({ ...filter, hiddenNodes: next });
  };

  const allVisible = filter.hiddenNodes.size === 0;

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100%',
      userSelect: 'none',
    }}>
      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: `1px solid ${theme.border}`, flexShrink: 0 }}>
        <button onClick={() => setTab('nodes')} style={tabStyle(tab === 'nodes')}>Nodes</button>
        <button onClick={() => setTab('mail')} style={tabStyle(tab === 'mail')}>Mail</button>
      </div>

      {/* Content */}
      <div style={{ padding: '8px 10px', overflowY: 'auto', flex: 1 }}>
        {tab === 'nodes' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            {/* Select all / none */}
            <div style={{ display: 'flex', gap: 8, marginBottom: 4 }}>
              <button
                onClick={() => onChange({ ...filter, hiddenNodes: new Set() })}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: allVisible ? theme.textDim + '66' : theme.stateColors['ACTIVE'],
                  fontSize: 9, padding: 0,
                }}
              >
                all
              </button>
              <button
                onClick={() => {
                  const all = new Set(agents.map(a => a.address));
                  onChange({ ...filter, hiddenNodes: all });
                }}
                style={{
                  background: 'none', border: 'none', cursor: 'pointer',
                  color: theme.textDim,
                  fontSize: 9, padding: 0,
                }}
              >
                none
              </button>
            </div>
            {agents.map(a => {
              const visible = !filter.hiddenNodes.has(a.address);
              const baseName = a.nickname || a.agent_name || a.address.split('/').pop() || '?';
              const name = a.is_human ? `${baseName} (human)` : baseName;
              const stateColor = a.is_human ? theme.text : (theme.stateColors[(a.state || '').toUpperCase()] || theme.stateColors['']);
              return (
                <label
                  key={a.address}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    cursor: 'pointer',
                    fontSize: 10,
                    color: visible ? theme.text : theme.textDim + '66',
                    padding: '2px 0',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={visible}
                    onChange={() => toggleNode(a.address)}
                    style={{ accentColor: stateColor, margin: 0 }}
                  />
                  <span style={{
                    width: 5, height: 5, borderRadius: '50%',
                    background: stateColor,
                    flexShrink: 0,
                    opacity: visible ? 1 : 0.3,
                  }} />
                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {name}
                  </span>
                </label>
              );
            })}
          </div>
        )}

        {tab === 'mail' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <p style={{ fontSize: 9, color: theme.textDim, margin: '0 0 4px' }}>
              Edge thickness reflects selected types.
            </p>
            {([
              { key: 'showDirect' as const, label: 'Direct (To)' },
              { key: 'showCC' as const, label: 'CC' },
              { key: 'showBCC' as const, label: 'BCC' },
            ]).map(({ key, label }) => (
              <label
                key={key}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  cursor: 'pointer',
                  fontSize: 11,
                  color: filter[key] ? theme.text : theme.textDim + '66',
                }}
              >
                <input
                  type="checkbox"
                  checked={filter[key]}
                  onChange={() => onChange({ ...filter, [key]: !filter[key] })}
                  style={{ accentColor: theme.edgeColors.mail, margin: 0 }}
                />
                {label}
              </label>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
