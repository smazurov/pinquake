import type { components } from "./api.generated";

export type OBSStatus = components["schemas"]["OBSStatusEvent"];
export type OBSScene = components["schemas"]["Scene"];
export type OBSItem = components["schemas"]["Item"];
export type OBSTarget = components["schemas"]["Target"];

export const NO_TARGET: OBSTarget = { scene: "", scene_uuid: "", source: "", source_uuid: "" };

export function hasTarget(t: OBSTarget): boolean {
  return t.source !== "" || t.source_uuid !== "";
}

export function targetLabel(t: OBSTarget): string {
  return `${t.scene} › ${t.source}`;
}

export function targetKey(t: OBSTarget): string {
  return `${t.scene_uuid}/${t.source_uuid}`;
}

export interface TargetOption {
  key: string;
  label: string;
  target: OBSTarget;
}

export interface TargetGroup {
  scene: string;
  options: TargetOption[];
}

/**
 * Lays scenes out like the OBS source list: one group per scene, items
 * top-first, and a group's children indented right under the group.
 */
export function targetOptions(scenes: OBSScene[]): TargetGroup[] {
  const groups = new Map(scenes.filter((s) => s.is_group).map((s) => [s.uuid, s]));
  const option = (holder: OBSScene, item: OBSItem, label: string): TargetOption => {
    const target = { scene: holder.name, scene_uuid: holder.uuid, source: item.name, source_uuid: item.uuid };
    return { key: targetKey(target), label, target };
  };

  return scenes
    .filter((s) => !s.is_group)
    .map((scene) => {
      const options: TargetOption[] = [];
      for (const item of scene.items ?? []) {
        options.push(option(scene, item, item.is_group ? `${item.name} (group)` : item.name));
        const group = item.is_group ? groups.get(item.uuid) : undefined;
        for (const child of group?.items ?? []) {
          options.push(option(group!, child, `\u00A0\u00A0\u00A0└ ${child.name}`));
        }
      }
      return { scene: scene.name, options };
    });
}

/** Finds the option for t: by UUIDs, else by names (as the server resolves). */
export function findTargetOption(groups: TargetGroup[], t: OBSTarget): TargetOption | undefined {
  if (!hasTarget(t)) return undefined;
  const all = groups.flatMap((g) => g.options);
  return (
    all.find((o) => o.key === targetKey(t)) ??
    all.find((o) => o.target.scene === t.scene && o.target.source === t.source)
  );
}

export type StatusTone = "off" | "busy" | "warn" | "ok";

export interface StatusView {
  tone: StatusTone;
  text: string;
  connected: boolean;
}

export function obsStatusView(status: OBSStatus | null): StatusView {
  switch (status?.state) {
    case "connecting":
      return { tone: "busy", text: `Connecting to ${status.server ?? "OBS"}…`, connected: false };
    case "waiting": {
      const retry = status.retry_in_s ? ` Retrying in ${Math.round(status.retry_in_s)}s.` : "";
      return { tone: "warn", text: `${status.error ?? "Connection failed"}.${retry}`, connected: false };
    }
    case "connected":
      return status.target_missing
        ? { tone: "warn", text: `${status.target} not found in OBS`, connected: true }
        : { tone: "busy", text: "Connected. Pick a source to drive.", connected: true };
    case "active":
      return { tone: "ok", text: `Driving ${status.target}`, connected: true };
    default:
      return { tone: "off", text: "Not connected", connected: false };
  }
}
