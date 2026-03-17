import { useEffect, useRef } from "react";
import * as d3 from "d3";
import type { NetworkNode, NetworkEdge, Particle } from "../types";

interface NetworkPageProps {
  nodes: NetworkNode[];
  edges: NetworkEdge[];
  particles: Particle[];
}

export function NetworkPage({ nodes, edges, particles }: NetworkPageProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const simRef = useRef<d3.Simulation<NetworkNode, NetworkEdge> | null>(null);
  const nodesRef = useRef<NetworkNode[]>([]);
  const edgesRef = useRef<NetworkEdge[]>([]);

  // Initialize and update D3 simulation
  useEffect(() => {
    const svg = d3.select(svgRef.current);
    const width = svgRef.current?.clientWidth || 800;
    const height = svgRef.current?.clientHeight || 600;

    // Copy nodes to preserve D3 positions across re-renders
    const nodeMap = new Map(nodesRef.current.map((n) => [n.id, n]));
    const simNodes: NetworkNode[] = nodes.map((n) => {
      const existing = nodeMap.get(n.id);
      return existing
        ? { ...n, x: existing.x, y: existing.y, vx: existing.vx, vy: existing.vy, fx: existing.fx, fy: existing.fy }
        : { ...n };
    });
    nodesRef.current = simNodes;

    // Resolve edges to node references
    const simEdges: NetworkEdge[] = edges.map((e) => ({
      source: simNodes.find((n) => n.id === (typeof e.source === "string" ? e.source : e.source.id)) || e.source,
      target: simNodes.find((n) => n.id === (typeof e.target === "string" ? e.target : e.target.id)) || e.target,
      count: e.count,
    }));
    edgesRef.current = simEdges;

    // Clear previous
    svg.selectAll("*").remove();

    // Defs — glow filter
    const defs = svg.append("defs");
    const filter = defs.append("filter").attr("id", "particle-glow");
    filter
      .append("feGaussianBlur")
      .attr("stdDeviation", "3")
      .attr("result", "blur");
    const merge = filter.append("feMerge");
    merge.append("feMergeNode").attr("in", "blur");
    merge.append("feMergeNode").attr("in", "SourceGraphic");

    // Edge group
    const edgeGroup = svg.append("g").attr("class", "edges");
    // Node group (on top of edges)
    const nodeGroup = svg.append("g").attr("class", "nodes");
    // Particle group (on top of everything)
    svg.append("g").attr("class", "particles");

    // Draw edges
    const edgeLines = edgeGroup
      .selectAll("line")
      .data(simEdges)
      .join("line")
      .attr("stroke", "#0f3460")
      .attr("stroke-width", (d: NetworkEdge) =>
        Math.min(1 + d.count * 0.5, 4)
      );

    // Draw nodes
    const nodeGs = nodeGroup
      .selectAll("g")
      .data(simNodes)
      .join("g")
      .attr("cursor", "grab")
      .call(
        d3
          .drag<SVGGElement, NetworkNode>()
          .on("start", (event, d) => {
            if (!event.active) simRef.current?.alphaTarget(0.3).restart();
            d.fx = d.x;
            d.fy = d.y;
          })
          .on("drag", (event, d) => {
            d.fx = event.x;
            d.fy = event.y;
          })
          .on("end", (event, d) => {
            if (!event.active) simRef.current?.alphaTarget(0);
            d.fx = null;
            d.fy = null;
          }) as any
      );

    // Admin nodes: outer ring
    nodeGs
      .filter((d: NetworkNode) => d.type === "admin")
      .append("circle")
      .attr("r", 28)
      .attr("fill", "none")
      .attr("stroke", (d: NetworkNode) => d.color)
      .attr("stroke-width", 1.5)
      .attr("stroke-dasharray", "4 2");

    // Main circle
    nodeGs
      .append("circle")
      .attr("r", 22)
      .attr("fill", "#16213e")
      .attr("stroke", (d: NetworkNode) => d.color)
      .attr("stroke-width", 2);

    // Name label
    nodeGs
      .append("text")
      .text((d: NetworkNode) => d.name)
      .attr("text-anchor", "middle")
      .attr("dy", "0.35em")
      .attr("fill", (d: NetworkNode) => d.color)
      .attr("font-size", "10px")
      .attr("font-weight", "bold")
      .attr("pointer-events", "none");

    // Status dot
    nodeGs
      .filter((d: NetworkNode) => d.type !== "user")
      .append("circle")
      .attr("cx", 16)
      .attr("cy", -16)
      .attr("r", 4)
      .attr("fill", (d: NetworkNode) =>
        d.status === "active" ? "#4ecdc4" : "#666"
      );

    // Force simulation
    const simulation = d3
      .forceSimulation<NetworkNode>(simNodes)
      .force(
        "link",
        d3
          .forceLink<NetworkNode, NetworkEdge>(simEdges)
          .id((d) => d.id)
          .distance(120)
      )
      .force("charge", d3.forceManyBody().strength(-300))
      .force("center", d3.forceCenter(width / 2, height / 2))
      .force("collide", d3.forceCollide(40))
      .on("tick", () => {
        edgeLines
          .attr("x1", (d: any) => d.source.x)
          .attr("y1", (d: any) => d.source.y)
          .attr("x2", (d: any) => d.target.x)
          .attr("y2", (d: any) => d.target.y);

        nodeGs.attr("transform", (d: NetworkNode) => `translate(${d.x},${d.y})`);
      });

    simRef.current = simulation;

    return () => {
      simulation.stop();
    };
  }, [nodes, edges]);

  // Render particles (separate effect — runs on every frame via React state)
  useEffect(() => {
    if (!svgRef.current) return;
    const svg = d3.select(svgRef.current);
    const particleGroup = svg.select("g.particles");
    if (particleGroup.empty()) return;

    particleGroup.selectAll("circle").remove();

    const now = performance.now();

    for (const p of particles) {
      const progress = (now - p.startTime) / p.duration;
      if (progress < 0 || progress > 1) continue;

      const sourceNode = nodesRef.current.find((n) => n.id === p.source);
      const targetNode = nodesRef.current.find((n) => n.id === p.target);
      if (!sourceNode?.x || !targetNode?.x) continue;

      const x = sourceNode.x + (targetNode.x - sourceNode.x) * progress;
      const y = sourceNode.y! + (targetNode.y! - sourceNode.y!) * progress;
      const opacity = progress < 0.8 ? 1 : 1 - (progress - 0.8) / 0.2;

      particleGroup
        .append("circle")
        .attr("cx", x)
        .attr("cy", y)
        .attr("r", 5)
        .attr("fill", p.color)
        .attr("opacity", opacity)
        .attr("filter", "url(#particle-glow)");
    }
  }, [particles]);

  return (
    <div className="flex-1 bg-bg flex items-center justify-center overflow-hidden">
      <svg
        ref={svgRef}
        className="w-full h-full"
        style={{ minHeight: "100%" }}
      />
    </div>
  );
}
