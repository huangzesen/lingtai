import { useEffect, useState, useRef, useCallback } from 'react';
import type { Network } from './types';
import { fetchNetwork, type TapeFrame, fetchManifest, fetchChunk, reconstructFrames, type ReplayManifest, type ChunkInfo } from './api';
import { Graph, type EdgeMode, type Bullet } from './Graph';
import { TopBar } from './TopBar';
import { BottomBar } from './BottomBar';
import { getTheme, loadThemePreference, saveThemePreference } from './theme';
import { FilterPanel, defaultFilter, type FilterState } from './FilterPanel';
import { t } from './i18n';

function mailKey(sender: string, recipient: string) {
  return `${sender}\0${recipient}`;
}

/** Diff two network snapshots and return bullets for new mails. */
function diffMailBullets(prev: Network | null, next: Network, realNow: number): Bullet[] {
  if (!prev) return [];
  const prevMap = new Map<string, number>();
  for (const e of prev.mail_edges) prevMap.set(mailKey(e.sender, e.recipient), e.count);

  const bullets: Bullet[] = [];
  for (const e of next.mail_edges) {
    const key = mailKey(e.sender, e.recipient);
    const prevCount = prevMap.get(key) ?? 0;
    const delta = e.count - prevCount;
    if (delta > 0 && prevCount > 0) {
      const count = Math.min(delta, 8);
      for (let i = 0; i < count; i++) {
        bullets.push({
          src: e.sender,
          dst: e.recipient,
          born: realNow + i * 150 + Math.random() * 100,
        });
      }
    }
  }
  return bullets;
}

export type VizMode = 'live' | 'replay';

const DEFAULT_SPEED = 10;

