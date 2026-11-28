# Live interaction decision and completion contract

The shared module owns live waiter identity, kind/revision binding, admission,
Stop/Close fencing and delivery. Trusted kind adapters own validation, domain
operations and explicit model-safe reply construction. There is no registry,
private action DTO, generic serialized payload or workflow table in this module.

## Methods and types

```go
type InteractionResolver func(context.Context, Interaction) (InteractionCompletion, error)

func (*Session) ResolveInteraction(context.Context, string, uint64, string, InteractionResolver) error
func (*Session) RedeliverInteraction(context.Context, string, uint64, string, InteractionResolver) error
func (*Session) RespondQuestions(context.Context, string, QuestionAction) error

type QuestionAction struct {
    Revision uint64
    Action   string
    Response QuestionResponse
}
```

The string arguments are interaction ID and expected kind. Revision zero is
invalid. A callback receives an isolated snapshot, including any adapter-owned
reference fields (the connector leaf's `ProposalID` is copied unchanged).

`InteractionCompletion` has private fields and can only be constructed by
trusted code inside `internal/agent`. Its fields are `kind`, `state`, `value`
(an explicit JSON projection encoded as a string), `cancelled`, `committed`,
and the question adapter's optional `questions` summary. An HTTP package cannot
initialize these fields from its private request. The common module checks
completion kind against both the stored waiter and expected kind before delivery.

A connector adapter constructor such as `APIConnectorCompletion(state)` must
whitelist terminal domain outcomes and explicitly marshal a status-only struct.
It sets `committed=true`, its own kind, and completed/cancelled UI state. A denied
domain result can use cancelled UI state while leaving protocol `cancelled=false`
so Pi receives the explicit safe status. It must reject a pending domain result.
The foundation does not define this constructor or implement a connector kind.

## Effect-free questions

`RespondQuestions` validates the submitted `QuestionAction` and prepares a safe
reply with `committed=false`. A clean zero-byte write failure does not cache
answers or change the revision. The browser may edit its draft and submit again;
the new submission is freshly validated and sent, never an older cached answer.
A partial/ambiguous write closes the session instead: its stream cannot safely
be reused, and it cannot be offered as a retryable editable waiter.

`AnswerQuestions` remains the legacy unversioned wrapper, but claims the revision
it observes through the same common seam. `RespondInteraction` and
`InteractionAction` have been removed; their question-only replacement is above.

## Committed domain decisions

A successful local domain decision returns a completion with `committed=true`.
The live interaction records `DeliveryPending=true` and increments its revision
before attempting delivery. No intermediate SSE is emitted while the original
POST is in flight. Successful delivery clears the flag, settles the waiter and
increments the revision again. Failed delivery publishes the pending flag and
revision, and returns `ErrInteractionDeliveryPending` without raw pipe details.
No reply value, callback or private input is cached for retry.

Only `RedeliverInteraction` may retry a waiter with `DeliveryPending=true`. Its
callback must reread an existing terminal domain decision, never approve, deny,
collect another credential or repeat a domain mutation. A removed account can
therefore project disconnected on retry. Multiple waiters may reference one
domain record: read-only reconciliation may use `ResolveInteraction` when that
particular waiter's flag is false, or `RedeliverInteraction` when true. Both paths
retain kind and revision checks. A pending domain result must be rejected by the
adapter, regardless of which method is used.

Domain decision durability is separate from the lifetime of the live Pi waiter.
After Stop, tool end or Close, a late callback cannot overwrite expiration or
revive delivery. A committed domain effect is not rolled back by closing a
waiter. Recovery in a new session starts from the authoritative domain record;
it never resurrects an old Pi RPC. This promises idempotent domain decisions,
not exactly-once delivery across a broken process transport.

## Ordering, locks and deadlines

A cancellable decision gate orders bounded local callbacks against Stop. The
first admitted local decision may commit before Stop; Stop cannot claim to
undo it. Callbacks receive a context capped at 30 seconds (shorter caller
deadlines win) and must honor cancellation. Do not do provider authentication or
other unbounded remote work in a callback.

The gate is released before pipe writes. A private per-waiter claim prevents
competing submissions during delivery; they receive `ErrInteractionBusy`.
`Session.mu` never spans callback, filesystem or pipe work. Snapshots and event
fanout continue during local decisions and writes. Cancellation/revision checks
before and after delivery prevent late writes from overwriting a retired waiter.

Stop marks `aborting`, closes waiter admission and expires pending waiters before
sending the abort RPC. New dialogs remain refused after the acknowledgement;
only an explicit new prompt reopens admission. Prompt writes compare the Stop
epoch, so a prompt prepared before a concurrent Stop cannot reopen that epoch.

A writer deadline force-closes the session without acquiring either gate.
Stop itself has a five-second ceiling, including waiting for the local decision
gate, pipe and acknowledgement. An unresponsive Pi can therefore lose its live
session rather than strand Stop. Close also bypasses both gates and closes stdin
to interrupt a blocked write. Trusted callbacks must honor their own context;
there is no detached callback goroutine that may commit later in the background.

## Error contract and migration

- `ErrInteractionStale`: reviewed revision changed; reconcile canonical state.
- `ErrInteractionClosed`: missing, expired, settled or stopped waiter.
- `ErrInteractionDeliveryPending`: decision saved; canonical refresh and explicit
  read-only reconciliation/redelivery are required.
- `ErrInteractionStopping`: Stop in progress / prompt crossed a Stop epoch.
- `ErrInteractionBusy`: another response is in flight; preserve the draft.
- `ErrInvalidInteractionAction`: wrong kind, zero revision, incompatible action,
  invalid completion, or redelivery requested before this waiter has a decision.

`ErrInteractionExpired` remains an alias for closed, not stale. Domain errors
return to their owning HTTP adapter for allowlisted mapping. Transport errors
are safe sentinels, never raw pipe diagnostics. The common module logs no input.

The connector leaf should remove its duplicate `abortMu`, `aborting`, Abort
implementation and `ResolveAPIConnector`; retain the foundation's fencing.
Replace the old question action call with `RespondQuestions(ctx, id,
QuestionAction{Revision: rev, Action: action, Response: response})`. Keep private
connector decoding and machine-readable browser error/reconciliation policy in
the leaf. No browser or private connector HTTP behavior is changed here.
