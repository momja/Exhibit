package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A small trusted test adapter exercises committed decisions without adding a
// production connector kind, a registry, or any private browser DTO to core.
const testDecisionKind = "test-decision"

func testDecisionCompletion(status string) InteractionCompletion {
	value, _ := json.Marshal(struct {
		Status string `json:"status"`
	}{status})
	state := InteractionCompleted
	if status == "denied" {
		state = InteractionCancelled
	}
	return InteractionCompletion{kind: testDecisionKind, state: state, value: string(value), committed: true}
}

func decisionSession(t *testing.T) (*Session, *questionStdin) {
	t.Helper()
	s, input := newQuestionSession(t)
	s.expireInteractions("")
	require.True(t, s.addInteraction(Interaction{ID: "decision-1", Kind: testDecisionKind, ToolCallID: "decision-tool", Blocking: true}))
	return s, input
}

func receiveResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("operation remained blocked")
		return nil
	}
}

func TestCommittedInteractionReconcilesInsteadOfCachingReply(t *testing.T) {
	s, input := decisionSession(t)
	input.fail = true
	decisions := 0
	decide := func(context.Context, Interaction) (InteractionCompletion, error) {
		decisions++
		return testDecisionCompletion("approved"), nil
	}
	err := s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind, decide)
	require.ErrorIs(t, err, ErrInteractionDeliveryPending)
	pending, _ := s.Interaction("decision-1")
	require.True(t, pending.DeliveryPending)
	require.EqualValues(t, 2, pending.Revision)
	require.True(t, pending.Pending())
	assert.ErrorIs(t, s.ResolveInteraction(context.Background(), "decision-1", 2, testDecisionKind, decide), ErrInteractionDeliveryPending)
	assert.Equal(t, 1, decisions, "a fresh decision cannot repeat a committed side effect")
	assert.ErrorIs(t, s.RedeliverInteraction(context.Background(), "decision-1", 1, testDecisionKind, decide), ErrInteractionStale)
	input.fail = false
	reconciliations := 0
	require.NoError(t, s.RedeliverInteraction(context.Background(), "decision-1", 2, testDecisionKind,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			reconciliations++
			// The account disappeared after approval; reread current domain state.
			return testDecisionCompletion("disconnected"), nil
		}))
	assert.Equal(t, 1, decisions)
	assert.Equal(t, 1, reconciliations)
	assert.Contains(t, input.String(), `\"status\":\"disconnected\"`)
	assert.NotContains(t, input.String(), "approved")
	completed, _ := s.Interaction("decision-1")
	assert.False(t, completed.DeliveryPending)
	assert.Equal(t, InteractionCompleted, completed.State)
	assert.EqualValues(t, 3, completed.Revision)
}

func TestEffectFreeQuestionFailureSendsTheEditedRetry(t *testing.T) {
	s, input := newQuestionSession(t)
	input.fail = true
	first := QuestionAction{Revision: 1, Action: "message", Response: QuestionResponse{Message: "Original draft"}}
	require.Error(t, s.RespondQuestions(context.Background(), "request-1", first))
	pending, _ := s.Interaction("request-1")
	require.False(t, pending.DeliveryPending)
	require.EqualValues(t, 1, pending.Revision)
	assert.Empty(t, pending.Questions.Message)
	assert.Empty(t, pending.Questions.Answers)
	assert.ErrorIs(t, s.RedeliverInteraction(context.Background(), "request-1", 1, InteractionQuestions,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			t.Fatal("question must stay editable")
			return InteractionCompletion{}, nil
		}), ErrInvalidInteractionAction)
	input.fail = false
	first.Response.Message = "Edited draft"
	require.NoError(t, s.RespondQuestions(context.Background(), "request-1", first))
	assert.NotContains(t, input.String(), "Original draft")
	assert.Contains(t, input.String(), "Edited draft")
}

func TestInteractionKindBindingPrecedesDomainWorkAndDelivery(t *testing.T) {
	s, input := newQuestionSession(t)
	called := false
	resolve := func(context.Context, Interaction) (InteractionCompletion, error) {
		called = true
		return testDecisionCompletion("approved"), nil
	}
	assert.ErrorIs(t, s.ResolveInteraction(context.Background(), "request-1", 1, testDecisionKind, resolve), ErrInvalidInteractionAction)
	assert.False(t, called)
	assert.ErrorIs(t, s.ResolveInteraction(context.Background(), "request-1", 1, InteractionQuestions, resolve), ErrInvalidInteractionAction)
	assert.True(t, called)
	assert.Empty(t, input.String(), "a completion constructed for another kind cannot reach Pi")
}

