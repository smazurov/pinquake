import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef } from "react";
import { scaleLinear } from "d3-scale";
import {
  decayValue,
  DEFAULT_CROSSHAIR_CONFIG,
  type CrosshairDisplayConfig,
} from "../lib/crosshair";

const FRAME_INTERVAL = 1000 / 60;
const SEGMENT_GAP = 2;
const MARGIN = 20;

export interface CrosshairHandle {
  pushSample: (x: number, y: number) => void;
}

export type { CrosshairDisplayConfig as CrosshairConfig };

interface CrosshairProps {
  width: number;
  height: number;
  config?: CrosshairDisplayConfig;
}

const Crosshair = forwardRef<CrosshairHandle, CrosshairProps>(function Crosshair(
  { width, height, config: configOverride },
  ref,
) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rafRef = useRef<number>(0);

  const targetRef = useRef({ right: 0, left: 0, up: 0, down: 0 });
  const displayRef = useRef({ right: 0, left: 0, up: 0, down: 0 });

  const configRef = useRef<CrosshairDisplayConfig>(DEFAULT_CROSSHAIR_CONFIG);
  useEffect(() => { configRef.current = { ...DEFAULT_CROSSHAIR_CONFIG, ...configOverride }; }, [configOverride]);

  const pushSample = useCallback((sx: number, sy: number) => {
    const x = configRef.current.swapXY ? sy : sx;
    const y = configRef.current.swapXY ? sx : sy;
    targetRef.current = {
      right: Math.max(x, 0),
      left: Math.max(-x, 0),
      up: Math.max(y, 0),
      down: Math.max(-y, 0),
    };
  }, []);

  useImperativeHandle(ref, () => ({ pushSample }), [pushSample]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const config = configRef.current;
    const target = targetRef.current;
    const display = displayRef.current;

    display.right = decayValue(display.right, target.right, config.decayS);
    display.left = decayValue(display.left, target.left, config.decayS);
    display.up = decayValue(display.up, target.up, config.decayS);
    display.down = decayValue(display.down, target.down, config.decayS);

    ctx.clearRect(0, 0, width, height);

    const cx = width / 2;
    const cy = height / 2;
    const armLength = Math.min(cx, cy) - MARGIN;

    const accelScale = scaleLinear()
      .domain([0, config.forceRedG * 1.5])
      .range([0, armLength])
      .clamp(true);

    const yellowNorm = config.forceYellowG / (config.forceRedG * 1.5);
    const redNorm = config.forceRedG / (config.forceRedG * 1.5);
    const colorScale = scaleLinear<string>()
      .domain([0, yellowNorm, redNorm, 1])
      .range(["#00ff00", "#ffff00", "#ff0000", "#ff0000"])
      .clamp(true);

    // Crosshair guide lines
    ctx.strokeStyle = "rgba(255, 255, 255, 0.15)";
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(cx - armLength, cy);
    ctx.lineTo(cx + armLength, cy);
    ctx.moveTo(cx, cy - armLength);
    ctx.lineTo(cx, config.hideNegY ? cy : cy + armLength);
    ctx.stroke();

    const segSize = config.segmentSize;
    const segCount = Math.floor(armLength / segSize);
    const barThickness = config.barThickness;

    const drawArm = (magnitude: number, dirX: number, dirY: number) => {
      const fillPx = accelScale(magnitude);
      for (let i = 0; i < segCount; i++) {
        const segStart = i * segSize;
        const segEnd = segStart + segSize - SEGMENT_GAP;
        const segMid = segStart + (segSize - SEGMENT_GAP) / 2;
        if (fillPx < segMid) break;

        const norm = (i + 0.5) / segCount;
        ctx.fillStyle = colorScale(norm);

        if (dirX !== 0) {
          const x = cx + dirX * segStart;
          const w = dirX * (segEnd - segStart);
          ctx.fillRect(
            dirX > 0 ? x : x + w,
            cy - barThickness / 2,
            Math.abs(w),
            barThickness,
          );
        } else {
          const y = cy + dirY * segStart;
          const h = dirY * (segEnd - segStart);
          ctx.fillRect(
            cx - barThickness / 2,
            dirY > 0 ? y : y + h,
            barThickness,
            Math.abs(h),
          );
        }
      }
    };

    drawArm(display.right, 1, 0);
    drawArm(display.left, -1, 0);
    drawArm(display.up, 0, -1);
    if (!config.hideNegY) drawArm(display.down, 0, 1);

    // Center dot
    ctx.fillStyle = "rgba(255, 255, 255, 0.6)";
    ctx.beginPath();
    ctx.arc(cx, cy, 3, 0, Math.PI * 2);
    ctx.fill();
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

export default Crosshair;
