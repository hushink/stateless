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

import (
	"context"
	"log/slog"
)

// StateMachine defines the public interface of an immutable, stateless state
// machine. After Builder.Build(), all configuration is frozen, so the interface
// methods are inherently concurrency-safe.
type StateMachine[S, E comparable] interface {
	// Fire triggers an event from the given current state and returns the new state.
	// If no valid transition is configured for the event in the current state, it
	// returns TriggerNotPermittedError. If a transition action or an entry/exit
	// action returns an error, the transition is aborted and that error is returned.
	Fire(ctx context.Context, state S, event E, args ...any) (S, error)
	// CanFire reports whether the given event can be triggered from the given state.
	// Guard conditions are evaluated; args are passed to the guards.
	CanFire(state S, event E, args ...any) bool
	// CanFireContext is the same as CanFire but accepts a context for use by guards.
	CanFireContext(ctx context.Context, state S, event E, args ...any) bool
	// PermittedTriggers returns every event that can be triggered from the given state.
	// Results follow event registration order, with the state's own events before
	// events inherited from parent states.
	PermittedTriggers(state S, args ...any) []E
	// PermittedTriggersContext is the same as PermittedTriggers but accepts a context
	// for use by guards.
	PermittedTriggersContext(ctx context.Context, state S, args ...any) []E
	// InitialState returns the machine's initial state. If the initial state has an
	// InitialTransition chain, it is recursively resolved to the final substate.
	InitialState() S
	// IsInState reports whether state is within possibleSuperstate, honoring the
	// hierarchy. It returns true when state == possibleSuperstate or when state is
	// a substate of possibleSuperstate.
	IsInState(state S, possibleSuperstate S) bool
	// ToDotGraph returns the Graphviz DOT representation of the state machine.
	ToDotGraph() string
	// ToMermaidGraph returns the Mermaid representation of the state machine.
	ToMermaidGraph() string
}

// stateMachine is the unexported implementation of the StateMachine interface.
type stateMachine[S, E comparable] struct {
	initialState S                                // Initial state
	config       map[S]*stateRepresentation[S, E] // Configuration of all states
	listeners    []Listener[S, E]                 // Global listener list
	debugLogger  *slog.Logger                     // Optional debug logger (nil means disabled)
}

// debugf emits a debug-level log message when debug logging is enabled.
func (sm *stateMachine[S, E]) debugf(ctx context.Context, msg string, args ...any) {
	if sm.debugLogger == nil {
		return
	}
	sm.debugLogger.DebugContext(ctx, msg, args...)
}

