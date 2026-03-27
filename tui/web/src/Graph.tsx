import { useMemo } from 'react';
import ForceGraph2D from 'react-force-graph-2d';
import type { Network } from './types';
import { inkStateColors, inkEdgeColors, inkBg, inkNodeTypeColors } from './theme';

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

  // 静态分层圆盘布局
  const positions = useMemo(() => {
    const pos: Record<string, { x: number; y: number }> = {};
    const W = 800, H = 600, cx = W / 2, cy = H / 2;

    // 找 orchestrator
    const orch = graphData.nodes.find(n => n.isOrchestrator);
    if (orch) pos[orch.id] = { x: cx, y: cy };

    // 分类 avatar / human
    const avatars = graphData.nodes.filter(n => !n.isOrchestrator && !n.isHuman);
    const humans = graphData.nodes.filter(n => n.isHuman);

    // avatar 圆周（r=120）
    avatars.forEach((n, i) => {
      const a = (2 * Math.PI * i) / avatars.length - Math.PI / 2;
      pos[n.id] = { x: cx + 120 * Math.cos(a), y: cy + 120 * Math.sin(a) };
    });

    // human 外圈（r=220）
    humans.forEach((n, i) => {
      const a = (2 * Math.PI * i) / (humans.length || 1) - Math.PI / 2;
      pos[n.id] = { x: cx + 220 * Math.cos(a), y: cy + 220 * Math.sin(a) };
    });

    return pos;
  }, [graphData]);

  return (
    <ForceGraph2D
      graphData={graphData}
      backgroundColor={inkBg}
      nodeRelSize={6}
      enableNodeDrag={false}
      d3AlphaDecay={1}
      // @ts-ignore
      nodePosition={node => positions[node.id] || { x: 0, y: 0 }}
      // @ts-ignore
      nodeCanvasObject={(node: any, ctx) => {
        const n = node as GraphNode;
        const r = n.isOrchestrator ? 12 : n.isHuman ? 10 : 8;
        const color = inkStateColors[n.state] || inkStateColors[''];

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

        // 所有节点都标 name，orchestrator 标琥珀色加粗，其余竹青色，9px
        ctx.font = `${n.isOrchestrator ? 'bold 11px' : '9px'} sans-serif`;
        ctx.textAlign = 'center';
        ctx.fillStyle = n.isOrchestrator ? inkNodeTypeColors.orchestrator : inkNodeTypeColors.avatar;
        ctx.fillText(n.name, node.x!, node.y! + r + 12);
      }}
      linkColor={(link: any) => {
        const l = link as GraphLink;
        return l.type === 'avatar' ? inkEdgeColors.avatar : inkEdgeColors.mail;
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
