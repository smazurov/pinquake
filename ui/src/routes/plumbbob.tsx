import { useEffect, useRef, useCallback, useState } from "react";
import { useBeforeUnload } from "react-router-dom";
import PlumbBob from "../components/PlumbBob";
import type { PlumbBobHandle, PlumbBobDisplayConfig } from "../components/PlumbBob";
import { SSEClient, api } from "../lib/api";

const DEFAULT_WIDTH = 400;
const DEFAULT_HEIGHT = 400;

function getQueryDimensions(): { width: number | null; height: number | null } {
  const params = new URLSearchParams(window.location.search);
  const w = params.get("width");
  const h = params.get("height");
  return {
    width: w ? Number(w) : null,
    height: h ? Number(h) : null,
  };
}

export default function PlumbBobRoute() {
  const plumbRef = useRef<PlumbBobHandle>(null);
  const sseRef = useRef<SSEClient<"/api/events"> | null>(null);
  const [dimensions, setDimensions] = useState(() => {
    const q = getQueryDimensions();
    return { width: q.width ?? DEFAULT_WIDTH, height: q.height ?? DEFAULT_HEIGHT };
  });
  const [enabled, setEnabled] = useState(true);
  const [visible, setVisible] = useState(false);
  const [plumbConfig, setPlumbConfig] = useState<PlumbBobDisplayConfig | undefined>();

  const fetchConfig = useCallback(() => {
    api.GET("/api/config/plumb_bob")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setEnabled(cfg.enabled);
        const q = getQueryDimensions();
        setDimensions({
          width: q.width ?? cfg.width,
          height: q.height ?? cfg.height,
        });
        setPlumbConfig({
          forceYellowG: cfg.force_yellow_g,
          forceRedG: cfg.force_red_g,
          bobDistance: cfg.bob_distance,
          dampingRatio: cfg.damping_ratio,
        });
      })
      .catch(() => {});
  }, []);

  useBeforeUnload(useCallback(() => {
    sseRef.current?.disconnect();
  }, []));

  useEffect(() => {
    fetchConfig();

    const client = new SSEClient({ endpoint: "/api/events" });
    sseRef.current = client;

    client.on("orientation", (data) => {
      plumbRef.current?.pushSample(data.x, data.y);
    });

    client.on("viz-trigger", (data) => {
      setVisible(data.visible);
    });

    client.on("config-changed", (data) => {
      if (data.section === "plumb_bob") fetchConfig();
    });

    client.connect();

    return () => {
      client.disconnect();
      sseRef.current = null;
    };
  }, [fetchConfig]);

  if (!enabled) {
    return <div style={{ background: "transparent" }} />;
  }

  return (
    <div
      style={{
        width: dimensions.width,
        height: dimensions.height,
        background: "transparent",
        overflow: "hidden",
        display: visible ? "block" : "none",
      }}
    >
      <PlumbBob ref={plumbRef} width={dimensions.width} height={dimensions.height} config={plumbConfig} />
    </div>
  );
}
