import { useMemo } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import type { Network } from './types';
import { stateColors, edgeColors } from './theme';

interface GraphNode {
  id: string;
  name: string;
  state: string;
  alive: boolean;
  isHuman: boolean;
  isOrchestrator: boolean;
}

interface GraphLink {
  source: string;
  target: string;
  type: 'avatar' | 'mail';
  count?: number;
}

export function Graph({ network }: { network: Network }) {
  const graphData = useMemo(() => {
    const nodes: GraphNode[] = network.nodes.map((n, i) => ({
      id: n.address,
      name: n.agent_name || n.address.split('/').pop() || 'unknown',
      state: n.state,
      alive: n.alive,
      isHuman: n.is_human,
      isOrchestrator: i === 0 && !n.is_human,
    }));

    const links: GraphLink[] = [
      ...network.avatar_edges.map(e => ({
        source: e.parent,
        target: e.child,
        type: 'avatar' as const,
      })),
      ...network.mail_edges.map(e => ({
        source: e.sender,
        target: e.recipient,
        type: 'mail' as const,
        count: e.count,
      })),
    ];

    return { nodes, links };
  }, [network]);

  return (
    <ForceGraph2D
      graphData={graphData}
      backgroundColor="#0a0a1a"
      nodeRelSize={6}
      // @ts-ignore
      nodeCanvasObject={(node: any, ctx) => {
        const n = node as GraphNode;
        const r = n.isOrchestrator ? 12 : n.isHuman ? 10 : 8;
        const color = stateColors[n.state] || stateColors[''];

        if (n.state === 'ACTIVE') {
          ctx.beginPath();
          ctx.arc(node.x!, node.y!, r + 4, 0, 2 * Math.PI);
          ctx.fillStyle = color + '22';
          ctx.fill();
        }

        ctx.beginPath();
        ctx.arc(node.x!, node.y!, r, 0, 2 * Math.PI);
        ctx.strokeStyle = color;
        ctx.lineWidth = n.isOrchestrator ? 2.5 : 1.5;
        ctx.stroke();

        ctx.font = `${n.isOrchestrator ? 11 : 9}px sans-serif`;
        ctx.textAlign = 'center';
        ctx.fillStyle = color;
        ctx.fillText(n.name, node.x!, node.y! + r + 12);
      }}
      linkColor={(link: any) => {
        const l = link as GraphLink;
        return l.type === 'avatar' ? edgeColors.avatar : edgeColors.mail;
      }}
      linkWidth={(link: any) => {
        const l = link as GraphLink;
        return l.type === 'mail' ? Math.min((l.count || 1) * 0.5, 4) : 1;
      }}
      linkLineDash={(link: any) => {
        const l = link as GraphLink;
        return l.type === 'avatar' ? [4, 4] : null;
      }}
    />
  );
}
