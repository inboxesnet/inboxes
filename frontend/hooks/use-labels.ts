import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";

export interface CustomLabel {
  id: string;
  name: string;
}

// useLabels shares one cached list of the custom labels. The sidebar and the
// toolbar read it; the settings modal invalidates it after each change.
export function useLabels(enabled = true) {
  return useQuery({
    queryKey: queryKeys.labels.list(),
    queryFn: () => api.get<CustomLabel[]>("/api/labels"),
    enabled,
  });
}
