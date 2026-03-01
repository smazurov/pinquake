import { API_BASE_URL } from "./api";

export type SSEStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting";

const INITIAL_RECONNECT_DELAY = 2000;
const MAX_RECONNECT_DELAY = 30000;

export interface SSEClientConfig {
  endpoint: string;
  onStatusChange?: (status: SSEStatus) => void;
  onConnect?: () => void;
  onError?: (willReconnect: boolean) => void;
}

type TypedEventHandler<T> = (data: T) => void;

export class SSEClient {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: number | null = null;
  private reconnectDelay = INITIAL_RECONNECT_DELAY;
  private readonly typedHandlers: Map<string, TypedEventHandler<unknown>> =
    new Map();
  private status: SSEStatus = "disconnected";

  constructor(private readonly config: SSEClientConfig) {}

  connect(): void {
    if (this.eventSource) return;

    this.setStatus("connecting");

    const sseUrl = `${API_BASE_URL}${this.config.endpoint}`;
    this.eventSource = new EventSource(sseUrl);

    this.eventSource.onopen = () => {
      this.reconnectDelay = INITIAL_RECONNECT_DELAY;
      this.setStatus("connected");
      this.config.onConnect?.();
    };

    for (const [eventType, handler] of this.typedHandlers) {
      this.attachTypedHandler(eventType, handler);
    }

    this.eventSource.onerror = () => {
      this.setStatus("reconnecting");
      this.eventSource?.close();
      this.eventSource = null;
      this.config.onError?.(true);
      this.scheduleReconnect();
    };
  }

  disconnect(): void {
    if (this.reconnectTimeout) {
      window.clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    this.setStatus("disconnected");
  }

  getStatus(): SSEStatus {
    return this.status;
  }

  on<T>(eventType: string, handler: TypedEventHandler<T>): void {
    this.typedHandlers.set(
      eventType,
      handler as TypedEventHandler<unknown>,
    );
    if (this.eventSource) {
      this.attachTypedHandler(
        eventType,
        handler as TypedEventHandler<unknown>,
      );
    }
  }

  off(eventType: string): void {
    this.typedHandlers.delete(eventType);
  }

  private attachTypedHandler(
    eventType: string,
    handler: TypedEventHandler<unknown>,
  ): void {
    this.eventSource?.addEventListener(eventType, (event: MessageEvent) => {
      try {
        const data: unknown = JSON.parse(String(event.data));
        handler(data);
      } catch (error) {
        console.error(`Error parsing ${eventType} event:`, error);
      }
    });
  }

  private setStatus(status: SSEStatus): void {
    this.status = status;
    this.config.onStatusChange?.(status);
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimeout) {
      window.clearTimeout(this.reconnectTimeout);
    }
    const currentDelay = this.reconnectDelay;
    this.reconnectTimeout = window.setTimeout(() => {
      this.connect();
      this.reconnectDelay = Math.min(currentDelay * 2, MAX_RECONNECT_DELAY);
    }, currentDelay);
  }
}
