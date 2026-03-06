import { API_BASE_URL } from "./api";

export type SSEStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting";

const INITIAL_RECONNECT_DELAY = 2000;
const MAX_RECONNECT_DELAY = 30000;
const STALENESS_CHECK_INTERVAL = 5000;
const STALENESS_THRESHOLD = 20000;

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
  private stalenessInterval: number | null = null;
  private lastEventTime = 0;
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
      const wasReconnecting = this.status === "reconnecting";
      this.reconnectDelay = INITIAL_RECONNECT_DELAY;
      this.lastEventTime = Date.now();
      this.setStatus("connected");
      this.config.onConnect?.();
      this.startStalenessWatchdog();
      if (wasReconnecting) {
        console.log(`SSE reconnected to ${this.config.endpoint}`);
      } else {
        console.log(`SSE connected to ${this.config.endpoint}`);
      }
    };

    this.eventSource.addEventListener("heartbeat", () => {
      this.lastEventTime = Date.now();
    });

    for (const [eventType, handler] of this.typedHandlers) {
      this.attachTypedHandler(eventType, handler);
    }

    this.eventSource.onerror = () => {
      this.stopStalenessWatchdog();
      this.setStatus("reconnecting");
      this.eventSource?.close();
      this.eventSource = null;
      this.config.onError?.(true);
      console.warn(
        `SSE disconnected from ${this.config.endpoint}, reconnecting in ${this.reconnectDelay}ms`,
      );
      this.scheduleReconnect();
    };
  }

  disconnect(): void {
    this.stopStalenessWatchdog();
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
      this.lastEventTime = Date.now();
      try {
        const data: unknown = JSON.parse(String(event.data));
        handler(data);
      } catch (error) {
        console.error(`Error parsing ${eventType} event:`, error);
      }
    });
  }

  private startStalenessWatchdog(): void {
    this.stopStalenessWatchdog();
    this.stalenessInterval = window.setInterval(() => {
      if (
        this.status === "connected" &&
        Date.now() - this.lastEventTime > STALENESS_THRESHOLD
      ) {
        console.warn(
          `SSE stale (no events for ${STALENESS_THRESHOLD / 1000}s), reconnecting`,
        );
        this.stopStalenessWatchdog();
        this.eventSource?.close();
        this.eventSource = null;
        this.setStatus("reconnecting");
        this.config.onError?.(true);
        this.scheduleReconnect();
      }
    }, STALENESS_CHECK_INTERVAL);
  }

  private stopStalenessWatchdog(): void {
    if (this.stalenessInterval) {
      window.clearInterval(this.stalenessInterval);
      this.stalenessInterval = null;
    }
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
