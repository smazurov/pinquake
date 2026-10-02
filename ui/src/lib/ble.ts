import type { components } from "./api.generated";

export type BLEStatus = components["schemas"]["BLEStatusEvent"];

export type BLETone = "off" | "busy" | "warn" | "ok";

export interface BLEStatusView {
  tone: BLETone;
  text: string;
  /** A device is chosen (searching, connecting or connected): no scanning. */
  chosen: boolean;
  connected: boolean;
}

export function bleDeviceLabel(status: BLEStatus): string {
  return status.device_name || status.device || "sensor";
}

/**
 * Describes the link for the BLE panel header. retryInS is the time left
 * before the next attempt while waiting, counted down by the caller.
 */
export function bleStatusView(status: BLEStatus | null, retryInS = 0): BLEStatusView {
  if (!status) return { tone: "off", text: "Not connected", chosen: false, connected: false };
  const name = bleDeviceLabel(status);
  const chosen = { chosen: true, connected: false };
  switch (status.status) {
    case "searching":
      return status.reason === "lost"
        ? { tone: "warn", text: `Connection lost. Searching for ${name}…`, ...chosen }
        : { tone: "busy", text: `Searching for ${name}…`, ...chosen };
    case "waiting": {
      const retry = retryInS >= 1 ? `Retrying in ${Math.ceil(retryInS)}s.` : "Retrying…";
      return { tone: "warn", text: `${name}: ${status.error ?? "connection failed"}. ${retry}`, ...chosen };
    }
    case "connecting":
      return { tone: "busy", text: `Connecting to ${name}…`, ...chosen };
    case "connected":
      return { tone: "ok", text: name, chosen: true, connected: true };
    default:
      return { tone: "off", text: "No sensor", chosen: false, connected: false };
  }
}
