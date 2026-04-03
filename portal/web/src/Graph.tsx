import { useEffect, useRef, useCallback } from 'react';
import type { Network } from './types';
import type { Theme } from './theme';

export type EdgeMode = 'avatar' | 'email';

const NODE_R = 4;
const HIT_R = 15;
const STORAGE_KEY = 'lingtai-viz-positions';

// Bullet travel time in ms
const BULLET_DURATION = 800;
// Impact expand + fade time in ms
const IMPACT_DURATION = 600;

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

/** Normalize state to uppercase for theme/color matching (kernel writes lowercase). */
function normState(s: string): string { return s ? s.toUpperCase() : ''; }

/** A bullet flying from sender → recipient. */
export interface Bullet {
  src: string;   // sender address (resolve to dot position at draw time)
  dst: string;   // recipient address
  born: number;  // timestamp when spawned (ms, performance.now scale)
}

/** An impact burst at the recipient after bullet arrives. */
interface Impact {
  x: number;
  y: number;
  born: number;
  rgb: [number, number, number];
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

  const human = nodes.find(n => n.is_human);
  const childSet = new Set(network.avatar_edges.map(e => e.child));
  const admins = nodes.filter(n => !n.is_human && !childSet.has(n.address));

  const childrenOf = new Map<string, string[]>();
  for (const e of network.avatar_edges) {
    const list = childrenOf.get(e.parent) || [];
    list.push(e.child);
    childrenOf.set(e.parent, list);
  }

  const cy = H * 0.5;
  const LEVEL_DX = W * 0.12;

  if (human) {
    pos[human.address] = { x: W * 0.20, y: cy };
  }

  const adminX = W / 3;
  const adminSpacing = Math.min(80, H * 0.3);
  const adminStartY = cy - ((admins.length - 1) * adminSpacing) / 2;
  for (let i = 0; i < admins.length; i++) {
    pos[admins[i].address] = { x: adminX, y: adminStartY + i * adminSpacing };
  }

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

  for (const admin of admins) {
    placeChildren(admin.address, 1);
  }

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

export function Graph({ network, edgeMode, theme, bullets, vizMode, showNames = true, filter }: {
  network: Network;
  edgeMode: EdgeMode;
  theme: Theme;
  bullets: Bullet[];
  vizMode?: 'live' | 'replay';
  showNames?: boolean;
  filter?: { hiddenNodes: Set<string>; showDirect: boolean; showCC: boolean; showBCC: boolean };
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const dotsRef = useRef<Map<string, Dot>>(new Map());
  const networkRef = useRef(network);
  const edgeModeRef = useRef(edgeMode);
  const themeRef = useRef(theme);
  const vizModeRef = useRef(vizMode ?? 'live');
  const showNamesRef = useRef(showNames);
  const filterRef = useRef(filter);
  const animRef = useRef(0);
  const grabbedRef = useRef<Dot | null>(null);

  // Mutable particle arrays — written by draw loop, not React state
  const bulletsRef = useRef<Bullet[]>([]);
  const impactsRef = useRef<Impact[]>([]);

  // Pan state
  const panRef = useRef({ x: 0, y: 0 });
  const panningRef = useRef<{ startX: number; startY: number; panStartX: number; panStartY: number } | null>(null);

  networkRef.current = network;
  edgeModeRef.current = edgeMode;
  themeRef.current = theme;
  vizModeRef.current = vizMode ?? 'live';
  showNamesRef.current = showNames;
  filterRef.current = filter;

  // Absorb new bullets from props into mutable ref
  useEffect(() => {
    if (bullets.length > 0) {
      bulletsRef.current.push(...bullets);
    }
  }, [bullets]);

  // Sync dots with network
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const W = canvas.clientWidth || 800;
    const H = canvas.clientHeight || 600;
    const old = dotsRef.current;
    const isReplay = vizModeRef.current === 'replay';

    // In replay mode, keep all existing dots (frozen positions) and just
    // update state/visibility. activeAddresses tracks who is "on screen".
    const activeAddresses = new Set(network.nodes.map(n => n.address));

    if (isReplay) {
      // Update existing dots and create new ones for nodes not yet seen.
      // Layout and stored positions are computed lazily — only when a
      // genuinely new node appears — to avoid the cost on most frames.
      const stored = loadPositions();
      let layout: Record<string, { x: number; y: number }> | null = null;

      for (const n of network.nodes) {
        const prev = old.get(n.address);
        if (prev) {
          prev.name = n.nickname || n.agent_name || n.address.split('/').pop() || '?';
          prev.state = normState(n.state);
          prev.alive = n.alive;
          prev.isHuman = n.is_human;
        } else {
          // Node appeared in a tape frame but wasn't in dotsRef yet — create it
          if (!layout) layout = computeLayout(network, W, H);
          const sp = stored[n.address];
          const lp = layout[n.address];
          old.set(n.address, {
            id: n.address,
            name: n.nickname || n.agent_name || n.address.split('/').pop() || '?',
            state: normState(n.state),
            alive: n.alive,
            isHuman: n.is_human,
            x: sp ? sp.x : lp ? lp.x : W * 0.5,
            y: sp ? sp.y : lp ? lp.y : H * 0.5,
          });
        }
      }
      // Mark dots not in this frame as hidden
      for (const d of old.values()) {
        if (!activeAddresses.has(d.id)) {
          d.state = '__hidden__';
        }
      }
    } else {
      // Live mode: full sync as before
      const next = new Map<string, Dot>();
      const stored = loadPositions();
      const layout = computeLayout(network, W, H);

      for (const n of network.nodes) {
        const prev = old.get(n.address);
        if (prev) {
          prev.name = n.nickname || n.agent_name || n.address.split('/').pop() || '?';
          prev.state = normState(n.state);
          prev.alive = n.alive;
          prev.isHuman = n.is_human;
          next.set(n.address, prev);
        } else {
          const sp = stored[n.address];
          const lp = layout[n.address];
          next.set(n.address, {
            id: n.address,
            name: n.nickname || n.agent_name || n.address.split('/').pop() || '?',
            state: normState(n.state),
            alive: n.alive,
            isHuman: n.is_human,
            x: sp ? sp.x : lp ? lp.x : W * 0.5,
            y: sp ? sp.y : lp ? lp.y : H * 0.5,
          });
        }
      }
      dotsRef.current = next;
    }
  }, [network]);

