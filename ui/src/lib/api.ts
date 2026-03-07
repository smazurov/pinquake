import createClient from "openapi-fetch";
import type { paths } from "./api.generated";

export const API_BASE_URL = "";

export const api = createClient<paths>({ baseUrl: API_BASE_URL });

// SSE type helpers — extract event→data map from generated path types
type GetContent<P extends keyof paths> =
  paths[P] extends { get: { responses: { 200: { content: infer C } } } } ? C : never;

type SSEPath = {
  [P in keyof paths]: GetContent<P> extends { "text/event-stream": unknown }
    ? P
    : never;
}[keyof paths];

type SSEStream<P extends SSEPath> = GetContent<P> extends {
  "text/event-stream": infer S;
}
  ? S
  : never;

type SSEEvent<P extends SSEPath> = SSEStream<P> extends (infer E)[] ? E : never;

type SSEEventMap<P extends SSEPath> = {
  [E in SSEEvent<P> as E extends { event: infer N extends string }
    ? N
    : never]: E extends { data: infer D } ? D : never;
};

export type SSEStatus =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting";

const INITIAL_RECONNECT_DELAY = 2000;
const MAX_RECONNECT_DELAY = 30000;
const STALENESS_CHECK_INTERVAL = 5000;
const STALENESS_THRESHOLD = 20000;

export interface SSEClientConfig<P extends SSEPath> {
  endpoint: P;
  onStatusChange?: (status: SSEStatus) => void;
  onConnect?: () => void;
  onError?: (willReconnect: boolean) => void;
}

type TypedEventHandler<T> = (data: T) => void;

export class SSEClient<P extends SSEPath> {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: number | null = null;
  private stalenessInterval: number | null = null;
  private lastEventTime = 0;
  private reconnectDelay = INITIAL_RECONNECT_DELAY;
  private readonly typedHandlers: Map<string, TypedEventHandler<unknown>> =
    new Map();
  private status: SSEStatus = "disconnected";

  constructor(private readonly config: SSEClientConfig<P>) {}

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
      this.teardownAndReconnect(
        `SSE disconnected from ${this.config.endpoint}, reconnecting in ${this.reconnectDelay}ms`,
      );
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

  on<K extends keyof SSEEventMap<P> & string>(
    eventType: K,
    handler: TypedEventHandler<SSEEventMap<P>[K]>,
  ): void {
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

  off(eventType: keyof SSEEventMap<P> & string): void {
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
        this.teardownAndReconnect(
          `SSE stale (no events for ${STALENESS_THRESHOLD / 1000}s), reconnecting`,
        );
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

  private teardownAndReconnect(reason: string): void {
    this.stopStalenessWatchdog();
    this.eventSource?.close();
    this.eventSource = null;
    this.setStatus("reconnecting");
    this.config.onError?.(true);
    console.warn(reason);
    this.scheduleReconnect();
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
