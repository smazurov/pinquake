import { useEffect, useRef, useCallback, useState } from "react";
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

function getCanvasDimensions(): { width: number; height: number } {
  const params = new URLSearchParams(window.location.search);
  const w = params.get("width");
  const h = params.get("height");
  return {
    width: w ? Number(w) : DEFAULT_WIDTH,
    height: h ? Number(h) : DEFAULT_HEIGHT,
  };
}

export default function VizRoute() {
  const waveformRef = useRef<WaveformHandle>(null);
  const { width, height } = getCanvasDimensions();
  const [waveformConfig, setWaveformConfig] = useState<WaveformParams | undefined>();

  const fetchConfig = useCallback(() => {
    api.GET("/api/config")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setWaveformConfig({
          logKnee: cfg.waveform.log_knee,
          forceYellowG: cfg.waveform.force_yellow_g,
          forceRedG: cfg.waveform.force_red_g,
          ampScale: cfg.waveform.amp_scale,
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
      waveformRef.current?.pushSample(data.gx, data.gy);
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
      <Waveform ref={waveformRef} width={width} height={height} config={waveformConfig} />
    </div>
  );
}
