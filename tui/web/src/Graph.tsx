import { useEffect, useRef, useCallback } from 'react';
import type { Network } from './types';
import { inkStateColors, inkBg, amberRgb } from './theme';

export type EdgeMode = 'avatar' | 'email';

const NODE_R = 4;
const HIT_R = 15;
const GREEN = [125, 171, 143];
const STORAGE_KEY = 'lingtai-viz-positions';

function rgba(r: number, g: number, b: number, a: number) {
  return `rgba(${r},${g},${b},${a})`;
}

function hexRgb(hex: string): [number, number, number] {
  const h = hex.replace('#', '');
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}

interface Dot {
  id: string;
  name: string;
  state: string;
  alive: boolean;
  isHuman: boolean;
  x: number;
  y: number;
}

function loadPositions(): Record<string, { x: number; y: number }> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function savePositions(dots: Map<string, Dot>) {
  try {
    const pos: Record<string, { x: number; y: number }> = {};
    for (const [id, d] of dots) {
      pos[id] = { x: d.x, y: d.y };
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(pos));
  } catch { /* storage full or unavailable */ }
}

/** Compute deterministic positions based on avatar tree structure. */
function computeLayout(network: Network, W: number, H: number): Record<string, { x: number; y: number }> {
  const pos: Record<string, { x: number; y: number }> = {};
  const nodes = network.nodes;
  if (nodes.length === 0) return pos;

  // Identify human and avatar children
  const human = nodes.find(n => n.is_human);
  const childSet = new Set(network.avatar_edges.map(e => e.child));

  // Admin = non-human nodes not in avatar children set
  const admins = nodes.filter(n => !n.is_human && !childSet.has(n.address));

  // Build parent → children map
  const childrenOf = new Map<string, string[]>();
  for (const e of network.avatar_edges) {
    const list = childrenOf.get(e.parent) || [];
    list.push(e.child);
    childrenOf.set(e.parent, list);
  }

  const cy = H * 0.5;
  const LEVEL_DX = W * 0.12; // uniform horizontal spacing per depth level

  // Place human on the far left
  if (human) {
    pos[human.address] = { x: W * 0.20, y: cy };
  }

  // Admin at ~1/3 from left — if multiple, spread vertically
  const adminX = W / 3;
  const adminSpacing = Math.min(80, H * 0.3);
  const adminStartY = cy - ((admins.length - 1) * adminSpacing) / 2;
  for (let i = 0; i < admins.length; i++) {
    pos[admins[i].address] = { x: adminX, y: adminStartY + i * adminSpacing };
  }

  // Walk avatar tree left-to-right: children placed one LEVEL_DX to the right, spread vertically
  const placed = new Set(Object.keys(pos));

  function placeChildren(parentAddr: string, depth: number) {
    const kids = childrenOf.get(parentAddr);
    if (!kids || kids.length === 0) return;
    const parent = pos[parentAddr];
    if (!parent) return;

    const childX = parent.x + LEVEL_DX;
    const spread = Math.min(H * 0.6, kids.length * 50);
    const startY = parent.y - spread / 2;

    for (let i = 0; i < kids.length; i++) {
      pos[kids[i]] = {
        x: childX,
        y: kids.length === 1 ? parent.y : startY + (i / (kids.length - 1)) * spread,
      };
      placed.add(kids[i]);
      placeChildren(kids[i], depth + 1);
    }
  }

  // Start tree walk from each admin
  for (const admin of admins) {
    placeChildren(admin.address, 1);
  }

  // Orphans — place to the right of admins, spread vertically
  const orphans = nodes.filter(n => !placed.has(n.address));
  if (orphans.length > 0) {
    const orphanX = adminX + LEVEL_DX;
    const spread = Math.min(H * 0.6, orphans.length * 50);
    const startY = cy - spread / 2;
    for (let i = 0; i < orphans.length; i++) {
      pos[orphans[i].address] = {
        x: orphanX,
        y: orphans.length === 1 ? cy : startY + (i / (orphans.length - 1)) * spread,
      };
    }
  }

  return pos;
}

