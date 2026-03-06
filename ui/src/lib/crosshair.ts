import { scaleLinear } from "d3-scale";

export interface CrosshairDisplayConfig {
  forceYellowG: number;
  forceRedG: number;
  smoothing: number;
  segmentSize: number;
  barThickness: number;
  swapXY: boolean;
}

export const DEFAULT_CROSSHAIR_CONFIG: CrosshairDisplayConfig = {
  forceYellowG: 0.03,
  forceRedG: 0.1,
  smoothing: 0.7,
  segmentSize: 10,
  barThickness: 12,
  swapXY: false,
};

export function computeColorScale(
  forceYellowG: number,
  forceRedG: number,
): (norm: number) => string {
  const maxG = forceRedG * 1.5;
  const yellowNorm = forceYellowG / maxG;
  const redNorm = forceRedG / maxG;
  return scaleLinear<string>()
    .domain([0, yellowNorm, redNorm, 1])
    .range(["#00ff00", "#ffff00", "#ff0000", "#ff0000"])
    .clamp(true);
}

export function computeSegmentCount(armLength: number, segmentSize: number): number {
  return Math.floor(armLength / segmentSize);
}

export function computeArmFill(
  magnitude: number,
  forceRedG: number,
  armLength: number,
): number {
  const maxG = forceRedG * 1.5;
  const scale = scaleLinear()
    .domain([0, maxG])
    .range([0, armLength])
    .clamp(true);
  return scale(magnitude) as number;
}

export function smoothValue(display: number, target: number, smoothing: number): number {
  return display * smoothing + target * (1 - smoothing);
}
