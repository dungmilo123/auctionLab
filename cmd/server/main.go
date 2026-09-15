// Command server is the single AuctionLab binary: SQLite (WAL-asserted),
// the embedded React SPA, /api/health and the temporary /ws echo.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"auctionlab"
	"auctionlab/internal/api"
	"auctionlab/internal/hub"
	"auctionlab/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 8080, "HTTP port to listen on")
	tunnel := flag.Bool("tunnel", false, "start a cloudflared quick tunnel (not implemented until Phase 7)")
	publicURL := flag.String("public-url", "", "public URL to advertise in QR codes (not implemented until Phase 7)")
	flag.Parse()

	// The flags are parsed now so the CLI signature is stable for Phase 7;
	// Phase 1 only reports that the tunnel path is not wired yet.
	if *tunnel {
		log.Printf("--tunnel parsed but not implemented until Phase 7; continuing without a tunnel")
	}
	if *publicURL != "" {
		log.Printf("--public-url=%s parsed but not implemented until Phase 7; continuing without a public URL", *publicURL)
	}

	dbPath := store.PathFromEnv()

	// Boot step 1 — SQLite. A non-WAL journal mode aborts the boot.
	st, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	// The boot log reports the state the assertion actually observed, not what
	// the DSN intends — same source /api/health is built from below.
	pragmas := st.Pragmas()
	log.Printf("sqlite: opened %s (journal_mode=%s, busy_timeout=%dms, foreign_keys=%t)",
		dbPath, pragmas.JournalMode, pragmas.BusyTimeout, pragmas.ForeignKeys)

	// Boot step 2 — the embedded SPA must be usable before the listener starts.
	spa, err := auctionlab.SPA()
	if err != nil {
		return fmt.Errorf("embedded SPA: %w", err)
	}
	spaHandler, err := api.NewStatic(spa)
	if err != nil {
		return err
	}

	// Boot step 3 — registration order matters: the health endpoint first, then
	// a JSON 404 for every other /api/* path, then /ws, and only then the SPA
	// catch-all. A catch-all registered first would swallow API and WS traffic.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.NewHealth(pragmas.JournalMode == "wal"))
	mux.HandleFunc("/api/", api.NotFoundJSON)
	mux.HandleFunc("/ws", hub.Echo)
	mux.Handle("/", spaHandler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("listening on %s", addr)

	return newServer(addr, mux).ListenAndServe()
}

// newServer builds the HTTP server this project can live with: a client that
// dribbles its request headers cannot hold a connection slot indefinitely
// (gosec G112 — the slowloris class), and idle keep-alives are bounded too.
//
// ReadTimeout and WriteTimeout stay unset on purpose: they cover the whole
// connection lifetime, and /ws is a long-lived upgrade that must never be cut
// off mid-presentation.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
