import type { SyncRun } from "@/lib/api";

import { MonthSelect, MonthSelectPlaceholder } from "@/components/month-select";
import { SyncControls } from "@/components/sync-controls";
import { formatRelativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";

const TRIGGER: Record<SyncRun["trigger"], string> = {
  startup: "on startup",
  scheduler: "scheduled",
  manual: "manual",
};

interface DashboardToolbarProps {
  months: string[];
  // undefined only until the first report or month list lands.
  month: string | undefined;
  monthChanging: boolean;
  onMonthChange: (month: string) => void;
  runs: SyncRun[];
  runsLoading: boolean;
  syncing: boolean;
  onSync: () => void;
}

// Sits directly above the KPI strip, because the month and the sync state are
// what those four figures are a function of.
export function DashboardToolbar({
  months,
  month,
  monthChanging,
  onMonthChange,
  runs,
  runsLoading,
  syncing,
  onSync,
}: DashboardToolbarProps) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-3">
      <LastSync runs={runs} loading={runsLoading} syncing={syncing} />

      <div className="flex items-center gap-1.5">
        {month && months.length > 0 ? (
          <MonthSelect
            months={months}
            value={month}
            loading={monthChanging}
            onChange={onMonthChange}
          />
        ) : (
          <MonthSelectPlaceholder />
        )}
        <SyncControls runs={runs} loading={runsLoading} syncing={syncing} onSync={onSync} />
      </div>
    </div>
  );
}

function LastSync({
  runs,
  loading,
  syncing,
}: {
  runs: SyncRun[];
  loading: boolean;
  syncing: boolean;
}) {
  const latest = runs[0];

  return (
    <p className="text-muted-foreground flex items-center gap-2 text-xs">
      <span
        aria-hidden
        className={cn(
          "size-1.5 rounded-full",
          syncing && "bg-caution animate-pulse",
          !syncing && latest?.status === "success" && "bg-gain",
          !syncing && latest?.status === "failed" && "bg-loss",
          !syncing && latest?.status === "running" && "bg-caution",
          !syncing && !latest && "bg-muted-foreground/40",
        )}
      />
      {syncing ? (
        "Syncing now"
      ) : latest ? (
        <>
          Last synced {formatRelativeTime(latest.started_at)}
          <span className="hidden sm:inline">
            &middot; {TRIGGER[latest.trigger]} &middot;{" "}
            <span className="tnum">{latest.records_fetched}</span> fetched,{" "}
            <span className="tnum">{latest.records_created + latest.records_updated}</span> changed
          </span>
        </>
      ) : loading ? (
        "Loading sync history"
      ) : (
        "No sync has run yet"
      )}
    </p>
  );
}
