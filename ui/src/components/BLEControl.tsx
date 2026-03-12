import { useEffect, useRef, useState, useCallback, useMemo } from "react";
import { LinkIcon, LinkSlashIcon, NoSymbolIcon, BoltIcon, Battery0Icon, Battery50Icon, Battery100Icon, LockClosedIcon, LockOpenIcon, ArrowPathIcon } from "@heroicons/react/20/solid";
import { SSEClient, api } from "../lib/api";
import type { SSEStatus } from "../lib/api";
import type { components } from "../lib/api.generated";

type BLEScanResult = components["schemas"]["BLEScanResultEvent"];
type LogEntry = components["schemas"]["LogEntry"];
import { ErrorAlert } from "./ErrorAlert";
import Collapsible from "./Collapsible";

const ICON_CLS = "h-[18px] w-[18px]";

type BLEState = "idle" | "scanning" | "connecting" | "connected" | "disconnected";

function statusColor(state: BLEState, scanning: boolean, reason: string | null): string {
  if (state === "connected") return "bg-green-400";
  if (state === "connecting" || scanning) return "bg-yellow-400";
  if (state === "disconnected" && reason === "lost") return "bg-orange-400";
  return "bg-red-400";
}

function StatusDot({ state, scanning, reason, flashKey }: Readonly<{ state: BLEState; scanning: boolean; reason: string | null; flashKey?: number }>) {
  const color = statusColor(state, scanning, reason);
  return (
    <span
      key={flashKey}
      className={`inline-block h-2 w-2 rounded-full ${color} ${flashKey ? "animate-[dot-flash_0.4s_ease-out]" : ""}`}
    />
  );
}

