package game

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file is D-81/D-84's config contract: `admin.config.update` is the ONE
// command that edits the persisted configuration, and the join URL the QR codes
// are rendered from is the same persisted server state (D-84) — a proposal list
// exists (D-83's enumeration, injected at boot) but the SELECTED value lives
// here, in game.config_json, because every consumer (admin panel, /screen
// projectors, a process that restarted mid-show) has to agree on it.

// ConfigInput is the `admin.config.update` payload (D-81/D-84): a partial
// update, so an absent field means "leave it alone". `starting_budget` is the
// Setup tab's budget field; `join_url` is the Setup tab's URL selection.
type ConfigInput struct {
	StartingBudget *int64  `json:"starting_budget"`
	JoinURL        *string `json:"join_url"`
}

// MaxJoinURLLen is D-86's cap on a join URL. The bound exists because the URL is
// rendered as QR text and echoed back to every projector: an unbounded string is
// a frame-size and a QR-encoding problem, and the cap is far above any real
// `http://192.168.x.y:8080/` (D-83's candidates are ~30 characters).
const MaxJoinURLLen = 200

// UpdateConfig is `admin.config.update` (D-81/D-84): an ordinary applyLocked
// mutation that edits the persisted configuration and nothing else.
//
// Invalid input is refused BEFORE anything is touched, so a bad form value is a
// refusal and never a half-applied edit — the same shape as every other admin
// command (D-66's validators). It also deliberately never touches a player's
// balance: `starting_budget` is what the NEXT join is minted with and what a
// reset restores (D-78), so a mid-game edit cannot silently move anyone's
// economics.
func (e *Engine) UpdateConfig(in ConfigInput) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := validateConfigInput(in); err != nil {
		return err
	}

	next := e.state.config
	if in.StartingBudget != nil {
		next.StartingBudget = *in.StartingBudget
	}
	if in.JoinURL != nil {
		next.JoinURL = *in.JoinURL
	}

	return e.applyLocked(func(s *state) error {
		s.config = next
		// D-81/D-84: the revision moves and every recipient is handed its OWN
		// projection — the admin panel learns the new budget and selection, each
		// /screen projector rebuilds its QR code, and a player view is unchanged
		// because it has no field that could carry the change. The fan-out is
		// per-role by construction (hub.Enqueue calls v.Snapshot(role, …)), so
		// "admin-only" needs no narrower channel.
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil)
}

// validateConfigInput runs D-81/D-86's rules in order and returns ONE error
// naming the field at fault, the shape every other admin validator in this
// package uses (items.go's validateItemInput). An absent field is not validated
// at all: a partial update only has to justify what it changes.
func validateConfigInput(in ConfigInput) error {
	if in.StartingBudget != nil {
		if *in.StartingBudget < 1 || *in.StartingBudget > MaxAmount {
			return fmt.Errorf("game: config starting_budget %d must be within [1, %d]", *in.StartingBudget, MaxAmount)
		}
	}
	if in.JoinURL != nil {
		if err := ValidateJoinURL(*in.JoinURL); err != nil {
			return err
		}
	}
	return nil
}

// ValidateJoinURL is D-86's rule, and nothing more: non-empty, `http://` or
// `https://`, at most MaxJoinURLLen characters.
//
// The URL is never dialled, never resolved and never compared against the
// boot-time candidates, because it is only ever the text a QR code encodes and a
// phone types (T-04-09) — and because D-86 explicitly allows the admin to select
// `location.origin`, a value the server cannot see and must not second-guess.
// Counted in RUNES like every other user-supplied string here (D-28/D-66): a
// byte count would refuse a URL the admin's own keyboard measured as short.
//
// Exported because D-127's `PUBLIC_URL` override is validated by this exact rule
// — the engine applies it to the boot input, and cmd/server reports whether the
// operator's value was accepted — and a second spelling of the rule in cmd/server
// is precisely how the two would drift.
func ValidateJoinURL(raw string) error {
	if raw == "" {
		return errors.New("game: config join_url must not be empty")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return fmt.Errorf("game: config join_url %q must start with http:// or https://", raw)
	}
	if utf8.RuneCountInString(raw) > MaxJoinURLLen {
		return fmt.Errorf("game: config join_url must be at most %d characters", MaxJoinURLLen)
	}
	return nil
}

// Join-URL sources: the closed vocabulary of D-127's rungs, carried to the
// admin panel as `join_url_source` so the presenter can see WHY a URL won. They
// are wire strings, and the panel maps each to a dictionary key — an empty
// source means "no URL at all", which renders no tag rather than a guess.
const (
	joinURLSourcePublic    = "public_url"
	joinURLSourceTunnel    = "tunnel"
	joinURLSourceSelected  = "selected"
	joinURLSourceCandidate = "candidate"
)

// effectiveJoinURL is D-127's ONE resolution rule: the boot `PUBLIC_URL`
// override > the tunnel URL while the probe calls it healthy > the persisted
// selection (D-84) > the first LAN candidate (D-83) > nothing. It answers with
// the URL and the rung that produced it in the same call, so the QR text and its
// provenance tag can never disagree.
//
// PUBLIC_URL outranks even the admin's own selection on purpose: it is the
// operator's hard override at the resolution layer, and removing it restores the
// selection untouched (it is never persisted).
//
// The persisted selection wins even when it is absent from the candidate list,
// because that is exactly the case D-86 creates: the admin may select
// `location.origin` (a dev box, a hotspot, a tunnel URL) and the server
// deliberately does not check it against its own enumeration. Re-defaulting
// because a URL is not in the list would silently undo the presenter's choice.
//
// The tunnel rung exists only while `healthy` is true AND the URL is non-empty:
// the zero Status is a DECLARED state meaning "there is no tunnel rung to stand
// on" (D-129), so a tunnel that is starting, dead or flapping falls straight
// through to persisted/LAN play and the room keeps bidding.
func (s *state) effectiveJoinURL() (string, string) {
	if s.publicURL != "" {
		return s.publicURL, joinURLSourcePublic
	}
	if s.tunnelHealthy && s.tunnelURL != "" {
		return s.tunnelURL, joinURLSourceTunnel
	}
	if s.config.JoinURL != "" {
		return s.config.JoinURL, joinURLSourceSelected
	}
	if len(s.joinURLCandidates) > 0 {
		return s.joinURLCandidates[0], joinURLSourceCandidate
	}
	return "", ""
}

// joinURLs is D-83's candidate list as the admin panel receives it: a copy, so
// no consumer can mutate the engine's boot data, and never nil, so the panel
// renders an empty list instead of a null it has to guard against.
func (s *state) joinURLs() []string {
	urls := make([]string, 0, len(s.joinURLCandidates))
	return append(urls, s.joinURLCandidates...)
}
