import { useState } from "react";

import { BudgetBars } from "@/components/budget-bars";
import { CategoryLedger } from "@/components/category-ledger";
import { DashboardToolbar } from "@/components/dashboard-toolbar";
import { ReportSkeleton } from "@/components/report-skeleton";
import { StatStrip } from "@/components/stat-strip";
import { ThemeToggle } from "@/components/theme-toggle";
import { ApiError } from "@/lib/api";
import { formatMonthLabel } from "@/lib/format";
import {
  useBudgetReport,
  useMonths,
  useSyncRuns,
  useTransactions,
  useTriggerSync,
} from "@/lib/queries";

function App() {
  // undefined means "the latest month with data", so a first load is never
  // blank just because the calendar moved past the ledger.
  const [month, setMonth] = useState<string | undefined>(undefined);
  const [selected, setSelected] = useState<string | null>(null);

  const months = useMonths();
  const report = useBudgetReport(month);
  const syncRuns = useSyncRuns();
  const triggerSync = useTriggerSync();

  // The months list is newest-first and is what the API itself defaults to, so
  // it settles the picker as soon as that one small request lands rather than
  // making it wait on the whole report.
  const activeMonth = month ?? months.data?.[0] ?? report.data?.month;
  // True only while a *different* month is in flight: keepPreviousData leaves
  // the old report on screen, so comparing the two is what distinguishes a
  // month change from a background refetch of the month already shown.
  const monthChanging = report.isFetching && report.data?.month !== activeMonth;
  const transactions = useTransactions(activeMonth, selected ?? undefined);

  return (
    <div className="bg-background min-h-svh">
      <header className="bg-background/80 sticky top-0 z-20 border-b backdrop-blur">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between gap-4 px-5">
          <div className="flex items-baseline gap-3">
            <span className="font-heading text-sm font-semibold tracking-tight">Ledgerline</span>
            <span className="tnum text-muted-foreground hidden text-[0.6875rem] md:inline">
              budget vs actual &middot; mock_accounting
            </span>
          </div>

          <ThemeToggle />
        </div>
      </header>

      <main className="mx-auto w-full max-w-6xl">
        <DashboardToolbar
          months={months.data ?? []}
          month={activeMonth}
          monthChanging={monthChanging}
          onMonthChange={(next) => {
            setMonth(next);
            setSelected(null);
          }}
          runs={syncRuns.data ?? []}
          runsLoading={syncRuns.isPending}
          syncing={triggerSync.isPending}
          onSync={() => triggerSync.mutate()}
        />

        <SyncNotice error={triggerSync.error} />

        {report.isError ? (
          <p className="text-loss bg-loss/10 mx-5 my-4 px-4 py-3 text-sm">
            Could not load the report:{" "}
            {report.error instanceof Error ? report.error.message : "unknown error"}
          </p>
        ) : null}

        {report.isPending ? <ReportSkeleton /> : null}

        {report.data ? (
          <>
            <StatStrip totals={report.data.totals} />

            <Section
              title="Budget vs actual"
              meta={`${formatMonthLabel(report.data.month)} · ${report.data.currency} · exact cents`}
            >
              <BudgetBars
                categories={report.data.categories}
                selected={selected}
                onSelect={setSelected}
              />
            </Section>

            <Section
              title="Ledger"
              meta="Select a category for the transactions behind it"
              className="border-t"
            >
              <CategoryLedger
                categories={report.data.categories}
                selected={selected}
                onSelect={setSelected}
                transactions={transactions.data?.transactions ?? []}
                transactionsTotal={transactions.data?.total ?? 0}
                transactionsLoading={transactions.isPending}
              />
            </Section>
          </>
        ) : null}

        <footer className="text-muted-foreground border-t px-5 py-6 text-xs">
          Go · PostgreSQL · React. Amounts are integer cents end to end; the sync job is idempotent
          and resumes from a change cursor.
        </footer>
      </main>
    </div>
  );
}

function SyncNotice({ error }: { error: unknown }) {
  if (!error) {
    return null;
  }

  const conflict = error instanceof ApiError && error.isConflict;

  return (
    <p
      className={
        conflict
          ? "bg-caution/10 text-caution px-5 py-2.5 text-xs"
          : "bg-loss/10 text-loss px-5 py-2.5 text-xs"
      }
    >
      {conflict
        ? "A sync is already running. The figures refresh when it lands."
        : `Sync failed: ${error instanceof Error ? error.message : "unknown error"}`}
    </p>
  );
}

function Section({
  title,
  meta,
  className,
  children,
}: {
  title: string;
  meta: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <section className={className}>
      <header className="flex flex-col gap-1 px-5 py-4">
        <h2 className="font-heading text-sm font-semibold tracking-tight">{title}</h2>
        <p className="text-muted-foreground text-xs">{meta}</p>
      </header>
      {children}
    </section>
  );
}

export default App;
