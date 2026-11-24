import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "@/lib/api";

// Keys in one place: what makes invalidating the whole namespace after a sync
// reliable, with no second spelling to forget.
export const keys = {
  all: ["ledgerline"] as const,
  report: (month: string | undefined) => ["ledgerline", "report", month ?? "latest"] as const,
  months: () => ["ledgerline", "months"] as const,
  transactions: (month: string | undefined, category: string | undefined) =>
    ["ledgerline", "transactions", month ?? "latest", category ?? "all"] as const,
  syncRuns: () => ["ledgerline", "sync-runs"] as const,
};

export function useBudgetReport(month: string | undefined) {
  return useQuery({
    queryKey: keys.report(month),
    queryFn: () => api.budgetVsActual(month),
    // Hold the previous month on screen while the next one loads. Without it
    // the report unmounts between months, which both flashes the skeleton and
    // robs the animated figures of a value to count from.
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

export function useMonths() {
  return useQuery({
    queryKey: keys.months(),
    queryFn: () => api.months(),
    staleTime: 60_000,
  });
}

export function useTransactions(month: string | undefined, category: string | undefined) {
  return useQuery({
    queryKey: keys.transactions(month, category),
    queryFn: () => api.transactions({ month, category, limit: 100 }),
    enabled: Boolean(category),
    placeholderData: keepPreviousData,
    staleTime: 15_000,
  });
}

export function useSyncRuns() {
  return useQuery({
    queryKey: keys.syncRuns(),
    queryFn: () => api.syncRuns(8),
    // Catches runs started by the scheduler, not just by the button.
    refetchInterval: 30_000,
    staleTime: 10_000,
  });
}

export function useTriggerSync() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: () => api.triggerSync(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.all });
    },
  });
}