function findNearest(dots: Map<string, Dot>, mx: number, my: number): Dot | null {
  let best: Dot | null = null;
  let bestDist = HIT_R * HIT_R;
  for (const d of dots.values()) {
    const dx = d.x - mx;
    const dy = d.y - my;
    const dist = dx * dx + dy * dy;
    if (dist < bestDist) {
      bestDist = dist;
      best = d;
    }
  }
  return best;
}

export function Graph({ network, edgeMode }: { network: Network; edgeMode: EdgeMode }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const dotsRef = useRef<Map<string, Dot>>(new Map());
  const networkRef = useRef(network);
  const edgeModeRef = useRef(edgeMode);
  const animRef = useRef(0);
  const grabbedRef = useRef<Dot | null>(null);

  networkRef.current = network;
  edgeModeRef.current = edgeMode;

  // Sync dots with network — preserve positions for existing nodes
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const W = canvas.clientWidth || 800;
    const H = canvas.clientHeight || 600;
    const old = dotsRef.current;
    const next = new Map<string, Dot>();
    const stored = loadPositions();
    const layout = computeLayout(network, W, H);

    for (const n of network.nodes) {
      const prev = old.get(n.address);
      if (prev) {
        prev.name = n.nickname || n.agent_name || n.address.split('/').pop() || '?';
        prev.state = n.state;
        prev.alive = n.alive;
        prev.isHuman = n.is_human;
        next.set(n.address, prev);
      } else {
        // localStorage overrides computed layout
        const sp = stored[n.address];
        const lp = layout[n.address];
        next.set(n.address, {
          id: n.address,
          name: n.nickname || n.agent_name || n.address.split('/').pop() || '?',
          state: n.state,
          alive: n.alive,
          isHuman: n.is_human,
          x: sp ? sp.x : lp ? lp.x : W * 0.5,
          y: sp ? sp.y : lp ? lp.y : H * 0.5,
        });
      }
    }
    dotsRef.current = next;
  }, [network]);

  // Drag handlers
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const onMouseDown = (e: MouseEvent) => {
      const dot = findNearest(dotsRef.current, e.offsetX, e.offsetY);
      if (dot) {
        grabbedRef.current = dot;
        canvas.style.cursor = 'grabbing';
        e.preventDefault();
      }
    };

    const onMouseMove = (e: MouseEvent) => {
      const grabbed = grabbedRef.current;
      if (grabbed) {
        grabbed.x = e.offsetX;
        grabbed.y = e.offsetY;
      } else {
        const hover = findNearest(dotsRef.current, e.offsetX, e.offsetY);
        canvas.style.cursor = hover ? 'grab' : 'default';
      }
    };

    const onMouseUp = () => {
      if (grabbedRef.current) {
        grabbedRef.current = null;
        canvas.style.cursor = 'default';
        savePositions(dotsRef.current);
      }
    };

    canvas.addEventListener('mousedown', onMouseDown);
    canvas.addEventListener('mousemove', onMouseMove);
    canvas.addEventListener('mouseup', onMouseUp);
    canvas.addEventListener('mouseleave', onMouseUp);

    return () => {
      canvas.removeEventListener('mousedown', onMouseDown);
      canvas.removeEventListener('mousemove', onMouseMove);
      canvas.removeEventListener('mouseup', onMouseUp);
      canvas.removeEventListener('mouseleave', onMouseUp);
    };
  }, []);

  const draw = useCallback((now: number) => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const W = canvas.clientWidth;
    const H = canvas.clientHeight;
    if (canvas.width !== W * dpr || canvas.height !== H * dpr) {
      canvas.width = W * dpr;
      canvas.height = H * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }
    ctx.clearRect(0, 0, W, H);

    const dots = dotsRef.current;
    const net = networkRef.current;
    const mode = edgeModeRef.current;

    // Heartbeat pulse: 1s cycle, sin wave 0→1→0
    const pulse = 0.5 + 0.5 * Math.sin(now * Math.PI * 2 / 1000);

    // Edges
    const edges: Array<{ src: string; tgt: string; weight: number }> = [];
    if (mode === 'avatar') {
      for (const e of net.avatar_edges)
        edges.push({ src: e.parent, tgt: e.child, weight: 1 });
      // Human → 本我: connect to non-human nodes that aren't avatar children
      const human = net.nodes.find(n => n.is_human);
      if (human) {
        const children = new Set(edges.map(e => e.tgt));
        for (const n of net.nodes)
          if (!n.is_human && !children.has(n.address))
            edges.push({ src: human.address, tgt: n.address, weight: 1 });
      }
    } else {
      for (const e of net.mail_edges)
        edges.push({ src: e.sender, tgt: e.recipient, weight: e.count });
      for (const e of net.contact_edges)
        if (!edges.some(x => x.src === e.owner && x.tgt === e.target))
          edges.push({ src: e.owner, tgt: e.target, weight: 0 });
    }

    for (const e of edges) {
      const a = dots.get(e.src);
      const b = dots.get(e.tgt);
      if (!a || !b) continue;

      const isAvatar = mode === 'avatar';
      const rgb = isAvatar ? amberRgb : GREEN;

      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.strokeStyle = rgba(rgb[0], rgb[1], rgb[2], 0.35);
      ctx.lineWidth = isAvatar ? 0.8 : Math.min(0.8 + (e.weight || 1) * 0.3, 2.5);
      ctx.setLineDash(isAvatar ? [] : [4, 4]);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    // Dots + labels
    const HUMAN_RGB: [number, number, number] = [232, 228, 223]; // 宣纸白
    for (const d of dots.values()) {
      const isAgent = !d.isHuman;
      // Agent dot color = state color; human = 宣纸白
      const stateHex = isAgent ? (inkStateColors[d.state] || inkStateColors['']) : '';
      const [dr, dg, db] = d.isHuman ? HUMAN_RGB : hexRgb(stateHex);

      // ACTIVE halo (agents only)
      if (isAgent && d.state === 'ACTIVE') {
        ctx.beginPath();
        ctx.arc(d.x, d.y, NODE_R + 5, 0, Math.PI * 2);
        ctx.fillStyle = rgba(dr, dg, db, 0.15);
        ctx.fill();
      }

      // Heartbeat glow — pulses only for alive agents (not humans)
      const beats = isAgent && d.alive;
      const glowAlpha = beats ? 0.04 + pulse * 0.08 : 0.06;
      const glowR = beats ? NODE_R * 3 + pulse * 4 : NODE_R * 3;
      ctx.beginPath();
      ctx.arc(d.x, d.y, glowR, 0, Math.PI * 2);
      ctx.fillStyle = rgba(dr, dg, db, glowAlpha);
      ctx.fill();

      // Dot
      ctx.beginPath();
      ctx.arc(d.x, d.y, d.isHuman ? NODE_R + 1 : NODE_R, 0, Math.PI * 2);
      ctx.fillStyle = rgba(dr, dg, db, 0.85);
      ctx.fill();

      // Name label
      ctx.font = '9px sans-serif';
      ctx.textAlign = 'center';
      ctx.fillStyle = rgba(dr, dg, db, 0.55);
      ctx.fillText(d.name, d.x, d.y + NODE_R + 12);
    }

    animRef.current = requestAnimationFrame(draw);
  }, []);

  useEffect(() => {
    animRef.current = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(animRef.current);
  }, [draw]);

  return (
    <canvas
      ref={canvasRef}
      style={{ width: '100%', height: '100%', background: inkBg, display: 'block' }}
    />
  );
}
