import { describe, expect, it } from 'vitest'
import {
  SOCKET_CLOSED,
  SOCKET_OPEN,
  initialGateState,
  nextOutcome,
  resumeOutcome,
  type GateState,
} from './gate'
import type { Envelope } from './protocol'

// One frame builder, so every case reads as the rule it pins rather than as
// JSON plumbing. Defaults are a mid-lobby mutation message at rev 3.
function frame(fields: Partial<Envelope> & { type: string }): Envelope {
  return { seq: 5, rev: 3, ts: 1_757_934_000_000, data: null, ...fields }
}

function at(overrides: Partial<GateState>): GateState {
  return { ...initialGateState(), ...overrides }
}

describe('the sequence rule (D-22)', () => {
  it('drops a frame whose seq is already accounted for', () => {
    const step = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'phase.changed', seq: 4 }))
    expect(step.outcome).toEqual({ kind: 'duplicate' })
    expect(step.state).toEqual(at({ lastSeq: 4 }))
  })

  it('drops an older replay', () => {
    expect(nextOutcome(at({ lastSeq: 9 }), frame({ type: 'price.tick', seq: 2 })).outcome).toEqual({
      kind: 'duplicate',
    })
  })

  it('applies the next frame in order without asking for a resync', () => {
    const step = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'phase.changed', seq: 5 }))
    expect(step.outcome).toEqual({ kind: 'apply', requestResync: false })
    expect(step.state.lastSeq).toBe(5)
  })

  it('applies a gap anyway and asks for exactly one coalesced resync', () => {
    const gap = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'bid.accepted', seq: 6 }))
    expect(gap.outcome).toEqual({ kind: 'apply', requestResync: true })
    expect(gap.state.lastSeq).toBe(6)
    expect(gap.state.resyncPending).toBe(true)

    // A second gap before the snapshot answers is still applied — the message
    // is newer — but the request does not repeat.
    const second = nextOutcome(gap.state, frame({ type: 'leaderboard', seq: 9 }))
    expect(second.outcome).toEqual({ kind: 'apply', requestResync: false })
    expect(second.state.lastSeq).toBe(9)

    // A snapshot ends the episode, so a later gap may ask again.
    const repaired = nextOutcome(second.state, frame({ type: 'state.snapshot', seq: 10, rev: 4 }))
    expect(repaired.state.resyncPending).toBe(false)
    const laterGap = nextOutcome(repaired.state, frame({ type: 'bid.accepted', seq: 12 }))
    expect(laterGap.outcome).toEqual({ kind: 'apply', requestResync: true })
  })
})

describe('the revision rule (D-22, Pitfall 3)', () => {
  it('applies two messages that share one revision', () => {
    const first = nextOutcome(at({ lastSeq: 3, rev: 7 }), frame({ type: 'phase.changed', seq: 4, rev: 7 }))
    expect(first.outcome).toEqual({ kind: 'apply', requestResync: false })

    // This is the case a "rev as the dedupe token" implementation silently
    // discards: the second sibling of revision 7 must still reach the UI.
    const second = nextOutcome(first.state, frame({ type: 'leaderboard', seq: 5, rev: 7 }))
    expect(second.outcome).toEqual({ kind: 'apply', requestResync: false })
    expect(second.state.rev).toBe(7)
    expect(second.state.lastSeq).toBe(5)
  })

  it('never lets rev decide a duplicate or a gap', () => {
    const staleRev = nextOutcome(at({ lastSeq: 4, rev: 9 }), frame({ type: 'price.tick', seq: 5, rev: 4 }))
    expect(staleRev.outcome).toEqual({ kind: 'apply', requestResync: false })
    expect(staleRev.state.rev).toBe(9) // the reconciled revision never regresses
  })
})