func TestLocalInteractionWorkDoesNotHoldSessionMutexOrSurviveToolEnd(t *testing.T) {
	s, input := decisionSession(t)
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
			func(context.Context, Interaction) (InteractionCompletion, error) {
				close(entered)
				<-release
				return testDecisionCompletion("approved"), nil
			})
	}()
	<-entered
	available := make(chan error, 1)
	go func() {
		snapshot, ok := s.Interaction("decision-1")
		if !ok || !snapshot.Pending() {
			available <- errors.New("missing snapshot")
			return
		}
		s.broadcast([]byte(`{"type":"test_progress"}`))
		s.expireInteractions("decision-tool")
		available <- nil
	}()
	err := receiveResult(t, available)
	close(release)
	require.NoError(t, err, "snapshots, event fanout and tool completion must not wait on local domain work")
	assert.ErrorIs(t, receiveResult(t, result), ErrInteractionClosed)
	assert.Empty(t, input.String())
	snapshot, _ := s.Interaction("decision-1")
	assert.Equal(t, InteractionExpired, snapshot.State)
}

func TestServerDrivenQuestionCompletionUsesTheSameSeam(t *testing.T) {
	s, input := newQuestionSession(t)
	// A verified server path can complete a concrete kind without a browser POST.
	require.NoError(t, s.ResolveInteraction(context.Background(), "request-1", 1, InteractionQuestions,
		func(ctx context.Context, snapshot Interaction) (InteractionCompletion, error) {
			return s.questionCompletion(snapshot, QuestionAction{Revision: 1, Action: "message", Response: QuestionResponse{Message: "Server-delivered answer"}})
		}))
	assert.Contains(t, input.String(), "Server-delivered answer")
	snapshot, _ := s.Interaction("request-1")
	assert.Equal(t, InteractionCompleted, snapshot.State)
}

type blockedInteractionStdin struct {
	entered   chan struct{}
	closed    chan struct{}
	enterOnce sync.Once
	closeOnce sync.Once
}

func newBlockedInteractionStdin() *blockedInteractionStdin {
	return &blockedInteractionStdin{entered: make(chan struct{}), closed: make(chan struct{})}
}
func (w *blockedInteractionStdin) Write([]byte) (int, error) {
	w.enterOnce.Do(func() { close(w.entered) })
	<-w.closed
	return 0, errors.New("sensitive diagnostic must never escape")
}
func (w *blockedInteractionStdin) Close() error {
	w.closeOnce.Do(func() { close(w.closed) })
	return nil
}

func TestBlockedInteractionWriterAllowsSnapshotBusyAndClose(t *testing.T) {
	s, _ := newQuestionSession(t)
	input := newBlockedInteractionStdin()
	s.stdin = input
	s.OwnerID = 1
	s.mgr.sessions = map[string]*Session{s.ID: s}
	result := make(chan error, 1)
	go func() { result <- s.AnswerQuestions("request-1", QuestionResponse{Message: "Draft"}) }()
	<-input.entered
	available := make(chan error, 1)
	go func() {
		_, _ = s.Interaction("request-1")
		available <- s.AnswerQuestions("request-1", QuestionResponse{Message: "Competing draft"})
	}()
	assert.ErrorIs(t, receiveResult(t, available), ErrInteractionBusy)
	closed := make(chan error, 1)
	go func() { s.mgr.Close(1, s.ID); closed <- nil }()
	require.NoError(t, receiveResult(t, closed))
	err := receiveResult(t, result)
	assert.ErrorIs(t, err, ErrInteractionClosed)
	assert.NotContains(t, err.Error(), "sensitive")
}

func TestBlockedInteractionWriterCannotStrandAbort(t *testing.T) {
	s, _ := newQuestionSession(t)
	input := newBlockedInteractionStdin()
	s.stdin = input
	s.pending = make(map[string]chan rpcResponse)
	s.done = make(chan struct{})
	result := make(chan error, 1)
	go func() { result <- s.AnswerQuestions("request-1", QuestionResponse{Message: "Draft"}) }()
	<-input.entered
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	aborted := make(chan error, 1)
	go func() { aborted <- s.Abort(ctx) }()
	require.Error(t, receiveResult(t, aborted))
	assert.ErrorIs(t, receiveResult(t, result), ErrInteractionClosed)
	snapshot, _ := s.Interaction("request-1")
	assert.Equal(t, InteractionExpired, snapshot.State)
	assert.False(t, snapshot.DeliveryPending)
	s.mu.Lock()
	stopping := s.stopping
	s.mu.Unlock()
	assert.True(t, stopping, "unresponsive delivery makes Stop force-close the session")
}

