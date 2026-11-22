import type { QueryClient, QueryFilters, QueryKey } from "@tanstack/react-query";

// setQueryData marks a query as fresh: it sets dataUpdatedAt to now and
// clears isInvalidated. A local patch from a WebSocket event or an
// optimistic update is not a server read. These helpers write the patch and
// keep the age of the data, so the staleTime refetch still occurs when
// events were missed.

/**
 * Patch the cached data of one query. Does nothing when the query has no
 * data, or when the updater returns undefined or the same object.
 */
export function patchQueryData<T>(
  qc: QueryClient,
  queryKey: QueryKey,
  updater: (old: T) => T | undefined
): void {
  const query = qc.getQueryCache().findAll({ queryKey, exact: true })[0];
  if (!query || query.state.data === undefined) return;

  const old = query.state.data as T;
  const next = updater(old);
  if (next === undefined || next === old) return;

  const wasInvalidated = query.state.isInvalidated;
  qc.setQueryData<T>(queryKey, () => next, {
    updatedAt: query.state.dataUpdatedAt,
  });
  if (wasInvalidated) query.invalidate();
}

/**
 * Patch the cached data of each query that matches the filters. The
 * updater receives the query key, so it can read the view (domain, label,
 * page) of a thread list.
 */
export function patchQueriesData<T>(
  qc: QueryClient,
  filters: QueryFilters,
  updater: (old: T, queryKey: QueryKey) => T | undefined
): void {
  for (const query of qc.getQueryCache().findAll(filters)) {
    patchQueryData<T>(qc, query.queryKey, (old) => updater(old, query.queryKey));
  }
}
