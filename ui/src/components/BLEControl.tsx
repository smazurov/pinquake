import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { LinkIcon, LinkSlashIcon, NoSymbolIcon, BoltIcon, Battery0Icon, Battery50Icon, Battery100Icon, LockClosedIcon, LockOpenIcon, ArrowPathIcon, XMarkIcon } from "@heroicons/react/20/solid";
import { SSEClient, api } from "../lib/api";
import type { SSEStatus } from "../lib/api";
import type { components } from "../lib/api.generated";
import type { OBSStatus } from "../lib/obs";
import { bleDeviceLabel, bleStatusView, scanNote, type BLEScanState, type BLEStatus, type BLEStatusView, type BLETone } from "../lib/ble";

type BLEScanResult = components["schemas"]["BLEScanResultEvent"];
type LogEntry = components["schemas"]["LogEntry"];
type FrameState = Pick<components["schemas"]["FrameStateBody"], "enabled" | "state">;
import { ErrorAlert } from "./ErrorAlert";
import Collapsible from "./Collapsible";

const ICON_CLS = "h-[18px] w-[18px]";
const FRAME_ENDPOINT = "/api/ble/frame" as const;

const TONE_DOT: Record<BLETone, string> = {
  off: "bg-red-400",
  busy: "bg-yellow-400",
  warn: "bg-orange-400",
  ok: "bg-green-400",
};

