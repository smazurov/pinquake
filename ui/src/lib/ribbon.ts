import { line, curveCatmullRom } from "d3-shape";

export interface WaveformConfig {
  logKnee: number;
  forceYellowG: number;
  forceRedG: number;
  ampScale: number;
  maxDisplacement: number;
}

export interface RibbonPoint {
  along: number;
  perp: number;
  sample: number;
  age: number;
}

export interface Vec2 {
  x: number;
  y: number;
}

export interface SmoothPoint extends Vec2 {
  paramT: number;
}

export interface EdgePair {
  top: Vec2;
  bot: Vec2;
}

export interface SegmentStyle {
  r: number;
  g: number;
  b: number;
  alpha: number;
}

// --- Pure helpers ---

export function logAmp(sample: number, config: WaveformConfig): number {
  const abs = Math.abs(sample);
  const compressed = Math.log(1 + abs / config.logKnee) * config.logKnee;
  const clamped = Math.min(compressed, config.maxDisplacement / config.ampScale);
  return Math.sign(sample) * clamped * config.ampScale;
}

export function forceColor(
  absG: number,
  yellowG: number,
  redG: number,
): [number, number, number] {
  if (absG < yellowG) {
    const t = absG / yellowG;
    return [Math.floor(255 * t), 255, 0];
  }
  const t = Math.min((absG - yellowG) / (redG - yellowG), 1.0);
  return [255, Math.floor(255 * (1 - t)), 0];
}

// --- Ribbon pipeline ---

export function extractRibbonPoints(
  buffer: Float32Array,
  head: number,
  len: number,
  config: WaveformConfig,
): RibbonPoint[] {
  const points = new Array<RibbonPoint>(len);
  for (let i = 0; i < len; i++) {
    const idx = (head + buffer.length - 1 - i) % buffer.length;
    const sample = buffer[idx]!;
    points[i] = {
      along: len === 1 ? 0 : i / (len - 1),
      perp: logAmp(sample, config),
      sample,
      age: len === 1 ? 0 : i / (len - 1),
    };
  }
  return points;
}

export function computeCenterline(
  points: RibbonPoint[],
  ox: number,
  oy: number,
  dx: number,
  dy: number,
  px: number,
  py: number,
  traceLen: number,
): Vec2[] {
  return points.map((p) => ({
    x: ox + dx * p.along * traceLen + px * p.perp * traceLen,
    y: oy + dy * p.along * traceLen + py * p.perp * traceLen,
  }));
}

type PathCommand =
  | { type: "moveTo"; args: [number, number] }
  | { type: "lineTo"; args: [number, number] }
  | { type: "bezierCurveTo"; args: [number, number, number, number, number, number] };

export class RecordingContext {
  commands: PathCommand[] = [];

  moveTo(x: number, y: number) {
    this.commands.push({ type: "moveTo", args: [x, y] });
  }

  lineTo(x: number, y: number) {
    this.commands.push({ type: "lineTo", args: [x, y] });
  }

  bezierCurveTo(
    cp1x: number,
    cp1y: number,
    cp2x: number,
    cp2y: number,
    x: number,
    y: number,
  ) {
    this.commands.push({
      type: "bezierCurveTo",
      args: [cp1x, cp1y, cp2x, cp2y, x, y],
    });
  }

  beginPath() {}
  closePath() {}
}

function deCasteljau(
  p0x: number,
  p0y: number,
  p1x: number,
  p1y: number,
  p2x: number,
  p2y: number,
  p3x: number,
  p3y: number,
  t: number,
): Vec2 {
  const u = 1 - t;
  const x =
    u * u * u * p0x +
    3 * u * u * t * p1x +
    3 * u * t * t * p2x +
    t * t * t * p3x;
  const y =
    u * u * u * p0y +
    3 * u * u * t * p1y +
    3 * u * t * t * p2y +
    t * t * t * p3y;
  return { x, y };
}

