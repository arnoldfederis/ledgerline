package api

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"ledgerline/apps/server/internal/ledger"
	"ledgerline/apps/server/internal/mocksource"
	"ledgerline/apps/server/internal/syncer"
	"ledgerline/apps/server/internal/utils"
)

// Deps is everything the HTTP layer needs, passed in from main rather than
// reached for through package-level state.
type Deps struct {
	Log         *slog.Logger
	Pool        *pgxpool.Pool
	Ledger      *ledger.Service
	Syncer      *syncer.Service
	MockSource  *mocksource.Handler
	CORSOrigins []string
	Version     string
	SourceName  string
}

func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	health := &HealthHandler{pool: deps.Pool, version: deps.Version, log: deps.Log}
	mux.HandleFunc("GET /health/live", health.HealthLive)
	mux.HandleFunc("GET /health/ready", health.HealthReady)

	reports := &ReportHandler{ledger: deps.Ledger, log: deps.Log}
	mux.HandleFunc("GET /api/v1/reports/budget-vs-actual", reports.BudgetVsActual)
	mux.HandleFunc("GET /api/v1/reports/months", reports.Months)
	mux.HandleFunc("GET /api/v1/categories", reports.Categories)

	transactions := &TransactionHandler{ledger: deps.Ledger, log: deps.Log}
	mux.HandleFunc("GET /api/v1/transactions", transactions.List)

	sync := &SyncHandler{syncer: deps.Syncer, ledger: deps.Ledger, log: deps.Log}
	mux.HandleFunc("POST /api/v1/sync", sync.Trigger)
	mux.HandleFunc("GET /api/v1/sync/runs", sync.Runs)

	// The stand-in provider is not part of the product's API; in a real
	// deployment this handler is simply not registered.
	if deps.MockSource != nil {
		mux.HandleFunc("GET /mock/accounting/transactions", deps.MockSource.Transactions)
	}

	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, _ *http.Request) {
		utils.WriteJSON(w, http.StatusOK, map[string]any{
			"service": "ledgerline",
			"version": deps.Version,
			"source":  deps.SourceName,
			"endpoints": []string{
				"GET  /api/v1/reports/budget-vs-actual?month=YYYY-MM",
				"GET  /api/v1/reports/months",
				"GET  /api/v1/categories",
				"GET  /api/v1/transactions?month=YYYY-MM&category=<code>&limit=&offset=",
				"POST /api/v1/sync",
				"GET  /api/v1/sync/runs?limit=",
			},
		})
	})

	// Unrouted requests get the same JSON envelope as everything else.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		notFound(w, "no route for "+r.Method+" "+r.URL.Path)
	})

	return chain(mux,
		recoverPanic(deps.Log),
		requestLogger(deps.Log),
		withCORS(deps.CORSOrigins),
	)
}
