import NumberFlow, { type Format } from "@number-flow/react";

import type { ReportTotals } from "@/lib/api";

import { bpsToRatioString, centsToDecimalString } from "@/lib/format";
import { cn } from "@/lib/utils";

const MONEY: Format = {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
};

const SIGNED_MONEY: Format = { ...MONEY, signDisplay: "exceptZero" };

const PERCENT: Format = {
  style: "percent",
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
};

interface StatStripProps {
  totals: ReportTotals;
}

export function StatStrip({ totals }: StatStripProps) {
  const overspent = totals.variance_cents < 0;

  return (
    <div className="grid grid-cols-2 divide-x divide-y border-y lg:grid-cols-4 lg:divide-y-0">
      <Stat label="Budgeted" value={centsToDecimalString(totals.budget_cents)} format={MONEY}>
        {totals.category_count} categories
      </Stat>

      <Stat label="Actual" value={centsToDecimalString(totals.actual_cents)} format={MONEY}>
        {totals.transaction_count} transactions
      </Stat>

      <Stat
        label="Variance"
        value={centsToDecimalString(totals.variance_cents)}
        format={SIGNED_MONEY}
        tone={overspent ? "loss" : "gain"}
      >
        {overspent ? "over budget" : "remaining"}
      </Stat>

      <Stat label="Utilization" value={bpsToRatioString(totals.utilization_bps)} format={PERCENT}>
        <UtilizationRule basisPoints={totals.utilization_bps} />
      </Stat>
    </div>
  );
}

function Stat({
  label,
  value,
  format,
  tone,
  children,
}: {
  label: string;
  value: string;
  format: Format;
  tone?: "gain" | "loss";
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-2 px-5 py-6">
      <span className="eyebrow">{label}</span>
      {/* Intl.NumberFormat formats a decimal string exactly, so the animated
          figure never introduces a rounding the ledger does not have. The
          bundled TypeScript lib still types the input as number-only, which is
          the only reason for the cast. */}
      <NumberFlow
        value={value as unknown as number}
        format={format}
        willChange
        className={cn(
          "tnum text-[1.65rem] leading-none font-medium",
          tone === "gain" && "text-gain",
          tone === "loss" && "text-loss",
        )}
      />
      <div className="text-muted-foreground text-xs">{children}</div>
    </div>
  );
}

// Fills to the share of budget spent, with a tick at 100%.
function UtilizationRule({ basisPoints }: { basisPoints: number }) {
  const spent = Math.min(100, Math.max(0, basisPoints / 100));
  const over = basisPoints > 10_000;

  return (
    <div className="bg-border relative mt-1 h-px w-full">
      <div
        className={cn(
          "absolute inset-y-0 left-0 transition-[width] duration-500",
          over ? "bg-loss" : "bg-foreground",
        )}
        style={{ width: `${spent}%` }}
      />
      <div className="bg-border absolute -top-1 right-0 h-2 w-px" />
    </div>
  );
}