export function smoothCenterline(
  center: Vec2[],
  subdivisions = 8,
): SmoothPoint[] {
  if (center.length < 2) {
    return center.map((p) => ({ ...p, paramT: 0 }));
  }

  const maxT = center.length - 1;

  const ctx = new RecordingContext();
  const gen = line<Vec2>()
    .x((d) => d.x)
    .y((d) => d.y)
    .curve(curveCatmullRom.alpha(0.5))
    .context(ctx as unknown as CanvasRenderingContext2D);

  gen(center);

  const result: SmoothPoint[] = [];
  let currentX = 0;
  let currentY = 0;
  let cmdSegment = 0;

  for (const cmd of ctx.commands) {
    if (cmd.type === "moveTo") {
      currentX = cmd.args[0];
      currentY = cmd.args[1];
      if (result.length === 0) {
        result.push({ x: currentX, y: currentY, paramT: 0 });
      }
    } else if (cmd.type === "lineTo") {
      for (let s = 1; s <= subdivisions; s++) {
        const t = s / subdivisions;
        result.push({
          x: currentX + (cmd.args[0] - currentX) * t,
          y: currentY + (cmd.args[1] - currentY) * t,
          paramT: Math.min(cmdSegment + t, maxT),
        });
      }
      currentX = cmd.args[0];
      currentY = cmd.args[1];
      cmdSegment++;
    } else if (cmd.type === "bezierCurveTo") {
      const [cp1x, cp1y, cp2x, cp2y, ex, ey] = cmd.args;
      for (let s = 1; s <= subdivisions; s++) {
        const t = s / subdivisions;
        const pt = deCasteljau(
          currentX, currentY,
          cp1x, cp1y,
          cp2x, cp2y,
          ex, ey,
          t,
        );
        result.push({
          ...pt,
          paramT: Math.min(cmdSegment + t, maxT),
        });
      }
      currentX = ex;
      currentY = ey;
      cmdSegment++;
    }
  }

  return result;
}

export function buildRibbonEdges(
  smooth: SmoothPoint[],
  halfWidth: number,
): EdgePair[] {
  const len = smooth.length;
  if (len === 0) return [];

  const edges = new Array<EdgePair>(len);

  for (let i = 0; i < len; i++) {
    let nx = 0;
    let ny = 0;

    if (i > 0) {
      const tdx = smooth[i]!.x - smooth[i - 1]!.x;
      const tdy = smooth[i]!.y - smooth[i - 1]!.y;
      const tl = Math.sqrt(tdx * tdx + tdy * tdy);
      if (tl > 0.001) {
        nx += -tdy / tl;
        ny += tdx / tl;
      }
    }
    if (i < len - 1) {
      const tdx = smooth[i + 1]!.x - smooth[i]!.x;
      const tdy = smooth[i + 1]!.y - smooth[i]!.y;
      const tl = Math.sqrt(tdx * tdx + tdy * tdy);
      if (tl > 0.001) {
        nx += -tdy / tl;
        ny += tdx / tl;
      }
    }

    const nl = Math.sqrt(nx * nx + ny * ny);
    if (nl > 0.001) {
      nx = (nx / nl) * halfWidth;
      ny = (ny / nl) * halfWidth;
    }

    edges[i] = {
      top: { x: smooth[i]!.x + nx, y: smooth[i]!.y + ny },
      bot: { x: smooth[i]!.x - nx, y: smooth[i]!.y - ny },
    };
  }

  return edges;
}

export function interpolateStyle(
  styles: SegmentStyle[],
  paramT: number,
): SegmentStyle {
  const i0 = Math.max(0, Math.min(Math.floor(paramT), styles.length - 1));
  const i1 = Math.min(i0 + 1, styles.length - 1);
  const f = paramT - i0;
  const s0 = styles[i0]!;
  const s1 = styles[i1]!;
  return {
    r: Math.round(s0.r + (s1.r - s0.r) * f),
    g: Math.round(s0.g + (s1.g - s0.g) * f),
    b: Math.round(s0.b + (s1.b - s0.b) * f),
    alpha: s0.alpha + (s1.alpha - s0.alpha) * f,
  };
}

export function computeSegmentStyles(
  points: RibbonPoint[],
  config: WaveformConfig,
): SegmentStyle[] {
  return points.map((p) => {
    const [r, g, b] = forceColor(
      Math.abs(p.sample),
      config.forceYellowG,
      config.forceRedG,
    );
    const fade = 0.7 + 0.3 * (1 - p.age * 0.5);
    const alpha = 1 - p.age * 0.8;
    return {
      r: Math.floor(r * fade),
      g: Math.floor(g * fade),
      b: Math.floor(b * fade),
      alpha,
    };
  });
}
