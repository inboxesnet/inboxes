"use client";

import { useEffect, useRef, useCallback, useState } from "react";

export type WebSocketEvent = {
  event: string;
  payload: unknown;
};

type MessageHandler = (event: WebSocketEvent) => void;

interface UseWebSocketOptions {
  onMessage?: MessageHandler;
}

interface UseWebSocketReturn {
  isConnected: boolean;
  subscribe: (handler: MessageHandler) => () => void;
}

export function useWebSocket(options?: UseWebSocketOptions): UseWebSocketReturn {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const handlersRef = useRef<Set<MessageHandler>>(new Set());
  const [isConnected, setIsConnected] = useState(false);
  const reconnectAttemptsRef = useRef(0);
  const maxReconnectDelay = 30000;
  const mountedRef = useRef(true);

  // Register the options.onMessage handler
  useEffect(() => {
    if (options?.onMessage) {
      handlersRef.current.add(options.onMessage);
      return () => {
        if (options?.onMessage) {
          handlersRef.current.delete(options.onMessage);
        }
      };
    }
  }, [options?.onMessage]);

  const connect = useCallback(() => {
    if (!mountedRef.current) return;

    const wsUrl = process.env.NEXT_PUBLIC_WS_URL || "ws://localhost:3001";
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      if (!mountedRef.current) {
        ws.close();
        return;
      }
      setIsConnected(true);
      reconnectAttemptsRef.current = 0;
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as WebSocketEvent;
        handlersRef.current.forEach((handler) => {
          handler(data);
        });
      } catch {
        // Ignore non-JSON messages (e.g., pong)
      }
    };

    ws.onclose = () => {
      setIsConnected(false);
      wsRef.current = null;
      if (!mountedRef.current) return;

      // Exponential backoff reconnect
      const delay = Math.min(
        1000 * Math.pow(2, reconnectAttemptsRef.current),
        maxReconnectDelay
      );
      reconnectAttemptsRef.current++;
      reconnectTimerRef.current = setTimeout(connect, delay);
    };

    ws.onerror = () => {
      ws.close();
    };

    wsRef.current = ws;
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    connect();

    return () => {
      mountedRef.current = false;
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [connect]);

  const subscribe = useCallback((handler: MessageHandler) => {
    handlersRef.current.add(handler);
    return () => {
      handlersRef.current.delete(handler);
    };
  }, []);

  return { isConnected, subscribe };
}