export default function App() {
  const [network, setNetwork] = useState<Network | null>(null);
  const [edgeMode, setEdgeMode] = useState<EdgeMode>('avatar');
  const [showNames, setShowNames] = useState(true);
  const [filter, setFilter] = useState<FilterState>(defaultFilter);
  const [showFilter, setShowFilter] = useState(false);
  const [themeMode, setThemeMode] = useState<'dark' | 'light'>(loadThemePreference);
  const [bullets, setBullets] = useState<Bullet[]>([]);

  // Viz mode
  const [vizMode, setVizMode] = useState<VizMode>('live');
  const [speed, setSpeed] = useState(DEFAULT_SPEED);
  const [playing, setPlaying] = useState(false);
  const [replayTime, setReplayTime] = useState(0); // virtual clock (unix ms)
  const [tapeRange, setTapeRange] = useState<[number, number]>([0, 0]);
  const [viewRange, setViewRange] = useState<[number, number]>([0, 0]); // user-adjustable sub-range
  const [replayLoading, setReplayLoading] = useState(false);

  // Replay engine refs (mutable, read by rAF loop)
  const replayRef = useRef({
    playing: false,
    speed: 1,
    virtualTime: 0,   // unix ms
    frameIndex: 0,     // current position in tape
    lastRealTime: 0,   // last rAF timestamp for delta calc
    tape: [] as TapeFrame[],
    prevNet: null as Network | null,
    lastDisplayedTime: 0,  // throttle setReplayTime
    viewEnd: 0,            // user-chosen end bound (0 = full tape)
  });
  const replayAnimRef = useRef(0);

  // Chunk manager: tracks which chunks are loaded
  const chunkManagerRef = useRef({
    manifest: null as ReplayManifest | null,
    loadedChunks: new Map<number, TapeFrame[]>(),
    loadedOrder: [] as number[],                   // FIFO eviction order
    loading: new Set<number>(),
  });
  const MAX_LOADED_CHUNKS = 3;

  // Live mode: use a ref for prev network to avoid stale closures
  const prevNetworkRef = useRef<Network | null>(null);

  // ── Live mode ────────────────────────────────────────────────

  const onNetworkUpdate = useCallback((net: Network) => {
    const prev = prevNetworkRef.current;
    const newBullets = diffMailBullets(prev, net, performance.now());
    prevNetworkRef.current = net;
    setNetwork(net);
    if (newBullets.length > 0) setBullets(newBullets);
  }, []); // no deps — uses ref, not state

  useEffect(() => {
    if (vizMode !== 'live') return;
    const poll = () => fetchNetwork().then(onNetworkUpdate).catch(console.error);
    poll();
    const id = setInterval(poll, 3000);
    return () => clearInterval(id);
  }, [onNetworkUpdate, vizMode]);

  // ── Replay rAF cleanup on unmount ────────────────────────────

  useEffect(() => {
    return () => cancelAnimationFrame(replayAnimRef.current);
  }, []);

  // ── Replay mode ──────────────────────────────────────────────

  const loadAndMergeChunk = useCallback(async (chunkInfo: ChunkInfo): Promise<TapeFrame[]> => {
    const cm = chunkManagerRef.current;
    const cached = cm.loadedChunks.get(chunkInfo.start);
    if (cached) return cached;
    if (cm.loading.has(chunkInfo.start)) return [];

    cm.loading.add(chunkInfo.start);
    try {
      const chunk = await fetchChunk(chunkInfo.start);
      const frames = reconstructFrames(chunk);
      cm.loadedChunks.set(chunkInfo.start, frames);
      cm.loadedOrder.push(chunkInfo.start);
      cm.loading.delete(chunkInfo.start);

      while (cm.loadedOrder.length > MAX_LOADED_CHUNKS) {
        const evict = cm.loadedOrder.shift()!;
        cm.loadedChunks.delete(evict);
      }

      rebuildTape();
      return frames;
    } catch (err) {
      cm.loading.delete(chunkInfo.start);
      console.error('Failed to load chunk:', err);
      return [];
    }
  }, []);

  const rebuildTape = useCallback(() => {
    const cm = chunkManagerRef.current;
    const allFrames: TapeFrame[] = [];
    const sortedKeys = Array.from(cm.loadedChunks.keys()).sort((a, b) => a - b);
    for (const key of sortedKeys) {
      allFrames.push(...cm.loadedChunks.get(key)!);
    }
    replayRef.current.tape = allFrames;
  }, []);

  const findChunkForTime = useCallback((unixMs: number): ChunkInfo | null => {
    const manifest = chunkManagerRef.current.manifest;
    if (!manifest) return null;
    for (const c of manifest.chunks) {
      if (unixMs >= c.start && unixMs <= c.end) return c;
    }
    if (manifest.chunks.length > 0) {
      if (unixMs < manifest.chunks[0].start) return manifest.chunks[0];
      return manifest.chunks[manifest.chunks.length - 1];
    }
    return null;
  }, []);

  const maybePrefetch = useCallback((virtualTime: number) => {
    const manifest = chunkManagerRef.current.manifest;
    if (!manifest) return;
    const currentChunk = findChunkForTime(virtualTime);
    if (!currentChunk) return;

    const chunkDuration = currentChunk.end - currentChunk.start;
    if (chunkDuration > 0 && (virtualTime - currentChunk.start) / chunkDuration > 0.8) {
      const idx = manifest.chunks.indexOf(currentChunk);
      if (idx >= 0 && idx < manifest.chunks.length - 1) {
        const nextChunk = manifest.chunks[idx + 1];
        if (!chunkManagerRef.current.loadedChunks.has(nextChunk.start)) {
          loadAndMergeChunk(nextChunk);
        }
      }
    }
  }, [findChunkForTime, loadAndMergeChunk]);

  const startReplayLoop = useCallback(() => {
    cancelAnimationFrame(replayAnimRef.current);

    const tick = (now: number) => {
      const r = replayRef.current;
      if (!r.playing) return; // stop loop when paused — no CPU waste

      const dt = now - r.lastRealTime;
      r.lastRealTime = now;
      r.virtualTime += dt * r.speed;

      // Clamp to view range end
      const lastT = r.viewEnd > 0 ? r.viewEnd : (r.tape[r.tape.length - 1]?.t ?? 0);
      if (r.virtualTime > lastT) {
        r.virtualTime = lastT;
        r.playing = false;
        setPlaying(false);
      }

      // Advance frame index and emit bullets for each crossed boundary
      let newBullets: Bullet[] = [];
      while (
        r.frameIndex < r.tape.length - 1 &&
        r.tape[r.frameIndex + 1].t <= r.virtualTime
      ) {
        r.frameIndex++;
        const frame = r.tape[r.frameIndex];
        const b = diffMailBullets(r.prevNet, frame.net, performance.now());
        newBullets = newBullets.concat(b);
        r.prevNet = frame.net;
        setNetwork(frame.net);
      }

      if (newBullets.length > 0) setBullets(newBullets);
      maybePrefetch(r.virtualTime);

      // Throttle setReplayTime — update only when displayed second changes
      const displayedSec = Math.floor(r.virtualTime / 1000);
      if (displayedSec !== r.lastDisplayedTime) {
        r.lastDisplayedTime = displayedSec;
        setReplayTime(r.virtualTime);
      }

      replayAnimRef.current = requestAnimationFrame(tick);
    };

    replayAnimRef.current = requestAnimationFrame(tick);
  }, [maybePrefetch]);

  const enterReplay = useCallback(async () => {
    setReplayLoading(true);

    try {
      const manifest = await fetchManifest();
      if (!manifest.chunks || manifest.chunks.length === 0) {
        setReplayLoading(false);
        return;
      }

      const cm = chunkManagerRef.current;
      cm.manifest = manifest;
      cm.loadedChunks.clear();
      cm.loadedOrder = [];
      cm.loading.clear();

      const chunksToLoad = manifest.chunks.slice(-MAX_LOADED_CHUNKS);
      await Promise.all(chunksToLoad.map(c => loadAndMergeChunk(c)));

      const tape = replayRef.current.tape;
      if (tape.length === 0) {
        setReplayLoading(false);
        return;
      }

      const t0 = manifest.tape_start;
      const t1 = manifest.tape_end;
      const playStart = tape[0].t;

      setTapeRange([t0, t1]);
      setViewRange([t0, t1]);
      setReplayTime(playStart);
      setPlaying(true);
      setVizMode('replay');
      setNetwork(tape[0].net);

      const ref = replayRef.current;
      ref.virtualTime = playStart;
      ref.frameIndex = 0;
      ref.lastRealTime = performance.now();
      ref.playing = true;
      ref.speed = speed;
      ref.prevNet = null;
      ref.lastDisplayedTime = 0;
      ref.viewEnd = t1;

      setReplayLoading(false);
      startReplayLoop();
    } catch (err) {
      console.error('Failed to enter replay:', err);
      setReplayLoading(false);
    }
  }, [speed, startReplayLoop, loadAndMergeChunk]);

  const exitReplay = useCallback(() => {
    cancelAnimationFrame(replayAnimRef.current);
    replayRef.current.playing = false;
    setVizMode('live');
    setPlaying(false);
    setReplayLoading(false);
    const cm = chunkManagerRef.current;
    cm.manifest = null;
    cm.loadedChunks.clear();
    cm.loadedOrder = [];
    cm.loading.clear();
    prevNetworkRef.current = null;
  }, []);

  const togglePlaying = useCallback(() => {
    const r = replayRef.current;
    if (!r.playing) {
      // If at end, restart from view range start
      const viewEnd = r.viewEnd > 0 ? r.viewEnd : (r.tape[r.tape.length - 1]?.t ?? 0);
      if (r.virtualTime >= viewEnd || r.frameIndex >= r.tape.length - 1) {
        const viewStart = viewRange[0];
        r.virtualTime = viewStart;
        // Find frame index for view start
        r.frameIndex = 0;
        for (let i = r.tape.length - 1; i >= 0; i--) {
          if (r.tape[i].t <= viewStart) { r.frameIndex = i; break; }
        }
        r.prevNet = r.frameIndex > 0 ? r.tape[r.frameIndex - 1].net : null;
      }
      r.lastRealTime = performance.now();
      r.playing = true;
      setPlaying(true);
      startReplayLoop(); // restart rAF loop
    } else {
      r.playing = false;
      setPlaying(false);
      // rAF loop stops itself when r.playing is false
    }
  }, [startReplayLoop, viewRange]);

  const seekTo = useCallback(async (unixMs: number) => {
    const r = replayRef.current;
    r.virtualTime = unixMs;

    const targetChunk = findChunkForTime(unixMs);
    if (targetChunk && !chunkManagerRef.current.loadedChunks.has(targetChunk.start)) {
      await loadAndMergeChunk(targetChunk);
    }

    let idx = 0;
    for (let i = r.tape.length - 1; i >= 0; i--) {
      if (r.tape[i].t <= unixMs) { idx = i; break; }
    }
    r.frameIndex = idx;
    r.prevNet = idx > 0 ? r.tape[idx - 1].net : null;
    r.lastRealTime = performance.now();
    setReplayTime(unixMs);
    if (r.tape[idx]) {
      setNetwork(r.tape[idx].net);
    }
  }, [findChunkForTime, loadAndMergeChunk]);

  const changeSpeed = useCallback((s: number) => {
    setSpeed(s);
    replayRef.current.speed = s;
  }, []);

  const changeViewRange = useCallback((range: [number, number]) => {
    const [t0, t1] = tapeRange;
    const v0 = Math.max(t0, Math.min(range[0], range[1]));
    const v1 = Math.min(t1, Math.max(range[0], range[1]));
    setViewRange([v0, v1]);
    replayRef.current.viewEnd = v1;
    // If current position is outside new range, seek to start
    if (replayRef.current.virtualTime < v0 || replayRef.current.virtualTime > v1) {
      seekTo(v0);
    }
  }, [tapeRange, seekTo]);

  // ── Theme ────────────────────────────────────────────────────

  const theme = getTheme(themeMode);
  const lang = network?.lang ?? 'en';

  const toggleTheme = () => {
    const next = themeMode === 'dark' ? 'light' : 'dark';
    setThemeMode(next);
    saveThemePreference(next);
  };

  // ── Render ───────────────────────────────────────────────────

  if (!network) {
    return (
      <div style={{ background: theme.bg, color: theme.textDim, height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        {t(lang, 'connecting')}
      </div>
    );
  }

  return (
    <div style={{ background: theme.bg, height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <TopBar
        lang={lang}
        theme={theme}
        themeMode={themeMode}
        vizMode={vizMode}
        playing={playing}
        replayLoading={replayLoading}
        speed={speed}
        replayTime={replayTime}
        tapeRange={tapeRange}
        viewRange={viewRange}
        showFilter={showFilter}
        onEnterReplay={enterReplay}
        onExitReplay={exitReplay}
        onTogglePlaying={togglePlaying}
        onSeek={seekTo}
        onChangeSpeed={changeSpeed}
        onSetViewRange={changeViewRange}
        onToggleTheme={toggleTheme}
        onToggleFilter={() => setShowFilter(v => !v)}
      />
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <Graph
            network={network}
            edgeMode={edgeMode}
            theme={theme}
            bullets={bullets}
            vizMode={vizMode}
            showNames={showNames}
            filter={filter}
          />
        </div>
        {showFilter && (
          <div style={{
            width: 180,
            flexShrink: 0,
            borderLeft: `1px solid ${theme.border}`,
            background: theme.barBg,
            overflow: 'hidden',
          }}>
            <FilterPanel
              network={network}
              filter={filter}
              lang={lang}
              theme={theme}
              showNames={showNames}
              onToggleNames={() => setShowNames(v => !v)}
              onChange={setFilter}
            />
          </div>
        )}
      </div>
      <BottomBar
        network={network}
        lang={lang}
        theme={theme}
        edgeMode={edgeMode}
        onToggleEdgeMode={() => setEdgeMode(m => m === 'avatar' ? 'email' : 'avatar')}
      />
    </div>
  );
}
