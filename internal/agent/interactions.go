package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Interaction is a server-owned live waiter. Kind adapters own its domain data
// and safe model projection; the common module owns admission and delivery.
// Questions is the compatibility view for the first concrete adapter.
type Interaction struct {
	ID              string           `json:"id"`
	ToolCallID      string           `json:"toolCallId"`
	Kind            string           `json:"kind"`
	State           string           `json:"state"`
	Revision        uint64           `json:"revision"`
	Blocking        bool             `json:"blocking"`
	DeliveryPending bool             `json:"deliveryPending"`
	Questions       *QuestionRequest `json:"questions,omitempty"`
	resolving       bool
	claim           uint64
}

const (
	InteractionQuestions = "questions"
	InteractionPending   = "pending"
	InteractionCompleted = "completed"
	InteractionCancelled = "cancelled"
	InteractionExpired   = "expired"
	InteractionBlocked   = "blocked"
)

var (
	ErrInteractionBusy            = errors.New("a response is already in progress; please wait")
	ErrInteractionStale           = errors.New("this request has changed; refresh it before responding")
	ErrInteractionClosed          = errors.New("this request is no longer waiting for a response")
	ErrInteractionDeliveryPending = errors.New("your decision was saved; refresh the request to send its result")
	ErrInteractionStopping        = errors.New("the current request is stopping; please try again when it finishes")
	ErrInvalidInteractionAction   = errors.New("invalid interaction action")
	// Compatibility for the question-only interface. Stale and closed are now
	// distinct on the common, revision-checked interface.
	ErrInteractionExpired        = ErrInteractionClosed
	errInteractionDeliveryFailed = errors.New("could not send your response; please try again")
)

func (interaction Interaction) Pending() bool { return interaction.State == InteractionPending }

// InteractionCompletion is constructed only by trusted adapters in this
// package. HTTP callers cannot initialize a reply from a private request DTO.
// value is an explicitly marshaled, model-safe projection; it is never cached.
// committed means the domain decision must survive a failed delivery. Such a
// failure enables only RedeliverInteraction, which must reread the domain result.
// Effect-free questions use committed=false and remain editable on failure.
type InteractionCompletion struct {
	kind      string
	state     string
	value     string
	cancelled bool
	committed bool
	questions *QuestionRequest
}

// InteractionResolver must honor ctx and do only bounded local work. It may
// capture a private HTTP input for the duration of that call, but neither that
// input nor the resolver is retained or logged by the session. Completion is a
// separate explicit safe projection. Reconciliation must only read a previously
// decided domain record; it cannot make a new decision.
type InteractionResolver func(context.Context, Interaction) (InteractionCompletion, error)

func (s *Session) Interaction(id string) (Interaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	interaction, ok := s.interactions[id]
	return interactionSnapshot(interaction), ok
}

func interactionSnapshot(interaction Interaction) Interaction {
	if interaction.Questions != nil {
		questions := cloneQuestionRequest(interaction.Questions)
		questions.Pending = interaction.Pending()
		questions.Cancelled = interaction.State == InteractionCancelled || interaction.State == InteractionExpired || interaction.State == InteractionBlocked
		questions.Expired = interaction.State == InteractionExpired
		questions.Blocked = interaction.State == InteractionBlocked
		interaction.Questions = questions
	}
	return interaction
}

// Caller holds mu. Only blocking pending interactions suspend ordinary prompts.
func (s *Session) hasPendingInteractions() bool {
	for _, interaction := range s.interactions {
		if interaction.Blocking && interaction.Pending() {
			return true
		}
	}
	return false
}

func (s *Session) addInteraction(interaction Interaction) bool {
	s.mu.Lock()
	if s.interactions == nil {
		s.interactions = map[string]Interaction{}
	}
	_, duplicate := s.interactions[interaction.ID]
	if s.closed || s.stopping || s.aborting || s.interactionsStopped || duplicate || len(s.interactions) >= 128 {
		s.mu.Unlock()
		return false
	}
	interaction.State, interaction.Revision, interaction.DeliveryPending = InteractionPending, 1, false
	interaction.Questions = cloneQuestionRequest(interaction.Questions)
	s.interactions[interaction.ID] = interaction
	s.lastActive = time.Now()
	s.mu.Unlock()
	if interaction.Blocking {
		s.clearTurnClock()
	}
	s.interactionChanged(interaction.ID)
	return true
}