  const screenToWorld = useCallback((sx: number, sy: number) => {
    const pan = panRef.current;
    return { x: sx - pan.x, y: sy - pan.y };
  }, []);

  // Drag & pan handlers
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const onMouseDown = (e: MouseEvent) => {
      const world = screenToWorld(e.offsetX, e.offsetY);
      const dot = findNearest(dotsRef.current, world.x, world.y);
      if (dot) {
        grabbedRef.current = dot;
        canvas.style.cursor = 'grabbing';
      } else {
        panningRef.current = {
          startX: e.offsetX,
          startY: e.offsetY,
          panStartX: panRef.current.x,
          panStartY: panRef.current.y,
        };
        canvas.style.cursor = 'move';
      }
      e.preventDefault();
    };

    const onMouseMove = (e: MouseEvent) => {
      const grabbed = grabbedRef.current;
      const panning = panningRef.current;
      if (grabbed) {
        const world = screenToWorld(e.offsetX, e.offsetY);
        grabbed.x = world.x;
        grabbed.y = world.y;
      } else if (panning) {
        panRef.current = {
          x: panning.panStartX + (e.offsetX - panning.startX),
          y: panning.panStartY + (e.offsetY - panning.startY),
        };
      } else {
        const world = screenToWorld(e.offsetX, e.offsetY);
        const hover = findNearest(dotsRef.current, world.x, world.y);
        canvas.style.cursor = hover ? 'grab' : 'default';
      }
    };

