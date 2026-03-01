"use client";

import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useCallback,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError } from "@/lib/api";
import { queryClient } from "@/lib/query-client";
import type { WSMessage } from "@/lib/types";

// In local dev, NEXT_PUBLIC_WS_URL points to backend (ws://localhost:8080).
// In production behind Caddy, derive from current page origin.
function getWsUrl(): string {
  if (process.env.NEXT_PUBLIC_WS_URL) return process.env.NEXT_PUBLIC_WS_URL;
  if (typeof window !== "undefined") {
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    return `${proto}//${window.location.host}`;
  }
  return ""; // SSR — will be set on client-side hydration
}

type EventHandler = (msg: WSMessage) => void;

interface NotificationContextType {
  connected: boolean;
  subscribe: (event: string, handler: EventHandler) => () => void;
  setLastEventId: (id: number) => void;
}

const NotificationContext = createContext<NotificationContextType | null>(null);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const wsRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef<Map<string, Set<EventHandler>>>(new Map());
  const [connected, setConnected] = useState(false);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastEventIdRef = useRef<number>(0);
  const hadConnectionRef = useRef(false);
  const reconnectAttemptRef = useRef(0);

  const dispatch = useCallback((msg: WSMessage) => {
    // Dispatch to event-specific handlers
    const handlers = handlersRef.current.get(msg.event);
    if (handlers) {
      handlers.forEach((handler) => handler(msg));
    }
    // Dispatch to wildcard handlers
    const wildcardHandlers = handlersRef.current.get("*");
    if (wildcardHandlers) {
      wildcardHandlers.forEach((handler) => handler(msg));
    }
  }, []);

  // Fetch missed events after reconnection
  const catchupEvents = useCallback(async () => {
    if (lastEventIdRef.current === 0) return;
    try {
      const data = await api.get<{ events: WSMessage[] }>(
        `/api/events?since=${lastEventIdRef.current}&limit=200`
      );
      for (const evt of data.events) {
        dispatch(evt);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 410) {
        // Events too old — invalidate all caches for a full refetch
        queryClient.invalidateQueries();
        lastEventIdRef.current = 0;
        return;
      }
      // Other catchup failures — handlers will refetch on next interaction
    }
  }, [dispatch]);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const url = getWsUrl();
    if (!url) return; // No URL available yet (SSR)

    const isReconnect = hadConnectionRef.current;
    const ws = new WebSocket(`${url}/api/ws`);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
      hadConnectionRef.current = true;
      reconnectAttemptRef.current = 0;
      // Catch up on missed events if this is a reconnection
      if (isReconnect) {
        catchupEvents();
      }
    };

    ws.onmessage = (wsEvent) => {
      try {
        const msg: WSMessage = JSON.parse(wsEvent.data);
        dispatch(msg);
      } catch {
        // ignore malformed messages
      }
    };

    ws.onclose = () => {
      setConnected(false);
      const attempt = reconnectAttemptRef.current;
      reconnectAttemptRef.current = attempt + 1;
      // Full jitter: random value between 1s and the exponential backoff cap.
      // Spreads reconnections over a wide window to prevent thundering herd.
      const baseDelay = Math.min(1000 * Math.pow(2, attempt), 30000);
      const delay = Math.max(1000, Math.random() * baseDelay);
      reconnectTimeoutRef.current = setTimeout(connect, delay);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [dispatch, catchupEvents]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  const subscribe = useCallback((event: string, handler: EventHandler) => {
    if (!handlersRef.current.has(event)) {
      handlersRef.current.set(event, new Set());
    }
    handlersRef.current.get(event)!.add(handler);

    return () => {
      handlersRef.current.get(event)?.delete(handler);
    };
  }, []);

  const setLastEventId = useCallback((id: number) => {
    if (id > lastEventIdRef.current) {
      lastEventIdRef.current = id;
    }
  }, []);

  return (
    <NotificationContext.Provider value={{ connected, subscribe, setLastEventId }}>
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotifications() {
  const ctx = useContext(NotificationContext);
  if (!ctx)
    throw new Error(
      "useNotifications must be used within NotificationProvider"
    );
  return ctx;
}
