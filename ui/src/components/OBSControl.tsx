import { useEffect, useMemo, useState } from "react";
import { ArrowPathIcon, EyeIcon, EyeSlashIcon } from "@heroicons/react/20/solid";
import { api } from "../lib/api";
import type { components } from "../lib/api.generated";
import {
  NO_TARGET,
  findTargetOption,
  hasTarget,
  obsStatusView,
  targetLabel,
  targetOptions,
  type OBSScene,
  type OBSStatus,
  type OBSTarget,
  type StatusTone,
} from "../lib/obs";
import Collapsible from "./Collapsible";
import { InputField } from "./InputField";
import { Button } from "./Button";
import { ErrorAlert } from "./ErrorAlert";

type OBSConfig = components["schemas"]["OBSConfig"];

const TONE_DOT: Record<StatusTone, string> = {
  off: "bg-red-400",
  busy: "bg-yellow-400",
  warn: "bg-orange-400",
  ok: "bg-green-400",
};

const NONE_KEY = "";
const MISSING_KEY = "missing";

function configTarget(c: OBSConfig): OBSTarget {
  return { scene: c.scene, scene_uuid: c.scene_uuid, source: c.source, source_uuid: c.source_uuid };
}

export default function OBSControl({ config, status }: Readonly<{ config: OBSConfig; status: OBSStatus | null }>) {
  const [server, setServer] = useState(config.server);
  const [password, setPassword] = useState(config.password);
  const [showPassword, setShowPassword] = useState(false);
  const [saved, setSaved] = useState({ server: config.server, password: config.password });
  const [target, setTarget] = useState<OBSTarget>(() => configTarget(config));
  const [scenes, setScenes] = useState<OBSScene[]>([]);
  const [reloadKey, setReloadKey] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const view = obsStatusView(status);
  const wanted = status !== null && status.state !== "off";
  const dirty = server !== saved.server || password !== saved.password;

  // List scenes whenever a session comes up (OBS may have restarted) and on
  // reload.
  useEffect(() => {
    if (!view.connected) return;
    let cancelled = false;
    void api.GET("/api/obs/scenes").then(({ data, error }) => {
      if (cancelled) return;
      if (error) setError(error.detail ?? "Failed to list OBS scenes");
      else setScenes(data.scenes ?? []);
    });
    return () => { cancelled = true; };
  }, [view.connected, reloadKey]);

  const handleConnect = async () => {
    setBusy(true);
    setError(null);
    const { error } = await api.POST("/api/obs/connect", { body: { server, password } });
    setBusy(false);
    if (error) {
      setError(error.detail ?? "Failed to connect");
      return;
    }
    setSaved({ server: server.trim(), password });
  };

  const handleDisconnect = async () => {
    setBusy(true);
    setError(null);
    const { error } = await api.POST("/api/obs/disconnect");
    setBusy(false);
    if (error) setError(error.detail ?? "Failed to disconnect");
  };

  const groups = useMemo(() => (view.connected ? targetOptions(scenes) : []), [view.connected, scenes]);
  const selected = findTargetOption(groups, target);
  let selectedKey = NONE_KEY;
  if (selected) selectedKey = selected.key;
  else if (hasTarget(target)) selectedKey = MISSING_KEY;

  const handleSelect = async (key: string) => {
    const next = key === NONE_KEY
      ? NO_TARGET
      : groups.flatMap((g) => g.options).find((o) => o.key === key)?.target;
    if (!next) return;
    setError(null);
    const { error } = await api.PUT("/api/obs/target", { body: next });
    if (error) {
      setError(error.detail ?? "Failed to set target");
      return;
    }
    setTarget(next);
  };

  const header = (
    <div className="flex items-center gap-2 min-w-0">
      <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${TONE_DOT[view.tone]}`} />
      <span>OBS</span>
      <span className="text-xs font-normal text-slate-400 truncate">{view.text}</span>
    </div>
  );

  return (
    <Collapsible id="obs" header={header} defaultOpen={false}>
      {error && <ErrorAlert message={error} />}

      <p className="text-xs text-slate-400">
        When connected, the trigger shows and hides a source or group in OBS and the overlays inside it stay drawn.
        Otherwise the overlays hide themselves.
      </p>

      <div className="grid grid-cols-2 gap-4">
        <InputField
          label="Server"
          placeholder="localhost:4455"
          value={server}
          onChange={(e) => setServer(e.target.value)}
        />
        <div className="space-y-1">
          <label className="block text-sm font-medium text-gray-300">Password</label>
          <div className="relative">
            <input
              type={showPassword ? "text" : "password"}
              placeholder="optional"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="off"
              className="block w-full rounded-sm border border-slate-300/20 bg-slate-800 px-3 py-2 pr-9 text-sm text-white focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            <button
              type="button"
              onClick={() => setShowPassword((s) => !s)}
              className="absolute inset-y-0 right-0 px-2 text-slate-400 hover:text-slate-300"
              title={showPassword ? "Hide password" : "Show password"}
            >
              {showPassword ? <EyeSlashIcon className="h-4 w-4" /> : <EyeIcon className="h-4 w-4" />}
            </button>
          </div>
        </div>
      </div>

      <div className="flex gap-2">
        {(!wanted || dirty) && (
          <Button
            size="SM"
            theme="primary"
            text={wanted ? "Reconnect" : "Connect"}
            disabled={busy || server.trim() === ""}
            onClick={() => void handleConnect()}
          />
        )}
        {wanted && (
          <Button size="SM" theme="light" text="Disconnect" disabled={busy} onClick={() => void handleDisconnect()} />
        )}
      </div>

      <div className="space-y-1">
        <label className="block text-sm font-medium text-gray-300">Show / hide in OBS</label>
        <div className="flex gap-2">
          <select
            value={selectedKey}
            disabled={!view.connected}
            onChange={(e) => void handleSelect(e.target.value)}
            className="block w-full min-w-0 rounded-sm border border-slate-300/20 bg-slate-800 px-3 py-2 text-sm text-white focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
          >
            <option value={NONE_KEY}>None (overlays hide themselves)</option>
            {selectedKey === MISSING_KEY && (
              <option value={MISSING_KEY} disabled>
                {targetLabel(target)}{view.connected ? " (not found)" : ""}
              </option>
            )}
            {groups.map((g) => (
              <optgroup key={g.scene} label={g.scene}>
                {g.options.map((o) => (
                  <option key={o.key} value={o.key}>{o.label}</option>
                ))}
              </optgroup>
            ))}
          </select>
          <button
            type="button"
            onClick={() => setReloadKey((k) => k + 1)}
            disabled={!view.connected}
            className="text-slate-400 hover:text-slate-300 disabled:opacity-40"
            title="Reload scenes from OBS"
          >
            <ArrowPathIcon className="h-[18px] w-[18px]" />
          </button>
        </div>
        <p className="text-xs text-slate-500">
          Keep &quot;Shutdown source when not visible&quot; off on PinQuake browser sources so they don&apos;t reload on every trigger.
        </p>
      </div>
    </Collapsible>
  );
}