    const onMouseUp = () => {
      if (grabbedRef.current) {
        grabbedRef.current = null;
        canvas.style.cursor = 'default';
        savePositions(dotsRef.current);
      }
      if (panningRef.current) {
        panningRef.current = null;
        canvas.style.cursor = 'default';
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
  }, [screenToWorld]);

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
    }

    const th = themeRef.current;
    const pan = panRef.current;

    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);

    ctx.save();
    ctx.translate(pan.x, pan.y);

    const dots = dotsRef.current;
    const net = networkRef.current;
    const mode = edgeModeRef.current;
    const filt = filterRef.current;
    const hidden = filt?.hiddenNodes ?? new Set<string>();

    const pulse = 0.5 + 0.5 * Math.sin(now * Math.PI * 2 / 1000);

    // ── Edges ──────────────────────────────────────────────
    const edges: Array<{ src: string; tgt: string; weight: number }> = [];
    if (mode === 'avatar') {
      for (const e of (net.avatar_edges || []))
        edges.push({ src: e.parent, tgt: e.child, weight: 1 });
      const human = (net.nodes || []).find(n => n.is_human);
      if (human) {
        const children = new Set(edges.map(e => e.tgt));
        for (const n of (net.nodes || []))
          if (!n.is_human && !children.has(n.address))
            edges.push({ src: human.address, tgt: n.address, weight: 1 });
      }
    } else {
      const sd = filt?.showDirect ?? true;
      const sc = filt?.showCC ?? true;
      const sb = filt?.showBCC ?? true;
      for (const e of (net.mail_edges || [])) {
        const w = (sd ? (e.direct || 0) : 0) + (sc ? (e.cc || 0) : 0) + (sb ? (e.bcc || 0) : 0);
        if (w > 0) edges.push({ src: e.sender, tgt: e.recipient, weight: w });
      }
      for (const e of (net.contact_edges || []))
        if (!edges.some(x => x.src === e.owner && x.tgt === e.target))
          edges.push({ src: e.owner, tgt: e.target, weight: 0 });
    }

    for (const e of edges) {
      const a = dots.get(e.src);
      const b = dots.get(e.tgt);
      if (!a || !b) continue;
      if (a.state === '__hidden__' || b.state === '__hidden__') continue;
      if (hidden.has(e.src) || hidden.has(e.tgt)) continue;

      const isAvatar = mode === 'avatar';
      const rgb = isAvatar ? th.amberRgb : hexRgb(th.edgeColors.mail);

      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.strokeStyle = rgba(rgb[0], rgb[1], rgb[2], th.edgeOpacity);
      ctx.lineWidth = isAvatar ? 0.8 : Math.min(0.8 + (e.weight || 1) * 0.5, 5);
      ctx.setLineDash(isAvatar ? [] : [4, 4]);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    // ── Bullets (flying mail) ──────────────────────────────
    const mailRgb = hexRgb(th.edgeColors.mail);
    const liveBullets: Bullet[] = [];
    for (const b of bulletsRef.current) {
      const age = now - b.born;
      if (age < 0) {
        // Not yet born (staggered delay) — keep it
        liveBullets.push(b);
        continue;
      }
      const t = age / BULLET_DURATION;
      if (t > 1) {
        // Arrived — spawn impact at destination
        const dst = dots.get(b.dst);
        if (dst) {
          impactsRef.current.push({ x: dst.x, y: dst.y, born: now, rgb: mailRgb });
        }
        continue; // remove bullet
      }

      const src = dots.get(b.src);
      const dst = dots.get(b.dst);
      if (!src || !dst) continue;

      // Ease-out for deceleration feel
      const ease = 1 - (1 - t) * (1 - t);
      const bx = src.x + (dst.x - src.x) * ease;
      const by = src.y + (dst.y - src.y) * ease;

      // Bullet dot — bright, small
      ctx.beginPath();
      ctx.arc(bx, by, 2.5, 0, Math.PI * 2);
      ctx.fillStyle = rgba(mailRgb[0], mailRgb[1], mailRgb[2], 0.9);
      ctx.fill();

      // Bullet trail — short fading tail
      const tailT = Math.max(0, ease - 0.08);
      const tx = src.x + (dst.x - src.x) * tailT;
      const ty = src.y + (dst.y - src.y) * tailT;
      ctx.beginPath();
      ctx.moveTo(tx, ty);
      ctx.lineTo(bx, by);
      ctx.strokeStyle = rgba(mailRgb[0], mailRgb[1], mailRgb[2], 0.4);
      ctx.lineWidth = 1.5;
      ctx.stroke();

      liveBullets.push(b);
    }
    bulletsRef.current = liveBullets;

    // ── Impacts (arrival bursts) ───────────────────────────
    const liveImpacts: Impact[] = [];
    for (const imp of impactsRef.current) {
      const age = now - imp.born;
      const t = age / IMPACT_DURATION;
      if (t > 1) continue; // expired

      const radius = 3 + t * 10;
      const alpha = 0.6 * (1 - t);
      ctx.beginPath();
      ctx.arc(imp.x, imp.y, radius, 0, Math.PI * 2);
      ctx.fillStyle = rgba(imp.rgb[0], imp.rgb[1], imp.rgb[2], alpha);
      ctx.fill();

      liveImpacts.push(imp);
    }
    impactsRef.current = liveImpacts;

    // ── Dots + labels ──────────────────────────────────────
    const humanRgb: [number, number, number] = hexRgb(th.text);
    for (const d of dots.values()) {
      if (d.state === '__hidden__') continue; // not in current replay frame
      if (hidden.has(d.id)) continue; // filtered out by user
      const isAgent = !d.isHuman;
      const stateHex = isAgent ? (th.stateColors[d.state] || th.stateColors['']) : '';
      const [dr, dg, db] = d.isHuman ? humanRgb : hexRgb(stateHex);

      // ACTIVE halo
      if (isAgent && d.state === 'ACTIVE') {
        ctx.beginPath();
        ctx.arc(d.x, d.y, NODE_R + 5, 0, Math.PI * 2);
        ctx.fillStyle = rgba(dr, dg, db, 0.15);
        ctx.fill();
      }

      // Heartbeat glow
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
      if (!showNamesRef.current) continue;
      const [lr, lg, lb] = th.labelColorRgb;
      ctx.font = '10px sans-serif';
      ctx.textAlign = 'center';
      ctx.fillStyle = rgba(lr, lg, lb, 0.9);
      ctx.fillText(d.name, d.x, d.y + NODE_R + 13);
    }

    ctx.restore();

    animRef.current = requestAnimationFrame(draw);
  }, []);

  useEffect(() => {
    animRef.current = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(animRef.current);
  }, [draw]);

  return (
    <canvas
      ref={canvasRef}
      style={{ width: '100%', height: '100%', background: theme.bg, display: 'block' }}
    />
  );
}