func TestToolEndDuringWriteCannotBeOverwritten(t *testing.T) {
	s, _ := newQuestionSession(t)
	input := newBlockedInteractionStdin()
	s.stdin = input
	result := make(chan error, 1)
	go func() { result <- s.AnswerQuestions("request-1", QuestionResponse{Message: "Late reply"}) }()
	<-input.entered
	s.expireInteractions("tool-1")
	require.NoError(t, input.Close())
	assert.ErrorIs(t, receiveResult(t, result), ErrInteractionClosed)
	snapshot, _ := s.Interaction("request-1")
	assert.Equal(t, InteractionExpired, snapshot.State)
	assert.Empty(t, snapshot.Questions.Message)
}

type interactionCommandStdin struct{ commands chan []byte }

func (w *interactionCommandStdin) Close() error { return nil }
func (w *interactionCommandStdin) Write(p []byte) (int, error) {
	w.commands <- append([]byte(nil), p...)
	return len(p), nil
}

func TestAbortFencesWaitersBeforeAndAfterAcknowledgement(t *testing.T) {
	s, _ := newQuestionSession(t)
	input := &interactionCommandStdin{commands: make(chan []byte, 8)}
	s.stdin = input
	s.pending = make(map[string]chan rpcResponse)
	s.done = make(chan struct{})
	result := make(chan error, 1)
	go func() { result <- s.Abort(context.Background()) }()
	line := <-input.commands
	assert.ErrorIs(t, s.AnswerQuestions("request-1", QuestionResponse{Message: "After Stop"}), ErrInteractionClosed)
	assert.False(t, s.addInteraction(Interaction{ID: "late", Kind: testDecisionKind}))
	var command struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(line, &command))
	assert.Equal(t, "abort", command.Type)
	response, _ := json.Marshal(struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}{"response", command.ID})
	s.handleLine(response)
	require.NoError(t, receiveResult(t, result))
	assert.False(t, s.addInteraction(Interaction{ID: "late-after-ack", Kind: testDecisionKind}))
}

func TestInteractionSnapshotCannotMutateStoredQuestionData(t *testing.T) {
	s, _ := newQuestionSession(t)
	snapshot, _ := s.Interaction("request-1")
	snapshot.Questions.Questions[0].Options[0].Label = "Mutated"
	again, _ := s.Interaction("request-1")
	assert.NotEqual(t, "Mutated", again.Questions.Questions[0].Options[0].Label)
}

func TestNoIntermediateDeliveryPendingEventDuringPost(t *testing.T) {
	s, _ := decisionSession(t)
	input := newBlockedInteractionStdin()
	s.stdin = input
	result := make(chan error, 1)
	go func() {
		result <- s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
			func(context.Context, Interaction) (InteractionCompletion, error) {
				return testDecisionCompletion("approved"), nil
			})
	}()
	<-input.entered
	s.mu.Lock()
	for _, event := range s.backlog {
		assert.False(t, strings.Contains(string(event), `"deliveryPending":true`))
	}
	s.mu.Unlock()
	require.NoError(t, input.Close())
	assert.ErrorIs(t, receiveResult(t, result), ErrInteractionDeliveryPending)
}

func TestDecisionAlreadyMadeElsewhereCanCompleteAnotherWaiter(t *testing.T) {
	s, input := decisionSession(t)
	assert.ErrorIs(t, s.RedeliverInteraction(context.Background(), "decision-1", 1, testDecisionKind,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			t.Fatal("not this waiter's delivery retry")
			return InteractionCompletion{}, nil
		}), ErrInvalidInteractionAction)
	reads := 0
	require.NoError(t, s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			reads++ // Reconcile an existing terminal domain decision without mutation.
			return testDecisionCompletion("approved"), nil
		}))
	assert.Equal(t, 1, reads)
	assert.Contains(t, input.String(), `\"status\":\"approved\"`)
}