// ResolveInteraction admits one new decision. A separate decision gate orders
// it against competing actions and Abort; mu is never held across local work or
// stdin I/O. Close and tool termination do not wait for the decision gate.
func (s *Session) ResolveInteraction(ctx context.Context, id string, revision uint64, expectedKind string, decide InteractionResolver) error {
	return s.resolveInteraction(ctx, id, revision, expectedKind, false, decide)
}

// RedeliverInteraction only reconciles a committed, undelivered decision. It
// always invokes reconcile again, so a removed connection can become
// disconnected instead of replaying a cached approved result.
func (s *Session) RedeliverInteraction(ctx context.Context, id string, revision uint64, expectedKind string, reconcile InteractionResolver) error {
	return s.resolveInteraction(ctx, id, revision, expectedKind, true, reconcile)
}

func (s *Session) resolveInteraction(ctx context.Context, id string, revision uint64, expectedKind string, redeliver bool, resolve InteractionResolver) error {
	if revision == 0 || expectedKind == "" || resolve == nil {
		return ErrInvalidInteractionAction
	}
	// A shorter caller deadline still wins. Adapters must honor cancellation;
	// no goroutine may outlive admission to commit a delayed decision.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	release, err := s.acquireDecision(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	s.mu.Lock()
	interaction, err := s.currentInteractionLocked(id, revision, expectedKind)
	if interaction.resolving && (err == nil || errors.Is(err, ErrInteractionStale)) {
		err = ErrInteractionBusy
	}
	if err == nil && interaction.DeliveryPending != redeliver {
		if interaction.DeliveryPending {
			err = ErrInteractionDeliveryPending
		} else {
			err = ErrInvalidInteractionAction
		}
	}
	if err == nil {
		interaction.claim++
		interaction.resolving = true
		s.interactions[id] = interaction
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	claim := interaction.claim
	defer func() {
		s.mu.Lock()
		current := s.interactions[id]
		if current.claim == claim {
			current.resolving = false
			s.interactions[id] = current
		}
		s.mu.Unlock()
	}()

	completion, err := resolve(ctx, interactionSnapshot(interaction))
	if err != nil {
		return err // The owning HTTP adapter maps allowlisted domain errors.
	}
	if completion.kind != expectedKind || (completion.state != InteractionCompleted && completion.state != InteractionCancelled) || completion.value == "" || (redeliver && !completion.committed) {
		return ErrInvalidInteractionAction
	}
	if completion.committed && !redeliver {
		s.mu.Lock()
		interaction, err = s.currentInteractionLocked(id, revision, expectedKind)
		if err == nil {
			interaction.DeliveryPending = true
			interaction.Revision++
			revision = interaction.Revision
			s.interactions[id] = interaction
		}
		s.mu.Unlock()
		if err != nil {
			return err
		}
	}
	// Admission ends after bounded local work. Delivery has its own per-waiter
	// claim; Stop never waits on the decision gate for a blocked pipe.
	release()
	release = nil
	if contextErr := ctx.Err(); contextErr != nil {
		if !completion.committed {
			return contextErr
		}
		s.interactionChanged(id)
		return ErrInteractionDeliveryPending
	}

	line, err := json.Marshal(struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Value     string `json:"value"`
		Cancelled bool   `json:"cancelled"`
	}{"extension_ui_response", id, completion.value, completion.cancelled})
	if err != nil {
		return ErrInvalidInteractionAction
	}
	err = s.writePi(ctx, append(line, '\n'), func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		_, err := s.currentInteractionLocked(id, revision, expectedKind)
		return err
	})

	if err == nil {
		s.mu.Lock()
		interaction, err = s.currentInteractionLocked(id, revision, expectedKind)
		if err == nil {
			interaction.State = completion.state
			interaction.DeliveryPending = false
			interaction.resolving = false
			interaction.Revision++
			if completion.questions != nil {
				interaction.Questions = cloneQuestionRequest(completion.questions)
			}
			s.interactions[id] = interaction
			s.lastActive = time.Now()
		}
		s.mu.Unlock()
	}
	if err != nil {
		s.mu.Lock()
		_, fenceErr := s.currentInteractionLocked(id, revision, expectedKind)
		s.mu.Unlock()
		if fenceErr != nil {
			return fenceErr
		}
		if completion.committed {
			s.interactionChanged(id) // Publish recovery only after delivery failed.
			return ErrInteractionDeliveryPending
		}
		return err // writePi returns only safe sentinels/context errors.
	}
	s.armTurnClock() // It checks all remaining blocking waiters under mu.
	s.interactionChanged(id)
	return nil
}

