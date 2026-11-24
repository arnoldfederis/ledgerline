import { ChevronRight } from "lucide-react";

import type { BudgetStatus, CategoryLine, Transaction } from "@/lib/api";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatBasisPoints, formatCents, formatDayLabel, formatSignedCents } from "@/lib/format";
import { cn } from "@/lib/utils";

const STATUS_LABEL: Record<BudgetStatus, string> = {
  on_track: "On track",
  at_risk: "At risk",
  over_budget: "Over",
  unbudgeted: "No budget",
};

interface CategoryLedgerProps {
  categories: CategoryLine[];
  selected: string | null;
  onSelect: (code: string | null) => void;
  transactions: Transaction[];
  transactionsTotal: number;
  transactionsLoading: boolean;
}

export function CategoryLedger({
  categories,
  selected,
  onSelect,
  transactions,
  transactionsTotal,
  transactionsLoading,
}: CategoryLedgerProps) {
  if (categories.length === 0) {
    return (
      <p className="text-muted-foreground px-5 py-10 text-sm">
        Nothing budgeted or spent in this month yet.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="eyebrow h-9 ps-5">Category</TableHead>
          <TableHead className="eyebrow h-9 text-right">Budget</TableHead>
          <TableHead className="eyebrow h-9 text-right">Actual</TableHead>
          <TableHead className="eyebrow h-9 text-right">Variance</TableHead>
          <TableHead className="eyebrow h-9 w-32 text-right">Used</TableHead>
          <TableHead className="eyebrow h-9 pe-5 text-right">Status</TableHead>
        </TableRow>
      </TableHeader>

      <TableBody>
        {categories.map((line) => {
          const isOpen = selected === line.category_code;

          return [
            <TableRow
              key={line.category_code}
              data-status={line.status}
              data-state={isOpen ? "open" : undefined}
              className={cn(
                "cursor-pointer",
                isOpen && "bg-muted/60",
                // A hairline, not a wash. Colour is never the only signal:
                // the variance is signed and the status named.
                line.status === "over_budget" && "shadow-[inset_2px_0_0_var(--loss)]",
                line.status === "at_risk" && "shadow-[inset_2px_0_0_var(--caution)]",
              )}
              onClick={() => onSelect(isOpen ? null : line.category_code)}
            >
              <TableCell className="ps-5">
                <button
                  type="button"
                  aria-expanded={isOpen}
                  className="flex items-center gap-1.5 text-left font-medium focus-visible:outline-none"
                  onClick={(event) => {
                    event.stopPropagation();
                    onSelect(isOpen ? null : line.category_code);
                  }}
                >
                  <ChevronRight
                    aria-hidden
                    className={cn(
                      "text-muted-foreground size-3.5 transition-transform",
                      isOpen && "rotate-90",
                    )}
                  />
                  {line.category_name}
                </button>
              </TableCell>

              <TableCell className="tnum text-muted-foreground text-right">
                {line.budgeted ? formatCents(line.budget_cents) : "--"}
              </TableCell>

              <TableCell className="tnum text-right">{formatCents(line.actual_cents)}</TableCell>

              <TableCell
                className={cn(
                  "tnum text-right",
                  line.budgeted && (line.variance_cents < 0 ? "text-loss" : "text-gain"),
                )}
              >
                {line.budgeted ? formatSignedCents(line.variance_cents) : "--"}
              </TableCell>

              <TableCell className="text-right">
                {line.budgeted ? <UsedCell basisPoints={line.utilization_bps} /> : "--"}
              </TableCell>

              <TableCell className="pe-5 text-right">
                <span
                  className={cn(
                    "text-xs",
                    line.status === "over_budget" && "text-loss font-medium",
                    line.status === "at_risk" && "text-caution",
                    line.status === "on_track" && "text-muted-foreground",
                    line.status === "unbudgeted" && "text-muted-foreground italic",
                  )}
                >
                  {STATUS_LABEL[line.status]}
                </span>
              </TableCell>
            </TableRow>,

            isOpen ? (
              <TableRow key={`${line.category_code}-detail`} className="hover:bg-transparent">
                <TableCell colSpan={6} className="bg-muted/30 p-0">
                  <TransactionRows
                    transactions={transactions}
                    total={transactionsTotal}
                    loading={transactionsLoading}
                  />
                </TableCell>
              </TableRow>
            ) : null,
          ];
        })}
      </TableBody>
    </Table>
  );
}

function UsedCell({ basisPoints }: { basisPoints: number }) {
  const filled = Math.min(100, Math.max(0, basisPoints / 100));
  const over = basisPoints > 10_000;

  return (
    <span className="flex items-center justify-end gap-2.5">
      <span className="bg-muted relative hidden h-1 w-14 sm:block">
        <span
          className={cn("absolute inset-y-0 left-0", over ? "bg-loss" : "bg-foreground/60")}
          style={{ width: `${filled}%` }}
        />
      </span>
      <span className={cn("tnum w-11 text-right text-xs", over && "text-loss")}>
        {formatBasisPoints(basisPoints)}
      </span>
    </span>
  );
}

function TransactionRows({
  transactions,
  total,
  loading,
}: {
  transactions: Transaction[];
  total: number;
  loading: boolean;
}) {
  if (loading) {
    return <p className="text-muted-foreground px-5 py-4 text-xs">Loading transactions...</p>;
  }

  if (transactions.length === 0) {
    return <p className="text-muted-foreground px-5 py-4 text-xs">No transactions this month.</p>;
  }

  return (
    <div className="flex flex-col">
      {transactions.map((transaction) => (
        <div
          key={transaction.id}
          className="grid grid-cols-[4.5rem_10rem_1fr_7rem] items-baseline gap-4 border-b px-5 py-2 text-xs last:border-b-0"
        >
          <span className="tnum text-muted-foreground">
            {formatDayLabel(transaction.occurred_on)}
          </span>
          <span className="truncate font-medium">{transaction.vendor || "--"}</span>
          <span className="text-muted-foreground truncate">{transaction.description}</span>
          <span
            className={cn("tnum text-right", transaction.direction === "credit" && "text-gain")}
          >
            {formatCents(transaction.signed_amount_cents)}
          </span>
        </div>
      ))}
      <p className="text-muted-foreground px-5 py-2 text-[0.6875rem]">
        {total} {total === 1 ? "transaction" : "transactions"} &middot; credits shown negative
      </p>
    </div>
  );
}