// Fire triggers an event from the given current state and returns the new state.
func (sm *stateMachine[S, E]) Fire(ctx context.Context, state S, event E, args ...any) (S, error) {
	sm.debugf(ctx, "fire", "state", state, "event", event, "argc", len(args))

	// 1. Fetch the stateRepresentation for the current state.
	repr, ok := sm.config[state]
	if !ok {
		sm.notifyUnhandledTrigger(ctx, state, event)
		return state, &TriggerNotPermittedError[S, E]{State: state, Event: event}
	}

	// 2. Find a matching trigger behavior, walking up the hierarchy as needed.
	//    source is the actual current state, not the parent where a behavior
	//    might be found.
	result, found := repr.findHandler(ctx, state, event, args...)

	// 3. No handler found.
	if !found {
		sm.notifyUnhandledTrigger(ctx, state, event)
		return state, &TriggerNotPermittedError[S, E]{State: state, Event: event}
	}

	// 4. Resolve the final destination by expanding the InitialTransition chain
	//    so listeners and actions observe a consistent final state.
	finalState := result.destination
	if !result.isInternal {
		finalState = sm.resolveInitialState(finalState)
	}

	// 5. Build the Transition value.
	transition := Transition[S, E]{
		Source:      state,
		Destination: finalState,
		Event:       event,
		Args:        args,
		IsReentry:   !result.isInternal && finalState == state,
	}

	sm.debugf(ctx, "transition", "source", state, "event", event,
		"destination", finalState, "internal", result.isInternal, "reentry", transition.IsReentry)

	// 6. Notify every listener via OnTransitioning.
	for _, l := range sm.listeners {
		l.OnTransitioning(ctx, transition)
	}

	// 7. Internal transition: run only the transition action, no entry/exit.
	if result.isInternal {
		if result.action != nil {
			sm.debugf(ctx, "run transition action", "state", state, "event", event)
			if err := result.action(ctx, transition); err != nil {
				return state, err
			}
		}
		sm.notifyTransitioned(ctx, transition)
		sm.debugf(ctx, "transition completed", "source", state, "event", event, "state", state)
		return state, nil
	}

	// 8. External transition (including reentry and external self-transitions):
	//    run the exit/action/entry sequence.
	if result.destination == state {
		// 8a. The target is the current state: exit and re-enter it.
		//     Permit(event, sameState) and PermitReentry behave identically here.
		sm.debugf(ctx, "exit", "state", state)
		if err := repr.executeExitActions(ctx, transition); err != nil {
			return state, err
		}
		if result.action != nil {
			sm.debugf(ctx, "run transition action", "state", state, "event", event)
			if err := result.action(ctx, transition); err != nil {
				return state, err
			}
		}
		sm.debugf(ctx, "enter", "state", state)
		if err := repr.executeEntryActions(ctx, transition); err != nil {
			return state, err
		}
		// If the current state is composite, continue entering its initial
		// substate chain.
		for _, entryRepr := range sm.collectEntryPath(finalState, repr) {
			sm.debugf(ctx, "enter", "state", entryRepr.state)
			if err := entryRepr.executeEntryActions(ctx, transition); err != nil {
				return state, err
			}
		}
	} else {
		// 8b. Compute the lowest common ancestor (LCA). The LCA must be computed
		//     from the requested target state, not from the final state after
		//     expanding InitialTransition.
		lca := sm.findLCA(state, result.destination)

		// 8c. Run exit actions: from the current state up to (excluding) the LCA.
		for _, exitRepr := range sm.collectExitPath(state, lca) {
			sm.debugf(ctx, "exit", "state", exitRepr.state)
			if err := exitRepr.executeExitActions(ctx, transition); err != nil {
				return state, err
			}
		}

		// 8d. Run the transition action, if any.
		if result.action != nil {
			sm.debugf(ctx, "run transition action", "state", state, "event", event)
			if err := result.action(ctx, transition); err != nil {
				return state, err
			}
		}

		// 8e. Run entry actions: from below the LCA down to the final state.
		//     Using the final state guarantees that when a composite state is
		//     entered, every intermediate state on the initial substate chain
		//     runs its entry action in parent-to-child order.
		for _, entryRepr := range sm.collectEntryPath(finalState, lca) {
			sm.debugf(ctx, "enter", "state", entryRepr.state)
			if err := entryRepr.executeEntryActions(ctx, transition); err != nil {
				return state, err
			}
		}
	}

	// 9. Notify every listener via OnTransitioned.
	sm.notifyTransitioned(ctx, transition)
	sm.debugf(ctx, "transition completed", "source", state, "event", event, "state", finalState)

	return finalState, nil
}

// notifyUnhandledTrigger notifies every listener that an event was not handled.
func (sm *stateMachine[S, E]) notifyUnhandledTrigger(ctx context.Context, state S, event E) {
	sm.debugf(ctx, "unhandled trigger", "state", state, "event", event)
	for _, l := range sm.listeners {
		l.OnUnhandledTrigger(ctx, state, event)
	}
}

// notifyTransitioned notifies every listener that a transition completed.
func (sm *stateMachine[S, E]) notifyTransitioned(ctx context.Context, transition Transition[S, E]) {
	for _, l := range sm.listeners {
		l.OnTransitioned(ctx, transition)
	}
}

// CanFire reports whether the given event can be triggered from the given state.
func (sm *stateMachine[S, E]) CanFire(state S, event E, args ...any) bool {
	return sm.CanFireContext(context.Background(), state, event, args...)
}

// CanFireContext reports whether the given event can be triggered from the given
// state, passing the context and arguments to guards.
func (sm *stateMachine[S, E]) CanFireContext(ctx context.Context, state S, event E, args ...any) bool {
	repr, ok := sm.config[state]
	if !ok {
		sm.debugf(ctx, "can fire", "state", state, "event", event, "permitted", false)
		return false
	}
	permitted := repr.canHandle(ctx, state, event, args...)
	sm.debugf(ctx, "can fire", "state", state, "event", event, "permitted", permitted)
	return permitted
}

