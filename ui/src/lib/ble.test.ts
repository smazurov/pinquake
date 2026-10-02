import { describe, it, expect } from "vitest";
import { bleStatusView, type BLEStatus } from "./ble";

const at = (s: Omit<BLEStatus, "timestamp">): BLEStatus => ({ ...s, timestamp: "" });
const DEV = { device: "EA:F0:F1:BC:59:DD", device_name: "WT901BLE68" };

describe("bleStatusView", () => {
  it("offers scanning only while no device is chosen", () => {
    expect(bleStatusView(at({ status: "idle" }))).toMatchObject({ text: "No sensor", chosen: false });
    for (const status of ["searching", "waiting", "connecting", "connected"] as const) {
      expect(bleStatusView(at({ status, ...DEV })).chosen).toBe(true);
    }
  });

  it("names the device being searched for", () => {
    expect(bleStatusView(at({ status: "searching", ...DEV }))).toMatchObject({ tone: "busy", text: "Searching for WT901BLE68…" });
  });

  it("flags a lost link while searching for it again", () => {
    expect(bleStatusView(at({ status: "searching", reason: "lost", ...DEV }))).toMatchObject({
      tone: "warn",
      text: "Connection lost. Searching for WT901BLE68…",
    });
  });

  it("shows why it is waiting and the time left", () => {
    const st = at({ status: "waiting", ...DEV, error: "device not found", retry_in_s: 8 });
    expect(bleStatusView(st, 7.2).text).toBe("WT901BLE68: device not found. Retrying in 8s.");
    expect(bleStatusView(st, 0).text).toBe("WT901BLE68: device not found. Retrying…");
  });

  it("falls back to the address without a name", () => {
    expect(bleStatusView(at({ status: "connecting", device: DEV.device })).text).toBe(`Connecting to ${DEV.device}…`);
  });
});