function formatStateLabel(state: BLEState, scanning: boolean, disconnecting: boolean, reason: string | null): string {
  if (disconnecting) return "Disconnecting";
  if (scanning) return "Scanning";
  if (state === "disconnected" && reason === "lost") return "Connection lost";
  return state.charAt(0).toUpperCase() + state.slice(1);
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

export default function BLEControl({ onSSEStatus, onSensorChange }: Readonly<{ onSSEStatus?: (status: SSEStatus) => void; onSensorChange?: (sensorName: string | null) => void }>) {
  const [bleState, setBleState] = useState<BLEState>("idle");
  const [scanResults, setScanResults] = useState<Map<string, BLEScanResult>>(
    new Map(),
  );
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deviceName, setDeviceName] = useState<string | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [frameLocked, setFrameLocked] = useState(false);
  const [battery, setBattery] = useState<{ percent: number; volts: number; charging: boolean } | null>(null);
  const [disconnectReason, setDisconnectReason] = useState<string | null>(null);
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);

  const mainSSE = useRef<SSEClient<"/api/events"> | null>(null);
  const scanSSE = useRef<SSEClient<"/api/ble/scan"> | null>(null);
  const onSSEStatusRef = useRef(onSSEStatus);
  useEffect(() => { onSSEStatusRef.current = onSSEStatus; }, [onSSEStatus]);
  const onSensorChangeRef = useRef(onSensorChange);
  useEffect(() => { onSensorChangeRef.current = onSensorChange; }, [onSensorChange]);
  useEffect(() => {
    const client = new SSEClient({
      endpoint: "/api/events",
      onStatusChange: (status) => {
        onSSEStatusRef.current?.(status);
        if (status === "reconnecting" || status === "disconnected") {
          setBleState("disconnected");
        }
      },
    });
    client.on("ble-status", (data) => {
      setBleState(data.status as BLEState);
      if (data.status === "disconnected") {
        setDisconnectReason(data.reason ?? null);
      } else {
        setDisconnectReason(null);
      }
      if (data.status === "idle" || data.status === "disconnected") {
        setDisconnecting(false);
        setFrameLocked(false);
        setDeviceName(null);
        setBattery(null);
        onSensorChangeRef.current?.(null);
      }
      if (data.status === "connected" || data.status === "connecting") {
        setDeviceName(data.device_name ?? null);
        setScanResults(new Map());
      }
      if (data.status === "connected") {
        onSensorChangeRef.current?.(data.sensor_name ?? null);
        void api.GET("/api/ble/frame").then(({ data }) => { if (data) setFrameLocked(data.locked); });
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
    setError(null);
    setScanning(true);

    const client = new SSEClient({
      endpoint: "/api/ble/scan",
      onError: () => {
        client.disconnect();
        scanSSE.current = null;
        setScanning(false);
        setError("BLE scan failed — check adapter");
      },
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
  }, []);

  const handleConnect = useCallback(
    async (device: { address: string; name: string }) => {
      stopScan();
      setError(null);
      const { error: err } = await api.POST("/api/ble/connect", {
        body: { address: device.address, name: device.name },
      });
      if (err) setError(err.detail ?? "Connection failed");
    },
    [stopScan],
  );

  const handleToggleFrameLock = useCallback(async () => {
    const { error: err } = frameLocked
      ? await api.POST("/api/ble/frame/unlock")
      : await api.POST("/api/ble/frame/lock");
    if (err) { setError(err.detail ?? "Frame lock failed"); return; }
    setFrameLocked(!frameLocked);
  }, [frameLocked]);

  const handleDisconnect = useCallback(async () => {
    setError(null);
    setDisconnecting(true);
    const { error: err } = await api.POST("/api/ble/disconnect");
    if (err) {
      setError(err.detail ?? "Disconnect failed");
      setDisconnecting(false);
    }
  }, []);

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

  const isIdle = (bleState === "idle" || bleState === "disconnected") && !scanning;
  const isConnecting = bleState === "connecting";
  const isConnected = bleState === "connected";

  const stateLabel = formatStateLabel(bleState, scanning, disconnecting, disconnectReason);

  const headerContent = (
    <div className="flex items-center justify-between w-full">
      <div className="flex items-center gap-2">
        <StatusDot state={bleState} scanning={scanning} reason={disconnectReason} flashKey={scanning ? sortFlash : undefined} />
        {isConnected && deviceName ? (
          <span className="flex items-center gap-2">
            <span className="text-xs text-slate-300 truncate max-w-[140px]">
              {deviceName}
            </span>
            {battery && (
              <span className="flex items-center gap-1 text-slate-400 shrink-0" title={`${battery.percent}%${battery.charging ? " (charging)" : ""}`}>
                <BatteryIndicator percent={battery.percent} />
                {battery.charging && <BoltIcon className="h-3.5 w-3.5" />}
              </span>
            )}
            <button
              onClick={(e) => { e.stopPropagation(); void handleToggleFrameLock(); }}
              className={`transition-colors ${
                frameLocked
                  ? "text-green-400 hover:text-green-300"
                  : "text-slate-400 hover:text-slate-300"
              }`}
              title={frameLocked ? "Disable auto-lock" : "Enable auto-lock"}
            >
              {frameLocked ? <LockClosedIcon className={ICON_CLS} /> : <LockOpenIcon className={ICON_CLS} />}
            </button>
            {frameLocked && (
              <button
                onClick={(e) => { e.stopPropagation(); void api.POST("/api/ble/frame/force-lock"); }}
                className="text-slate-400 hover:text-slate-300 transition-colors"
                title="Force lock now"
              >
                <ArrowPathIcon className={ICON_CLS} />
              </button>
            )}
          </span>
        ) : (
          <span className="text-xs text-slate-400">{stateLabel}</span>
        )}
      </div>
      <div className="flex items-center gap-3">
        {isIdle && (
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
        {isConnecting && (
          <span className="text-slate-400" title="Connecting...">
            <LinkIcon className={`${ICON_CLS} animate-pulse`} />
          </span>
        )}
        {isConnected && (
          <button
            onClick={(e) => { e.stopPropagation(); void handleDisconnect(); }}
            className={`text-red-400 hover:text-red-300 transition-colors ${disconnecting ? "opacity-50 pointer-events-none" : ""}`}
            title="Disconnect"
            disabled={disconnecting}
          >
            <LinkSlashIcon className={ICON_CLS} />
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

      {sortedResults.length > 0 && !isConnected && (
        <div
          className="max-h-[280px] overflow-y-auto space-y-1"
          onMouseEnter={() => { hoveringRef.current = true; }}
          onMouseLeave={() => { hoveringRef.current = false; }}
        >
          {sortedResults.slice(0, 10).map((device) => (
            <button
              key={device.address}
              className="w-full flex items-center justify-between rounded px-3 py-2 text-left text-sm hover:bg-slate-700/50 transition-colors disabled:opacity-50 disabled:pointer-events-none"
              disabled={isConnecting}
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
