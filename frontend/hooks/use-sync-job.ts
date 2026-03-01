import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import { api } from "@/lib/api";
import type { SyncJob } from "@/lib/types";

const POLL_INTERVAL = 2000;

export function useSyncJob() {
  const [job, setJob] = useState<SyncJob | null>(null);
  const [error, setError] = useState<string>("");
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stopPolling = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const pollJob = useCallback(
    (jobId: string) => {
      stopPolling();
      timerRef.current = setInterval(async () => {
        try {
          const data = await api.get<SyncJob>(`/api/sync/${jobId}`);
          setJob(data);
          if (data.status === "completed" || data.status === "failed") {
            stopPolling();
            if (data.status === "failed" && data.error_message) {
              setError(data.error_message);
            }
          }
        } catch {
          setError("Failed to check sync status");
          stopPolling();
        }
      }, POLL_INTERVAL);
    },
    [stopPolling]
  );

  const startSync = useCallback(async () => {
    setError("");
    setJob(null);
    try {
      const data = await api.post<SyncJob>("/api/sync");
      setJob(data);
      pollJob(data.id);
      return data;
    } catch {
      setError("Failed to start sync");
      return null;
    }
  }, [pollJob]);

  const resumeJob = useCallback(
    (jobId: string) => {
      setError("");
      // Fetch once immediately, then start polling
      api
        .get<SyncJob>(`/api/sync/${jobId}`)
        .then((data) => {
          setJob(data);
          if (data.status !== "completed" && data.status !== "failed") {
            pollJob(jobId);
          }
        })
        .catch(() => setError("Failed to resume sync"));
    },
    [pollJob]
  );

  // Cleanup on unmount
  useEffect(() => {
    return () => stopPolling();
  }, [stopPolling]);

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
                  ? "Addresses discovered — starting import..."
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
