import { describe, it, expect } from "vitest";
import { softLimitRadius } from "./pendulum";

describe("softLimitRadius", () => {
  it("passes radii below the knee through unchanged", () => {
    for (const r of [0, 0.3, 0.85]) expect(softLimitRadius(r, 0.85)).toBe(r);
  });

  it("never reaches the ring however far the bob swings", () => {
    for (const r of [0.9, 1, 2, 6.7, 1e6]) {
      expect(softLimitRadius(r, 0.85)).toBeLessThanOrEqual(1);
      expect(softLimitRadius(r, 0.85)).toBeGreaterThan(0.85);
    }
  });

  it("keeps growing past the knee", () => {
    expect(softLimitRadius(1.2, 0.85)).toBeGreaterThan(softLimitRadius(1, 0.85));
  });
});
