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
import { queryKeys } from "@/lib/query-keys";
import type { WSMessage } from "@/lib/types";

// In local dev, NEXT_PUBLIC_WS_URL points to backend (ws://localhost:8080).
// In production behind a reverse proxy, derive from current page origin.
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

// The server sends this message every 30 seconds on a healthy socket.
const HEARTBEAT_EVENT = "ping";
// No message for this long means the socket is dead, although the browser
// can still show it as open (after sleep or a network change).
const HEARTBEAT_TIMEOUT_MS = 75_000;
const CATCHUP_PAGE = 200;
const CATCHUP_MAX_PAGES = 10;
const SEEN_IDS_MAX = 1000;

// The event log could not give the missed events. Fetch the live data again.
function resyncAll() {
  queryClient.invalidateQueries({ queryKey: queryKeys.threads.all });
  queryClient.invalidateQueries({ queryKey: queryKeys.domains.all });
  queryClient.invalidateQueries({ queryKey: queryKeys.drafts.all });
  queryClient.invalidateQueries({ queryKey: queryKeys.notifications.all });
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  const wsRef = useRef<WebSocket | null>(null);
  const handlersRef = useRef<Map<string, Set<EventHandler>>>(new Map());
  const [connected, setConnected] = useState(false);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastEventIdRef = useRef<number>(0);
  const hadConnectionRef = useRef(false);
  const reconnectAttemptRef = useRef(0);
  const seenIdsRef = useRef<Set<number>>(new Set());
  const lastMessageAtRef = useRef(0);
  const heartbeatSeenRef = useRef(false);
  const catchupRunningRef = useRef(false);

  const dispatch = useCallback((msg: WSMessage) => {
    if (msg.id) {
      // The same event can arrive on the socket and in a catchup page.
      // Apply it one time only: the handlers add deltas to counters.
      const seen = seenIdsRef.current;
      if (seen.has(msg.id)) return;
      seen.add(msg.id);
      if (seen.size > SEEN_IDS_MAX) {
        const oldest = seen.values().next().value;
        if (oldest !== undefined) seen.delete(oldest);
      }
      if (msg.id > lastEventIdRef.current) lastEventIdRef.current = msg.id;
    }
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

  // Fetch missed events after reconnection. When the event log cannot
  // cover the gap, fetch the live data again.
  const catchupEvents = useCallback(async () => {
    if (catchupRunningRef.current) return;
    if (lastEventIdRef.current === 0) {
      // This tab received no event before the gap, so it has no position
      // in the event log.
      resyncAll();
      return;
    }
    catchupRunningRef.current = true;
    try {
      for (let page = 0; page < CATCHUP_MAX_PAGES; page++) {
        const data = await api.get<{ events: WSMessage[] }>(
          `/api/events?since=${lastEventIdRef.current}&limit=${CATCHUP_PAGE}`
        );
        const before = lastEventIdRef.current;
        for (const evt of data.events) {
          dispatch(evt);
        }
        if (data.events.length < CATCHUP_PAGE) return;
        // A full page that does not move the position would loop.
        if (lastEventIdRef.current === before) break;
      }
      // Too many missed events to replay.
      resyncAll();
    } catch (err) {
      if (err instanceof ApiError && err.status === 410) {
        // Events too old — invalidate all caches for a full refetch
        queryClient.invalidateQueries();
        lastEventIdRef.current = 0;
        return;
      }
      // A session error stops all requests; other errors get a refetch.
      if (!(err instanceof ApiError && err.status === 401)) resyncAll();
    } finally {
      catchupRunningRef.current = false;
    }
  }, [dispatch]);

  const connect = useCallback(() => {
    const current = wsRef.current;
    if (
      current &&
      (current.readyState === WebSocket.OPEN ||
        current.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    const url = getWsUrl();
    if (!url) return; // No URL available yet (SSR)

    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    const isReconnect = hadConnectionRef.current;
    const ws = new WebSocket(`${url}/api/ws`);
    wsRef.current = ws;
    heartbeatSeenRef.current = false;

    ws.onopen = () => {
      if (wsRef.current !== ws) return;
      setConnected(true);
      hadConnectionRef.current = true;
      reconnectAttemptRef.current = 0;
      lastMessageAtRef.current = Date.now();
      // Catch up on missed events if this is a reconnection
      if (isReconnect) {
        catchupEvents();
      }
    };

    ws.onmessage = (wsEvent) => {
      if (wsRef.current !== ws) return;
      lastMessageAtRef.current = Date.now();
      try {
        const msg: WSMessage = JSON.parse(wsEvent.data);
        if (msg.event === HEARTBEAT_EVENT) {
          heartbeatSeenRef.current = true;
          return;
        }
        dispatch(msg);
      } catch {
        // ignore malformed messages
      }
    };

    ws.onclose = () => {
      // A socket that was replaced or released must not change the state
      // of the current one, and must not schedule a second connection.
      if (wsRef.current !== ws) return;
      wsRef.current = null;
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

  // Drop the current socket and connect again now. The new connection
  // runs the catchup.
  const forceReconnect = useCallback(() => {
    const old = wsRef.current;
    wsRef.current = null;
    if (old) {
      try {
        old.close();
      } catch {
        // already closed
      }
    }
    setConnected(false);
    reconnectAttemptRef.current = 0;
    connect();
  }, [connect]);

  // Find a dead socket. The browser does not report one by itself.
  const checkHeartbeat = useCallback(() => {
    const ws = wsRef.current;
    if (
      ws &&
      ws.readyState === WebSocket.OPEN &&
      heartbeatSeenRef.current &&
      Date.now() - lastMessageAtRef.current > HEARTBEAT_TIMEOUT_MS
    ) {
      forceReconnect();
    }
  }, [forceReconnect]);

  // The user came back to the tab: do not wait for the backoff timer.
  const checkOnReturn = useCallback(() => {
    const ws = wsRef.current;
    if (!ws || ws.readyState === WebSocket.CLOSED || ws.readyState === WebSocket.CLOSING) {
      forceReconnect();
      return;
    }
    checkHeartbeat();
  }, [forceReconnect, checkHeartbeat]);

  useEffect(() => {
    connect();

    const watchdog = setInterval(checkHeartbeat, 15_000);
    const onVisible = () => {
      if (document.visibilityState === "visible") checkOnReturn();
    };
    // The network came back: the old socket is not reliable.
    const onOnline = () => forceReconnect();
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("online", onOnline);

    return () => {
      clearInterval(watchdog);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("online", onOnline);
      if (reconnectTimeoutRef.current) clearTimeout(reconnectTimeoutRef.current);
      // Release the socket first, so its onclose does not reconnect.
      const ws = wsRef.current;
      wsRef.current = null;
      ws?.close();
    };
  }, [connect, checkHeartbeat, checkOnReturn, forceReconnect]);

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