function StatusDot({ tone, flashKey }: Readonly<{ tone: BLETone; flashKey?: number }>) {
  return (
    <span
      key={flashKey}
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${TONE_DOT[tone]} ${flashKey ? "animate-[dot-flash_0.4s_ease-out]" : ""}`}
    />
  );
}

function LockIcon({ frame }: Readonly<{ frame: FrameState }>) {
  if (!frame.enabled) return <LockOpenIcon className={ICON_CLS} />;
  if (frame.state === "locked") return <LockClosedIcon className={ICON_CLS} />;
  return <LockClosedIcon className={`${ICON_CLS} animate-pulse`} />;
}

function lockButtonStyle(frame: FrameState): { cls: string; title: string } {
  if (!frame.enabled) return { cls: "text-slate-400 hover:text-slate-300", title: "Enable auto-lock" };
  if (frame.state === "locked") return { cls: "text-green-400 hover:text-green-300", title: "Frame locked. Click to disable auto-lock" };
  return { cls: "text-amber-400 hover:text-amber-300", title: "Waiting for a stable reading to lock. Click to disable auto-lock" };
}

function headerLabel(view: BLEStatusView, scanning: boolean, forgetting: boolean): string {
  if (forgetting) return view.connected ? "Disconnecting…" : "Forgetting…";
  if (scanning) return "Scanning…";
  return view.text;
}

/** When the next attempt is due (ms since epoch), if waiting. */
function retryDeadline(status: BLEStatus, now: number): number | null {
  return status.status === "waiting" && status.retry_in_s ? now + status.retry_in_s * 1000 : null;
}

function batteryProps(percent: number) {
  if (percent <= 25) return { Icon: Battery0Icon, color: "text-red-500" };
  if (percent <= 75) return { Icon: Battery50Icon, color: "text-yellow-500" };
  return { Icon: Battery100Icon, color: "text-green-500" };
}

function BatteryIndicator({ percent }: Readonly<{ percent: number }>) {
  const { Icon, color } = batteryProps(percent);
  return <Icon className={`${ICON_CLS} ${color}`} />;
}

const levelColor: Record<string, string> = {
  info: "text-slate-400",
  warn: "text-amber-400",
  error: "text-red-400",
};

function formatTime(timestamp: string): string {
  const d = new Date(timestamp);
  return d.toLocaleTimeString("en-GB", { hour12: false });
}

export default function BLEControl({ onSSEStatus, onSensorChange, onOBSStatus }: Readonly<{
  onSSEStatus?: (status: SSEStatus) => void;
  onSensorChange?: (sensorName: string | null) => void;
  onOBSStatus?: (status: OBSStatus) => void;
}>) {
  const [status, setStatus] = useState<BLEStatus | null>(null);
  // Waiting: when the next attempt is due, and a clock ticking towards it.
  const [retryAt, setRetryAt] = useState<number | null>(null);
  const [now, setNow] = useState(0);
  const [scanResults, setScanResults] = useState<Map<string, BLEScanResult>>(
    new Map(),
  );
  const [scanning, setScanning] = useState(false);
  const [scanState, setScanState] = useState<BLEScanState | null>(null);
  const [choosing, setChoosing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [forgetting, setForgetting] = useState(false);
  const [frame, setFrame] = useState<FrameState>({ enabled: false, state: "unlocked" });
  const [battery, setBattery] = useState<{ percent: number; volts: number; charging: boolean } | null>(null);
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
  const [lockSpinKey, setLockSpinKey] = useState(0);

  const mainSSE = useRef<SSEClient<"/api/events"> | null>(null);
  const scanSSE = useRef<SSEClient<"/api/ble/scan"> | null>(null);
  const onSSEStatusRef = useRef(onSSEStatus);
  useEffect(() => { onSSEStatusRef.current = onSSEStatus; }, [onSSEStatus]);
  const onSensorChangeRef = useRef(onSensorChange);
  useEffect(() => { onSensorChangeRef.current = onSensorChange; }, [onSensorChange]);
  const onOBSStatusRef = useRef(onOBSStatus);
  useEffect(() => { onOBSStatusRef.current = onOBSStatus; }, [onOBSStatus]);
  useEffect(() => {
    const client = new SSEClient({
      endpoint: "/api/events",
      onStatusChange: (status) => {
        onSSEStatusRef.current?.(status);
        if (status === "reconnecting" || status === "disconnected") {
          setStatus(null); // unknown until the server sends it again
          setRetryAt(null);
        }
      },
    });
    client.on("ble-status", (data) => {
      setStatus(data);
      const t = Date.now();
      setNow(t);
      setRetryAt(retryDeadline(data, t));
      if (data.status === "idle") setForgetting(false);
      // Scanning is for choosing a device; once one is chosen, stop.
      if (data.status !== "idle") {
        scanSSE.current?.disconnect();
        scanSSE.current = null;
        setScanning(false);
        setScanResults(new Map());
        setScanState(null);
      }
      if (data.status === "connected") {
        onSensorChangeRef.current?.(data.sensor_name ?? null);
        void api.GET(FRAME_ENDPOINT).then(({ data }) => { if (data) setFrame({ enabled: data.enabled, state: data.state }); });
      } else {
        setBattery(null);
        onSensorChangeRef.current?.(null);
      }
    });
    client.on("battery", (data) => {
      setBattery((prev) => {
        if (prev && prev.percent === data.battery_percent && prev.volts === data.battery_volts && prev.charging === data.charging) return prev;
        return { percent: data.battery_percent, volts: data.battery_volts, charging: data.charging };
      });
    });
    client.on("log", (data) => {
      setLogEntries((prev) => [...prev, data].slice(-200));
    });
    client.on("obs-status", (data) => {
      onOBSStatusRef.current?.(data);
    });
    client.on("frame-state", (data) => {
      setFrame({ enabled: data.enabled, state: data.state });
      if (data.reason) setLockSpinKey((n) => n + 1);
    });
    client.connect();
    mainSSE.current = client;
    return () => {
      client.disconnect();
      mainSSE.current = null;
    };
  }, []);

  const startScan = useCallback(() => {
    if (scanSSE.current) return;
    setScanResults(new Map());
    setScanState(null);
    setError(null);
    setScanning(true);

    const client = new SSEClient({
      endpoint: "/api/ble/scan",
      onError: () => {
        client.disconnect();
        scanSSE.current = null;
        setScanning(false);
        setError("Lost the scan connection to the server");
      },
    });
    client.on("scan-state", (data) => {
      setScanState(data);
      if (data.state !== "ended") return;
      // The server closes the stream; stop before the client would reconnect.
      client.disconnect();
      scanSSE.current = null;
      setScanning(false);
      // Timed out: the list stays pickable. Device chosen: it's done.
      if (data.reason !== "timeout") setScanResults(new Map());
    });
    client.on("device", (data) => {
      setScanResults((prev) => {
        const next = new Map(prev);
        next.set(data.address, data);
        return next;
      });
    });
    client.connect();
    scanSSE.current = client;
  }, []);

  const stopScan = useCallback(() => {
    if (scanSSE.current) {
      scanSSE.current.disconnect();
      scanSSE.current = null;
    }
    setScanning(false);
    setScanResults(new Map());
    setScanState(null);
  }, []);

  // Choosing a device ends the scan on the server, which closes the list.
  const handleConnect = useCallback(
    async (device: { address: string; name: string }) => {
      setError(null);
      setChoosing(true);
      const { error: err } = await api.POST("/api/ble/connect", {
        body: { address: device.address, name: device.name },
      });
      setChoosing(false);
      if (err) setError(err.detail ?? "Connection failed");
    },
    [],
  );

  const handleToggleFrameLock = useCallback(async () => {
    const action = frame.enabled ? "disable" : "enable";
    const { data, error: err } = await api.POST(FRAME_ENDPOINT, { body: { action } });
    if (err) { setError(err.detail ?? "Frame lock failed"); return; }
    setFrame({ enabled: data.enabled, state: data.state });
  }, [frame.enabled]);

  const lockStyle = lockButtonStyle(frame);

  // Disconnects, or stops searching for the chosen device, and forgets it.
  const handleForget = useCallback(async () => {
    setError(null);
    setForgetting(true);
    const { error: err } = await api.POST("/api/ble/disconnect");
    if (err) {
      setError(err.detail ?? "Disconnect failed");
      setForgetting(false);
    }
  }, []);

  useEffect(() => {
    if (retryAt === null) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [retryAt]);

  const hoveringRef = useRef(false);
  const [sortFlash, setSortFlash] = useState(0);
  const [sortOrder, setSortOrder] = useState<string[]>([]);
  const scanResultsRef = useRef(scanResults);
  useEffect(() => {
    scanResultsRef.current = scanResults;
  }, [scanResults]);

  const recomputeOrder = useCallback(() => {
    const sorted = [...scanResultsRef.current.values()].sort((a, b) => {
      const aKnown = a.sensor_name ? 1 : 0;
      const bKnown = b.sensor_name ? 1 : 0;
      if (aKnown !== bKnown) return bKnown - aKnown;
      return b.rssi - a.rssi;
    });
    setSortOrder(sorted.map((d) => d.address));
    setSortFlash((n) => n + 1);
  }, []);

  useEffect(() => {
    recomputeOrder();
    const id = setInterval(() => {
      if (!hoveringRef.current) recomputeOrder();
    }, 1000);
    return () => clearInterval(id);
  }, [scanning, recomputeOrder]);

  const sortedResults = useMemo(() => {
    const orderMap = new Map(sortOrder.map((addr, i) => [addr, i]));
    return [...scanResults.values()].sort((a, b) => {
      const ai = orderMap.get(a.address) ?? Infinity;
      const bi = orderMap.get(b.address) ?? Infinity;
      return ai - bi;
    });
  }, [scanResults, sortOrder]);

  const reversedLog = useMemo(() => [...logEntries].reverse(), [logEntries]);

  const view = bleStatusView(status, retryAt === null ? 0 : Math.max(0, (retryAt - now) / 1000));
  const canScan = status?.status === "idle" && !scanning;
  const name = status ? bleDeviceLabel(status) : "";

  const label = headerLabel(view, scanning, forgetting);
  const note = scanNote(scanState);

  const headerContent = (
    <div className="flex items-center justify-between gap-2 w-full min-w-0">
      <div className="flex items-center gap-2 min-w-0">
        <StatusDot tone={scanning ? "busy" : view.tone} flashKey={scanning ? sortFlash : undefined} />
        {view.connected && !forgetting ? (
          <span className="flex items-center gap-2">
            <span className="text-xs text-slate-300 truncate max-w-[140px]">
              {view.text}
            </span>
            {battery && (
              <span className="flex items-center gap-1 text-slate-400 shrink-0" title={`${battery.percent}%${battery.charging ? " (charging)" : ""}`}>
                <BatteryIndicator percent={battery.percent} />
                {battery.charging && <BoltIcon className="h-3.5 w-3.5" />}
              </span>
            )}
            <button
              onClick={(e) => { e.stopPropagation(); void handleToggleFrameLock(); }}
              className={`transition-colors ${lockStyle.cls}`}
              title={lockStyle.title}
            >
              <LockIcon frame={frame} />
            </button>
            {frame.enabled && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  // Spins when the lock lands (frame-state event, ~0.5s).
                  void api.POST(FRAME_ENDPOINT, { body: { action: "trigger" } });
                }}
                className="text-slate-400 hover:text-slate-300 transition-colors"
                title="Force lock now"
              >
                <ArrowPathIcon
                  key={lockSpinKey}
                  className={`${ICON_CLS}${lockSpinKey > 0 ? " animate-[spin_0.5s_ease-in-out]" : ""}`}
                />
              </button>
            )}
          </span>
        ) : (
          <span className="text-xs font-normal text-slate-400 truncate" title={label}>{label}</span>
        )}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        {canScan && (
          <button
            onClick={(e) => { e.stopPropagation(); startScan(); }}
            className="text-blue-400 hover:text-blue-300 transition-colors"
            title="Scan for devices"
          >
            <LinkIcon className={ICON_CLS} />
          </button>
        )}
        {scanning && (
          <button
            onClick={(e) => { e.stopPropagation(); stopScan(); }}
            className="text-yellow-400 hover:text-yellow-300 transition-colors"
            title="Stop scan"
          >
            <NoSymbolIcon className={ICON_CLS} />
          </button>
        )}
        {view.chosen && (
          <button
            onClick={(e) => { e.stopPropagation(); void handleForget(); }}
            className={`text-red-400 hover:text-red-300 transition-colors ${forgetting ? "opacity-50 pointer-events-none" : ""}`}
            title={view.connected ? `Disconnect and forget ${name}` : `Stop searching and forget ${name}`}
            disabled={forgetting}
          >
            {view.connected ? <LinkSlashIcon className={ICON_CLS} /> : <XMarkIcon className={ICON_CLS} />}
          </button>
        )}
      </div>
    </div>
  );

  return (
    <Collapsible id="ble" header={headerContent} defaultOpen={true} forceOpen={scanning}>
      {error && (
        <div className="mb-3">
          <ErrorAlert message={error} />
        </div>
      )}

      {sortedResults.length > 0 && !view.chosen && (
        <div
          className="max-h-[280px] overflow-y-auto space-y-1"
          onMouseEnter={() => { hoveringRef.current = true; }}
          onMouseLeave={() => { hoveringRef.current = false; }}
        >
          {sortedResults.slice(0, 10).map((device) => (
            <button
              key={device.address}
              className="w-full flex items-center justify-between rounded px-3 py-2 text-left text-sm hover:bg-slate-700/50 transition-colors disabled:opacity-50 disabled:pointer-events-none"
              disabled={choosing}
              onClick={() => void handleConnect(device)}
            >
              <div className="min-w-0">
                <div className="text-slate-200 truncate">
                  {device.name || "Unknown"}
                  {device.sensor_name && (
                    <span className="ml-2 rounded bg-emerald-900/60 px-1.5 py-0.5 text-[10px] font-medium text-emerald-400">
                      {device.sensor_name}
                    </span>
                  )}
                </div>
                <div className="text-xs text-slate-500 font-mono">
                  {device.address}
                </div>
              </div>
              <span className="text-xs text-slate-400 shrink-0 ml-3">
                {device.rssi} dBm
              </span>
            </button>
          ))}
        </div>
      )}

      {note && !view.chosen && <p className="text-xs text-slate-400">{note}</p>}

      {reversedLog.length > 0 && (
        <div className="max-h-48 overflow-y-auto space-y-1 font-mono text-xs border-t border-slate-700 pt-3">
          {reversedLog.map((entry, i) => (
            <div key={i} className={levelColor[entry.level] ?? "text-slate-400"}>
              <span className="text-slate-500 mr-2">{formatTime(entry.timestamp)}</span>
              {entry.message}
            </div>
          ))}
        </div>
      )}
    </Collapsible>
  );
}
