"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";

type NotificationPermissionState = "default" | "granted" | "denied";

interface UseNotificationsReturn {
  permission: NotificationPermissionState;
  requestPermission: () => Promise<boolean>;
  showNotification: (title: string, options?: NotificationOptions & { threadId?: string }) => void;
  isSupported: boolean;
  serviceWorkerReady: boolean;
}

export function useNotifications(): UseNotificationsReturn {
  const router = useRouter();
  const [permission, setPermission] = useState<NotificationPermissionState>("default");
  const [isSupported, setIsSupported] = useState(false);
  const [serviceWorkerReady, setServiceWorkerReady] = useState(false);
  const swRegistrationRef = useRef<ServiceWorkerRegistration | null>(null);

  // Check if notifications are supported and register service worker
  useEffect(() => {
    if (typeof window === "undefined") return;

    const supported = "Notification" in window && "serviceWorker" in navigator;
    setIsSupported(supported);

    if (supported) {
      setPermission(Notification.permission as NotificationPermissionState);

      // Register service worker
      navigator.serviceWorker
        .register("/sw.js")
        .then((registration) => {
          swRegistrationRef.current = registration;
          setServiceWorkerReady(true);
        })
        .catch((err) => {
          console.error("Service worker registration failed:", err);
        });

      // Listen for messages from service worker (notification clicks)
      const handleMessage = (event: MessageEvent) => {
        if (event.data?.type === "NOTIFICATION_CLICK" && event.data?.url) {
          router.push(event.data.url);
        }
      };

      navigator.serviceWorker.addEventListener("message", handleMessage);

      return () => {
        navigator.serviceWorker.removeEventListener("message", handleMessage);
      };
    }
  }, [router]);

  const requestPermission = useCallback(async (): Promise<boolean> => {
    if (!isSupported) return false;

    try {
      const result = await Notification.requestPermission();
      setPermission(result as NotificationPermissionState);
      return result === "granted";
    } catch {
      return false;
    }
  }, [isSupported]);

  const showNotification = useCallback(
    (title: string, options?: NotificationOptions & { threadId?: string }) => {
      if (!isSupported || permission !== "granted") return;

      // Check if tab is focused - don't show notification if focused
      if (document.hasFocus()) return;

      const { threadId, ...notificationOptions } = options || {};

      // Use service worker to show notification if available
      if (swRegistrationRef.current) {
        // Note: renotify and data are valid ServiceWorkerRegistration.showNotification options
        // but not in the base NotificationOptions type
        const swOptions = {
          icon: "/favicon.ico",
          badge: "/favicon.ico",
          tag: threadId || "default", // Prevents duplicate notifications for same thread
          renotify: true,
          data: { threadId },
          ...notificationOptions,
        };
        swRegistrationRef.current.showNotification(title, swOptions as NotificationOptions);
      } else {
        // Fallback to regular Notification API
        const notification = new Notification(title, {
          icon: "/favicon.ico",
          tag: threadId || "default",
          ...notificationOptions,
        });

        notification.onclick = () => {
          window.focus();
          if (threadId) {
            router.push(`/inbox/${threadId}`);
          }
          notification.close();
        };
      }
    },
    [isSupported, permission, router]
  );

  return {
    permission,
    requestPermission,
    showNotification,
    isSupported,
    serviceWorkerReady,
  };
}
