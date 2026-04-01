import { useEffect, useState } from 'react';
import type { Theme } from './theme';
import type { VizMode } from './App';
import { t } from './i18n';

function formatTime(date: Date): string {
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  const s = String(date.getSeconds()).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

function formatDateTime(unixMs: number): string {
  const d = new Date(unixMs);
  const mon = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${mon}-${day} ${h}:${m}:${s}`;
}

export function TopBar({ lang, theme, vizMode, playing, speed, replayTime, tapeRange, onEnterReplay, onExitReplay, onTogglePlaying, onSeek, onChangeSpeed }: {
  lang: string;
  theme: Theme;
  vizMode: VizMode;
  playing: boolean;
  speed: number;
  replayTime: number;
  tapeRange: [number, number];
  onEnterReplay: () => void;
  onExitReplay: () => void;
  onTogglePlaying: () => void;
  onSeek: (unixMs: number) => void;
  onChangeSpeed: (s: number) => void;
}) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    if (vizMode !== 'live') return;
    const id = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(id);
  }, [vizMode]);

  const btnStyle = (active?: boolean): React.CSSProperties => ({
    background: active ? theme.stateColors['ACTIVE'] + '30' : 'transparent',
    border: `1px solid ${theme.border}`,
    borderRadius: 4,
    padding: '2px 8px',
    cursor: 'pointer',
    color: active ? theme.stateColors['ACTIVE'] : theme.textDim,
    fontSize: 10,
    letterSpacing: 0.5,
    flexShrink: 0,
  });

  if (vizMode === 'live') {
    return (
      <div style={{
        background: theme.barBg,
        borderBottom: `1px solid ${theme.border}`,
        padding: '6px 16px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        flexShrink: 0,
        userSelect: 'none',
      }}>
        {/* Left: live indicator */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span style={{
            display: 'inline-block',
            width: 6,
            height: 6,
            borderRadius: '50%',
            background: theme.stateColors['ACTIVE'],
            boxShadow: `0 0 4px ${theme.stateColors['ACTIVE']}`,
          }} />
          <span style={{
            fontSize: 10,
            fontWeight: 600,
            letterSpacing: 1,
            color: theme.stateColors['ACTIVE'],
          }}>
            {t(lang, 'topbar.live')}
          </span>
        </div>

        {/* Center: replay button */}
        <button onClick={onEnterReplay} style={btnStyle()}>
          {'⏮ ' + t(lang, 'topbar.replay')}
        </button>

        {/* Right: clock */}
        <div style={{
          fontFamily: 'monospace',
          fontSize: 12,
          color: theme.textDim,
          letterSpacing: 1,
        }}>
          {formatTime(now)}
        </div>
      </div>
    );
  }

  // ── Replay mode ──────────────────────────────────────────────

  const [t0, t1] = tapeRange;
  const duration = t1 - t0 || 1;
  const progress = (replayTime - t0) / duration;

  return (
    <div style={{
      background: theme.barBg,
      borderBottom: `1px solid ${theme.border}`,
      padding: '6px 16px',
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      flexShrink: 0,
      userSelect: 'none',
    }}>
      {/* Back to live */}
      <button onClick={onExitReplay} style={btnStyle()}>
        {'● ' + t(lang, 'topbar.live')}
      </button>

      {/* Play / Pause */}
      <button onClick={onTogglePlaying} style={btnStyle(playing)}>
        {playing ? '⏸' : '▶'}
      </button>

      {/* Speed input */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
        <input
          type="text"
          inputMode="numeric"
          value={speed}
          onChange={e => {
            const raw = e.target.value.replace(/[^0-9]/g, '');
            if (raw === '') return;
            const v = Math.max(1, Math.min(9999, Number(raw)));
            onChangeSpeed(v);
          }}
          style={{
            background: 'transparent',
            border: `1px solid ${theme.border}`,
            borderRadius: 4,
            padding: '2px 4px',
            color: theme.stateColors['ACTIVE'],
            fontSize: 11,
            fontFamily: 'monospace',
            width: 48,
            textAlign: 'right' as const,
            outline: 'none',
          }}
        />
        <span style={{ fontSize: 10, color: theme.textDim }}>×</span>
      </div>

      {/* Scrubber */}
      <input
        type="range"
        min={t0}
        max={t1}
        step={1000}
        value={replayTime}
        onChange={e => onSeek(Number(e.target.value))}
        style={{
          flex: 1,
          accentColor: theme.stateColors['ACTIVE'],
          cursor: 'pointer',
          height: 4,
        }}
      />

      {/* Progress % */}
      <span style={{ fontSize: 10, color: theme.textDim, minWidth: 32, textAlign: 'right' }}>
        {Math.round(progress * 100)}%
      </span>

      {/* Virtual clock */}
      <div style={{
        fontFamily: 'monospace',
        fontSize: 12,
        color: theme.gold,
        letterSpacing: 1,
        minWidth: 110,
        textAlign: 'right',
      }}>
        {formatDateTime(replayTime)}
      </div>
    </div>
  );
}