// PermittedTriggers returns every event that can be triggered from the given state.
func (sm *stateMachine[S, E]) PermittedTriggers(state S, args ...any) []E {
	return sm.PermittedTriggersContext(context.Background(), state, args...)
}

// PermittedTriggersContext returns every event that can be triggered from the
// given state, passing the context and arguments to guards.
func (sm *stateMachine[S, E]) PermittedTriggersContext(ctx context.Context, state S, args ...any) []E {
	repr, ok := sm.config[state]
	if !ok {
		return nil
	}
	events := repr.permittedTriggers(ctx, state, args...)
	sm.debugf(ctx, "permitted triggers", "state", state, "events", events)
	return events
}

// InitialState returns the machine's initial state. If the initial state has an
// InitialTransition chain, it is recursively resolved to the final substate.
func (sm *stateMachine[S, E]) InitialState() S {
	return sm.resolveInitialState(sm.initialState)
}

// resolveInitialState resolves the final state along the InitialTransition chain.
// Build guarantees that initial transitions are acyclic, so the loop always
// terminates; the step bound is a defensive safeguard.
func (sm *stateMachine[S, E]) resolveInitialState(state S) S {
	for steps := 0; steps <= len(sm.config); steps++ {
		repr, ok := sm.config[state]
		if !ok || repr.initialTransitionTarget == nil {
			return state
		}
		state = *repr.initialTransitionTarget
	}
	return state
}

// IsInState reports whether state is within possibleSuperstate, honoring the hierarchy.
func (sm *stateMachine[S, E]) IsInState(state S, possibleSuperstate S) bool {
	if state == possibleSuperstate {
		return true
	}
	repr, ok := sm.config[state]
	if !ok {
		return false
	}
	return repr.isIncludedIn(possibleSuperstate)
}

// ToDotGraph returns the Graphviz DOT representation of the state machine. The
// generated graph contains every configured state and transition and supports
// hierarchical cluster nesting, guard labels, and a UML-style initial-state entry
// point.
func (sm *stateMachine[S, E]) ToDotGraph() string {
	return sm.generateDotGraph()
}

// ToMermaidGraph returns the Mermaid state diagram representation of the state
// machine. The generated graph contains every configured state and transition and
// supports hierarchical nesting syntax, guard labels, and initial-state entry.
func (sm *stateMachine[S, E]) ToMermaidGraph() string {
	return sm.generateMermaidGraph()
}

// findLCA finds the lowest common ancestor of two states. It returns nil when
// there is no common ancestor.
func (sm *stateMachine[S, E]) findLCA(source, dest S) *stateRepresentation[S, E] {
	// Collect the ancestor chain of source (including itself).
	sourceAncestors := make(map[S]bool)
	if sourceRepr, ok := sm.config[source]; ok {
		current := sourceRepr
		for current != nil {
			sourceAncestors[current.state] = true
			current = current.superstate
		}
	}

	// Walk up from dest to find the first state on source's ancestor chain.
	if destRepr, ok := sm.config[dest]; ok {
		current := destRepr
		for current != nil {
			if sourceAncestors[current.state] {
				return current
			}
			current = current.superstate
		}
	}

	return nil
}

// collectExitPath collects the states that must be exited between the current
// state and the LCA (excluding the LCA), in child-to-parent order.
func (sm *stateMachine[S, E]) collectExitPath(state S, lca *stateRepresentation[S, E]) []*stateRepresentation[S, E] {
	var path []*stateRepresentation[S, E]
	repr, ok := sm.config[state]
	if !ok {
		return path
	}
	current := repr
	for current != nil && current != lca {
		path = append(path, current)
		current = current.superstate
	}
	return path
}

// collectEntryPath collects the states that must be entered between the LCA
// (excluding the LCA) and the target state, in parent-to-child order.
func (sm *stateMachine[S, E]) collectEntryPath(state S, lca *stateRepresentation[S, E]) []*stateRepresentation[S, E] {
	var path []*stateRepresentation[S, E]
	repr, ok := sm.config[state]
	if !ok {
		return path
	}
	current := repr
	for current != nil && current != lca {
		path = append(path, current)
		current = current.superstate
	}
	// Reverse into parent-to-child order.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
