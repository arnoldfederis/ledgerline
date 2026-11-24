import type { SyncRun } from "@/lib/api";

import { formatDuration, formatRelativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";

const TRIGGER: Record<SyncRun["trigger"], string> = {
  startup: "startup",
  scheduler: "scheduled",
  manual: "manual",
};

interface SyncRunListProps {
  runs: SyncRun[];
  loading: boolean;
}

// The audit trail as a job log. The counters are the point: a second run
// reporting 0 new and 0 changed is the ingest proving it is idempotent.
export function SyncRunList({ runs, loading }: SyncRunListProps) {
  if (loading) {
    return <p className="text-muted-foreground px-6 py-4 text-xs">Loading history...</p>;
  }

  if (runs.length === 0) {
    return <p className="text-muted-foreground px-6 py-4 text-xs">No sync has run yet.</p>;
  }

  return (
    <ol className="flex flex-col border-t">
      {runs.map((run) => (
        <li key={run.id} className="flex gap-3 border-b px-6 py-3 last:border-b-0">
          <span
            aria-hidden
            className={cn(
              "mt-1.5 size-1.5 shrink-0 rounded-full",
              run.status === "success" && "bg-gain",
              run.status === "failed" && "bg-loss",
              run.status === "running" && "bg-caution",
            )}
          />
          <div className="flex min-w-0 flex-col gap-1">
            <p className="text-xs">
              <span className="tnum font-medium">#{run.id}</span>{" "}
              <span className="text-muted-foreground">
                {TRIGGER[run.trigger]} &middot; {formatRelativeTime(run.started_at)}
              </span>
            </p>

            {run.status === "failed" ? (
              <p className="text-loss text-[0.6875rem] break-words">
                {run.error_message ?? "failed"}
              </p>
            ) : (
              <p className="tnum text-muted-foreground flex flex-wrap gap-x-3 gap-y-0.5 text-[0.6875rem]">
                <span>{run.records_fetched} fetched</span>
                <span className={cn(run.records_created > 0 && "text-gain")}>
                  {run.records_created} new
                </span>
                <span className={cn(run.records_updated > 0 && "text-foreground")}>
                  {run.records_updated} changed
                </span>
                <span>{run.records_skipped} same</span>
                <span>{formatDuration(run.duration_ms)}</span>
              </p>
            )}
          </div>
        </li>
      ))}
    </ol>
  );
}
