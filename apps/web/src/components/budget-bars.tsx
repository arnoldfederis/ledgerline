import type { CategoryLine } from "@/lib/api";

import { formatBasisPoints, formatCents, formatCompactCents } from "@/lib/format";
import { cn } from "@/lib/utils";

interface BudgetBarsProps {
  categories: CategoryLine[];
  selected: string | null;
  onSelect: (code: string | null) => void;
}

// A bullet chart: one shared scale, a bar for actual, a tick for the budget it
// is measured against. Grouped bars need twice the height to say the same thing.
export function BudgetBars({ categories, selected, onSelect }: BudgetBarsProps) {
  if (categories.length === 0) {
    return <p className="text-muted-foreground px-5 py-10 text-sm">Nothing to chart this month.</p>;
  }

  const rows = [...categories].sort((a, b) => b.budget_cents - a.budget_cents);
  const scale = Math.max(...rows.map((row) => Math.max(row.budget_cents, row.actual_cents)), 1);

  return (
    <div className="flex flex-col">
      <div className="text-muted-foreground flex items-center justify-between px-5 pb-3 text-[0.6875rem]">
        <span className="tnum">$0</span>
        <span className="tnum">{formatCompactCents(scale)}</span>
      </div>

      <ul className="flex flex-col border-t">
        {rows.map((row) => {
          const isSelected = selected === row.category_code;
          const over = row.budgeted && row.actual_cents > row.budget_cents;
          const actualWidth = (Math.max(row.actual_cents, 0) / scale) * 100;
          const budgetMark = (row.budget_cents / scale) * 100;

          return (
            <li key={row.category_code}>
              <button
                type="button"
                aria-pressed={isSelected}
                onClick={() => onSelect(isSelected ? null : row.category_code)}
                className={cn(
                  "group flex w-full flex-col gap-2 px-5 py-3 text-left transition-colors",
                  "hover:bg-muted/50 focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-2",
                  isSelected && "bg-muted/70",
                )}
              >
                <div className="flex items-baseline justify-between gap-4">
                  <span className="truncate text-[0.8125rem] font-medium">{row.category_name}</span>
                  <span className="flex items-baseline gap-3">
                    <span className="tnum text-muted-foreground text-xs">
                      {formatCents(row.actual_cents)}
                    </span>
                    <span
                      className={cn(
                        "tnum w-12 text-right text-xs",
                        over && "text-loss font-medium",
                      )}
                    >
                      {row.budgeted ? formatBasisPoints(row.utilization_bps) : "--"}
                    </span>
                  </span>
                </div>

                <div className="bg-muted/60 relative h-2.5 w-full">
                  {/* band: budget, bar: actual */}
                  <div
                    className="bg-foreground/15 absolute inset-y-0 left-0"
                    style={{ width: `${budgetMark}%` }}
                  />
                  <div
                    className={cn(
                      "absolute inset-x-0 top-[5px] h-[5px] transition-[width] duration-500",
                      over ? "bg-loss" : "bg-foreground/80 group-hover:bg-foreground",
                    )}
                    style={{ width: `${actualWidth}%` }}
                  />
                  {row.budgeted ? (
                    <div
                      className="bg-foreground/60 absolute -top-0.5 -bottom-0.5 w-px"
                      style={{ left: `${budgetMark}%` }}
                      title={`Budget ${formatCents(row.budget_cents)}`}
                    />
                  ) : null}
                </div>
              </button>
            </li>
          );
        })}
      </ul>

      <p className="text-muted-foreground border-t px-5 py-3 text-[0.6875rem]">
        Narrow bar is actual spend, measured against the wide band and tick of the approved budget.
      </p>
    </div>
  );
}
