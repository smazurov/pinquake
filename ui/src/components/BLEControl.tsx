import { useEffect, useRef, useState, useCallback } from "react";
import { SSEClient } from "../lib/api_sse";
import { connectDevice, disconnectDevice, getConfig, lockFrame, unlockFrame, getFrameState } from "../lib/api";
import type { BLEScanResult } from "../lib/api";
import { Card, CardHeader, CardContent } from "./Card";

type BLEState = "idle" | "scanning" | "connecting" | "connected" | "disconnected";

function StatusDot({ state, scanning }: { state: BLEState; scanning: boolean }) {
  const color =
    state === "connected"
      ? "bg-green-400"
      : state === "connecting" || scanning
        ? "bg-yellow-400"
        : "bg-red-400";
  return <span className={`inline-block h-2 w-2 rounded-full ${color}`} />;
}

function ScanIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
      <path
        fillRule="evenodd"
        d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z"
        clipRule="evenodd"
      />
    </svg>
  );
}

function StopIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
      <rect x="4" y="4" width="12" height="12" rx="2" />
    </svg>
  );
}

function DisconnectIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 20 20" fill="currentColor" width="18" height="18">
      <path
        fillRule="evenodd"
        d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.28 7.22a.75.75 0 00-1.06 1.06L8.94 10l-1.72 1.72a.75.75 0 101.06 1.06L10 11.06l1.72 1.72a.75.75 0 101.06-1.06L11.06 10l1.72-1.72a.75.75 0 00-1.06-1.06L10 8.94 8.28 7.22z"
        clipRule="evenodd"
      />
    </svg>
  );
}

function Spinner({ className }: { className?: string }) {
  return (
    <svg className={`animate-spin ${className ?? ""}`} viewBox="0 0 20 20" fill="none" width="18" height="18">
      <circle cx="10" cy="10" r="7" stroke="currentColor" strokeWidth="2" opacity="0.25" />
      <path d="M10 3a7 7 0 017 7" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

export default function BLEControl() {
  const [bleState, setBleState] = useState<BLEState>("idle");
  const [scanResults, setScanResults] = useState<Map<string, BLEScanResult>>(
    new Map(),
  );
  const [scanning, setScanning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deviceName, setDeviceName] = useState<string | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [frameLocked, setFrameLocked] = useState(false);

  const mainSSE = useRef<SSEClient | null>(null);
  const scanSSE = useRef<SSEClient | null>(null);

  useEffect(() => {
    void getConfig().then((cfg) => setDeviceName(cfg.ble.device_name));
  }, []);

  useEffect(() => {
    const client = new SSEClient({
      endpoint: "/api/events",
      onStatusChange: (status) => {
        if (status === "reconnecting" || status === "disconnected") {
          setBleState("disconnected");
        }
      },
    });
    client.on<{ status: string }>("ble-status", (data) => {
      setBleState(data.status as BLEState);
      if (data.status === "idle" || data.status === "disconnected") {
        setDisconnecting(false);
        setFrameLocked(false);
      }
      if (data.status === "connected") {
        void getFrameState().then((s) => setFrameLocked(s.locked));
      }
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
        setScanning(false);
        scanSSE.current = null;
      },
    });
    client.on<BLEScanResult>("device", (data) => {
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
  }, []);

  const handleConnect = useCallback(
    async (address: string) => {
      stopScan();
      setError(null);
      try {
        await connectDevice(address);
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : String(err));
      }
    },
    [stopScan],
  );

  const handleToggleFrameLock = useCallback(async () => {
    try {
      if (frameLocked) {
        await unlockFrame();
        setFrameLocked(false);
      } else {
        await lockFrame();
        setFrameLocked(true);
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, [frameLocked]);

  const handleDisconnect = useCallback(async () => {
    setError(null);
    setDisconnecting(true);
    try {
      await disconnectDevice();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
      setDisconnecting(false);
    }
  }, []);

  const sortedResults = [...scanResults.values()].sort(
    (a, b) => b.rssi - a.rssi,
  );

  const isIdle = (bleState === "idle" || bleState === "disconnected") && !scanning;
  const isConnecting = bleState === "connecting";
  const isConnected = bleState === "connected";

  const stateLabel = scanning ? "scanning" : bleState;

  return (
    <Card padding="lg">
      <CardHeader className={isConnected ? "border-b-0 pb-0 mb-0" : ""}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <StatusDot state={bleState} scanning={scanning} />
            <span className="text-xs text-slate-400">{stateLabel}</span>
          </div>
          <div className="flex items-center gap-2">
            {isConnected && deviceName && (
              <span className="text-xs text-slate-300 truncate max-w-[140px]">
                {deviceName}
              </span>
            )}
            {isIdle && (
              <button
                onClick={startScan}
                className="text-blue-400 hover:text-blue-300 transition-colors"
                title="Scan for devices"
              >
                <ScanIcon />
              </button>
            )}
            {scanning && (
              <button
                onClick={stopScan}
                className="text-yellow-400 hover:text-yellow-300 transition-colors"
                title="Stop scan"
              >
                <StopIcon />
              </button>
            )}
            {isConnecting && (
              <span className="text-slate-400" title="Connecting...">
                <Spinner />
              </span>
            )}
            {isConnected && (
              <button
                onClick={() => void handleToggleFrameLock()}
                className={`text-xs px-2 py-0.5 rounded transition-colors ${
                  frameLocked
                    ? "bg-green-700 text-green-200 hover:bg-green-600"
                    : "bg-slate-700 text-slate-300 hover:bg-slate-600"
                }`}
                title={frameLocked ? "Unlock reference frame" : "Lock reference frame"}
              >
                {frameLocked ? "Locked" : "Lock"}
              </button>
            )}
            {isConnected && (
              <button
                onClick={() => void handleDisconnect()}
                className={`text-red-400 hover:text-red-300 transition-colors ${disconnecting ? "opacity-50 pointer-events-none" : ""}`}
                title="Disconnect"
                disabled={disconnecting}
              >
                <DisconnectIcon />
              </button>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {error && (
          <div className="rounded bg-red-900/50 px-3 py-2 text-xs text-red-300 mb-3">
            {error}
          </div>
        )}

        {sortedResults.length > 0 && !isConnected && (
          <div className="max-h-[280px] overflow-y-auto space-y-1">
            {sortedResults.slice(0, 10).map((device) => (
              <button
                key={device.address}
                className="w-full flex items-center justify-between rounded px-3 py-2 text-left text-sm hover:bg-slate-700/50 transition-colors disabled:opacity-50 disabled:pointer-events-none"
                disabled={isConnecting}
                onClick={() => void handleConnect(device.address)}
              >
                <div className="min-w-0">
                  <div className="text-slate-200 truncate">
                    {device.name || "Unknown"}
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
      </CardContent>
    </Card>
  );
}
