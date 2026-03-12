import { useEffect, useRef, useCallback, useState } from "react";
import { useBeforeUnload } from "react-router-dom";
import Waveform from "../components/Waveform";
import type { WaveformHandle } from "../components/Waveform";
import { SSEClient, api } from "../lib/api";

const DEFAULT_WIDTH = 608;
const DEFAULT_HEIGHT = 1080;

interface WaveformParams {
  logKnee: number;
  forceYellowG: number;
  forceRedG: number;
  ampScale: number;
}

function getQueryDimensions(): { width: number | null; height: number | null } {
  const params = new URLSearchParams(window.location.search);
  const w = params.get("width");
  const h = params.get("height");
  return {
    width: w ? Number(w) : null,
    height: h ? Number(h) : null,
  };
}

export default function VizRoute() {
  const waveformRef = useRef<WaveformHandle>(null);
  const sseRef = useRef<SSEClient<"/api/events"> | null>(null);
  const [dimensions, setDimensions] = useState(() => {
    const q = getQueryDimensions();
    return { width: q.width ?? DEFAULT_WIDTH, height: q.height ?? DEFAULT_HEIGHT };
  });
  const [enabled, setEnabled] = useState(true);
  const [visible, setVisible] = useState(false);
  const [waveformConfig, setWaveformConfig] = useState<WaveformParams | undefined>();

  const fetchConfig = useCallback(() => {
    api.GET("/api/config/waveform")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setEnabled(cfg.enabled);
        const q = getQueryDimensions();
        setDimensions({
          width: q.width ?? cfg.width,
          height: q.height ?? cfg.height,
        });
        setWaveformConfig({
          logKnee: cfg.log_knee,
          forceYellowG: cfg.force_yellow_g,
          forceRedG: cfg.force_red_g,
          ampScale: cfg.amp_scale,
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
      waveformRef.current?.pushSample(data.x, data.y);
    });

    client.on("viz-trigger", (data) => {
      setVisible(data.visible);
    });

    client.on("config-changed", (data) => {
      if (data.section === "waveform") fetchConfig();
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
      <Waveform ref={waveformRef} width={dimensions.width} height={dimensions.height} config={waveformConfig} />
    </div>
  );
}
