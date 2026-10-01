import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef } from "react";
import { computeColorScale } from "../lib/crosshair";
import { stepPendulum, createState, type PendulumState, type PendulumParams } from "../lib/pendulum";

const FRAME_INTERVAL = 1000 / 60;
const PHYSICS_SUBSTEPS = 2;
const PHYSICS_DT = FRAME_INTERVAL / 1000 / PHYSICS_SUBSTEPS;
const G_TO_MS2 = 9.81;
const RING_STROKE = "rgba(180, 180, 190, 0.35)";
const BOB_RADIUS = 10;

export interface PlumbBobHandle {
  pushSample: (x: number, y: number) => void;
}

export interface PlumbBobDisplayConfig {
  forceYellowG: number;
  forceRedG: number;
  bobDistance: number;
  dampingRatio: number;
}

const DEFAULT_CONFIG: PlumbBobDisplayConfig = {
  forceYellowG: 0.03,
  forceRedG: 0.10,
  bobDistance: 0.10,
  dampingRatio: 0.15,
};

interface PlumbBobProps {
  width: number;
  height: number;
  config?: PlumbBobDisplayConfig;
}

const PlumbBob = forwardRef<PlumbBobHandle, PlumbBobProps>(function PlumbBob(
  { width, height, config: configOverride },
  ref,
) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const rafRef = useRef<number>(0);

  const accelRef = useRef({ x: 0, y: 0 });
  const stateXRef = useRef<PendulumState>(createState());
  const stateYRef = useRef<PendulumState>(createState());

  const configRef = useRef<PlumbBobDisplayConfig>(DEFAULT_CONFIG);
  useEffect(() => {
    configRef.current = { ...DEFAULT_CONFIG, ...configOverride };
  }, [configOverride]);

  const pushSample = useCallback((x: number, y: number) => {
    accelRef.current = { x, y };
  }, []);

  useImperativeHandle(ref, () => ({ pushSample }), [pushSample]);

  const draw = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const config = configRef.current;
    const params: PendulumParams = {
      bobDistance: config.bobDistance,
      dampingRatio: config.dampingRatio,
    };

    const ax = accelRef.current.x * G_TO_MS2;
    const ay = accelRef.current.y * G_TO_MS2;

    for (let i = 0; i < PHYSICS_SUBSTEPS; i++) {
      stateXRef.current = stepPendulum(stateXRef.current, ax, params, PHYSICS_DT);
      stateYRef.current = stepPendulum(stateYRef.current, ay, params, PHYSICS_DT);
    }

    const cx = width / 2;
    const cy = height / 2;
    const ringRadius = Math.min(cx, cy) * 0.75;

    ctx.clearRect(0, 0, width, height);

    // Ring
    ctx.strokeStyle = RING_STROKE;
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.arc(cx, cy, ringRadius, 0, Math.PI * 2);
    ctx.stroke();

    // Map pendulum angle to pixel displacement
    // At forceRedG * 1.5 steady lateral accel, the bob should be near the ring edge.
    // Steady-state angle: θ ≈ atan(a/g) ≈ a/g for small a.
    const maxAngle = (config.forceRedG * 1.5 * G_TO_MS2) / G_TO_MS2; // in radians (≈ forceRedG * 1.5)
    const scale = ringRadius / Math.sin(maxAngle > 0 ? maxAngle : 0.15);

    const bobX = cx + scale * Math.sin(stateXRef.current.theta);
    const bobY = cy - scale * Math.sin(stateYRef.current.theta);

    // Radial distance for color
    const dx = bobX - cx;
    const dy = bobY - cy;
    const radialDist = Math.sqrt(dx * dx + dy * dy);
    const radialNorm = Math.min(radialDist / ringRadius, 1);

    const colorScale = computeColorScale(config.forceYellowG, config.forceRedG);
    const bobColor = colorScale(radialNorm);

    // Bob glow
    ctx.beginPath();
    ctx.arc(bobX, bobY, BOB_RADIUS + 4, 0, Math.PI * 2);
    ctx.fillStyle = bobColor.replace("rgb", "rgba").replace(")", ", 0.2)");
    ctx.fill();

    // Bob
    ctx.beginPath();
    ctx.arc(bobX, bobY, BOB_RADIUS, 0, Math.PI * 2);
    ctx.fillStyle = bobColor;
    ctx.fill();

    // Center dot (rest position)
    ctx.fillStyle = "rgba(255, 255, 255, 0.25)";
    ctx.beginPath();
    ctx.arc(cx, cy, 2, 0, Math.PI * 2);
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

export default PlumbBob;
