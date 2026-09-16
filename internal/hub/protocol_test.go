package hub

import (
	"fmt"
	"slices"
	"testing"
)

// This file is the frozen-protocol drift lock (D-21/D-24). Two rules shape it:
//
//   - D-21 supersedes PLAN.md §8's original envelope wording, which used a
//     single `seq` that WAS the revision. seq is now the per-connection stream
//     sequence and rev is the game revision; PLAN.md §8's message/error tables
//     survive, its "seq = revision" reading does not.
//   - D-24 freezes the semantics: fields and catalog entries may be ADDED to,
//     never renamed, removed or repurposed. Every assertion below therefore
//     writes the wire value as a literal and requires production's identifier
//     to agree with it — a rename fails here, not in a client.

// TestProtocol pins the frozen envelope, the sibling-revision rule and the
// complete catalog on the wire.
func TestProtocol(t *testing.T) {
	t.Run("the envelope carries exactly type/seq/rev/ts/data", func(t *testing.T) {
		h, _ := newTestHub(t)
		conn := dialWS(t, h)
		writeFrame(t, conn, `{"type":"join","role":"screen"}`)

		env := readFrame(t, conn)
		assertEnvelopeKeys(t, env)
		if env.TS <= 0 {
			t.Errorf("ts = %d, want a positive server millisecond clock (D-21)", env.TS)
		}
	})

	t.Run("two messages may share one revision", func(t *testing.T) {
		h, _ := newTestHub(t)
		conn := dialWS(t, h)
		writeFrame(t, conn, `{"type":"join","role":"screen"}`)

		first := readFrame(t, conn)
		second := readFrame(t, conn)

		if first.Rev != second.Rev {
			t.Fatalf("rev %d then %d, want the same revision — one mutation legitimately produces several messages (D-22)",
				first.Rev, second.Rev)
		}
		if second.Seq != first.Seq+1 {
			t.Errorf("seq %d then %d, want consecutive numbers (D-22: gap detection is seq's job, never rev's)",
				first.Seq, second.Seq)
		}
	})

	t.Run("the message catalog matches Plan §8 literally", func(t *testing.T) {
		catalog := []struct{ literal, constant string }{
			// client → server
			{"join", TypeJoin},
			{"resync", TypeResync},
			{"bid.place", TypeBidPlace},
			{"dutch.accept", TypeDutchAccept},
			{"sealed.submit", TypeSealedSubmit},
			{"admin.item.upsert", TypeAdminItemUpsert},
			{"admin.item.delete", TypeAdminItemDelete},
			{"admin.item.reorder", TypeAdminItemReorder},
			{"admin.item.start", TypeAdminItemStart},
			{"admin.item.force_end", TypeAdminItemForceEnd},
			{"admin.item.next", TypeAdminItemNext},
			{"admin.game.reset", TypeAdminGameReset},
			{"admin.config.update", TypeAdminConfigUpdate},
			{"admin.player.release", TypeAdminPlayerRelease},
			// server → client
			{"hello", TypeHello},
			{"state.snapshot", TypeStateSnapshot},
			{"phase.changed", TypePhaseChanged},
			{"item.intro", TypeItemIntro},
			{"item.live", TypeItemLive},
			{"price.tick", TypePriceTick},
			{"bid.accepted", TypeBidAccepted},
			{"bid.rejected", TypeBidRejected},
			{"sealed.received", TypeSealedReceived},
			{"item.settled", TypeItemSettled},
			{"leaderboard", TypeLeaderboard},
			{"finale", TypeFinale},
			{"tunnel.status", TypeTunnelStatus},
			{"error", TypeError},
		}

		seen := make(map[string]int, len(catalog))
		for _, entry := range catalog {
			if entry.constant != entry.literal {
				t.Errorf("catalog entry = %q, want the literal %q", entry.constant, entry.literal)
			}
			seen[entry.literal]++
		}
		for literal, count := range seen {
			if count > 1 {
				t.Errorf("catalog value %q is claimed by %d entries — two types would be indistinguishable on the wire", literal, count)
			}
		}
	})

	t.Run("every required error code is present, spelled exactly", func(t *testing.T) {
		codes := []struct{ literal, constant string }{
			{"nickname_invalid", CodeNicknameInvalid},
			{"nickname_taken", CodeNicknameTaken},
			{"token_invalid", CodeTokenInvalid},
			{"not_joined", CodeNotJoined},
			{"bad_message", CodeBadMessage},
			{"unsupported", CodeUnsupported},
			{"not_authorized", CodeNotAuthorized}, // Phase 3: admin-role misuse (D-65); Phase 4 adds ADMIN_KEY (ADMN-04)
		}
		for _, entry := range codes {
			if entry.constant != entry.literal {
				t.Errorf("error code = %q, want the literal %q", entry.constant, entry.literal)
			}
		}
	})

	t.Run("the still-unimplemented frozen commands answer unsupported today", func(t *testing.T) {
		// D-24: the catalog is defined now, so a client that sends a later
		// phase's command gets a typed answer instead of silence. A command not
		// in the catalog is not distinguished here on purpose — the catalog is
		// freeze-only-additive, so "frozen but not implemented" is the answer.
		//
		// Phase 3 removed `bid.place` and the six `admin.item.*` commands from
		// this list: they are routed now, and their own answers are pinned by
		// auction_test.go (a role gate for the wrong role, a mutation or a
		// typed bad_message for the right one).
		//
		// Phase 4's 04-03 removes `admin.game.reset` and `admin.config.update`
		// for the same reason: they route to the engine now, and their
		// transport contracts are pinned by TestAdminReset/TestAdminRelease and
		// by the config cases in internal/game.
		//
		// Phase 5's 05-01 removes `dutch.accept`: it routes to the engine now,
		// and its transport contract is pinned by dutch_test.go (player-only,
		// with the private refusal frames).
		future := []string{
			TypeSealedSubmit,
		}

		h, _ := newTestHub(t)
		conn := dialWS(t, h)
		writeFrame(t, conn, adminJoinJSON)
		_ = readFrame(t, conn) // hello
		_ = readFrame(t, conn) // state.snapshot

		for _, kind := range future {
			writeFrame(t, conn, fmt.Sprintf(`{"type":%q}`, kind))
			env := readFrame(t, conn)
			if env.Type != "error" {
				t.Fatalf("%s: frame type = %q, want \"error\"", kind, env.Type)
			}
			if got := frameString(t, frameData(t, env), "code"); got != "unsupported" {
				t.Fatalf("%s: error code = %q, want %q", kind, got, "unsupported")
			}
		}
	})

	t.Run("tunnel.status is the documented control-plane exception", func(t *testing.T) {
		// Phase 7 owns the behaviour; the freeze owns the exception. A client
		// must apply tunnel.status regardless of seq and must never put it
		// through the revision gate, or a URL rotation whose rev looked stale
		// would be dropped mid-presentation.
		if tunnelStatusControlPlane != "tunnel.status" {
			t.Errorf("tunnelStatusControlPlane = %q, want the literal \"tunnel.status\"", tunnelStatusControlPlane)
		}
		if tunnelStatusControlPlane != TypeTunnelStatus {
			t.Errorf("the control-plane marker %q disagrees with the catalog entry %q", tunnelStatusControlPlane, TypeTunnelStatus)
		}
	})
}

