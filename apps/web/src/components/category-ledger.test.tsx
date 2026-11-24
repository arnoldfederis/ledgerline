import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import type { CategoryLine, Transaction } from "@/lib/api";

import { CategoryLedger } from "@/components/category-ledger";

function line(overrides: Partial<CategoryLine> = {}): CategoryLine {
  return {
    category_code: "software",
    category_name: "Software & Subscriptions",
    budget_cents: 275000,
    actual_cents: 319000,
    variance_cents: -44000,
    utilization_bps: 11600,
    status: "over_budget",
    budgeted: true,
    transaction_count: 6,
    ...overrides,
  };
}

function transaction(overrides: Partial<Transaction> = {}): Transaction {
  return {
    id: 1,
    external_id: "txn_00001",
    category_code: "software",
    category_name: "Software & Subscriptions",
    vendor: "Figma",
    description: "Figma subscription",
    amount_cents: 48000,
    signed_amount_cents: 48000,
    currency: "USD",
    direction: "debit",
    occurred_on: "2026-11-04",
    source_updated_at: "2026-11-04T09:12:00Z",
    ...overrides,
  };
}

function renderLedger(props: Partial<Parameters<typeof CategoryLedger>[0]> = {}) {
  const onSelect = vi.fn();

  render(
    <CategoryLedger
      categories={[line()]}
      selected={null}
      onSelect={onSelect}
      transactions={[]}
      transactionsTotal={0}
      transactionsLoading={false}
      {...props}
    />,
  );

  return { onSelect };
}

describe("CategoryLedger", () => {
  it("flags an over-budget category without relying on colour", () => {
    renderLedger();

    const row = screen.getByRole("row", { name: /Software & Subscriptions/ });
    expect(row.getAttribute("data-status")).toBe("over_budget");

    // Three signals that survive a black-and-white print.
    expect(within(row).getByText("Over")).toBeDefined();
    expect(within(row).getByText("-$440.00")).toBeDefined();
    expect(within(row).getByText("116%")).toBeDefined();
  });

  it("shows a positive variance for a category under budget", () => {
    renderLedger({
      categories: [
        line({
          category_code: "office",
          category_name: "Office & Supplies",
          budget_cents: 90000,
          actual_cents: 47700,
          variance_cents: 42300,
          utilization_bps: 5300,
          status: "on_track",
        }),
      ],
    });

    expect(screen.getByText("On track")).toBeDefined();
    expect(screen.getByText("+$423.00")).toBeDefined();
  });

  it("does not invent a variance for an unbudgeted category", () => {
    renderLedger({
      categories: [
        line({
          category_code: "legal",
          category_name: "Legal",
          budget_cents: 0,
          variance_cents: 0,
          utilization_bps: 0,
          status: "unbudgeted",
          budgeted: false,
        }),
      ],
    });

    expect(screen.getByText("No budget")).toBeDefined();
    expect(screen.getAllByText("--")).toHaveLength(3);
  });

  it("reports the category when a row is opened and again when it is closed", async () => {
    const user = userEvent.setup();

    const open = renderLedger();
    await user.click(screen.getByRole("button", { name: /Software & Subscriptions/ }));
    expect(open.onSelect).toHaveBeenCalledWith("software");
  });

  it("clears the selection when the open row is activated again", async () => {
    const user = userEvent.setup();

    const { onSelect } = renderLedger({ selected: "software" });
    await user.click(screen.getByRole("button", { name: /Software & Subscriptions/ }));

    expect(onSelect).toHaveBeenCalledWith(null);
  });

  it("lists the transactions behind the open category, credits negative", () => {
    renderLedger({
      selected: "software",
      transactions: [
        transaction(),
        transaction({
          id: 2,
          external_id: "txn_00002",
          vendor: "Linear",
          description: "Refund -- Linear",
          direction: "credit",
          amount_cents: 12000,
          signed_amount_cents: -12000,
        }),
      ],
      transactionsTotal: 2,
    });

    expect(screen.getByText("$480.00")).toBeDefined();
    expect(screen.getByText("-$120.00")).toBeDefined();
  });

  it("renders an empty state rather than a bare table", () => {
    renderLedger({ categories: [] });

    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.getByText(/Nothing budgeted or spent/)).toBeDefined();
  });
});
