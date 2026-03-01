import { describe, it, expect } from "vitest";
import {
  logAmp,
  forceColor,
  extractRibbonPoints,
  computeCenterline,
  smoothCenterline,
  buildRibbonEdges,
  computeSegmentStyles,
  interpolateStyle,
  type WaveformConfig,
  type Vec2,
  type SegmentStyle,
} from "./ribbon";

const DEFAULT_CONFIG: WaveformConfig = {
  logKnee: 0.02,
  forceYellowG: 0.03,
  forceRedG: 0.1,
  ampScale: 1.0,
  maxDisplacement: 0.5,
};

describe("logAmp", () => {
  it("returns zero for zero input", () => {
    expect(logAmp(0, DEFAULT_CONFIG)).toBe(0);
  });

  it("preserves sign", () => {
    expect(logAmp(0.05, DEFAULT_CONFIG)).toBeGreaterThan(0);
    expect(logAmp(-0.05, DEFAULT_CONFIG)).toBeLessThan(0);
  });

  it("compresses large values", () => {
    const small = Math.abs(logAmp(0.01, DEFAULT_CONFIG));
    const large = Math.abs(logAmp(1.0, DEFAULT_CONFIG));
    expect(large).toBeGreaterThan(small);
    expect(large / small).toBeLessThan(1.0 / 0.01);
  });

  it("clamps at maxDisplacement", () => {
    const result = Math.abs(logAmp(100, DEFAULT_CONFIG));
    expect(result).toBeLessThanOrEqual(DEFAULT_CONFIG.maxDisplacement + 0.001);
  });
});

describe("forceColor", () => {
  it("returns green at zero force", () => {
    const [r, g, b] = forceColor(0, 0.03, 0.1);
    expect(r).toBe(0);
    expect(g).toBe(255);
    expect(b).toBe(0);
  });

  it("returns yellow at threshold", () => {
    const [r, g, b] = forceColor(0.03, 0.03, 0.1);
    expect(r).toBe(255);
    expect(g).toBe(255);
    expect(b).toBe(0);
  });

  it("returns red above red threshold", () => {
    const [r, g, b] = forceColor(0.1, 0.03, 0.1);
    expect(r).toBe(255);
    expect(g).toBe(0);
    expect(b).toBe(0);
  });
});

describe("extractRibbonPoints", () => {
  it("returns correct length", () => {
    const buf = new Float32Array(8);
    buf[0] = 0.1;
    buf[1] = 0.2;
    buf[2] = 0.3;
    const points = extractRibbonPoints(buf, 3, 3, DEFAULT_CONFIG);
    expect(points).toHaveLength(3);
  });

  it("age ranges from 0 to ~1", () => {
    const buf = new Float32Array(8);
    const points = extractRibbonPoints(buf, 4, 4, DEFAULT_CONFIG);
    expect(points[0]!.age).toBeCloseTo(0);
    expect(points[points.length - 1]!.age).toBeCloseTo(1);
  });

  it("reads newest first from circular buffer", () => {
    const buf = new Float32Array(4);
    buf[0] = 0.4;
    buf[1] = 0.1;
    buf[2] = 0.2;
    buf[3] = 0.3;
    // head=1 means newest write was at index 0
    const points = extractRibbonPoints(buf, 1, 4, DEFAULT_CONFIG);
    expect(points[0]!.sample).toBeCloseTo(0.4);
    expect(points[1]!.sample).toBeCloseTo(0.3);
    expect(points[2]!.sample).toBeCloseTo(0.2);
    expect(points[3]!.sample).toBeCloseTo(0.1);
  });
});

describe("computeCenterline", () => {
  it("produces a straight line when perp=0", () => {
    const points = [
      { along: 0, perp: 0, sample: 0, age: 0 },
      { along: 0.5, perp: 0, sample: 0, age: 0.5 },
      { along: 1, perp: 0, sample: 0, age: 1 },
    ];
    const center = computeCenterline(points, 0, 0, 1, 0, 0, -1, 100);
    expect(center[0]!.x).toBeCloseTo(0);
    expect(center[0]!.y).toBeCloseTo(0);
    expect(center[1]!.x).toBeCloseTo(50);
    expect(center[1]!.y).toBeCloseTo(0);
    expect(center[2]!.x).toBeCloseTo(100);
    expect(center[2]!.y).toBeCloseTo(0);
  });

  it("displaces in perpendicular direction", () => {
    const points = [{ along: 0, perp: 0.1, sample: 0.1, age: 0 }];
    const center = computeCenterline(points, 0, 0, 1, 0, 0, -1, 100);
    expect(center[0]!.x).toBeCloseTo(0);
    expect(center[0]!.y).toBeCloseTo(-10);
  });
});

