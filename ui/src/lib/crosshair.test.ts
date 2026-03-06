import { describe, it, expect } from "vitest";
import {
  computeColorScale,
  computeSegmentCount,
  computeArmFill,
  smoothValue,
} from "./crosshair";

describe("computeColorScale", () => {
  const colorScale = computeColorScale(0.03, 0.1);

  it("returns green at zero", () => {
    expect(colorScale(0)).toBe("rgb(0, 255, 0)");
  });

  it("returns red at 1", () => {
    expect(colorScale(1)).toBe("rgb(255, 0, 0)");
  });

  it("returns yellow near yellow threshold", () => {
    const yellowNorm = 0.03 / (0.1 * 1.5);
    const color = colorScale(yellowNorm);
    expect(color).toContain("255");
  });
});

describe("computeSegmentCount", () => {
  it("computes correct segment count", () => {
    expect(computeSegmentCount(100, 10)).toBe(10);
    expect(computeSegmentCount(100, 15)).toBe(6);
    expect(computeSegmentCount(50, 10)).toBe(5);
  });

  it("returns 0 for arm shorter than segment", () => {
    expect(computeSegmentCount(5, 10)).toBe(0);
  });
});

describe("computeArmFill", () => {
  it("returns 0 for zero magnitude", () => {
    expect(computeArmFill(0, 0.1, 200)).toBe(0);
  });

  it("returns full arm length at max G", () => {
    const fill = computeArmFill(0.15, 0.1, 200);
    expect(fill).toBeCloseTo(200, 5);
  });

  it("clamps beyond max G", () => {
    const fill = computeArmFill(1.0, 0.1, 200);
    expect(fill).toBe(200);
  });

  it("scales linearly within range", () => {
    const half = computeArmFill(0.075, 0.1, 200);
    expect(half).toBeCloseTo(100, 0);
  });
});

describe("smoothValue", () => {
  it("returns target when smoothing is 0", () => {
    expect(smoothValue(5, 10, 0)).toBe(10);
  });

  it("returns display when smoothing is 1", () => {
    expect(smoothValue(5, 10, 1)).toBe(5);
  });

  it("blends proportionally", () => {
    expect(smoothValue(0, 1, 0.7)).toBeCloseTo(0.3);
  });
});
