/**
 * Typed client for the Ledgerline API. Every amount is integer cents and every
 * ratio is integer basis points, mirroring the Go types; the frontend never
 * parses a decimal because the API never sends one.
 */

const BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

export type BudgetStatus = "on_track" | "at_risk" | "over_budget" | "unbudgeted";

export interface CategoryLine {
  category_code: string;
  category_name: string;
  budget_cents: number;
  actual_cents: number;
  /** budget - actual: positive is money left, negative is an overspend. */
  variance_cents: number;
  /** actual / budget in basis points; 10000 is exactly on budget. */
  utilization_bps: number;
  status: BudgetStatus;
  budgeted: boolean;
  transaction_count: number;
}

export interface ReportTotals {
  budget_cents: number;
  actual_cents: number;
  variance_cents: number;
  utilization_bps: number;
  category_count: number;
  over_budget_count: number;
  transaction_count: number;
}

export interface BudgetReport {
  month: string;
  period_start: string;
  period_end: string;
  currency: string;
  totals: ReportTotals;
  categories: CategoryLine[];
}

export interface Transaction {
  id: number;
  external_id: string;
  category_code: string;
  category_name: string;
  vendor: string;
  description: string;
  amount_cents: number;
  /** Negative for a credit, so the dashboard need not know the convention. */
  signed_amount_cents: number;
  currency: string;
  direction: "debit" | "credit";
  occurred_on: string;
  source_updated_at: string;
}

export interface TransactionPage {
  transactions: Transaction[];
  total: number;
  limit: number;
  offset: number;
}

export type SyncStatus = "running" | "success" | "failed";

export interface SyncRun {
  id: number;
  source: string;
  trigger: "startup" | "scheduler" | "manual";
  status: SyncStatus;
  cursor_before: string | null;
  cursor_after: string | null;
  pages_fetched: number;
  records_fetched: number;
  records_created: number;
  records_updated: number;
  records_skipped: number;
  error_message: string | null;
  started_at: string;
  finished_at: string | null;
  duration_ms: number | null;
}

export interface SyncResult {
  run_id: number;
  source: string;
  trigger: string;
  status: SyncStatus;
  cursor_before: string | null;
  cursor_after: string | null;
  pages_fetched: number;
  records_fetched: number;
  records_created: number;
  records_updated: number;
  records_skipped: number;
  started_at: string;
  finished_at: string;
  duration_ms: number;
  error_message?: string;
}

/** Carries the API's own error code so callers can branch on it. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }

  /** A sync is already running: a wait, not a failure. */
  get isConflict(): boolean {
    return this.status === 409;
  }
}

interface ErrorEnvelope {
  error?: { code?: string; message?: string };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${path}`, {
    headers: { Accept: "application/json" },
    ...init,
  });

  if (!response.ok) {
    // Failures are JSON envelopes, but a proxy or a crash can produce
    // something else -- fall back rather than throw twice.
    let code = "unknown";
    let message = `request failed with status ${response.status}`;

    try {
      const body = (await response.json()) as ErrorEnvelope;
      code = body.error?.code ?? code;
      message = body.error?.message ?? message;
    } catch {
      // keep the fallback
    }

    throw new ApiError(response.status, code, message);
  }

  return (await response.json()) as T;
}

export const api = {
  budgetVsActual(month?: string): Promise<BudgetReport> {
    const query = month ? `?month=${encodeURIComponent(month)}` : "";

    return request<BudgetReport>(`/reports/budget-vs-actual${query}`);
  },

  months(): Promise<string[]> {
    return request<{ months: string[] | null }>("/reports/months").then(
      (body) => body.months ?? [],
    );
  },

  transactions(params: {
    month?: string;
    category?: string;
    limit?: number;
  }): Promise<TransactionPage> {
    const query = new URLSearchParams();
    if (params.month) query.set("month", params.month);
    if (params.category) query.set("category", params.category);
    if (params.limit) query.set("limit", String(params.limit));

    const suffix = query.size > 0 ? `?${query.toString()}` : "";

    return request<TransactionPage>(`/transactions${suffix}`);
  },

  syncRuns(limit = 8): Promise<SyncRun[]> {
    return request<{ runs: SyncRun[] | null }>(`/sync/runs?limit=${limit}`).then(
      (body) => body.runs ?? [],
    );
  },

  triggerSync(): Promise<SyncResult> {
    return request<SyncResult>("/sync", { method: "POST" });
  },
};
