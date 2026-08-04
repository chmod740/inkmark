package main

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestCloseGuardRequiresConfirmationAndUsesOneShotBypass(t *testing.T) {
	state := closeGuardState{}

	state, first := transitionCloseGuard(state, closeAttempt)
	if !first.preventClose || !first.emitCloseRequest || first.quit {
		t.Fatalf("unexpected first close effect: %#v", first)
	}
	if !state.requestPending || state.bypassNext {
		t.Fatalf("unexpected pending state: %#v", state)
	}

	state, repeated := transitionCloseGuard(state, closeAttempt)
	if !repeated.preventClose || repeated.emitCloseRequest || repeated.quit {
		t.Fatalf("pending close must be prevented without another event: %#v", repeated)
	}

	state, confirmed := transitionCloseGuard(state, closeConfirmed)
	if !confirmed.quit || confirmed.preventClose || confirmed.emitCloseRequest {
		t.Fatalf("unexpected confirmation effect: %#v", confirmed)
	}
	if state.requestPending || !state.bypassNext {
		t.Fatalf("confirmation must arm a one-shot bypass: %#v", state)
	}

	state, allowed := transitionCloseGuard(state, closeAttempt)
	if allowed.preventClose || allowed.emitCloseRequest || allowed.quit {
		t.Fatalf("confirmed close must be allowed: %#v", allowed)
	}
	if state != (closeGuardState{}) {
		t.Fatalf("bypass must be consumed after one close: %#v", state)
	}

	_, later := transitionCloseGuard(state, closeAttempt)
	if !later.preventClose || !later.emitCloseRequest {
		t.Fatalf("a later close must start a new request: %#v", later)
	}
}

func TestCloseGuardCancellationAllowsRetry(t *testing.T) {
	state, _ := transitionCloseGuard(closeGuardState{}, closeAttempt)
	state, cancelled := transitionCloseGuard(state, closeCancelled)
	if cancelled != (closeGuardEffect{}) || state != (closeGuardState{}) {
		t.Fatalf("unexpected cancellation result: state=%#v effect=%#v", state, cancelled)
	}

	_, retried := transitionCloseGuard(state, closeAttempt)
	if !retried.preventClose || !retried.emitCloseRequest {
		t.Fatalf("close after cancellation must request confirmation again: %#v", retried)
	}
}

func TestCloseGuardIgnoresStaleConfirmationAndCancellation(t *testing.T) {
	state := closeGuardState{}
	state, confirmed := transitionCloseGuard(state, closeConfirmed)
	if confirmed != (closeGuardEffect{}) || state != (closeGuardState{}) {
		t.Fatalf("confirmation without a pending request must be ignored: state=%#v effect=%#v", state, confirmed)
	}

	bypassing := closeGuardState{bypassNext: true}
	state, cancelled := transitionCloseGuard(bypassing, closeCancelled)
	if cancelled != (closeGuardEffect{}) || state != bypassing {
		t.Fatalf("late cancellation must not revoke a confirmed bypass: state=%#v effect=%#v", state, cancelled)
	}
}

func TestCloseGuardEmitsOnlyOnceForConcurrentAttempts(t *testing.T) {
	app := &App{}
	const attempts = 64
	var emitted atomic.Int32
	var allowed atomic.Int32
	var wait sync.WaitGroup
	wait.Add(attempts)

	for range attempts {
		go func() {
			defer wait.Done()
			effect := app.applyCloseGuardAction(closeAttempt)
			if effect.emitCloseRequest {
				emitted.Add(1)
			}
			if !effect.preventClose {
				allowed.Add(1)
			}
		}()
	}
	wait.Wait()

	if got := emitted.Load(); got != 1 {
		t.Fatalf("expected one close request event, got %d", got)
	}
	if got := allowed.Load(); got != 0 {
		t.Fatalf("all unconfirmed close attempts must be prevented, got %d allowed", got)
	}

	app.CancelQuitRequest()
	retried := app.applyCloseGuardAction(closeAttempt)
	if !retried.preventClose || !retried.emitCloseRequest {
		t.Fatalf("cancelled request must be retryable: %#v", retried)
	}
}
