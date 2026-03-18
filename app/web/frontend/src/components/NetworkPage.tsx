import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ForceGraph3D, { type ForceGraphMethods } from "react-force-graph-3d";
import * as THREE from "three";
import SpriteText from "three-spritetext";
import { forceRadial } from "d3-force-3d";
import type { GraphNode, GraphLink, NodeActivity } from "../types";

const GLOW_COLORS: Record<NodeActivity["type"], string> = {
  thinking: "#f0a500",
  tool: "#6b9bcb",
  diary: "#6bcb77",
};

const BASE_EMISSIVE_ACTIVE = 0.6;
const BASE_EMISSIVE_SLEEPING = 0.15;
const GLOW_INTENSITY = 1.8;
const GLOW_DECAY_MS = 4000;

interface NetworkPageProps {
  graphData: { nodes: GraphNode[]; links: GraphLink[] };
  nodeActivity: Map<string, NodeActivity[]>;
  lightMode: boolean;
}

export function NetworkPage({ graphData, nodeActivity, lightMode }: NetworkPageProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const fgRef = useRef<ForceGraphMethods<GraphNode, GraphLink> | undefined>(undefined);
  const materialsRef = useRef<Map<string, THREE.MeshStandardMaterial>>(new Map());
  const animRef = useRef<number>(0);
  const [viewMode, setViewMode] = useState<"comm" | "activity">("comm");
  const [layoutMode, setLayoutMode] = useState<"default" | "volume" | "cluster">("default");
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 });

  // Container sizing with ResizeObserver
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0) {
          setDimensions({ width, height });
        }
      }
    });
    ro.observe(container);

    // Initial size
    const { clientWidth, clientHeight } = container;
    if (clientWidth > 0 && clientHeight > 0) {
      setDimensions({ width: clientWidth, height: clientHeight });
    }

    return () => ro.disconnect();
  }, []);

  // Activity glow animation loop
  const nodeActivityRef = useRef(nodeActivity);
  nodeActivityRef.current = nodeActivity;
  const viewModeRef = useRef(viewMode);
  viewModeRef.current = viewMode;

  useEffect(() => {
    const tick = () => {
      const now = Date.now();
      const materials = materialsRef.current;
      const activity = nodeActivityRef.current;
      const mode = viewModeRef.current;

      for (const [nodeId, mat] of materials) {
        const events = activity.get(nodeId);
        const isActive = (mat.userData as { status?: string })?.status === "active";
        const baseIntensity = isActive ? BASE_EMISSIVE_ACTIVE : BASE_EMISSIVE_SLEEPING;
        const baseColor = (mat.userData as { baseColor?: string })?.baseColor || "#ffffff";

        if (events && events.length > 0) {
          // Find the most recent event
          const latest = events[events.length - 1];
          const age = now - latest.time;

          if (age < GLOW_DECAY_MS) {
            const progress = age / GLOW_DECAY_MS;
            const intensity = baseIntensity + (GLOW_INTENSITY - baseIntensity) * (1 - progress);
            const glowColor = GLOW_COLORS[latest.type];

            // Lerp color toward glow color based on freshness
            const glowFactor = mode === "activity" ? (1 - progress) : (1 - progress) * 0.7;
            const base = new THREE.Color(baseColor);
            const glow = new THREE.Color(glowColor);
            base.lerp(glow, glowFactor);

            mat.emissive.copy(base);
            mat.emissiveIntensity = mode === "activity" ? intensity * 1.3 : intensity;
            continue;
          }
        }

        // No recent activity — return to base
        mat.emissive.set(baseColor);
        mat.emissiveIntensity = baseIntensity;
      }

      animRef.current = requestAnimationFrame(tick);
    };

    animRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(animRef.current);
  }, []);

  // Precompute max volume and max link count for layout modes
  const maxVolume = useMemo(
    () => Math.max(...graphData.nodes.map((n) => n._volume || 0), 1),
    [graphData.nodes]
  );
  const maxLinkCount = useMemo(
    () => Math.max(...graphData.links.map((l) => l.count), 1),
    [graphData.links]
  );

  // Apply layout forces when layoutMode changes
  useEffect(() => {
    const fg = fgRef.current;
    if (!fg) return;

    if (layoutMode === "volume") {
      fg.d3Force(
        "radial",
        forceRadial((node: GraphNode) => {
          const normalized = (node._volume || 0) / maxVolume;
          return 120 * (1 - normalized);
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

  // Custom node rendering
  const nodeThreeObject = useCallback((node: GraphNode) => {
    const group = new THREE.Group();

    const isActive = node.status === "active";
    const emissiveIntensity = isActive ? BASE_EMISSIVE_ACTIVE : BASE_EMISSIVE_SLEEPING;

    let geometry: THREE.BufferGeometry;
    if (node.type === "user") {
      geometry = new THREE.OctahedronGeometry(6);
    } else {
      geometry = new THREE.SphereGeometry(5, 32, 32);
    }

    const material = new THREE.MeshStandardMaterial({
      color: 0x111111,
      emissive: new THREE.Color(node.color),
      emissiveIntensity,
      metalness: 0.3,
      roughness: 0.4,
      transparent: true,
      opacity: isActive ? 1.0 : 0.6,
    });

    // Store metadata for the animation loop
    material.userData = { status: node.status, baseColor: node.color };

    // Register material for glow animation
    materialsRef.current.set(node.id, material);

    const mesh = new THREE.Mesh(geometry, material);
    group.add(mesh);

    // Admin: outer wireframe icosahedron shell
    if (node.type === "admin") {
      const shellGeometry = new THREE.IcosahedronGeometry(8, 1);
      const shellMaterial = new THREE.MeshBasicMaterial({
        color: new THREE.Color(node.color),
        wireframe: true,
        transparent: true,
        opacity: 0.15,
      });
      const shell = new THREE.Mesh(shellGeometry, shellMaterial);
      group.add(shell);
    }

    // SpriteText label above node
    const label = new SpriteText(node.name, 3.5, node.color);
    label.position.set(0, 10, 0);
    label.material.depthWrite = false;
    group.add(label);

    return group;
  }, []);

  const bgColor = lightMode ? "#faf6f0" : "#0a0a1a";
  const linkBaseColor = lightMode ? "#d4c9b8" : "#1a3a5c";

  return (
    <div
      ref={containerRef}
      className="flex-1 bg-bg overflow-hidden"
      style={{ position: "relative" }}
    >
      <ForceGraph3D<GraphNode, GraphLink>
        ref={fgRef}
        graphData={graphData}
        width={dimensions.width}
        height={dimensions.height}
        backgroundColor={bgColor}
        nodeId="id"
        nodeLabel={(node: GraphNode) => `${node.name} (${node.status}) [${node.type}]`}
        nodeThreeObject={nodeThreeObject}
        nodeThreeObjectExtend={false}
        linkSource="source"
        linkTarget="target"
        linkColor={() => linkBaseColor}
        linkWidth={(link: GraphLink) => Math.min(1 + link.count * 0.3, 4)}
        linkOpacity={viewMode === "activity" ? 0.05 : 0.6}
        linkCurvature={0.15}
        linkDirectionalParticles={viewMode === "activity"
          ? 0
          : (link: GraphLink) => (link.count > 0 ? 2 : 0)
        }
        linkDirectionalParticleWidth={3}
        linkDirectionalParticleSpeed={0.015}
        linkDirectionalArrowLength={3.5}
        linkDirectionalArrowRelPos={1}
        d3AlphaDecay={0.02}
        d3VelocityDecay={0.3}
      />

      {/* Control panel */}
      <div
        style={{
          position: "absolute",
          top: 12,
          right: 12,
          display: "flex",
          flexDirection: "column",
          gap: 6,
          alignItems: "flex-end",
          zIndex: 10,
        }}
      >
        {/* View mode row */}
        <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <span style={{ color: "#555", fontSize: 9, textTransform: "uppercase", letterSpacing: 1, marginRight: 2 }}>View</span>
          {(["comm", "activity"] as const).map((m) => (
            <button
              key={m}
              onClick={() => setViewMode(m)}
              style={{
                padding: "4px 10px",
                background: viewMode === m ? "rgba(233, 69, 96, 0.6)" : "rgba(15, 52, 96, 0.8)",
                color: "#e0e0e0",
                border: `1px solid ${viewMode === m ? "#e94560" : "#1a3a5c"}`,
                borderRadius: 4,
                cursor: "pointer",
                fontSize: 11,
                fontFamily: "'Courier New', monospace",
              }}
            >
              {m === "comm" ? "Communication" : "Activity"}
            </button>
          ))}
        </div>
        {/* Layout mode row */}
        <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
          <span style={{ color: "#555", fontSize: 9, textTransform: "uppercase", letterSpacing: 1, marginRight: 2 }}>Layout</span>
          {([
            ["default", "Default"],
            ["volume", "Email Volume"],
            ["cluster", "Contact Cluster"],
          ] as const).map(([l, label]) => (
            <button
              key={l}
              onClick={() => setLayoutMode(l)}
              style={{
                padding: "4px 10px",
                background: layoutMode === l ? "rgba(233, 69, 96, 0.6)" : "rgba(15, 52, 96, 0.8)",
                color: "#e0e0e0",
                border: `1px solid ${layoutMode === l ? "#e94560" : "#1a3a5c"}`,
                borderRadius: 4,
                cursor: "pointer",
                fontSize: 11,
                fontFamily: "'Courier New', monospace",
              }}
            >
              {label}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
