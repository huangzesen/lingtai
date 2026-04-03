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

export function FilterPanel({ network, filter, theme, onClose, onChange }: {
  network: Network;
  filter: FilterState;
  theme: Theme;
  onClose: () => void;
  onChange: (f: FilterState) => void;
}) {
  const [tab, setTab] = useState<'nodes' | 'mail'>('nodes');

  const agents = (network.nodes || []).filter(n => !n.is_human);

  const tabStyle = (active: boolean): React.CSSProperties => ({
    background: active ? theme.stateColors['ACTIVE'] + '20' : 'transparent',
    border: 'none',
    borderBottom: active ? `2px solid ${theme.stateColors['ACTIVE']}` : '2px solid transparent',
    padding: '6px 16px',
    cursor: 'pointer',
    color: active ? theme.text : theme.textDim,
    fontSize: 11,
    fontWeight: active ? 600 : 400,
  });

  const toggleNode = (addr: string) => {
    const next = new Set(filter.hiddenNodes);
    if (next.has(addr)) next.delete(addr); else next.add(addr);
    onChange({ ...filter, hiddenNodes: next });
  };

  const allVisible = filter.hiddenNodes.size === 0;

  return (
    <div style={{
      position: 'absolute',
      top: 48,
      right: 16,
      background: theme.barBg,
      border: `1px solid ${theme.border}`,
      borderRadius: 6,
      boxShadow: '0 4px 16px rgba(0,0,0,0.3)',
      zIndex: 100,
      minWidth: 220,
      maxHeight: '60vh',
      display: 'flex',
      flexDirection: 'column',
      userSelect: 'none',
    }}>
      {/* Header */}
      <div style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '8px 12px 0',
      }}>
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.text }}>Filter</span>
        <button
          onClick={onClose}
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            color: theme.textDim,
            fontSize: 14,
            padding: '0 4px',
          }}
        >
          ×
        </button>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', borderBottom: `1px solid ${theme.border}` }}>
        <button onClick={() => setTab('nodes')} style={tabStyle(tab === 'nodes')}>Nodes</button>
        <button onClick={() => setTab('mail')} style={tabStyle(tab === 'mail')}>Mail</button>
      </div>

      {/* Content */}
      <div style={{ padding: '8px 12px', overflowY: 'auto', flex: 1 }}>
        {tab === 'nodes' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
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
              const name = a.nickname || a.agent_name || a.address.split('/').pop() || '?';
              const stateColor = theme.stateColors[(a.state || '').toUpperCase()] || theme.stateColors[''];
              return (
                <label
                  key={a.address}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 6,
                    cursor: 'pointer',
                    fontSize: 11,
                    color: visible ? theme.text : theme.textDim + '66',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={visible}
                    onChange={() => toggleNode(a.address)}
                    style={{ accentColor: stateColor }}
                  />
                  <span style={{
                    width: 6, height: 6, borderRadius: '50%',
                    background: stateColor,
                    flexShrink: 0,
                    opacity: visible ? 1 : 0.3,
                  }} />
                  {name}
                </label>
              );
            })}
          </div>
        )}

        {tab === 'mail' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <p style={{ fontSize: 10, color: theme.textDim, margin: '0 0 4px' }}>
              Filter which email types count toward edge thickness and display.
            </p>
            {([
              { key: 'showDirect' as const, label: 'Direct (To)', desc: 'Primary recipients' },
              { key: 'showCC' as const, label: 'CC', desc: 'Carbon copy' },
              { key: 'showBCC' as const, label: 'BCC', desc: 'Blind carbon copy' },
            ]).map(({ key, label, desc }) => (
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
                  style={{ accentColor: theme.edgeColors.mail }}
                />
                <div>
                  <div>{label}</div>
                  <div style={{ fontSize: 9, color: theme.textDim }}>{desc}</div>
                </div>
              </label>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
