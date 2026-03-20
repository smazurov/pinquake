import { useEffect, useRef, useCallback, useState, useMemo } from "react";
import { useBeforeUnload } from "react-router-dom";
import type { ShaderVizHandle, ShaderVizConfig } from "../lib/shaderViz";
import experimentRegistry from "../lib/experimentRegistry";
import { SSEClient, api } from "../lib/api";

const SIZE = 400;

function getShowIds(): Set<string> {
  const param = new URLSearchParams(window.location.search).get("show");
  if (!param) return new Set(experimentRegistry.map((e) => e.id));
  return new Set(param.split(",").filter(Boolean));
}

function ExperimentRenderer(
  { entry, config, visible, onHandle }: Readonly<{
    entry: (typeof experimentRegistry)[number];
    config: ShaderVizConfig | undefined;
    visible: boolean;
    onHandle: (id: string, handle: ShaderVizHandle | null) => void;
  }>,
) {
  const ref = useRef<ShaderVizHandle>(null);

  useEffect(() => {
    if (ref.current) onHandle(entry.id, ref.current);
    return () => { onHandle(entry.id, null); };
  }, [entry.id, onHandle]);

  return (
    <div style={{ display: visible ? "block" : "none" }}>
      <entry.Component ref={ref} width={SIZE} height={SIZE} config={config} />
    </div>
  );
}

export default function ExperimentRoute() {
  const [enabled, setEnabled] = useState(true);
  const [visible, setVisible] = useState(false);
  const [shaderConfig, setShaderConfig] = useState<ShaderVizConfig | undefined>();
  const sseRef = useRef<SSEClient<"/api/events"> | null>(null);
  const handleRefs = useRef(new Map<string, ShaderVizHandle>());

  const showIds = useMemo(() => getShowIds(), []);
  const active = useMemo(
    () => experimentRegistry.filter((e) => showIds.has(e.id)),
    [showIds],
  );

  const fetchConfig = useCallback(() => {
    api.GET("/api/config/experiment")
      .then(({ data: cfg }) => {
        if (!cfg) return;
        setEnabled(cfg.enabled);
        setShaderConfig({
          forceYellowG: cfg.force_yellow_g,
          forceRedG: cfg.force_red_g,
          decayS: cfg.decay_s,
        });
      })
      .catch(() => {});
  }, []);

  const onHandle = useCallback((id: string, handle: ShaderVizHandle | null) => {
    if (handle) handleRefs.current.set(id, handle);
    else handleRefs.current.delete(id);
  }, []);

  useBeforeUnload(useCallback(() => {
    sseRef.current?.disconnect();
  }, []));

  useEffect(() => {
    fetchConfig();

    const client = new SSEClient({ endpoint: "/api/events" });
    sseRef.current = client;

    client.on("orientation", (data) => {
      for (const handle of handleRefs.current.values()) {
        handle.pushSample(data.x, data.y);
      }
    });

    client.on("viz-trigger", (data) => {
      setVisible(data.visible);
    });

    client.on("config-changed", (data) => {
      if (data.section === "experiment") fetchConfig();
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
        display: visible ? "flex" : "none",
        gap: 8,
        background: "transparent",
      }}
    >
      {active.map((entry) => (
        <ExperimentRenderer
          key={entry.id}
          entry={entry}
          config={shaderConfig}
          visible={visible}
          onHandle={onHandle}
        />
      ))}
    </div>
  );
}