func TestDecisionFirstFinishesLocalWorkBeforeAbortFence(t *testing.T) {
	s, _ := decisionSession(t)
	input := &interactionCommandStdin{commands: make(chan []byte, 8)}
	s.stdin = input
	s.pending = make(map[string]chan rpcResponse)
	s.done = make(chan struct{})
	entered, release := make(chan struct{}), make(chan struct{})
	resolved := make(chan error, 1)
	go func() {
		resolved <- s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
			func(context.Context, Interaction) (InteractionCompletion, error) {
				close(entered)
				<-release
				return testDecisionCompletion("approved"), nil
			})
	}()
	<-entered
	aborted := make(chan error, 1)
	go func() { aborted <- s.Abort(context.Background()) }()
	// The domain decision owns first admission. Stop may fence delivery after
	// it returns, but cannot claim to have prevented the local decision.
	s.mu.Lock()
	assert.False(t, s.aborting)
	s.mu.Unlock()
	close(release)
	for {
		select {
		case line := <-input.commands:
			var command struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}
			require.NoError(t, json.Unmarshal(line, &command))
			if command.Type != "abort" {
				continue
			}
			response, _ := json.Marshal(struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}{"response", command.ID})
			s.handleLine(response)
			require.NoError(t, receiveResult(t, aborted))
			err := receiveResult(t, resolved)
			assert.True(t, err == nil || errors.Is(err, ErrInteractionClosed))
			return
		case <-time.After(2 * time.Second):
			t.Fatal("Stop did not follow the admitted local decision")
		}
	}
}

func TestInteractionAdmissionHonorsCanceledContext(t *testing.T) {
	s, _ := newQuestionSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := s.ResolveInteraction(ctx, "request-1", 1, InteractionQuestions,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			called = true
			return InteractionCompletion{}, nil
		})
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, called)
}

func TestCommittedDeliveryCompetingVisibleRevisionIsBusy(t *testing.T) {
	s, _ := decisionSession(t)
	input := newBlockedInteractionStdin()
	s.stdin = input
	result := make(chan error, 1)
	go func() {
		result <- s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
			func(context.Context, Interaction) (InteractionCompletion, error) {
				return testDecisionCompletion("approved"), nil
			})
	}()
	<-input.entered
	pending, _ := s.Interaction("decision-1")
	require.EqualValues(t, 2, pending.Revision, "the browser's still-visible revision remains 1")
	err := s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			t.Fatal("a duplicate during delivery must not repeat the decision")
			return InteractionCompletion{}, nil
		})
	assert.ErrorIs(t, err, ErrInteractionBusy)
	assert.NotErrorIs(t, err, ErrInteractionStale)
	require.NoError(t, input.Close())
	assert.ErrorIs(t, receiveResult(t, result), ErrInteractionDeliveryPending)
	// Once the active attempt has ended, that old revision is genuinely stale.
	assert.ErrorIs(t, s.ResolveInteraction(context.Background(), "decision-1", 1, testDecisionKind,
		func(context.Context, Interaction) (InteractionCompletion, error) {
			t.Fatal("stale decision")
			return InteractionCompletion{}, nil
		}), ErrInteractionStale)
}

func TestCancellationAfterPreparationDistinguishesEffectFreeAndCommitted(t *testing.T) {
	t.Run("question remains editable", func(t *testing.T) {
		s, input := newQuestionSession(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := s.ResolveInteraction(ctx, "request-1", 1, InteractionQuestions,
			func(_ context.Context, snapshot Interaction) (InteractionCompletion, error) {
				completion, err := s.questionCompletion(snapshot, QuestionAction{Revision: 1, Action: "message", Response: QuestionResponse{Message: "Canceled draft"}})
				cancel() // Deterministically exercises the sole post-decision check.
				return completion, err
			})
		assert.ErrorIs(t, err, context.Canceled)
		assert.NotErrorIs(t, err, ErrInteractionDeliveryPending)
		pending, _ := s.Interaction("request-1")
		assert.True(t, pending.Pending())
		assert.False(t, pending.DeliveryPending)
		assert.EqualValues(t, 1, pending.Revision)
		assert.Empty(t, pending.Questions.Message)
		assert.Empty(t, input.String())
	})
	t.Run("committed decision awaits reconciliation", func(t *testing.T) {
		s, input := decisionSession(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := s.ResolveInteraction(ctx, "decision-1", 1, testDecisionKind,
			func(context.Context, Interaction) (InteractionCompletion, error) {
				cancel()
				return testDecisionCompletion("approved"), nil
			})
		assert.ErrorIs(t, err, ErrInteractionDeliveryPending)
		pending, _ := s.Interaction("decision-1")
		assert.True(t, pending.DeliveryPending)
		assert.EqualValues(t, 2, pending.Revision)
		assert.Empty(t, input.String())
	})
}
