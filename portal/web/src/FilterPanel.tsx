import React, { useState } from 'react';
import type { Network, AgentNode } from './types';
import type { Theme } from './theme';
import { t } from './i18n';

export interface FilterState {
  hiddenNodes: Set<string>;
  showDirect: boolean;
  showCC: boolean;
  showBCC: boolean;
}

export function defaultFilter(): FilterState {
  return { hiddenNodes: new Set(), showDirect: true, showCC: true, showBCC: true };
}

function Toggle({ on, color, onChange }: { on: boolean; color: string; onChange: () => void }) {
  return (
    <button
      onClick={onChange}
      style={{
        width: 28,
        height: 14,
        borderRadius: 7,
        border: 'none',
        background: on ? color + '40' : 'rgba(128,128,128,0.15)',
        position: 'relative',
        cursor: 'pointer',
        transition: 'background 0.2s',
        flexShrink: 0,
        padding: 0,
      }}
    >
      <span style={{
        display: 'block',
        width: 10,
        height: 10,
        borderRadius: '50%',
        background: on ? color : 'rgba(128,128,128,0.4)',
        position: 'absolute',
        top: 2,
        left: on ? 16 : 2,
        transition: 'left 0.2s, background 0.2s',
        boxShadow: on ? `0 0 4px ${color}60` : 'none',
      }} />
    </button>
  );
}

