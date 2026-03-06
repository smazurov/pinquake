import { useEffect, useState, useCallback } from "react";
import { LinkSlashIcon } from "@heroicons/react/20/solid";
import { connectOBS, disconnectOBS, getConfig, getOBSSources } from "../lib/api";
import type { OBSSource, PinQuakeConfig } from "../lib/api";
import Collapsible from "./Collapsible";

type OBSState = "disconnected" | "connecting" | "connected";

function statusColor(state: OBSState): string {
  if (state === "connected") return "bg-green-400";
  if (state === "connecting") return "bg-yellow-400";
  return "bg-red-400";
}

function StatusDot({ state }: Readonly<{ state: OBSState }>) {
  const color = statusColor(state);
  return <span className={`inline-block h-2 w-2 rounded-full ${color}`} />;
}

interface OBSControlProps {
  obsState: OBSState;
  config: PinQuakeConfig | null;
  onConnectionSave?: (host: string, port: number, password: string) => void;
  onSelectSource?: (sceneName: string, sourceName: string) => void;
}

export type { OBSState };

export default function OBSControl({ obsState, config, onConnectionSave, onSelectSource }: Readonly<OBSControlProps>) {
  const [error, setError] = useState<string | null>(null);
  const [host, setHost] = useState("localhost");
  const [port, setPort] = useState(4455);
  const [password, setPassword] = useState("");
  const [disconnecting, setDisconnecting] = useState(false);
  const [sources, setSources] = useState<OBSSource[]>([]);

  const selectedScene = config?.obs.scene_name ?? "";
  const selectedSource = config?.obs.source_name ?? "";

  useEffect(() => {
    getConfig().then((cfg) => {
      if (cfg.obs.host) setHost(cfg.obs.host);
      if (cfg.obs.port) setPort(cfg.obs.port);
      if (cfg.obs.password) setPassword(cfg.obs.password);
    }).catch(() => {});
  }, []);

  useEffect(() => {
    if (obsState === "connected") {
      let cancelled = false;
      getOBSSources()
        .then((result) => { if (!cancelled) setSources(result); })
        .catch((error_: unknown) => { if (!cancelled) setError(error_ instanceof Error ? error_.message : String(error_)); });
      return () => { cancelled = true; };
    }
    return undefined;
  }, [obsState]);

  const visibleSources = obsState === "connected" ? sources : [];
  const showDisconnecting = obsState !== "disconnected" && disconnecting;

  const handleConnect = useCallback(async () => {
    setError(null);
    try {
      await connectOBS(host, port, password);
      onConnectionSave?.(host, port, password);
    } catch (error_: unknown) {
      setError(error_ instanceof Error ? error_.message : String(error_));
    }
  }, [host, port, password, onConnectionSave]);

  const handleDisconnect = useCallback(async (e: React.MouseEvent) => {
    e.stopPropagation();
    setError(null);
    setDisconnecting(true);
    try {
      await disconnectOBS();
    } catch (error_: unknown) {
      setError(error_ instanceof Error ? error_.message : String(error_));
      setDisconnecting(false);
    }
  }, []);

  const isDisconnected = obsState === "disconnected";
  const isConnecting = obsState === "connecting";
  const isConnected = obsState === "connected";

  const sceneGroups = new Map<string, OBSSource[]>();
  for (const src of visibleSources) {
    const group = sceneGroups.get(src.scene_name) ?? [];
    group.push(src);
    sceneGroups.set(src.scene_name, group);
  }

  const headerContent = (
    <div className="flex items-center justify-between w-full">
      <div className="flex items-center gap-2">
        <StatusDot state={obsState} />
        <span>
          {isConnecting ? "Connecting..." : "OBS"}
        </span>
      </div>
      {isConnected && (
        <span
          role="button"
          tabIndex={0}
          onClick={(e) => void handleDisconnect(e)}
          onKeyDown={(e) => { if (e.key === "Enter") void handleDisconnect(e as unknown as React.MouseEvent); }}
          className={`text-red-400 hover:text-red-300 transition-colors cursor-pointer ${showDisconnecting ? "opacity-50 pointer-events-none" : ""}`}
          title="Disconnect from OBS"
        >
          <LinkSlashIcon className="h-[18px] w-[18px]" />
        </span>
      )}
    </div>
  );

  return (
    <Collapsible id="obs" header={headerContent} defaultOpen={true}>
      {error && (
        <div className="rounded bg-red-900/50 px-3 py-2 text-xs text-red-300">
          {error}
        </div>
      )}

      {isDisconnected && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs text-slate-400 mb-1">Host</label>
              <input
                type="text"
                value={host}
                onChange={(e) => setHost(e.target.value)}
                className="w-full rounded bg-slate-700 border border-slate-600 px-2 py-1.5 text-xs text-slate-200"
              />
            </div>
            <div>
              <label className="block text-xs text-slate-400 mb-1">Port</label>
              <input
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
                className="w-full rounded bg-slate-700 border border-slate-600 px-2 py-1.5 text-xs text-slate-200"
              />
            </div>
          </div>
          <div>
            <label className="block text-xs text-slate-400 mb-1">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded bg-slate-700 border border-slate-600 px-2 py-1.5 text-xs text-slate-200"
              placeholder="(optional)"
            />
          </div>
          <button
            onClick={() => void handleConnect()}
            className="w-full rounded bg-blue-600 hover:bg-blue-500 px-3 py-1.5 text-xs font-medium text-white transition-colors"
          >
            Connect
          </button>
        </div>
      )}

      {isConnecting && (
        <p className="text-xs text-slate-400 animate-pulse">Connecting to OBS...</p>
      )}

      {isConnected && visibleSources.length === 0 && !error && (
        <p className="text-xs text-slate-500">No browser sources found</p>
      )}

      {isConnected && visibleSources.length > 0 && (
        <div className="space-y-2 max-h-[280px] overflow-y-auto">
          {[...sceneGroups.entries()].map(([sceneName, items]) => (
            <div key={sceneName}>
              <div className="text-[10px] uppercase tracking-wider text-slate-500 mb-1">
                {sceneName}
              </div>
              {items.map((src) => {
                const isSelected = src.scene_name === selectedScene && src.source_name === selectedSource;
                return (
                  <button
                    key={`${src.scene_name}/${src.source_name}`}
                    className={`w-full text-left rounded px-3 py-2 text-sm transition-colors ${
                      isSelected
                        ? "bg-blue-600/30 border border-blue-500/50"
                        : "hover:bg-slate-700/50"
                    }`}
                    onClick={() => onSelectSource?.(src.scene_name, src.source_name)}
                  >
                    <div className="text-slate-200 truncate">{src.source_name}</div>
                    {src.url && (
                      <div className="text-xs text-slate-500 truncate font-mono">
                        {src.url}
                      </div>
                    )}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </Collapsible>
  );
}
