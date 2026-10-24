import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../lib/api.generated";
import type { OBSStatus } from "../lib/obs";
import OBSControl from "./OBSControl";

const config: components["schemas"]["OBSConfig"] = {
  server: "localhost:4455",
  password: "",
  connect: true,
  scene: "Main",
  scene_uuid: "scene-main",
  source: "Overlays",
  source_uuid: "src-overlays",
};

const SHUTDOWN_SETTING = "Shutdown source when not visible";
const ALERT_MARKUP = 'role="alert"';
const warning = `Waveform: turn off "${SHUTDOWN_SETTING}" in OBS to avoid reloading on each trigger.`;

function render(status: OBSStatus | null) {
  return renderToStaticMarkup(createElement(OBSControl, { config, status }));
}

describe("OBS browser settings warning", () => {
  beforeEach(() => {
    vi.stubGlobal("localStorage", { getItem: () => "true" });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders a warning only for a confirmed misconfiguration", () => {
    const html = render({ state: "active", timestamp: "", target: "Main › Overlays", browser_warning: warning });
    expect(html).toContain(ALERT_MARKUP);
    expect(html).toContain("Waveform: turn off");
    expect(html).toContain(SHUTDOWN_SETTING);
    expect(html).not.toContain(">Fix<");
  });

  it.each([undefined, ""])("hides the note completely when there is no warning (%s)", (browserWarning) => {
    const html = render({ state: "active", timestamp: "", browser_warning: browserWarning });
    expect(html).not.toContain(ALERT_MARKUP);
    expect(html).not.toContain(SHUTDOWN_SETTING);
  });

  it.each(["off", "connecting", "waiting", "connected"] as const)("hides stale warnings while %s", (state) => {
    const html = render({ state, timestamp: "", browser_warning: warning });
    expect(html).not.toContain(ALERT_MARKUP);
    expect(html).not.toContain(SHUTDOWN_SETTING);
  });
});
