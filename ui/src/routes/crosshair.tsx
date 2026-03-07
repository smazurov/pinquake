import { useEffect, useRef, useCallback, useState } from "react";
import Crosshair from "../components/Crosshair";
import type { CrosshairHandle, CrosshairConfig } from "../components/Crosshair";
import { SSEClient, api } from "../lib/api";

const DEFAULT_WIDTH = 608;
const DEFAULT_HEIGHT = 1080;

function getCanvasDimensions(): { width: number; height: number } {
  const params = new URLSearchParams(window.location.search);
  const w = params.get("width");
  const h = params.get("height");
  return {
    width: w ? Number(w) : DEFAULT_WIDTH,
    height: h ? Number(h) : DEFAULT_HEIGHT,
  };
}

export default function CrosshairRoute() {
  const crosshairRef = useRef<CrosshairHandle>(null);
  const { width, height } = getCanvasDimensions();
  const [crosshairConfig, setCrosshairConfig] = useState<CrosshairConfig | undefined>();

  const fetchConfig = useCallback(() => {
    api.GET("/api/config")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setCrosshairConfig({
          forceYellowG: cfg.crosshair.force_yellow_g,
          forceRedG: cfg.crosshair.force_red_g,
          smoothing: cfg.crosshair.smoothing,
          segmentSize: cfg.crosshair.segment_size,
          barThickness: cfg.crosshair.bar_thickness,
          swapXY: cfg.crosshair.swap_xy,
        });
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    fetchConfig();

    const client = new SSEClient({
      endpoint: "/api/events",
    });

    client.on("orientation", (data) => {
      crosshairRef.current?.pushSample(data.gx, data.gy);
    });

    client.on("config-changed", () => {
      fetchConfig();
    });

    client.connect();

    return () => client.disconnect();
  }, [fetchConfig]);

  return (
    <div
      style={{
        width,
        height,
        background: "transparent",
        overflow: "hidden",
      }}
    >
      <Crosshair ref={crosshairRef} width={width} height={height} config={crosshairConfig} />
    </div>
  );
}
