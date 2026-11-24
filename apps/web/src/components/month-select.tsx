import { Loader2 } from "lucide-react";

import { FixedWidth } from "@/components/fixed-width";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatMonthLabel } from "@/lib/format";

// The trigger takes its natural width. `September` is the longest month name,
// so sizing both the real trigger and its placeholder against this label keeps
// the control one width whatever month is selected, and makes the first-load
// swap free of any reflow.
const WIDEST_LABEL = "September 2026";

interface MonthSelectProps {
  months: string[];
  value: string;
  onChange: (month: string) => void;
  /** A month change is in flight: hold still and show it. */
  loading?: boolean;
}

export function MonthSelect({ months, value, onChange, loading }: MonthSelectProps) {
  return (
    <Select
      value={value}
      disabled={loading}
      onValueChange={(next) => {
        // Base UI types the value as nullable because a Select can be
        // clearable. This one has no clear affordance, so null never arrives.
        if (next !== null) {
          onChange(next);
        }
      }}
    >
      <SelectTrigger
        size="sm"
        aria-label="Reporting month"
        className="w-40"
        aria-busy={loading}
        icon={
          loading ? (
            // The chevron's slot is 16px, so the spinner gets a 16px slot too
            // and spins at 14px inside it. Matching the box alone was not
            // enough: a filled circle reads heavier than a thin chevron, and
            // `animate-spin` wobbles its painted bounds by a fraction of a
            // pixel. A fixed slot pins the layout, the smaller glyph evens out
            // the optical weight.
            <span className="flex size-4 shrink-0 items-center justify-center">
              <Loader2 aria-hidden className="text-muted-foreground size-3.5 animate-spin" />
            </span>
          ) : undefined
        }
      >
        <SelectValue>
          {/* The trigger holds the API's `2026-11`; render the readable label. */}
          {(month: string) => (
            <FixedWidth reserve={WIDEST_LABEL}>{formatMonthLabel(month)}</FixedWidth>
          )}
        </SelectValue>
      </SelectTrigger>
      <SelectContent align="end">
        {months.map((month) => (
          <SelectItem key={month} value={month}>
            {formatMonthLabel(month)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Stands in until the month list arrives, in the same box as the real trigger. */
export function MonthSelectPlaceholder() {
  return (
    <div
      aria-hidden
      className="bg-input/50 flex h-7 items-center justify-between gap-1.5 rounded-2xl px-3 text-sm w-40"
    />
  );
}
