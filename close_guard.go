package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type closeGuardAction uint8

const (
	closeAttempt closeGuardAction = iota
	closeConfirmed
	closeCancelled
)

// closeGuardState is kept separate from App so each state transition can be
// verified without constructing a Wails runtime. App.mu protects the live
// instance of this state.
type closeGuardState struct {
	requestPending bool
	bypassNext     bool
}

type closeGuardEffect struct {
	preventClose     bool
	emitCloseRequest bool
	quit             bool
}

func transitionCloseGuard(state closeGuardState, action closeGuardAction) (closeGuardState, closeGuardEffect) {
	switch action {
	case closeAttempt:
		if state.bypassNext {
			state.bypassNext = false
			return state, closeGuardEffect{}
		}
		if state.requestPending {
			return state, closeGuardEffect{preventClose: true}
		}
		state.requestPending = true
		return state, closeGuardEffect{preventClose: true, emitCloseRequest: true}
	case closeConfirmed:
		if !state.requestPending {
			return state, closeGuardEffect{}
		}
		state.requestPending = false
		state.bypassNext = true
		return state, closeGuardEffect{quit: true}
	case closeCancelled:
		// Cancellation only applies to an unanswered request. If confirmation
		// already won a race, its one-shot bypass must remain in place so the
		// runtime.Quit call is not intercepted again.
		if state.requestPending {
			state.requestPending = false
		}
		return state, closeGuardEffect{}
	default:
		return state, closeGuardEffect{}
	}
}

func (a *App) applyCloseGuardAction(action closeGuardAction) closeGuardEffect {
	a.mu.Lock()
	next, effect := transitionCloseGuard(a.closeGuard, action)
	a.closeGuard = next
	a.mu.Unlock()
	return effect
}

func (a *App) beforeClose(ctx context.Context) bool {
	effect := a.applyCloseGuardAction(closeAttempt)
	if effect.emitCloseRequest && ctx != nil {
		runtime.EventsEmit(ctx, closeRequestEvent)
	}
	return effect.preventClose
}

// ConfirmQuit is called by the frontend after the user has resolved any
// unsaved changes. It arms a one-shot bypass before asking Wails to quit so
// the resulting native close callback is allowed through.
func (a *App) ConfirmQuit() {
	effect := a.applyCloseGuardAction(closeConfirmed)
	if !effect.quit {
		return
	}
	if ctx := a.currentContext(); ctx != nil {
		runtime.Quit(ctx)
	}
}

// CancelQuitRequest clears an unanswered native close request. A later close
// attempt can therefore ask the frontend again.
func (a *App) CancelQuitRequest() {
	a.applyCloseGuardAction(closeCancelled)
}
