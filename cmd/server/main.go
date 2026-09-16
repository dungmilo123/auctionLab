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
	"auctionlab/internal/game"
	"auctionlab/internal/hub"
	"auctionlab/internal/netinfo"
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

	// Boot step 2 — the complete PLAN.md §5 schema, then the engine that owns
	// every mutation (D-36/D-39). A commit failure exits the process instead of
	// continuing on state the database does not have: an auction must never
	// silently diverge from what a crash would reload (D-34).
	if err := store.Schema(st.DB); err != nil {
		return err
	}

	// Boot step 2b — D-74's admin capability. The key comes from ADMIN_KEY, the
	// persisted server_config row, or one minted and written to that row before
	// the listener opens; every failure exits rather than serving a key the file
	// does not hold. It deliberately does NOT enter game.Config: the admin
	// projection is built from config_json, and a key that could reach a
	// projection would be a leak.
	adminKey, err := st.ResolveAdminKey(os.Getenv, game.NewToken)
	if err != nil {
		return err
	}
	// The one line the presenter needs. It is printed to the server console
	// only, and it is the sole source of the key after boot (D-74/D-77).
	log.Printf("admin: /admin?k=%s", adminKey)

	// Boot step 2c — D-83's join-URL candidates. The enumeration runs once, here,
	// because interfaces do not change during a show (a hotspot switch is a
	// restart), and its product is a BOOT INPUT rather than durable state:
	// config_json persists the SELECTED url (D-84), never the proposals.
	//
	// It is best-effort by contract (netinfo.System, T-04-11): a machine with no
	// private address — or a kernel that refuses to list its interfaces — offers
	// no candidate and the boot continues, because D-86's "this page's URL"
	// fallback is what keeps the panel usable. Aborting the boot here would take
	// the show down over a cosmetic list.
	candidates, err := netinfo.Enumerate(netinfo.System, *port)
	if err != nil {
		log.Printf("netinfo: %v (offering no LAN join URL; the admin can still enter one)", err)
	}
	if len(candidates) > 0 {
		log.Printf("lan: %d join URL candidate(s), first=%s", len(candidates), candidates[0])
	} else {
		log.Printf("lan: no private IPv4 address on this machine; the admin panel will offer its own origin")
	}

	engine, err := game.New(game.Config{
		Store: st,
		Now:   time.Now,
		Fatal: func(err error) {
			log.Printf("fatal: unrecoverable game state: %v", err)
			os.Exit(1)
		},
		// D-84/D-86: what the admin panel may offer. The engine projects the
		// list; it never persists it (the selection is what config_json holds).
		JoinURLCandidates: candidates,
	})
	if err != nil {
		return err
	}
	// D-62: the boot log names the run this process picked up, so an operator
	// can read the resumed item and deadline without opening the database. A
	// deadline that passed while the process was down has already settled
	// inside game.New, which is why this line reports the phase New returned.
	if itemID, deadline := engine.ResumedAuction(); itemID != "" {
		log.Printf("game: resumed %s item=%s deadline=%d at revision %d with %d player(s)",
			engine.Phase(), itemID, deadline, engine.Rev(), engine.PlayerCount())
	} else {
		log.Printf("game: resumed %s at revision %d with %d player(s)", engine.Phase(), engine.Rev(), engine.PlayerCount())
	}

	// Boot step 3 — the hub. It owns /ws and it IS the engine's fan-out edge:
	// the engine calls Enqueue with its exclusive lock held after every
	// committed mutation and every presence change, so the enqueue can never
	// lose a race with the state it describes (D-33). It also owns D-76's admin
	// authorization, which is why the resolved key goes here and not into
	// game.Config.
	ws := hub.New(engine, adminKey)
	engine.SetPublisher(ws.Fanout)

	// Boot step 4 — the D-56 ticker. It starts only after the publisher is
	// installed (a beat before that would have no fan-out edge to deliver
	// through) and after game.New returned, which is where a deadline that
	// passed while the process was down already settled. The deferred stop runs
	// before the store's own deferred Close — defers are LIFO and st.Close was
	// registered first — so no beat can outlive the database it reads (Pitfall
	// 11).
	stopTicker := ws.StartTicker()
	defer stopTicker()

	// Boot step 3 — the embedded SPA must be usable before the listener starts.
	spa, err := auctionlab.SPA()
	if err != nil {
		return fmt.Errorf("embedded SPA: %w", err)
	}
	spaHandler, err := api.NewStatic(spa)
	if err != nil {
		return err
	}

	// Boot step 3 — the route table. Go 1.22+ ServeMux matches patterns by
	// specificity, not registration order, so the "/" fallback cannot swallow
	// API or WS traffic no matter where it is registered (the real failure mode
	// of an overlapping pattern is a registration-time panic). What must hold
	// is that "/api/" and "/ws" stay non-overlapping with the "/" SPA fallback.
	//
	// D-75: the admin capability gate owns /admin and everything under it —
	// registered here, ahead of the fallback, so no deep link can reach the SPA
	// document without a valid capability cookie. Step 2b resolved the key.
	admin := api.AdminGate(adminKey, spaHandler)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.NewHealth(pragmas.JournalMode == "wal"))
	mux.HandleFunc("/api/", api.NotFoundJSON)
	mux.HandleFunc("/ws", ws.ServeWS)
	mux.Handle("GET /admin", admin)
	mux.Handle("/admin/", admin)
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
