export const API_BASE_URL = "";

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
    throw new Error(`API request failed: ${response.statusText}`);
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
  viz: {
    width: number;
    height: number;
  };
}

export async function getConfig(): Promise<PinQuakeConfig> {
  return apiGet<PinQuakeConfig>("/api/config");
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

export interface BLEScanResult {
  address: string;
  name: string;
  rssi: number;
  timestamp: string;
}

export interface BLEStateResponse {
  state: "idle" | "scanning" | "connecting" | "connected";
}

export async function connectDevice(
  address: string,
): Promise<{ ok: boolean }> {
  return apiPost<{ ok: boolean }>("/api/ble/connect", { address });
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

export async function getFrameState(): Promise<FrameState> {
  return apiGet<FrameState>("/api/ble/frame");
}
