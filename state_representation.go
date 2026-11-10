// Copyright 2026 hushink
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package stateless

import "context"

// entryActionBehavior wraps an entry action together with its trigger condition.
// When fromEvent is nil, the action runs whenever the state is entered by any
// event; when fromEvent is non-nil, it runs only when the state is entered
// because of that specific event (OnEntryFrom semantics).
type entryActionBehavior[S, E comparable] struct {
	action    ActionFunc[S, E] // Entry action
	fromEvent *E               // If non-nil, run only when triggered by this event
}

// stateRepresentation stores the complete configuration of a single state,
// including trigger behaviors for every event, entry/exit actions, and hierarchy
// relationships. It is unexported and used internally by the Builder and the
// state machine engine.
type stateRepresentation[S, E comparable] struct {
	state                   S                             // State value
	triggerBehaviors        map[E][]triggerBehavior[S, E] // Trigger behaviors indexed by event
	triggerOrder            []E                           // Order in which events were first registered (deterministic iteration)
	entryActions            []entryActionBehavior[S, E]   // Entry actions
	exitActions             []ActionFunc[S, E]            // Exit actions
	superstate              *stateRepresentation[S, E]    // Parent state (hierarchy)
	substates               []*stateRepresentation[S, E]  // Child states
	initialTransitionTarget *S                            // Initial substate target (auto-transition when a composite state is entered)
}

// newStateRepresentation creates the representation for the given state.
func newStateRepresentation[S, E comparable](state S) *stateRepresentation[S, E] {
	return &stateRepresentation[S, E]{
		state:            state,
		triggerBehaviors: make(map[E][]triggerBehavior[S, E]),
	}
}

// addTriggerBehavior adds a trigger behavior for the given event.
// The same event may have multiple behaviors (for example, branches with
// different guard conditions).
func (sr *stateRepresentation[S, E]) addTriggerBehavior(event E, behavior triggerBehavior[S, E]) {
	if _, exists := sr.triggerBehaviors[event]; !exists {
		sr.triggerOrder = append(sr.triggerOrder, event)
	}
	sr.triggerBehaviors[event] = append(sr.triggerBehaviors[event], behavior)
}

// addEntryAction adds a generic entry action that runs whenever this state is
// entered by any event.
func (sr *stateRepresentation[S, E]) addEntryAction(action ActionFunc[S, E]) {
	sr.entryActions = append(sr.entryActions, entryActionBehavior[S, E]{
		action: action,
	})
}

// addEntryActionFrom adds an entry action that runs only when the state is
// entered because of the specified event (OnEntryFrom semantics).
func (sr *stateRepresentation[S, E]) addEntryActionFrom(event E, action ActionFunc[S, E]) {
	sr.entryActions = append(sr.entryActions, entryActionBehavior[S, E]{
		action:    action,
		fromEvent: &event,
	})
}

// addExitAction adds an exit action that runs when this state is left.
func (sr *stateRepresentation[S, E]) addExitAction(action ActionFunc[S, E]) {
	sr.exitActions = append(sr.exitActions, action)
}

// canHandle reports whether this state (including hierarchical parent states) can
// handle the given event. source is the caller's actual current state, which may
// be deeper in the hierarchy; it is passed through unchanged so guards inherited
// from a parent see the correct source state.
func (sr *stateRepresentation[S, E]) canHandle(ctx context.Context, source S, event E, args ...any) bool {
	_, found := sr.findHandler(ctx, source, event, args...)
	return found
}

// findHandler looks up a trigger behavior that can handle the given event,
// walking up the hierarchy when necessary. It first checks the current state's
// behaviors for one whose guard conditions are satisfied; if none matches, it
// recursively checks the parent state. source always remains the actual current
// state and does not become the parent during recursion. It returns the matching
// transition result and whether a handler was found.
func (sr *stateRepresentation[S, E]) findHandler(ctx context.Context, source S, event E, args ...any) (transitionResult[S, E], bool) {
	// Look in the current state first.
	if behaviors, ok := sr.triggerBehaviors[event]; ok {
		for _, behavior := range behaviors {
			result, ok := behavior.resultsInTransitionFrom(ctx, source, args...)
			if ok {
				return result, true
			}
		}
	}
	// The current state cannot handle it; recurse into the parent state.
	if sr.superstate != nil {
		return sr.superstate.findHandler(ctx, source, event, args...)
	}
	return transitionResult[S, E]{}, false
}

// executeEntryActions runs all matching entry actions for this state. Actions
// with fromEvent set run only when the triggering event matches. If any action
// returns an error, execution stops immediately and the error is returned.
func (sr *stateRepresentation[S, E]) executeEntryActions(ctx context.Context, transition Transition[S, E]) error {
	for _, ea := range sr.entryActions {
		// If fromEvent is set, check whether the triggering event matches.
		if ea.fromEvent != nil && *ea.fromEvent != transition.Event {
			continue
		}
		if err := ea.action(ctx, transition); err != nil {
			return err
		}
	}
	return nil
}

// executeExitActions runs all exit actions for this state. If any action returns
// an error, execution stops immediately and the error is returned.
func (sr *stateRepresentation[S, E]) executeExitActions(ctx context.Context, transition Transition[S, E]) error {
	for _, action := range sr.exitActions {
		if err := action(ctx, transition); err != nil {
			return err
		}
	}
	return nil
}

// isIncludedIn reports whether this state is a substate of the given state,
// checking recursively. It returns true when the current state equals the given
// state; otherwise it walks up the parent chain.
func (sr *stateRepresentation[S, E]) isIncludedIn(state S) bool {
	if sr.state == state {
		return true
	}
	if sr.superstate != nil {
		return sr.superstate.isIncludedIn(state)
	}
	return false
}

// permittedTriggers returns all events that can be triggered from the current
// state, including events inherited from parent states. An event is considered
// triggerable when at least one of its behaviors has satisfied guard conditions.
// source is the actual current state used to evaluate guards. Results follow
// event registration order, with the state's own events before inherited ones.
func (sr *stateRepresentation[S, E]) permittedTriggers(ctx context.Context, source S, args ...any) []E {
	// De-duplicate with a map.
	seen := make(map[E]bool)
	var result []E

	// Collect triggerable events for the current state.
	sr.collectPermittedTriggers(ctx, source, seen, &result, args...)

	return result
}

// collectPermittedTriggers recursively collects triggerable events from the
// current state and its parent states.
func (sr *stateRepresentation[S, E]) collectPermittedTriggers(ctx context.Context, source S, seen map[E]bool, result *[]E, args ...any) {
	for _, event := range sr.triggerOrder {
		if seen[event] {
			continue
		}
		// Check whether at least one behavior can produce a transition.
		for _, behavior := range sr.triggerBehaviors[event] {
			if _, ok := behavior.resultsInTransitionFrom(ctx, source, args...); ok {
				seen[event] = true
				*result = append(*result, event)
				break
			}
		}
	}
	// Recursively check events inherited from the parent state.
	if sr.superstate != nil {
		sr.superstate.collectPermittedTriggers(ctx, source, seen, result, args...)
	}
}
