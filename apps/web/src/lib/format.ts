/**
 * The API sends integer cents and integer basis points; this is the only place
 * they become text. Nothing here divides a money value and rounds -- `cents /
 * 100` is a float, which is what the backend went to trouble to avoid.
 */

const groupFormatter = new Intl.NumberFormat("en-US", {
  maximumFractionDigits: 0,
  useGrouping: true,
});

function parts(cents: number): { sign: string; dollars: number; remainder: number } {
  const rounded = Math.trunc(cents);
  const absolute = Math.abs(rounded);

  return {
    sign: rounded < 0 ? "-" : "",
    dollars: Math.trunc(absolute / 100),
    remainder: absolute % 100,
  };
}

/** `123456` -> `$1,234.56`. */
export function formatCents(cents: number): string {
  const { sign, dollars, remainder } = parts(cents);

  return `${sign}$${groupFormatter.format(dollars)}.${String(remainder).padStart(2, "0")}`;
}

/**
 * `123456` -> `"1234.56"`: the exact decimal, built with integer arithmetic.
 *
 * `Intl.NumberFormat` accepts a decimal *string* and formats it exactly, so
 * handing it this instead of `cents / 100` keeps the display path as
 * float-free as the rest of the pipeline. NumberFlow takes the same type.
 */
export function centsToDecimalString(cents: number): string {
  const { sign, dollars, remainder } = parts(cents);

  return `${sign}${dollars}.${String(remainder).padStart(2, "0")}`;
}

/**
 * `10060` basis points -> `"1.0060"`, the ratio as an exact decimal string for
 * a percent-styled formatter.
 */
export function bpsToRatioString(basisPoints: number): string {
  const rounded = Math.trunc(basisPoints);
  const absolute = Math.abs(rounded);

  return `${rounded < 0 ? "-" : ""}${Math.trunc(absolute / 10_000)}.${String(absolute % 10_000).padStart(4, "0")}`;
}

/** Always carries a sign, for variance figures. */
export function formatSignedCents(cents: number): string {
  if (cents > 0) {
    return `+${formatCents(cents)}`;
  }

  return formatCents(cents);
}

/** `1234567` -> `$12.3k`, where a full figure will not fit. */
export function formatCompactCents(cents: number): string {
  const { sign, dollars } = parts(cents);

  if (dollars >= 1_000_000) {
    return `${sign}$${(dollars / 1_000_000).toFixed(1)}m`;
  }
  if (dollars >= 1_000) {
    return `${sign}$${(dollars / 1_000).toFixed(1)}k`;
  }

  return `${sign}$${groupFormatter.format(dollars)}`;
}

/** `12750` basis points -> `127.5%`. */
export function formatBasisPoints(basisPoints: number): string {
  const percent = basisPoints / 100;
  const decimals = Number.isInteger(percent) ? 0 : 1;

  return `${percent.toFixed(decimals)}%`;
}

/** `2026-11` -> `November 2026`. */
export function formatMonthLabel(month: string): string {
  const [year, index] = month.split("-");
  const parsed = new Date(Date.UTC(Number(year), Number(index) - 1, 1));

  if (Number.isNaN(parsed.getTime())) {
    return month;
  }

  return parsed.toLocaleDateString("en-US", { month: "long", year: "numeric", timeZone: "UTC" });
}

/** `2026-11-04` -> `Nov 4`. */
export function formatDayLabel(date: string): string {
  const parsed = new Date(`${date}T00:00:00Z`);

  if (Number.isNaN(parsed.getTime())) {
    return date;
  }

  return parsed.toLocaleDateString("en-US", { month: "short", day: "numeric", timeZone: "UTC" });
}

export function formatRelativeTime(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) {
    return "never";
  }

  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) {
    return "unknown";
  }

  const seconds = Math.round((now.getTime() - parsed.getTime()) / 1000);
  if (seconds < 45) {
    return "just now";
  }
  if (seconds < 3600) {
    return `${Math.round(seconds / 60)}m ago`;
  }
  if (seconds < 86_400) {
    return `${Math.round(seconds / 3600)}h ago`;
  }

  return `${Math.round(seconds / 86_400)}d ago`;
}

export function formatDuration(milliseconds: number | null | undefined): string {
  if (milliseconds === null || milliseconds === undefined) {
    return "--";
  }
  if (milliseconds < 1000) {
    return `${milliseconds}ms`;
  }

  return `${(milliseconds / 1000).toFixed(1)}s`;
}
