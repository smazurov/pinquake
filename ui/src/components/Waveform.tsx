import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef } from "react";
import { area, curveLinear } from "d3-shape";
import {
  extractRibbonPoints,
  computeCenterline,
  smoothCenterline,
  buildRibbonEdges,
  computeSegmentStyles,
  type EdgePair,
  type WaveformConfig,
} from "@/lib/ribbon";

const MAX_BUFFER = 256;
const RIBBON_HALF_WIDTH = 3.5;
const MARGIN = 40;
const SUBDIVISIONS = 8;
const FRAME_INTERVAL = 1000 / 60;

const DEFAULT_CONFIG: WaveformConfig = {
  logKnee: 0.02,
  forceYellowG: 0.03,
  forceRedG: 0.1,
  ampScale: 1.0,
  maxDisplacement: 0.5,
};

export interface WaveformHandle {
  pushSample: (gx: number, gy: number) => void;
}

interface WaveformProps {
  width: number;
  height: number;
  config?: Partial<WaveformConfig>;
}

const Waveform = forwardRef<WaveformHandle, WaveformProps>(function Waveform(
  { width, height, config: configOverride },
  ref,
) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const gxBufferRef = useRef<Float32Array>(new Float32Array(MAX_BUFFER));
  const gyBufferRef = useRef<Float32Array>(new Float32Array(MAX_BUFFER));
  const headRef = useRef(0);
  const lenRef = useRef(0);
  const rafRef = useRef<number>(0);

  const configRef = useRef<WaveformConfig>(DEFAULT_CONFIG);
  configRef.current = { ...DEFAULT_CONFIG, ...configOverride };

  const pushSample = useCallback((gx: number, gy: number) => {
    const head = headRef.current;
    gxBufferRef.current[head] = gx;
    gyBufferRef.current[head] = gy;
    headRef.current = (head + 1) % MAX_BUFFER;
    if (lenRef.current < MAX_BUFFER) {
      lenRef.current++;
    }
  }, []);

  useImperativeHandle(ref, () => ({ pushSample }), [pushSample]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    ctx.clearRect(0, 0, width, height);

    const len = lenRef.current;
    if (len < 2) return;

    const head = headRef.current;
    const gxBuf = gxBufferRef.current;
    const gyBuf = gyBufferRef.current;
    const config = configRef.current;

    const yConfig: WaveformConfig = {
      ...config,
      ampScale: config.ampScale * 1.2,
      maxDisplacement: config.maxDisplacement * 1.2,
    };

    const xLen = width / 2 - MARGIN - 30;
    const yLen = height - 2 * MARGIN;

    // X trace: right half (original)
    drawTrace(ctx, gxBuf, head, len, width / 2, height - MARGIN, 1, 0, 0, -1, xLen, config);
    // X trace: left half (mirror)
    drawTrace(ctx, gxBuf, head, len, width / 2, height - MARGIN, -1, 0, 0, -1, xLen, config);
    // Y trace: right edge (original)
    drawTrace(ctx, gyBuf, head, len, width - MARGIN, height - MARGIN, 0, -1, -1, 0, yLen, yConfig);
    // Y trace: left edge (mirror)
    drawTrace(ctx, gyBuf, head, len, MARGIN, height - MARGIN, 0, -1, 1, 0, yLen, yConfig);
  }, [width, height]);

  useEffect(() => {
    let lastTime = 0;
    const loop = (now: number) => {
      rafRef.current = requestAnimationFrame(loop);
      if (now - lastTime < FRAME_INTERVAL) return;
      lastTime = now;
      draw();
    };
    rafRef.current = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(rafRef.current);
  }, [draw]);

  return (
    <canvas
      ref={canvasRef}
      width={width}
      height={height}
      style={{ width, height, background: "transparent" }}
    />
  );
});

export default Waveform;

function drawTrace(
  ctx: CanvasRenderingContext2D,
  buffer: Float32Array,
  head: number,
  len: number,
  ox: number,
  oy: number,
  dx: number,
  dy: number,
  px: number,
  py: number,
  traceLen: number,
  config: WaveformConfig,
) {
  // 1. Extract ribbon points from circular buffer
  const points = extractRibbonPoints(buffer, head, len, config);

  // 2. Convert to screen-space centerline
  const center = computeCenterline(points, ox, oy, dx, dy, px, py, traceLen);

  // 3. Smooth with Catmull-Rom spline
  const smooth = smoothCenterline(center, SUBDIVISIONS);

  // 4. Build ribbon edges (top/bottom offset from smooth centerline)
  const edges = buildRibbonEdges(smooth, RIBBON_HALF_WIDTH);

  // 5. Compute per-segment colors and alpha
  const styles = computeSegmentStyles(points, config);

  // 6. Build fill gradient — fade alpha to 0 along trace
  const endPt = { x: ox + dx * traceLen, y: oy + dy * traceLen };
  const grad = ctx.createLinearGradient(ox, oy, endPt.x, endPt.y);
  for (let i = 0; i < styles.length; i++) {
    const t = styles.length === 1 ? 0 : i / (styles.length - 1);
    const s = styles[i]!;
    const fade = 1 - t;
    grad.addColorStop(t, `rgba(${s.r}, ${s.g}, ${s.b}, ${s.alpha * fade})`);
  }

  // 7. Build stroke gradient — white fading to transparent
  const strokeGrad = ctx.createLinearGradient(ox, oy, endPt.x, endPt.y);
  strokeGrad.addColorStop(0, "white");
  strokeGrad.addColorStop(1, "rgba(255,255,255,0)");

  // 8. Draw ribbon as single d3.area path
  const ribbon = area<EdgePair>()
    .x0((d) => d.top.x)
    .y0((d) => d.top.y)
    .x1((d) => d.bot.x)
    .y1((d) => d.bot.y)
    .curve(curveLinear)
    .context(ctx);

  ctx.beginPath();
  ribbon(edges);
  ctx.fillStyle = grad;
  ctx.fill();
  ctx.strokeStyle = strokeGrad;
  ctx.lineWidth = 1;
  ctx.stroke();
}
