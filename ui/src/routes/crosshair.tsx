import { useEffect, useRef, useCallback, useState } from "react";
import { useBeforeUnload } from "react-router-dom";
import Crosshair from "../components/Crosshair";
import type { CrosshairHandle, CrosshairConfig } from "../components/Crosshair";
import { SSEClient, api } from "../lib/api";

const DEFAULT_WIDTH = 200;
const DEFAULT_HEIGHT = 200;

function getQueryDimensions(): { width: number | null; height: number | null } {
  const params = new URLSearchParams(window.location.search);
  const w = params.get("width");
  const h = params.get("height");
  return {
    width: w ? Number(w) : null,
    height: h ? Number(h) : null,
  };
}

export default function CrosshairRoute() {
  const crosshairRef = useRef<CrosshairHandle>(null);
  const sseRef = useRef<SSEClient<"/api/events"> | null>(null);
  const [dimensions, setDimensions] = useState(() => {
    const q = getQueryDimensions();
    return { width: q.width ?? DEFAULT_WIDTH, height: q.height ?? DEFAULT_HEIGHT };
  });
  const [enabled, setEnabled] = useState(true);
  const [visible, setVisible] = useState(false);
  const [crosshairConfig, setCrosshairConfig] = useState<CrosshairConfig | undefined>();

  const fetchConfig = useCallback(() => {
    api.GET("/api/config/crosshair")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setEnabled(cfg.enabled);
        const q = getQueryDimensions();
        setDimensions({
          width: q.width ?? cfg.width,
          height: q.height ?? cfg.height,
        });
        setCrosshairConfig({
          forceYellowG: cfg.force_yellow_g,
          forceRedG: cfg.force_red_g,
          decayS: cfg.decay_s,
          segmentSize: cfg.segment_size,
          barThickness: cfg.bar_thickness,
          hideNegY: cfg.hide_neg_y,
        });
      })
      .catch(() => {});
  }, []);

  useBeforeUnload(useCallback(() => {
    sseRef.current?.disconnect();
  }, []));

  useEffect(() => {
    fetchConfig();

    const client = new SSEClient({
      endpoint: "/api/events",
    });
    sseRef.current = client;

    client.on("orientation", (data) => {
      crosshairRef.current?.pushSample(data.x, data.y);
    });

    client.on("viz-trigger", (data) => {
      setVisible(data.visible);
    });

    client.on("config-changed", (data) => {
      if (data.section === "crosshair") fetchConfig();
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
      <Crosshair ref={crosshairRef} width={dimensions.width} height={dimensions.height} config={crosshairConfig} />
    </div>
  );
}
