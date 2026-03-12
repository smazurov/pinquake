import { describe, it, expect } from "vitest";
import type { components } from "./api.generated";

type PinQuakeConfig = components["schemas"]["PinQuakeConfig"];

function mergeDefaults(partial: Partial<PinQuakeConfig>): PinQuakeConfig {
  const defaults: PinQuakeConfig = {
    ble: { device_address: "", device_name: "", sensor_name: "" },
    waveform: {
      enabled: true,
      width: 608,
      height: 1080,
      buffer_size: 256,
      log_knee: 0.02,
      force_yellow_g: 0.03,
      force_red_g: 0.1,
      amp_scale: 1.0,
      swap_xy: false,
    },
    crosshair: {
      enabled: true,
      width: 200,
      height: 200,
      force_yellow_g: 0.03,
      force_red_g: 0.1,
      decay_s: 0.3,
      segment_size: 10,
      bar_thickness: 12,
      swap_xy: false,
      hide_neg_y: false,
    },
    auto_lock: { timeout: 10, epsilon: 0.01 },
    display: { delay_ms: 0, trigger_g: 0.02, fade_s: 5 },
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
    expect(cfg.crosshair.decay_s).toBe(0.3);
    expect(cfg.waveform.width).toBe(608);
  });

  it("overrides with provided section", () => {
    const cfg = mergeDefaults({
      crosshair: {
        enabled: true,
        width: 200,
        height: 200,
        force_yellow_g: 0.05,
        force_red_g: 0.2,
        decay_s: 0.5,
        segment_size: 15,
        bar_thickness: 20,
        swap_xy: true,
        hide_neg_y: false,
      },
    });
    expect(cfg.crosshair.decay_s).toBe(0.5);
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