// Caller holds mu. Revision checks are also a cancellation fence after I/O:
// tool termination or Close cannot be overwritten by a late completion.
func (s *Session) currentInteractionLocked(id string, revision uint64, expectedKind string) (Interaction, error) {
	interaction, ok := s.interactions[id]
	if !ok || !interaction.Pending() || s.closed || s.stopping || s.interactionsStopped {
		return Interaction{}, ErrInteractionClosed
	}
	if s.aborting {
		return Interaction{}, ErrInteractionStopping
	}
	if interaction.Kind != expectedKind {
		return Interaction{}, ErrInvalidInteractionAction
	}
	if interaction.Revision != revision {
		// Admission still needs the active claim to distinguish a duplicate
		// carrying the browser's pre-decision revision from a stale draft.
		return interaction, ErrInteractionStale
	}
	return interaction, nil
}

// acquireDecision serializes bounded local work, with cancellable admission.
// The gate is initialized under mu to support both managed and test sessions.
func (s *Session) acquireDecision(ctx context.Context) (func(), error) {
	s.mu.Lock()
	if s.decisionGate == nil {
		s.decisionGate = make(chan struct{}, 1)
	}
	gate := s.decisionGate
	s.mu.Unlock()
	select {
	case gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate
			return nil, err
		}
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// writePi is the only context-bound RPC/interaction writer. A deadline closes
// the session independently of either gate, interrupting a blocked Write and
// releasing writeMu. Partial/ambiguous writes also close the pipe; only a clean
// zero-byte failure leaves an effect-free question editable.
func (s *Session) writePi(ctx context.Context, line []byte, admit func() error) error {
	stopKill := context.AfterFunc(ctx, s.kill)
	defer stopKill()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if admit != nil {
		if err := admit(); err != nil {
			return err
		}
	}
	n, err := s.stdin.Write(line)
	if err != nil || n != len(line) {
		if n != 0 {
			s.kill()
			return ErrInteractionClosed
		}
		return errInteractionDeliveryFailed
	}
	return nil
}

// expireInteractions retires waiters independently of the decision gate. A
// late local callback or pipe write cannot revive a stopped/completed tool.
func (s *Session) expireInteractions(toolCallID string) {
	s.mu.Lock()
	var changed []string
	for id, interaction := range s.interactions {
		if interaction.Pending() && (toolCallID == "" || interaction.ToolCallID == toolCallID) {
			interaction.State = InteractionExpired
			interaction.DeliveryPending = false
			interaction.resolving = false
			interaction.Revision++
			s.interactions[id] = interaction
			changed = append(changed, id)
		}
	}
	s.mu.Unlock()
	for _, id := range changed {
		s.interactionChanged(id)
	}
}

func (s *Session) interactionChanged(id string) {
	interaction, ok := s.Interaction(id)
	if !ok {
		return
	}
	event, _ := json.Marshal(map[string]any{
		"type": "exhibit_interaction", "requestId": id, "kind": interaction.Kind,
		"state": interaction.State, "revision": interaction.Revision,
		"blocking": interaction.Blocking, "pending": interaction.Pending(),
		"deliveryPending": interaction.DeliveryPending,
	})
	s.broadcast(event)
}