describe('the snapshot rule (D-22)', () => {
  it('applies a snapshot at the current revision and resets the stream position', () => {
    const step = nextOutcome(
      at({ lastSeq: 12, rev: 4 }),
      frame({ type: 'state.snapshot', seq: 13, rev: 4 }),
    )
    expect(step.outcome).toEqual({ kind: 'snapshot', apply: true })
    expect(step.state).toEqual({ lastSeq: 13, rev: 4, resyncPending: false })

    // The expectation was reset to the snapshot, so the next event is not
    // mistaken for a gap.
    const next = nextOutcome(step.state, frame({ type: 'phase.changed', seq: 14, rev: 5 }))
    expect(next.outcome).toEqual({ kind: 'apply', requestResync: false })
  })

  it('applies a newer snapshot', () => {
    const step = nextOutcome(
      at({ lastSeq: 3, rev: 4 }),
      frame({ type: 'state.snapshot', seq: 9, rev: 6 }),
    )
    expect(step.outcome).toEqual({ kind: 'snapshot', apply: true })
    expect(step.state.rev).toBe(6)
    expect(step.state.lastSeq).toBe(9)
  })

  it('discards a snapshot older than the reconciled revision', () => {
    const step = nextOutcome(
      at({ lastSeq: 3, rev: 6 }),
      frame({ type: 'state.snapshot', seq: 4, rev: 5 }),
    )
    expect(step.outcome).toEqual({ kind: 'snapshot', apply: false })
    expect(step.state.rev).toBe(6)
  })

  it('does not ask for a resync after a discarded snapshot advanced the stream', () => {
    const discarded = nextOutcome(
      at({ lastSeq: 3, rev: 6 }),
      frame({ type: 'state.snapshot', seq: 8, rev: 5 }),
    )
    const next = nextOutcome(discarded.state, frame({ type: 'phase.changed', seq: 9, rev: 7 }))
    expect(next.outcome).toEqual({ kind: 'apply', requestResync: false })
  })
})

describe('the control-plane exception (D-24)', () => {
  it('applies tunnel.status even when its seq is a duplicate, and never on rev', () => {
    const step = nextOutcome(
      at({ lastSeq: 9, rev: 2 }),
      frame({ type: 'tunnel.status', seq: 4, rev: 1, data: { url: 'https://x.trycloudflare.com', healthy: true } }),
    )
    expect(step.outcome).toEqual({ kind: 'control' })
    expect(step.state.rev).toBe(2)
    expect(step.state.resyncPending).toBe(false)
  })

  it('does not turn a gap in the control plane into a resync request', () => {
    const step = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'tunnel.status', seq: 40 }))
    expect(step.outcome).toEqual({ kind: 'control' })
    expect(step.state.resyncPending).toBe(false)
    // Its stream position still counts, so the next game message is not a gap.
    const next = nextOutcome(step.state, frame({ type: 'phase.changed', seq: 41 }))
    expect(next.outcome).toEqual({ kind: 'apply', requestResync: false })
  })
})

describe('the heartbeat (D-132)', () => {
  it('is applied at any seq, like tunnel.status, and never on rev', () => {
    // The wire literal, not the constant: the gate's addition is the code under
    // test, and a renamed literal must fail here rather than in a quiet room.
    const step = nextOutcome(
      at({ lastSeq: 9, rev: 6 }),
      frame({ type: 'heartbeat', seq: 4, rev: 2, data: { ts: 1_757_934_000_000 } }),
    )
    expect(step.outcome).toEqual({ kind: 'control' })
    expect(step.state.rev).toBe(6)
    expect(step.state.resyncPending).toBe(false)
  })

  it('counts its stream position so the next game frame is not a gap', () => {
    const step = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'heartbeat', seq: 5 }))
    expect(step.outcome).toEqual({ kind: 'control' })
    expect(step.state.lastSeq).toBe(5)
    // A heartbeat never asks for a resync, even when it arrives across a gap:
    // it carries no game state that a snapshot could repair.
    const gapped = nextOutcome(at({ lastSeq: 4 }), frame({ type: 'heartbeat', seq: 40 }))
    expect(gapped.outcome).toEqual({ kind: 'control' })
    expect(gapped.state.resyncPending).toBe(false)
    const next = nextOutcome(gapped.state, frame({ type: 'phase.changed', seq: 41 }))
    expect(next.outcome).toEqual({ kind: 'apply', requestResync: false })
  })

  it('never lets a heartbeat advance the reconciled revision', () => {
    const step = nextOutcome(
      at({ lastSeq: 3, rev: 7 }),
      frame({ type: 'heartbeat', seq: 4, rev: 99 }),
    )
    expect(step.state.rev).toBe(7)
  })
})

describe('the resume decision (D-45)', () => {
  it('reconnects once when a stored token exists and the socket is not open', () => {
    expect(resumeOutcome(SOCKET_CLOSED, true)).toBe('reconnect')
  })

  it('does nothing while the socket is open', () => {
    expect(resumeOutcome(SOCKET_OPEN, true)).toBe('none')
  })

  it('does nothing without a stored token', () => {
    // No token means no identity to restore: the join form is the recovery
    // path, and opening a socket that has nothing to say would be noise.
    expect(resumeOutcome(SOCKET_CLOSED, false)).toBe('none')
  })
})
