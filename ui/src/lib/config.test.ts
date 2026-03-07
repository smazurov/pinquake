import { describe, it, expect } from "vitest";
import type { components } from "./api.generated";

type PinQuakeConfig = components["schemas"]["PinQuakeConfig"];

function mergeDefaults(partial: Partial<PinQuakeConfig>): PinQuakeConfig {
  const defaults: PinQuakeConfig = {
    ble: { device_address: "", device_name: "", sensor_name: "" },
    waveform: {
      buffer_size: 256,
      log_knee: 0.02,
      force_yellow_g: 0.03,
      force_red_g: 0.1,
      amp_scale: 1.0,
      swap_xy: false,
    },
    crosshair: {
      force_yellow_g: 0.03,
      force_red_g: 0.1,
      smoothing: 0.7,
      segment_size: 10,
      bar_thickness: 12,
      swap_xy: false,
    },
    viz: { width: 608, height: 1080 },
    auto_lock: { timeout: 10, epsilon: 0.01 },
  };
  return { ...defaults, ...partial };
}

function buildVizUrl(
  base: string,
  vizType: "canvas" | "crosshair",
  width: number,
  height: number,
): string {
  return `${base}/${vizType}?width=${width}&height=${height}`;
}

describe("mergeDefaults", () => {
  it("returns full config from empty partial", () => {
    const cfg = mergeDefaults({});
    expect(cfg.waveform.buffer_size).toBe(256);
    expect(cfg.crosshair.smoothing).toBe(0.7);
    expect(cfg.viz.width).toBe(608);
  });

  it("overrides with provided section", () => {
    const cfg = mergeDefaults({
      crosshair: {
        force_yellow_g: 0.05,
        force_red_g: 0.2,
        smoothing: 0.5,
        segment_size: 15,
        bar_thickness: 20,
        swap_xy: true,
      },
    });
    expect(cfg.crosshair.smoothing).toBe(0.5);
    expect(cfg.waveform.buffer_size).toBe(256);
  });
});

describe("buildVizUrl", () => {
  it("builds canvas URL", () => {
    const url = buildVizUrl("http://localhost:5173", "canvas", 200, 400);
    expect(url).toBe("http://localhost:5173/canvas?width=200&height=400");
  });

  it("builds crosshair URL", () => {
    const url = buildVizUrl("http://localhost:5173", "crosshair", 608, 1080);
    expect(url).toBe("http://localhost:5173/crosshair?width=608&height=1080");
  });
});
