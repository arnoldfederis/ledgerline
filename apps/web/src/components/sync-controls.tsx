import { History, RefreshCw } from "lucide-react";

import type { SyncRun } from "@/lib/api";

import { FixedWidth } from "@/components/fixed-width";
import { SyncRunList } from "@/components/sync-run-list";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { formatRelativeTime } from "@/lib/format";
import { cn } from "@/lib/utils";

interface SyncControlsProps {
  runs: SyncRun[];
  loading: boolean;
  syncing: boolean;
  onSync: () => void;
}

export function SyncControls({ runs, loading, syncing, onSync }: SyncControlsProps) {
  const latest = runs[0];
  const failing = latest?.status === "failed";

  return (
    <div className="flex items-center gap-1.5">
      <Sheet>
        <SheetTrigger
          render={
            <Button variant="secondary" size="sm" className="relative gap-1.5 font-semibold">
              <History aria-hidden />
              <span className="hidden sm:inline">History</span>
              {/* Absolute, so a failed run tints the button without widening
                  it and shoving the row along. */}
              {failing ? (
                <span
                  aria-hidden
                  className="bg-loss absolute top-1 right-1 size-1.5 rounded-full"
                />
              ) : null}
            </Button>
          }
        />
        <SheetContent side="right" className="gap-0 p-0 data-[side=right]:sm:max-w-md">
          <SheetHeader className="gap-1 px-6 py-5">
            <SheetTitle>Sync history</SheetTitle>
            <SheetDescription className="text-xs">
              {latest
                ? `Last run ${formatRelativeTime(latest.started_at)}. A run that fetches records but creates and changes none is the ingest proving it is idempotent.`
                : "Every automatic and manual sync is recorded here."}
            </SheetDescription>
          </SheetHeader>
          <div className="overflow-y-auto">
            <SyncRunList runs={runs} loading={loading} />
          </div>
        </SheetContent>
      </Sheet>

      <Button size="sm" onClick={onSync} disabled={syncing} className="gap-1.5 font-semibold">
        <RefreshCw aria-hidden className={cn(syncing && "animate-spin")} />
        {/* "Syncing" is 11px narrower than "Sync now". Letting the button
            shrink dragged the whole control row -- month picker included --
            sideways on every sync. */}
        <FixedWidth reserve="Sync now" contentClassName="justify-center">
          {syncing ? "Syncing" : "Sync now"}
        </FixedWidth>
      </Button>
    </div>
  );
}
