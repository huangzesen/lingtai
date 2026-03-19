import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ForceGraph2D from "react-force-graph-2d";
import { forceRadial } from "d3-force-3d";
import type { GraphNode, GraphLink, NodeActivity } from "../types";
import type { EmailEvent } from "../hooks/useNetwork";

const GLOW_COLORS: Record<NodeActivity["type"], string> = {
  thinking: "#f0a500",
  tool: "#6b9bcb",
  diary: "#6bcb77",
};

const GLOW_DECAY_MS = 4000;

type LayoutMode = "default" | "volume" | "cluster";
type ViewMode = "comm" | "activity";

interface NetworkPageProps {
  graphData: { nodes: GraphNode[]; links: GraphLink[] };
  nodeActivity: Map<string, NodeActivity[]>;
  pendingEmails: EmailEvent[];
  lightMode: boolean;
}

export function NetworkPage({ graphData, nodeActivity, pendingEmails, lightMode }: NetworkPageProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const fgRef = useRef<any>(undefined);
  const [viewMode, setViewMode] = useState<ViewMode>("comm");
  const [layoutMode, setLayoutMode] = useState<LayoutMode>("default");
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  // Container sizing
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0) setDimensions({ width, height });
      }
    });
    ro.observe(container);
    const { clientWidth, clientHeight } = container;
    if (clientWidth > 0 && clientHeight > 0) setDimensions({ width: clientWidth, height: clientHeight });
    return () => ro.disconnect();
  }, []);

  // Precompute layout params
  const maxVolume = useMemo(
    () => Math.max(...graphData.nodes.map((n) => n._volume || 0), 1),
    [graphData.nodes]
  );
  const maxLinkCount = useMemo(
    () => Math.max(...graphData.links.map((l) => l.count), 1),
    [graphData.links]
  );

  // Apply layout forces
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg) return;

    if (layoutMode === "volume") {
      fg.d3Force(
        "radial",
        forceRadial((node: GraphNode) => {
          const normalized = (node._volume || 0) / maxVolume;
          return 200 * (1 - normalized);
        }).strength(0.4)
      );
      const charge = fg.d3Force("charge");
      if (charge && typeof charge.strength === "function") charge.strength(-30);
      const link = fg.d3Force("link");
      if (link && typeof link.strength === "function") link.strength(null);
    } else if (layoutMode === "cluster") {
      fg.d3Force("radial", null);
      const link = fg.d3Force("link");
      if (link && typeof link.strength === "function") {
        link.strength((l: GraphLink) => 0.3 + 0.7 * (l.count / maxLinkCount));
      }
      const charge = fg.d3Force("charge");
      if (charge && typeof charge.strength === "function") charge.strength(-80);
    } else {
      fg.d3Force("radial", null);
      const charge = fg.d3Force("charge");
      if (charge && typeof charge.strength === "function") charge.strength(-30);
      const link = fg.d3Force("link");
      if (link && typeof link.strength === "function") link.strength(null);
    }

    fg.d3ReheatSimulation();
  }, [layoutMode, maxVolume, maxLinkCount]);

  // Emit single particles for new email events
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg || pendingEmails.length === 0 || viewMode === "activity") return;

    for (const email of pendingEmails) {
      // Find the link that connects these two nodes
      const link = graphData.links.find((l) => {
        const src = typeof l.source === "string" ? l.source : l.source.id;
        const tgt = typeof l.target === "string" ? l.target : l.target.id;
        const pair = [src, tgt].sort().join("--");
        const emailPair = [email.from, email.to].sort().join("--");
        return pair === emailPair;
      });
      if (link) {
        fg.emitParticle(link);
      }
    }
  }, [pendingEmails, viewMode, graphData.links]);

  // Custom node rendering on Canvas
  const nodeCanvasObject = useCallback(
    (node: GraphNode, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const x = node.x ?? 0;
      const y = node.y ?? 0;
      const isActive = node.status === "active";
      const baseRadius = node.type === "user" ? 7 : 5;

      // Check for activity glow
      const events = nodeActivity.get(node.id);
      let glowColor: string | null = null;
      let glowIntensity = 0;
      if (events && events.length > 0) {
        const latest = events[events.length - 1];
        const age = Date.now() - latest.time;
        if (age < GLOW_DECAY_MS) {
          const progress = age / GLOW_DECAY_MS;
          glowIntensity = 1 - progress;
          glowColor = GLOW_COLORS[latest.type];
        }
      }

      // Glow halo
      if (glowColor && glowIntensity > 0) {
        const glowRadius = baseRadius + 8 * glowIntensity;
        const gradient = ctx.createRadialGradient(x, y, baseRadius, x, y, glowRadius);
        gradient.addColorStop(0, glowColor + Math.round(glowIntensity * 100).toString(16).padStart(2, "0"));
        gradient.addColorStop(1, glowColor + "00");
        ctx.beginPath();
        ctx.arc(x, y, glowRadius, 0, 2 * Math.PI);
        ctx.fillStyle = gradient;
        ctx.fill();
      }

      // Admin: outer dashed ring
      if (node.type === "admin") {
        ctx.beginPath();
        ctx.arc(x, y, baseRadius + 4, 0, 2 * Math.PI);
        ctx.strokeStyle = node.color;
        ctx.lineWidth = 1;
        ctx.setLineDash([3, 2]);
        ctx.globalAlpha = 0.4;
        ctx.stroke();
        ctx.setLineDash([]);
        ctx.globalAlpha = 1;
      }

      // User: diamond shape
      if (node.type === "user") {
        const s = baseRadius;
        ctx.beginPath();
        ctx.moveTo(x, y - s);
        ctx.lineTo(x + s, y);
        ctx.lineTo(x, y + s);
        ctx.lineTo(x - s, y);
        ctx.closePath();
        ctx.fillStyle = isActive ? node.color : node.color + "66";
        ctx.fill();
        ctx.strokeStyle = node.color;
        ctx.lineWidth = 1.5;
        ctx.stroke();
      } else {
        // Regular node: filled circle
        ctx.beginPath();
        ctx.arc(x, y, baseRadius, 0, 2 * Math.PI);
        ctx.fillStyle = lightMode ? "#f5efe6" : "#16213e";
        ctx.fill();
        ctx.strokeStyle = node.color;
        ctx.lineWidth = 2;
        ctx.globalAlpha = isActive ? 1 : 0.4;
        ctx.stroke();
        ctx.globalAlpha = 1;
      }

      // Status dot (top-right)
      if (node.type !== "user") {
        ctx.beginPath();
        ctx.arc(x + baseRadius - 1, y - baseRadius + 1, 2, 0, 2 * Math.PI);
        ctx.fillStyle = isActive ? "#4ecdc4" : "#666";
        ctx.fill();
      }

      // Label
      const fontSize = Math.max(10 / globalScale, 3);
      ctx.font = `bold ${fontSize}px 'Courier New', monospace`;
      ctx.textAlign = "center";
      ctx.textBaseline = "top";
      ctx.fillStyle = node.color;
      ctx.globalAlpha = isActive ? 1 : 0.5;
      ctx.fillText(node.name, x, y + baseRadius + 2);
      ctx.globalAlpha = 1;
    },
    [nodeActivity, lightMode]
  );

  const bgColor = lightMode ? "#faf6f0" : "#0a0a1a";
  const linkBaseColor = lightMode ? "#d4c9b8" : "#1a3a5c";

  return (
    <div
      ref={containerRef}
      className="flex-1 bg-bg overflow-hidden"
      style={{ position: "relative" }}
    >
      {dimensions.width > 0 && dimensions.height > 0 && (
        <ForceGraph2D
          ref={fgRef}
          graphData={graphData}
          width={dimensions.width}
          height={dimensions.height}
          backgroundColor={bgColor}
          nodeId="id"
          nodeLabel={(node: GraphNode) => `${node.name} (${node.status}) [${node.type}] — ${node._volume || 0} emails`}
          nodeCanvasObject={nodeCanvasObject}
          nodePointerAreaPaint={(node: GraphNode, color, ctx) => {
            const r = node.type === "user" ? 7 : 5;
            ctx.beginPath();
            ctx.arc(node.x ?? 0, node.y ?? 0, r + 4, 0, 2 * Math.PI);
            ctx.fillStyle = color;
            ctx.fill();
          }}
          linkColor={() => {
            const opacity = viewMode === "activity" ? 0.05 : 0.6;
            return linkBaseColor + Math.round(opacity * 255).toString(16).padStart(2, "0");
          }}
          linkWidth={(link: GraphLink) => Math.min(0.5 + link.count * 0.3, 3)}
          linkDirectionalParticles={0}
          linkDirectionalParticleWidth={4}
          linkDirectionalParticleSpeed={0.005}
          linkDirectionalParticleColor={() => "#4ecdc4"}
          d3AlphaDecay={0.02}
          d3VelocityDecay={0.3}
          cooldownTicks={100}
          enableNodeDrag={true}
          enableZoomInteraction={true}
          enablePanInteraction={true}
        />
      )}

      {/* Control panel */}
      <div style={{
        position: "absolute", top: 12, right: 12,
        display: "flex", flexDirection: "column", gap: 6, alignItems: "flex-end", zIndex: 10,
      }}>
        <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <span style={{ color: "#555", fontSize: 9, textTransform: "uppercase", letterSpacing: 1, marginRight: 2 }}>View</span>
          {(["comm", "activity"] as const).map((m) => (
            <button key={m} onClick={() => setViewMode(m)} style={{
              padding: "4px 10px",
              background: viewMode === m ? "rgba(233, 69, 96, 0.6)" : "rgba(15, 52, 96, 0.8)",
              color: "#e0e0e0", border: `1px solid ${viewMode === m ? "#e94560" : "#1a3a5c"}`,
              borderRadius: 4, cursor: "pointer", fontSize: 11, fontFamily: "'Courier New', monospace",
            }}>
              {m === "comm" ? "Communication" : "Activity"}
            </button>
          ))}
        </div>
        <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <span style={{ color: "#555", fontSize: 9, textTransform: "uppercase", letterSpacing: 1, marginRight: 2 }}>Layout</span>
          {([["default", "Default"], ["volume", "Email Volume"], ["cluster", "Contact Cluster"]] as const).map(([l, label]) => (
            <button key={l} onClick={() => setLayoutMode(l)} style={{
              padding: "4px 10px",
              background: layoutMode === l ? "rgba(233, 69, 96, 0.6)" : "rgba(15, 52, 96, 0.8)",
              color: "#e0e0e0", border: `1px solid ${layoutMode === l ? "#e94560" : "#1a3a5c"}`,
              borderRadius: 4, cursor: "pointer", fontSize: 11, fontFamily: "'Courier New', monospace",
            }}>
              {label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