describe("smoothCenterline", () => {
  const straight: Vec2[] = [
    { x: 0, y: 0 },
    { x: 10, y: 0 },
    { x: 20, y: 0 },
    { x: 30, y: 0 },
  ];

  it("output length = (N-1)*subdivisions + 1", () => {
    const subs = 8;
    const result = smoothCenterline(straight, subs);
    expect(result).toHaveLength((straight.length - 1) * subs + 1);
  });

  it("paramT spans [0, ~N-1] and is monotonically non-decreasing", () => {
    const result = smoothCenterline(straight, 4);
    expect(result[0]!.paramT).toBe(0);
    expect(result[result.length - 1]!.paramT).toBeCloseTo(straight.length - 1, 1);
    for (let i = 1; i < result.length; i++) {
      expect(result[i]!.paramT).toBeGreaterThanOrEqual(result[i - 1]!.paramT);
    }
  });

  it("straight input stays approximately straight", () => {
    const result = smoothCenterline(straight, 4);
    for (const p of result) {
      expect(Math.abs(p.y)).toBeLessThan(0.5);
    }
  });

  it("handles 2-point input", () => {
    const two: Vec2[] = [
      { x: 0, y: 0 },
      { x: 10, y: 0 },
    ];
    const result = smoothCenterline(two, 4);
    expect(result.length).toBeGreaterThanOrEqual(2);
  });
});

describe("buildRibbonEdges", () => {
  it("produces constant width for straight line", () => {
    const smooth = [
      { x: 0, y: 0, paramT: 0 },
      { x: 10, y: 0, paramT: 0.5 },
      { x: 20, y: 0, paramT: 1 },
    ];
    const halfW = 3;
    const edges = buildRibbonEdges(smooth, halfW);

    for (const e of edges) {
      const width = Math.sqrt(
        (e.top.x - e.bot.x) ** 2 + (e.top.y - e.bot.y) ** 2,
      );
      expect(width).toBeCloseTo(halfW * 2, 1);
    }
  });

  it("normals are perpendicular to tangent", () => {
    const smooth = [
      { x: 0, y: 0, paramT: 0 },
      { x: 10, y: 0, paramT: 0.5 },
      { x: 20, y: 0, paramT: 1 },
    ];
    const edges = buildRibbonEdges(smooth, 3);
    // For a horizontal line, normals should be vertical
    for (const e of edges) {
      expect(e.top.x).toBeCloseTo(e.bot.x, 1);
      expect(e.top.y).not.toBeCloseTo(e.bot.y, 1);
    }
  });
});

describe("interpolateStyle", () => {
  const styles: SegmentStyle[] = [
    { r: 0, g: 255, b: 0, alpha: 1.0 },
    { r: 255, g: 255, b: 0, alpha: 0.5 },
    { r: 255, g: 0, b: 0, alpha: 0.0 },
  ];

  it("returns exact style at integer paramT", () => {
    expect(interpolateStyle(styles, 0)).toEqual(styles[0]);
    expect(interpolateStyle(styles, 1)).toEqual(styles[1]);
    expect(interpolateStyle(styles, 2)).toEqual(styles[2]);
  });

  it("interpolates midpoint between two styles", () => {
    const mid = interpolateStyle(styles, 0.5);
    expect(mid.r).toBe(128);
    expect(mid.g).toBe(255);
    expect(mid.b).toBe(0);
    expect(mid.alpha).toBeCloseTo(0.75);
  });

  it("clamps at last style for paramT at boundary", () => {
    const last = interpolateStyle(styles, 2);
    expect(last).toEqual(styles[2]);
  });
});

describe("computeSegmentStyles", () => {
  it("returns known colors at boundary values", () => {
    const points = [
      { along: 0, perp: 0, sample: 0, age: 0 },
      { along: 1, perp: 0, sample: 0.1, age: 1 },
    ];
    const styles = computeSegmentStyles(points, DEFAULT_CONFIG);

    // Zero force, age=0 → green, full brightness, full alpha
    expect(styles[0]!.g).toBeGreaterThan(styles[0]!.r);
    expect(styles[0]!.alpha).toBeCloseTo(1.0);

    // age=1 → alpha should be 0.2
    expect(styles[1]!.alpha).toBeCloseTo(0.2);
  });

  it("alpha stays in [0, 1]", () => {
    const points = [
      { along: 0, perp: 0, sample: 0.05, age: 0 },
      { along: 0.5, perp: 0, sample: 0.05, age: 0.5 },
      { along: 1, perp: 0, sample: 0.05, age: 1 },
    ];
    const styles = computeSegmentStyles(points, DEFAULT_CONFIG);
    for (const s of styles) {
      expect(s.alpha).toBeGreaterThanOrEqual(0);
      expect(s.alpha).toBeLessThanOrEqual(1);
    }
  });
});
