import { describe, it, expect } from "vitest";
import { findTargetOption, obsStatusView, targetOptions, type OBSScene } from "./obs";

const item = (id: number, name: string, uuid: string, isGroup = false) => ({
  id, name, uuid, kind: isGroup ? "" : "browser_source", is_group: isGroup, enabled: true,
});

const OVERLAYS_UUID = "src-overlays";

const scenes: OBSScene[] = [
  { name: "Main", uuid: "scene-main", is_group: false, items: [item(7, "Overlays", OVERLAYS_UUID, true), item(1, "Camera", "src-camera")] },
  { name: "Overlays", uuid: OVERLAYS_UUID, is_group: true, items: [item(2, "Waveform", "src-waveform")] },
  { name: "BRB", uuid: "scene-brb", is_group: false, items: [] },
];

describe("targetOptions", () => {
  it("nests group children under their group and skips groups as top-level scenes", () => {
    const groups = targetOptions(scenes);
    expect(groups.map((g) => g.scene)).toEqual(["Main", "BRB"]);
    expect(groups[0]!.options.map((o) => o.label)).toEqual(["Overlays (group)", "\u00A0\u00A0\u00A0└ Waveform", "Camera"]);
  });

  it("targets a group child inside the group, not the scene", () => {
    const child = targetOptions(scenes)[0]!.options[1]!;
    expect(child.target).toEqual({ scene: "Overlays", scene_uuid: OVERLAYS_UUID, source: "Waveform", source_uuid: "src-waveform" });
  });
});

describe("findTargetOption", () => {
  const groups = targetOptions(scenes);

  it("matches by UUIDs even after a rename", () => {
    const found = findTargetOption(groups, { scene: "Old", scene_uuid: "scene-main", source: "Old", source_uuid: "src-camera" });
    expect(found?.target.source).toBe("Camera");
  });

  it("falls back to names", () => {
    const found = findTargetOption(groups, { scene: "Main", scene_uuid: "", source: "Camera", source_uuid: "" });
    expect(found?.key).toBe("scene-main/src-camera");
  });

  it("finds nothing for no target", () => {
    expect(findTargetOption(groups, { scene: "", scene_uuid: "", source: "", source_uuid: "" })).toBeUndefined();
  });
});

describe("obsStatusView", () => {
  const base = { timestamp: "" };

  it("is off without a status", () => {
    expect(obsStatusView(null)).toEqual({ tone: "off", text: "Not connected", connected: false });
  });

  it("explains retries", () => {
    const v = obsStatusView({ ...base, state: "waiting", error: "connection refused", retry_in_s: 4 });
    expect(v).toEqual({ tone: "warn", text: "connection refused. Retrying in 4s.", connected: false });
  });

  it("flags a missing target while connected", () => {
    const v = obsStatusView({ ...base, state: "connected", target: "Main › Gone", target_missing: true });
    expect(v.tone).toBe("warn");
    expect(v.connected).toBe(true);
  });

  it("names the driven target when active", () => {
    expect(obsStatusView({ ...base, state: "active", target: "Main › Overlays" }).text).toBe("Driving Main › Overlays");
  });
});
