export const API_BASE_URL = "";

async function extractErrorDetail(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as { detail?: string };
    if (body.detail) return body.detail;
  } catch {
    // ignore parse errors
  }
  return `API request failed: ${response.statusText}`;
}

export async function apiGet<T>(endpoint: string): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    headers: { "Content-Type": "application/json" },
  });
  if (!response.ok) {
    throw new Error(`API request failed: ${response.statusText}`);
  }
  return response.json() as Promise<T>;
}

export async function apiPost<T>(endpoint: string, data?: unknown): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: data !== undefined ? JSON.stringify(data) : undefined,
  });
  if (!response.ok) {
    const detail = await extractErrorDetail(response);
    throw new Error(detail);
  }
  return response.json() as Promise<T>;
}

export async function apiPut<T>(endpoint: string, data: unknown): Promise<T> {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  if (!response.ok) {
    throw new Error(`API request failed: ${response.statusText}`);
  }
  return response.json() as Promise<T>;
}

export interface PinQuakeConfig {
  ble: {
    device_address: string;
    device_name: string;
  };
  waveform: {
    buffer_size: number;
    log_knee: number;
    force_yellow_g: number;
    force_red_g: number;
    amp_scale: number;
    swap_xy: boolean;
  };
  crosshair: {
    force_yellow_g: number;
    force_red_g: number;
    smoothing: number;
    segment_size: number;
    bar_thickness: number;
    swap_xy: boolean;
  };
  viz: {
    width: number;
    height: number;
  };
  auto_lock: {
    timeout: number;
    epsilon: number;
  };
  obs: {
    host: string;
    port: number;
    password: string;
    scene_name: string;
    source_name: string;
  };
}

export async function getConfig(): Promise<PinQuakeConfig> {
  return apiGet<PinQuakeConfig>("/api/config");
}

export async function getOpenAPISchema(): Promise<Record<string, unknown>> {
  return apiGet<Record<string, unknown>>("/openapi.json");
}

export async function updateConfig(
  config: PinQuakeConfig,
): Promise<PinQuakeConfig> {
  return apiPut<PinQuakeConfig>("/api/config", config);
}

export interface HealthData {
  status: string;
  message: string;
}

export async function getHealth(): Promise<HealthData> {
  return apiGet<HealthData>("/api/health");
}

export interface LogEntry {
  message: string;
  level: string;
  timestamp: string;
}

export interface BLEScanResult {
  address: string;
  name: string;
  rssi: number;
  sensor_name?: string;
  timestamp: string;
}

export interface BLEStateResponse {
  state: "idle" | "scanning" | "connecting" | "connected";
}

export async function connectDevice(
  address: string,
  name: string,
): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/connect", { address, name });
}

export async function disconnectDevice(): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/disconnect");
}

export async function getBLEState(): Promise<BLEStateResponse> {
  return apiGet<BLEStateResponse>("/api/ble/state");
}

export interface FrameState {
  locked: boolean;
}

export async function lockFrame(): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/frame/lock");
}

export async function unlockFrame(): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/frame/unlock");
}

export async function forceLockFrame(): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/frame/force-lock");
}

export async function getFrameState(): Promise<FrameState> {
  return apiGet<FrameState>("/api/ble/frame");
}

export interface OBSStateResponse {
  status: string;
}

export interface OBSSource {
  scene_name: string;
  source_name: string;
  url: string;
  scene_item_id: number;
  enabled: boolean;
}

export async function getOBSState(): Promise<OBSStateResponse> {
  return apiGet<OBSStateResponse>("/api/obs/state");
}

export async function connectOBS(
  host: string,
  port: number,
  password: string,
): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/obs/connect", { host, port, password });
}

export async function disconnectOBS(): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/obs/disconnect");
}

export async function getOBSSources(): Promise<OBSSource[]> {
  return apiGet<OBSSource[]>("/api/obs/sources");
}

export async function toggleOBSSource(
  enabled: boolean,
): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/obs/toggle", { enabled });
}