// TestProtocolFreeze is the D-24 freeze made executable (end of Phase 2).
//
// TestProtocol above compares each constant against the literal it publishes.
// This test is the other half: it treats the message catalog and the error codes
// as CLOSED sets, compared against literals written by hand here and never
// derived from production. That is what makes the freeze's three prohibitions
// real —
//
//   - rename: fails TestProtocol (the constant no longer equals its literal) and
//     the comparison below;
//   - removal: the inventory in protocol.go stops matching the pinned list here;
//   - semantic replacement: an entry repurposed to a different literal fails
//     both tests, and a duplicate literal is caught below.
//
// An addition is allowed (D-24) and this test is where it becomes deliberate:
// the new entry must be appended to frozenCatalog/frozenErrorCodes AND to the
// pinned lists below, so the wire surface can only widen in a reviewable diff.
func TestProtocolFreeze(t *testing.T) {
	frozen := []string{
		// client → server
		"join",
		"resync",
		"bid.place",
		"dutch.accept",
		"sealed.submit",
		"admin.item.upsert",
		"admin.item.delete",
		"admin.item.reorder",
		"admin.item.start",
		"admin.item.force_end",
		"admin.item.next",
		"admin.game.reset",
		"admin.config.update",
		// Phase 4 (D-80): the one deliberate additive client→server type — the
		// console's per-player session release. It stays inside the client→server
		// group, so the two pinned inventories and frozenCatalog agree exactly.
		"admin.player.release",
		// server → client
		"hello",
		"state.snapshot",
		"phase.changed",
		"item.intro",
		"item.live",
		"price.tick",
		"bid.accepted",
		"bid.rejected",
		"sealed.received",
		"item.settled",
		"leaderboard",
		"finale",
		"tunnel.status",
		"error",
	}

	if got := frozenCatalog[:]; !slices.Equal(got, frozen) {
		t.Errorf("the frozen message catalog changed:\n got %q\nwant %q\n"+
			"D-24 forbids renames and removals; an addition must be appended to both lists",
			got, frozen)
	}

	seen := make(map[string]int, len(frozen))
	for _, literal := range frozen {
		seen[literal]++
	}
	for literal, count := range seen {
		if count > 1 {
			t.Errorf("the frozen catalog lists %q %d times — one wire value cannot mean two types", literal, count)
		}
	}

	codes := []string{
		"nickname_invalid",
		"nickname_taken",
		"token_invalid",
		"not_joined",
		"bad_message",
		"unsupported",
		"not_authorized",
	}
	if got := frozenErrorCodes[:]; !slices.Equal(got, codes) {
		t.Errorf("the frozen error catalog changed:\n got %q\nwant %q\n"+
			"D-24 forbids renames and removals; an addition must be appended to both lists",
			got, codes)
	}
}