export function FilterPanel({ network, filter, lang, theme, showNames, onToggleNames, onChange }: {
  network: Network;
  filter: FilterState;
  lang: string;
  theme: Theme;
  showNames: boolean;
  onToggleNames: () => void;
  onChange: (f: FilterState) => void;
}) {
  const [tab, setTab] = useState<'nodes' | 'mail'>('nodes');
  const agents = network.nodes || [];
  const childSet = new Set((network.avatar_edges || []).map(e => e.child));
  const adminAddrs = new Set(agents.filter(a => !a.is_human && !childSet.has(a.address)).map(a => a.address));

  const toggleNode = (addr: string) => {
    const next = new Set(filter.hiddenNodes);
    if (next.has(addr)) next.delete(addr); else next.add(addr);
    onChange({ ...filter, hiddenNodes: next });
  };

  const allVisible = filter.hiddenNodes.size === 0;
  const noneVisible = filter.hiddenNodes.size === agents.length;

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100%',
      userSelect: 'none',
      fontFamily: "'Georgia', 'Noto Serif SC', serif",
    }}>
      {/* Name toggle row — top of sidebar */}
      <div
        onClick={onToggleNames}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '8px 10px',
          cursor: 'pointer',
          borderBottom: `1px solid ${theme.border}`,
          flexShrink: 0,
        }}
      >
        <span style={{ fontSize: 11, color: theme.text }}>{t(lang, 'menu.names')}</span>
        <Toggle on={showNames} color={theme.gold} onChange={() => {}} />
      </div>

      {/* Tab strip */}
      <div style={{
        display: 'flex',
        flexShrink: 0,
        borderBottom: `1px solid ${theme.border}`,
      }}>
        {(['nodes', 'mail'] as const).map(tb => {
          const active = tab === tb;
          return (
            <button
              key={tb}
              onClick={() => setTab(tb)}
              style={{
                flex: 1,
                background: 'transparent',
                border: 'none',
                borderBottom: active ? `2px solid ${theme.gold}` : '2px solid transparent',
                padding: '8px 0 6px',
                cursor: 'pointer',
                color: active ? theme.gold : theme.textDim + '80',
                fontSize: 10,
                fontWeight: 500,
                letterSpacing: 1.5,
                textTransform: 'uppercase',
                fontFamily: 'inherit',
                transition: 'color 0.2s, border-color 0.2s',
              }}
            >
              {t(lang, `filter.${tb}`)}
            </button>
          );
        })}
      </div>

      {/* Content area */}
      <div style={{
        flex: 1,
        overflowY: 'auto',
        padding: '8px 0',
      }}>
        {tab === 'nodes' && (
          <>
            <div style={{
              display: 'flex',
              gap: 4,
              padding: '0 10px 6px',
              borderBottom: `1px solid ${theme.border}40`,
              marginBottom: 4,
            }}>
              <button
                onClick={() => onChange({ ...filter, hiddenNodes: new Set() })}
                style={{
                  background: allVisible ? theme.gold + '18' : 'transparent',
                  border: `1px solid ${allVisible ? theme.gold + '40' : theme.border}`,
                  borderRadius: 3,
                  padding: '2px 8px',
                  cursor: 'pointer',
                  color: allVisible ? theme.gold : theme.textDim,
                  fontSize: 9,
                  letterSpacing: 0.5,
                  fontFamily: 'inherit',
                }}
              >
                {t(lang, 'filter.all')}
              </button>
              <button
                onClick={() => {
                  onChange({ ...filter, hiddenNodes: new Set(agents.map(a => a.address)) });
                }}
                style={{
                  background: noneVisible ? theme.gold + '18' : 'transparent',
                  border: `1px solid ${noneVisible ? theme.gold + '40' : theme.border}`,
                  borderRadius: 3,
                  padding: '2px 8px',
                  cursor: 'pointer',
                  color: noneVisible ? theme.gold : theme.textDim,
                  fontSize: 9,
                  letterSpacing: 0.5,
                  fontFamily: 'inherit',
                }}
              >
                {t(lang, 'filter.none')}
              </button>
            </div>

            {agents.map(a => {
              const visible = !filter.hiddenNodes.has(a.address);
              const baseName = a.nickname || a.agent_name || a.address.split('/').pop() || '?';
              const stateColor = a.is_human
                ? theme.text
                : (theme.stateColors[(a.state || '').toUpperCase()] || theme.stateColors['']);

              return (
                <div
                  key={a.address}
                  onClick={() => toggleNode(a.address)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    padding: '5px 10px',
                    cursor: 'pointer',
                    opacity: visible ? 1 : 0.35,
                    transition: 'opacity 0.15s, background 0.15s',
                    borderRadius: 2,
                  }}
                  onMouseEnter={e => { (e.currentTarget as HTMLDivElement).style.background = theme.textDim + '0a'; }}
                  onMouseLeave={e => { (e.currentTarget as HTMLDivElement).style.background = 'transparent'; }}
                >
                  <span style={{
                    width: 6,
                    height: 6,
                    borderRadius: '50%',
                    background: stateColor,
                    flexShrink: 0,
                    boxShadow: visible ? `0 0 3px ${stateColor}50` : 'none',
                    transition: 'box-shadow 0.2s',
                  }} />
                  <span style={{
                    fontSize: 10,
                    color: visible ? theme.text : theme.textDim,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    flex: 1,
                    fontFamily: a.is_human ? 'inherit' : "'SF Mono', 'Menlo', monospace",
                    letterSpacing: a.is_human ? 0.5 : 0,
                  }}>
                    {baseName}
                  </span>
                  {a.is_human && (
                    <span style={{
                      fontSize: 8,
                      color: theme.gold,
                      border: `1px solid ${theme.gold}40`,
                      borderRadius: 2,
                      padding: '0 3px',
                      letterSpacing: 0.3,
                      flexShrink: 0,
                    }}>
                      {t(lang, 'filter.human')}
                    </span>
                  )}
                  {adminAddrs.has(a.address) && (
                    <span style={{
                      fontSize: 8,
                      color: theme.gold,
                      border: `1px solid ${theme.gold}40`,
                      borderRadius: 2,
                      padding: '0 3px',
                      letterSpacing: 0.3,
                      flexShrink: 0,
                    }}>
                      {t(lang, 'filter.admin')}
                    </span>
                  )}
                  <Toggle on={visible} color={stateColor} onChange={() => {}} />
                </div>
              );
            })}

            {/* Avatar tree */}
            <div style={{
              borderTop: `1px solid ${theme.border}40`,
              marginTop: 6,
              padding: '8px 10px 4px',
            }}>
              <div style={{
                fontSize: 9,
                color: theme.textDim + '80',
                letterSpacing: 1,
                textTransform: 'uppercase',
                marginBottom: 6,
              }}>
                {t(lang, 'filter.tree')}
              </div>
              <AvatarTree network={network} theme={theme} />
            </div>
          </>
        )}

        {tab === 'mail' && (
          <div style={{ padding: '4px 10px' }}>
            <p style={{
              fontSize: 9,
              color: theme.textDim,
              margin: '0 0 10px',
              lineHeight: 1.5,
              fontStyle: 'italic',
            }}>
              {t(lang, 'filter.mail_desc')}
            </p>

            {([
              { key: 'showDirect' as const, labelKey: 'filter.direct', subKey: 'filter.direct_sub' },
              { key: 'showCC' as const, labelKey: 'filter.cc', subKey: 'filter.cc_sub' },
              { key: 'showBCC' as const, labelKey: 'filter.bcc', subKey: 'filter.bcc_sub' },
            ]).map(({ key, labelKey, subKey }) => (
              <div
                key={key}
                onClick={() => onChange({ ...filter, [key]: !filter[key] })}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  padding: '8px 0',
                  cursor: 'pointer',
                  borderBottom: `1px solid ${theme.border}30`,
                }}
              >
                <Toggle on={filter[key]} color={theme.edgeColors.mail} onChange={() => {}} />
                <div style={{ flex: 1 }}>
                  <div style={{
                    fontSize: 11,
                    color: filter[key] ? theme.text : theme.textDim + '66',
                    fontWeight: 500,
                    transition: 'color 0.2s',
                  }}>
                    {t(lang, labelKey)}
                  </div>
                  <div style={{
                    fontSize: 8,
                    color: theme.textDim + '80',
                    marginTop: 1,
                    letterSpacing: 0.3,
                  }}>
                    {t(lang, subKey)}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** Simple indented avatar tree showing parent→child relationships. */
function AvatarTree({ network, theme }: { network: Network; theme: Theme }) {
  const nodes = network.nodes || [];
  const edges = network.avatar_edges || [];
  if (nodes.length === 0) return null;

  const nodeMap = new Map<string, AgentNode>();
  for (const n of nodes) nodeMap.set(n.address, n);

  // Build children map
  const childrenOf = new Map<string, string[]>();
  const childSet = new Set<string>();
  for (const e of edges) {
    const list = childrenOf.get(e.parent) || [];
    list.push(e.child);
    childrenOf.set(e.parent, list);
    childSet.add(e.child);
  }

  // Roots: human first, then admins (no parent), then orphans
  const human = nodes.find(n => n.is_human);
  const admins = nodes.filter(n => !n.is_human && !childSet.has(n.address));
  const roots: AgentNode[] = [];
  if (human) roots.push(human);
  for (const a of admins) roots.push(a);

  function nameOf(n: AgentNode): string {
    return n.nickname || n.agent_name || n.address.split('/').pop() || '?';
  }

  function colorOf(n: AgentNode): string {
    if (n.is_human) return theme.text;
    return theme.stateColors[(n.state || '').toUpperCase()] || theme.stateColors[''];
  }

  function renderNode(addr: string, prefix: string, isLast: boolean, isRoot: boolean): React.ReactElement[] {
    const node = nodeMap.get(addr);
    if (!node) return [];

    const connector = isRoot ? '' : (isLast ? '└ ' : '├ ');
    const color = colorOf(node);
    const elements: React.ReactElement[] = [
      <div key={addr} style={{
        fontSize: 9,
        lineHeight: '16px',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        color,
      }}>
        <span style={{ color: theme.textDim + '50' }}>{prefix}{connector}</span>
        <span style={{
          fontFamily: node.is_human ? 'inherit' : "'SF Mono', 'Menlo', monospace",
        }}>
          {nameOf(node)}
        </span>
      </div>,
    ];

    const children = childrenOf.get(addr) || [];
    const childPrefix = isRoot ? '' : (prefix + (isLast ? '  ' : '│ '));
    for (let i = 0; i < children.length; i++) {
      elements.push(...renderNode(children[i], childPrefix, i === children.length - 1, false));
    }
    return elements;
  }

  const elements: React.ReactElement[] = [];
  for (let i = 0; i < roots.length; i++) {
    elements.push(...renderNode(roots[i].address, '', i === roots.length - 1, true));
  }

  return <>{elements}</>;
}
