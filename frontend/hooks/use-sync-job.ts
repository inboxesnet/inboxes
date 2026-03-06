import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { useNotifications } from "@/contexts/notification-context";
import { api } from "@/lib/api";
import type { SyncJob, WSMessage } from "@/lib/types";

/** Progress polls at 5s (was 2s). WS delivers completion instantly. */
const PROGRESS_INTERVAL = 5000;

export function useSyncJob() {
  const { subscribe } = useNotifications();
  const [job, setJob] = useState<SyncJob | null>(null);
  const [error, setError] = useState<string>("");
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const activeJobId = useRef<string | null>(null);

  const stopTracking = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    activeJobId.current = null;
  }, []);

  // WS: instant completion — no need to wait for next poll cycle
  useEffect(() => {
    const unsub = subscribe("sync.completed", (msg: WSMessage) => {
      if (!activeJobId.current) return;
      stopTracking();
      setJob((prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          status: "completed" as const,
          phase: "done",
          sent_count:
            (msg.payload?.sent_count as number) ?? prev.sent_count,
          received_count:
            (msg.payload?.received_count as number) ?? prev.received_count,
          thread_count:
            (msg.payload?.thread_count as number) ?? prev.thread_count,
        };
      });
    });
    return unsub;
  }, [subscribe, stopTracking]);

  const pollProgress = useCallback(
    (jobId: string) => {
      timerRef.current = setTimeout(async () => {
        try {
          const data = await api.get<SyncJob>(`/api/sync/${jobId}`);
          setJob(data);
          if (data.status === "completed" || data.status === "failed") {
            stopTracking();
            if (data.status === "failed" && data.error_message) {
              setError(data.error_message);
            }
            return;
          }
        } catch {
          // Silently continue — WS may still deliver completion
        }
        if (activeJobId.current) pollProgress(jobId);
      }, PROGRESS_INTERVAL);
    },
    [stopTracking]
  );

  const startTracking = useCallback(
    (jobId: string) => {
      activeJobId.current = jobId;
      pollProgress(jobId);
    },
    [pollProgress]
  );

  const startSync = useCallback(async () => {
    setError("");
    setJob(null);
    try {
      const data = await api.post<SyncJob>("/api/sync");
      setJob(data);
      if (data.status !== "completed" && data.status !== "failed") {
        startTracking(data.id);
      }
      return data;
    } catch {
      setError("Failed to start sync");
      return null;
    }
  }, [startTracking]);

  const resumeJob = useCallback(
    (jobId: string) => {
      setError("");
      api
        .get<SyncJob>(`/api/sync/${jobId}`)
        .then((data) => {
          setJob(data);
          if (data.status !== "completed" && data.status !== "failed") {
            startTracking(jobId);
          }
        })
        .catch(() => setError("Failed to resume sync"));
    },
    [startTracking]
  );

  // Cleanup on unmount
  useEffect(() => {
    return () => stopTracking();
  }, [stopTracking]);

  const aliasesReady = !!(
    job &&
    (job.phase === "aliases_ready" ||
      job.phase === "importing" ||
      job.phase === "addresses" ||
      job.phase === "done")
  );

  const progress = useMemo(
    () =>
      job
        ? {
            phase: job.phase,
            imported: job.imported,
            total: job.total,
            message:
              job.phase === "scanning" || job.phase === "aliases"
                ? "Scanning emails..."
                : job.phase === "aliases_ready"
                  ? "Addresses discovered - starting import..."
                  : job.phase === "importing" && job.total > 0
                    ? `Importing ${job.imported} of ${job.total} emails`
                    : job.phase === "addresses"
                      ? "Discovering addresses..."
                      : job.phase === "done"
                        ? `Imported ${job.sent_count} sent + ${job.received_count} received into ${job.thread_count} threads`
                        : "Preparing...",
          }
        : null,
    [job?.phase, job?.imported, job?.total, job?.sent_count, job?.received_count, job?.thread_count]
  );

  const result = useMemo(
    () =>
      job && (job.status === "completed" || job.phase === "done")
        ? {
            sent_count: job.sent_count,
            received_count: job.received_count,
            thread_count: job.thread_count,
            address_count: job.address_count,
          }
        : null,
    [job?.status, job?.phase, job?.sent_count, job?.received_count, job?.thread_count, job?.address_count]
  );

  return {
    job,
    error,
    isRunning:
      job?.status === "pending" || job?.status === "running",
    isComplete: job?.status === "completed",
    isFailed: job?.status === "failed",
    aliasesReady,
    progress,
    result,
    startSync,
    resumeJob,
  };
}
