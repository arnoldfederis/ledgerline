import { describe, expect, it } from "vitest";

import {
  bpsToRatioString,
  centsToDecimalString,
  formatBasisPoints,
  formatCents,
  formatCompactCents,
  formatDuration,
  formatMonthLabel,
  formatRelativeTime,
  formatSignedCents,
} from "./format";

describe("formatCents", () => {
  it.each([
    [0, "$0.00"],
    [1, "$0.01"],
    [9, "$0.09"],
    [90, "$0.90"],
    [100, "$1.00"],
    [123456, "$1,234.56"],
    [123456789, "$1,234,567.89"],
    [-1, "-$0.01"],
    [-4450000, "-$44,500.00"],
  ])("renders %i cents as %s", (cents, expected) => {
    expect(formatCents(cents)).toBe(expected);
  });

  it("never loses a cent to floating point", () => {
    // 0.1 + 0.2 === 0.30000000000000004 in JavaScript. In cents it is 30,
    // and the formatter must keep it that way.
    expect(formatCents(10 + 20)).toBe("$0.30");
    expect(formatCents(70)).toBe("$0.70");
    expect(formatCents(1_000_000_01)).toBe("$1,000,000.01");
  });
});

describe("formatSignedCents", () => {
  it("marks money left over with an explicit plus", () => {
    expect(formatSignedCents(51268)).toBe("+$512.68");
  });

  it("keeps the minus sign on an overspend", () => {
    expect(formatSignedCents(-15000)).toBe("-$150.00");
  });

  it("leaves zero unsigned", () => {
    expect(formatSignedCents(0)).toBe("$0.00");
  });
});

describe("formatCompactCents", () => {
  it.each([
    [45000, "$450"],
    [123456, "$1.2k"],
    [444_500_00, "$444.5k"],
    [12_345_678_00, "$12.3m"],
    [-650000, "-$6.5k"],
  ])("renders %i cents as %s", (cents, expected) => {
    expect(formatCompactCents(cents)).toBe(expected);
  });
});

describe("formatBasisPoints", () => {
  it.each([
    [10000, "100%"],
    [0, "0%"],
    [9921, "99.2%"],
    [12400, "124%"],
    [10545, "105.5%"],
    [-200, "-2%"],
  ])("renders %i basis points as %s", (bps, expected) => {
    expect(formatBasisPoints(bps)).toBe(expected);
  });
});

describe("formatMonthLabel", () => {
  it("expands an API month into a readable label", () => {
    expect(formatMonthLabel("2026-11")).toBe("November 2026");
    expect(formatMonthLabel("2026-01")).toBe("January 2026");
  });

  it("passes unparseable input straight through", () => {
    expect(formatMonthLabel("")).toBe("");
  });
});

describe("formatRelativeTime", () => {
  const now = new Date("2026-11-23T12:00:00Z");

  it.each([
    ["2026-11-23T11:59:50Z", "just now"],
    ["2026-11-23T11:55:00Z", "5m ago"],
    ["2026-11-23T09:00:00Z", "3h ago"],
    ["2026-11-21T12:00:00Z", "2d ago"],
  ])("renders %s as %s", (iso, expected) => {
    expect(formatRelativeTime(iso, now)).toBe(expected);
  });

  it("handles a missing timestamp", () => {
    expect(formatRelativeTime(null, now)).toBe("never");
    expect(formatRelativeTime(undefined, now)).toBe("never");
  });
});

describe("formatDuration", () => {
  it.each([
    [14, "14ms"],
    [999, "999ms"],
    [1420, "1.4s"],
  ])("renders %i ms as %s", (ms, expected) => {
    expect(formatDuration(ms)).toBe(expected);
  });

  it("handles a run that has not finished", () => {
    expect(formatDuration(null)).toBe("--");
  });
});

describe("centsToDecimalString", () => {
  it.each([
    [0, "0.00"],
    [1, "0.01"],
    [90, "0.90"],
    [123456, "1234.56"],
    [3235000, "32350.00"],
    [-19334, "-193.34"],
  ])("renders %i cents as %s", (cents, expected) => {
    expect(centsToDecimalString(cents)).toBe(expected);
  });

  // The point of the string: Intl formats it exactly, with no float between
  // the ledger and the screen.
  it.each([1, 7, 70, 103, 99_999_999])("round-trips %i cents through Intl", (cents) => {
    const formatted = new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
    }).format(centsToDecimalString(cents) as unknown as number);

    expect(formatted).toBe(formatCents(cents));
  });
});

describe("bpsToRatioString", () => {
  it.each([
    [10000, "1.0000"],
    [10060, "1.0060"],
    [8648, "0.8648"],
    [0, "0.0000"],
    [-200, "-0.0200"],
    [12400, "1.2400"],
  ])("renders %i basis points as %s", (bps, expected) => {
    expect(bpsToRatioString(bps)).toBe(expected);
  });

  it("feeds a percent formatter the figure the ledger computed", () => {
    const percent = new Intl.NumberFormat("en-US", {
      style: "percent",
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    });

    expect(percent.format(bpsToRatioString(10060) as unknown as number)).toBe("100.6%");
    expect(percent.format(bpsToRatioString(8648) as unknown as number)).toBe("86.5%");
  });
});
