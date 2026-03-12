import { scaleLinear } from "d3-scale";

export interface CrosshairDisplayConfig {
  forceYellowG: number;
  forceRedG: number;
  decayS: number;
  segmentSize: number;
  barThickness: number;
  swapXY: boolean;
  hideNegY: boolean;
}

export const DEFAULT_CROSSHAIR_CONFIG: CrosshairDisplayConfig = {
  forceYellowG: 0.03,
  forceRedG: 0.1,
  decayS: 0.3,
  segmentSize: 10,
  barThickness: 12,
  swapXY: false,
  hideNegY: false,
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

export function decayValue(display: number, target: number, decayS: number): number {
  if (target >= display) return target;
  if (decayS === 0) return target;
  const alpha = Math.exp(-3 / (decayS * 60));
  return display * alpha + target * (1 - alpha);
}
