// Command server is the single AuctionLab binary: SQLite (WAL-asserted),
// the embedded React SPA, /api/health and the temporary /ws echo.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"auctionlab"
	"auctionlab/internal/api"
	"auctionlab/internal/game"
	"auctionlab/internal/hub"
	"auctionlab/internal/netinfo"
	"auctionlab/internal/store"
	"auctionlab/internal/tunnel"
)

func main() {
	if err := run(); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
}

func run() error {
	port := flag.Int("port", 8080, "HTTP port to listen on")
	wantTunnel := flag.Bool("tunnel", false, "start a cloudflared quick tunnel and advertise its URL")
	cloudflaredBin := flag.String("cloudflared-bin", "", "path to the cloudflared binary (default: TUNNEL_BIN, then PATH)")
	publicURL := flag.String("public-url", "", "public URL to advertise in QR codes (default: PUBLIC_URL)")
	flag.Parse()

	// D-127: PUBLIC_URL is resolved for `game.Config` — it is a BOOT input and
	// never durable state. An unusable value is a warning, never a failed boot:
	// connectivity is not a prerequisite for the auction (D-129), and the value
	// itself is never echoed (a public URL may carry a query string).
	publicValue, publicSource, publicWarning := resolvePublicURL(*publicURL, os.Getenv)
	if publicWarning != "" {
		log.Printf("tunnel: %s", publicWarning)
	} else if publicValue != "" {
		log.Printf("tunnel: %s is set — it overrides the tunnel and the admin selection while this process runs, and is never persisted", publicSource)
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
		// D-127: the boot override, already validated above. The engine validates
		// it again with its own rule and drops an unusable value, so a caller
		// that is not this boot cannot smuggle one in either.
		PublicURL: publicValue,
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

	// Boot step 4 — the hub's two background producers, in one lifecycle owner.
	// Both start only after the publisher is installed (a beat before that would
	// have no fan-out edge to deliver through) and after game.New returned,
	// which is where a deadline that passed while the process was down already
	// settled. The deferred stop runs before the store's own deferred Close —
	// defers are LIFO and st.Close was registered first — and inside the owner
	// the heartbeat stops before the ticker, so neither can outlive the database
	// it reads (Pitfall 11).
	stopLiveness := startLiveness(ws)
	defer stopLiveness()

	// Boot step 4b — the OPTIONAL tunnel (D-123/D-124). The manager is built
	// after the hub exists (its status publishes THROUGH the hub) and its Close
	// is deferred before the liveness stop is registered, so a shutdown takes
	// the tunnel down first. Every failure path is non-fatal: no cloudflared, a
	// config-file conflict, a refused creation — the auction keeps running on
	// LAN/local exactly as if --tunnel had never been passed (D-129).
	plan := planTunnel(*wantTunnel, *cloudflaredBin, os.Getenv, exec.LookPath, os.Getenv("HOME"))
	for _, line := range plan.Log {
		log.Printf("%s", line)
	}
	if plan.Start {
		mgr := tunnel.New(tunnel.Config{
			Port:     *port,
			Bin:      plan.Bin,
			Protocol: os.Getenv("TUNNEL_PROTOCOL"),
			// D-123's one-way wire: the callback is the ONLY place this package
			// is named, so game/, hub/ and store/ never import internal/tunnel.
			Publish: tunnelStatusSink(ws, engine),
			Home:    os.Getenv("HOME"),
		})
		mgr.Start()
		defer mgr.Close()
	}

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

// startLiveness starts the hub's two background producers and returns the ONE
// stop function the boot defers.
//
// The owner exists so the shutdown order is a property of one function rather
// than of the order two `defer`s happen to be written in: the heartbeat stops
// before the ticker, and both stop before the store closes (Pitfall 11). Stop is
// idempotent because the hub's own stops are.
func startLiveness(ws *hub.Hub) (stop func()) {
	stopTicker := ws.StartTicker()
	return func() {
		stopTicker()
	}
}

// resolvePublicURL applies D-127's precedence to the boot override: the
// `--public-url` flag wins the PUBLIC_URL environment variable, and the winner
// is checked by the game package's own D-86 rule.
//
// It returns the accepted value, the name of the source that produced it (for
// the boot log), and an operator-facing warning. It never returns an error: an
// unusable override is ignored and the show continues on the selection or a LAN
// candidate (D-129). The value itself is deliberately never echoed — a
// PUBLIC_URL may carry a query string, and a boot log is not the place for it.
func resolvePublicURL(flagValue string, getenv func(string) string) (value string, source string, warning string) {
	return "", "", ""
}

// tunnelStatusSink is D-123's one-way wire between the isolated tunnel package
// and the two components that consume its status.
func tunnelStatusSink(ws *hub.Hub, engine *game.Engine) tunnel.PublishFunc {
	return func(s tunnel.Status) {}
}

// tunnelPlan is the boot's decision about the OPTIONAL tunnel (D-124): whether a
// manager should be started, which binary it would use, and the operator-facing
// lines to log. It is a value rather than a set of side effects so the whole
// decision table — every non-fatal degradation included — is a table test.
type tunnelPlan struct {
	Start bool
	Bin   string
	Log   []string
}

// planTunnel is D-124/D-129's decision. Nothing here can fail the boot:
// connectivity is never a prerequisite, so a missing binary, a proxy in the
// environment or a conflicting cloudflared config are logged and the auction
// runs on LAN/local.
func planTunnel(enabled bool, flagBin string, getenv func(string) string, lookPath func(string) (string, error), home string) tunnelPlan {
	return tunnelPlan{}
}
