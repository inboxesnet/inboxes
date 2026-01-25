"use client";

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  ReactNode,
} from "react";
import { useWebSocket } from "@/hooks/use-websocket";

interface NotificationContextValue {
  unreadCount: number;
  soundEnabled: boolean;
  setSoundEnabled: (enabled: boolean) => void;
  refreshUnreadCount: () => Promise<void>;
}

const NotificationContext = createContext<NotificationContextValue | null>(
  null
);

// Play a simple notification beep using Web Audio API
function playNotificationSound() {
  try {
    const AudioContextClass =
      window.AudioContext ||
      (window as unknown as { webkitAudioContext: typeof AudioContext })
        .webkitAudioContext;
    if (!AudioContextClass) return;

    const audioContext = new AudioContextClass();

    // Create oscillator for the beep
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    // Set up a pleasant notification tone
    oscillator.frequency.value = 880; // A5 note
    oscillator.type = "sine";

    // Fade in and out for smoother sound
    gainNode.gain.setValueAtTime(0, audioContext.currentTime);
    gainNode.gain.linearRampToValueAtTime(0.3, audioContext.currentTime + 0.05);
    gainNode.gain.linearRampToValueAtTime(0, audioContext.currentTime + 0.2);

    oscillator.start(audioContext.currentTime);
    oscillator.stop(audioContext.currentTime + 0.2);
  } catch {
    // Ignore audio errors
  }
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [unreadCount, setUnreadCount] = useState(0);
  const [soundEnabled, setSoundEnabledState] = useState(false);
  const audioContextUnlockedRef = useRef(false);

  // Load sound preference from localStorage on mount
  useEffect(() => {
    const stored = localStorage.getItem("notification-sound-enabled");
    if (stored === "true") {
      setSoundEnabledState(true);
    }
  }, []);

  // Unlock audio context on first user interaction
  useEffect(() => {
    const unlockAudio = () => {
      if (!audioContextUnlockedRef.current) {
        audioContextUnlockedRef.current = true;
      }
    };

    document.addEventListener("click", unlockAudio, { once: true });
    document.addEventListener("keydown", unlockAudio, { once: true });

    return () => {
      document.removeEventListener("click", unlockAudio);
      document.removeEventListener("keydown", unlockAudio);
    };
  }, []);

  const setSoundEnabled = useCallback((enabled: boolean) => {
    setSoundEnabledState(enabled);
    localStorage.setItem("notification-sound-enabled", String(enabled));
  }, []);

  const refreshUnreadCount = useCallback(async () => {
    try {
      const res = await fetch("/api/threads/unread-count");
      if (res.ok) {
        const data = await res.json();
        setUnreadCount(data.count);
      }
    } catch {
      // Ignore errors silently
    }
  }, []);

  // Initial fetch of unread count
  useEffect(() => {
    refreshUnreadCount();
  }, [refreshUnreadCount]);

  // Subscribe to WebSocket events
  const { subscribe } = useWebSocket();

  useEffect(() => {
    const unsubscribe = subscribe((event) => {
      if (event.event === "new_email") {
        // Increment unread count when new email arrives
        setUnreadCount((prev) => prev + 1);

        // Play notification sound if enabled and tab is not focused
        if (soundEnabled && !document.hasFocus()) {
          playNotificationSound();
        }
      }
    });
    return unsubscribe;
  }, [subscribe, soundEnabled]);

  return (
    <NotificationContext.Provider
      value={{ unreadCount, soundEnabled, setSoundEnabled, refreshUnreadCount }}
    >
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotificationContext() {
  const context = useContext(NotificationContext);
  if (!context) {
    throw new Error(
      "useNotificationContext must be used within NotificationProvider"
    );
  }
  return context;
}
