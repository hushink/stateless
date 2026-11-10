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
	"errors"
	"fmt"
)

// transitionResult represents the result of evaluating a trigger behavior.
// It carries the destination state, a transition-kind flag, and an optional
// transition action.
type transitionResult[S, E comparable] struct {
	destination S                // Destination state
	isInternal  bool             // Whether this is an internal transition (no entry/exit)
	action      ActionFunc[S, E] // Optional transition action
}

// triggerBehavior defines the strategy interface for a trigger's behavior from a
// given source state. Implementations correspond to different transition
// semantics (unconditional, guarded, dynamic, internal, reentry, ignored).
type triggerBehavior[S, E comparable] interface {
	// resultsInTransitionFrom evaluates whether this behavior produces a
	// transition from the given source state. source is the actual current state;
	// it returns the transition result and whether a valid transition occurred.
	resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool)

	// isGuarded reports whether this behavior has guard conditions.
	isGuarded() bool

	// guardDescription returns a textual description of the guard conditions,
	// used for graph export and debugging.
	guardDescription() string

	// validate checks that the behavior configuration is valid (for example,
	// guards and selectors must not be nil). It is called during Build so that
	// configuration errors surface as errors instead of panics at Fire time.
	validate() error
}

// validateGuards checks that the guard list contains no nil elements.
func validateGuards[S, E comparable](guards []GuardFunc[S, E]) error {
	for i, guard := range guards {
		if guard == nil {
			return fmt.Errorf("guard at index %d is nil", i)
		}
	}
	return nil
}

// cloneGuards copies the guard slice so later mutations by the caller cannot
// affect a state machine that has already been built.
func cloneGuards[S, E comparable](guards []GuardFunc[S, E]) []GuardFunc[S, E] {
	if guards == nil {
		return nil
	}
	cloned := make([]GuardFunc[S, E], len(guards))
	copy(cloned, guards)
	return cloned
}

// ---------------------------------------------------------------------------
// permittedBehavior — unconditional transition
// ---------------------------------------------------------------------------

// permittedBehavior represents an unconditionally permitted state transition.
// It has no guard conditions and always produces a transition when evaluated.
type permittedBehavior[S, E comparable] struct {
	destination S // Destination state
}

// newPermittedBehavior creates an unconditionally permitted transition behavior.
func newPermittedBehavior[S, E comparable](destination S) *permittedBehavior[S, E] {
	return &permittedBehavior[S, E]{
		destination: destination,
	}
}

// resultsInTransitionFrom always returns true and the destination state, because
// this behavior is unconditionally permitted.
func (b *permittedBehavior[S, E]) resultsInTransitionFrom(_ context.Context, _ S, _ ...any) (transitionResult[S, E], bool) {
	return transitionResult[S, E]{
		destination: b.destination,
	}, true
}

// isGuarded returns false because an unconditional behavior has no guards.
func (b *permittedBehavior[S, E]) isGuarded() bool {
	return false
}

// guardDescription returns an empty string because there are no guards.
func (b *permittedBehavior[S, E]) guardDescription() string {
	return ""
}

// validate reports no errors for an unconditional behavior.
func (b *permittedBehavior[S, E]) validate() error {
	return nil
}

// ---------------------------------------------------------------------------
// guardedBehavior — guarded transition
// ---------------------------------------------------------------------------

// guardedBehavior represents a state transition with guard conditions.
// The transition takes effect only when every guard function returns true.
type guardedBehavior[S, E comparable] struct {
	event       E                 // Triggering event
	destination S                 // Destination state
	guards      []GuardFunc[S, E] // Guards; the transition is allowed when all return true
	description string            // Guard description for debugging and graph export
}

// newGuardedBehavior creates a guarded transition behavior.
func newGuardedBehavior[S, E comparable](event E, destination S, guards []GuardFunc[S, E], description string) *guardedBehavior[S, E] {
	return &guardedBehavior[S, E]{
		event:       event,
		destination: destination,
		guards:      cloneGuards(guards),
		description: description,
	}
}

// resultsInTransitionFrom evaluates all guards. A transition occurs only when
// every guard returns true.
func (b *guardedBehavior[S, E]) resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool) {
	t := Transition[S, E]{
		Source:      source,
		Destination: b.destination,
		Event:       b.event,
		Args:        args,
	}
	// Evaluate each guard in turn.
	for _, guard := range b.guards {
		if !guard(ctx, t) {
			return transitionResult[S, E]{}, false
		}
	}
	return transitionResult[S, E]{
		destination: b.destination,
	}, true
}

// isGuarded returns true, indicating that this behavior has guard conditions.
func (b *guardedBehavior[S, E]) isGuarded() bool {
	return true
}

// guardDescription returns the textual description of the guard conditions.
func (b *guardedBehavior[S, E]) guardDescription() string {
	return b.description
}

// validate checks the guard condition list.
func (b *guardedBehavior[S, E]) validate() error {
	return validateGuards(b.guards)
}

// ---------------------------------------------------------------------------
// dynamicBehavior — dynamically selected destination
// ---------------------------------------------------------------------------

// dynamicBehavior represents a transition whose destination is selected at
// runtime. A DynamicSelectorFunc decides the target from the current context,
// which models scenarios such as Choice pseudostates.
type dynamicBehavior[S, E comparable] struct {
	event    E                         // Triggering event
	selector DynamicSelectorFunc[S, E] // Dynamic target selector
	guards   []GuardFunc[S, E]         // Optional guard conditions
}

// newDynamicBehavior creates a transition behavior with a dynamic destination.
func newDynamicBehavior[S, E comparable](event E, selector DynamicSelectorFunc[S, E], guards []GuardFunc[S, E]) *dynamicBehavior[S, E] {
	return &dynamicBehavior[S, E]{
		event:    event,
		selector: selector,
		guards:   cloneGuards(guards),
	}
}

// resultsInTransitionFrom first checks optional guards, then uses the selector
// to decide the destination state dynamically.
func (b *dynamicBehavior[S, E]) resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool) {
	t := Transition[S, E]{
		Source: source,
		Event:  b.event,
		Args:   args,
	}
	// Check optional guard conditions.
	for _, guard := range b.guards {
		if !guard(ctx, t) {
			return transitionResult[S, E]{}, false
		}
	}
	// Use the selector to decide the destination state.
	destination := b.selector(ctx, t)
	return transitionResult[S, E]{
		destination: destination,
	}, true
}

// isGuarded reports whether guard conditions are present.
func (b *dynamicBehavior[S, E]) isGuarded() bool {
	return len(b.guards) > 0
}

// guardDescription returns a guard description. Dynamic behaviors have no static
// description, so a fixed string is returned.
func (b *dynamicBehavior[S, E]) guardDescription() string {
	if len(b.guards) > 0 {
		return "dynamic (guarded)"
	}
	return "dynamic"
}

// validate checks the dynamic selector and guard condition list.
func (b *dynamicBehavior[S, E]) validate() error {
	if b.selector == nil {
		return errors.New("dynamic selector is nil")
	}
	return validateGuards(b.guards)
}

// ---------------------------------------------------------------------------
// internalBehavior — internal transition (no entry/exit)
// ---------------------------------------------------------------------------

// internalBehavior represents an internal transition. It executes an action
// without triggering the state's entry/exit actions; the machine's current state
// remains unchanged before and after the transition.
type internalBehavior[S, E comparable] struct {
	event  E                 // Triggering event
	action ActionFunc[S, E]  // Action executed by the internal transition
	guards []GuardFunc[S, E] // Optional guard conditions
}

// newInternalBehavior creates an internal transition behavior.
func newInternalBehavior[S, E comparable](event E, action ActionFunc[S, E], guards []GuardFunc[S, E]) *internalBehavior[S, E] {
	return &internalBehavior[S, E]{
		event:  event,
		action: action,
		guards: cloneGuards(guards),
	}
}

// resultsInTransitionFrom returns an internal transition result after checking
// guards. The internal transition is marked isInternal and its destination
// equals the source state.
func (b *internalBehavior[S, E]) resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool) {
	t := Transition[S, E]{
		Source:      source,
		Destination: source, // An internal transition targets its source
		Event:       b.event,
		Args:        args,
	}
	// Check optional guard conditions.
	for _, guard := range b.guards {
		if !guard(ctx, t) {
			return transitionResult[S, E]{}, false
		}
	}
	return transitionResult[S, E]{
		destination: source,
		isInternal:  true,
		action:      b.action,
	}, true
}

// isGuarded reports whether guard conditions are present.
func (b *internalBehavior[S, E]) isGuarded() bool {
	return len(b.guards) > 0
}

// guardDescription returns a guard description.
func (b *internalBehavior[S, E]) guardDescription() string {
	if len(b.guards) > 0 {
		return "internal (guarded)"
	}
	return ""
}

// validate checks the guard condition list (the internal action may be nil).
func (b *internalBehavior[S, E]) validate() error {
	return validateGuards(b.guards)
}

// ---------------------------------------------------------------------------
// reentryBehavior — reentry transition (source == destination, runs entry/exit)
// ---------------------------------------------------------------------------

// reentryBehavior represents a reentry transition. The source equals the
// destination, but unlike an internal transition, reentry runs exit and entry
// actions.
type reentryBehavior[S, E comparable] struct {
	event  E                 // Triggering event
	guards []GuardFunc[S, E] // Optional guard conditions
}

// newReentryBehavior creates a reentry transition behavior.
func newReentryBehavior[S, E comparable](event E, guards []GuardFunc[S, E]) *reentryBehavior[S, E] {
	return &reentryBehavior[S, E]{
		event:  event,
		guards: cloneGuards(guards),
	}
}

// resultsInTransitionFrom returns a reentry transition result after checking
// guards. The destination equals the actual source state, and guards observe
// Transition.IsReentry == true.
func (b *reentryBehavior[S, E]) resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool) {
	t := Transition[S, E]{
		Source:      source,
		Destination: source, // A reentry transition targets its source
		Event:       b.event,
		Args:        args,
		IsReentry:   true,
	}
	// Check optional guard conditions.
	for _, guard := range b.guards {
		if !guard(ctx, t) {
			return transitionResult[S, E]{}, false
		}
	}
	return transitionResult[S, E]{
		destination: source,
	}, true
}

// isGuarded reports whether guard conditions are present.
func (b *reentryBehavior[S, E]) isGuarded() bool {
	return len(b.guards) > 0
}

// guardDescription returns a guard description.
func (b *reentryBehavior[S, E]) guardDescription() string {
	if len(b.guards) > 0 {
		return "reentry (guarded)"
	}
	return ""
}

// validate checks the guard condition list.
func (b *reentryBehavior[S, E]) validate() error {
	return validateGuards(b.guards)
}

// ---------------------------------------------------------------------------
// ignoredBehavior — ignored event (silently discarded)
// ---------------------------------------------------------------------------

// ignoredBehavior represents a behavior that ignores an event. When the event
// fires, this behavior produces no transition and the event is silently discarded.
type ignoredBehavior[S, E comparable] struct {
	event  E                 // Triggering event
	guards []GuardFunc[S, E] // Optional guard conditions
}

// newIgnoredBehavior creates an ignored-event behavior.
func newIgnoredBehavior[S, E comparable](event E, guards []GuardFunc[S, E]) *ignoredBehavior[S, E] {
	return &ignoredBehavior[S, E]{
		event:  event,
		guards: cloneGuards(guards),
	}
}

// resultsInTransitionFrom returns true when the guards are satisfied (or absent),
// indicating the event has been handled (silently ignored). It uses
// isInternal=true and action=nil so Fire performs no action and leaves the state
// unchanged. If guards exist and are not satisfied, it returns false so the
// caller can try other behaviors.
func (b *ignoredBehavior[S, E]) resultsInTransitionFrom(ctx context.Context, source S, args ...any) (transitionResult[S, E], bool) {
	// If guards exist, first check whether this ignore rule applies.
	if len(b.guards) > 0 {
		t := Transition[S, E]{
			Source:      source,
			Destination: source,
			Event:       b.event,
			Args:        args,
		}
		for _, guard := range b.guards {
			if !guard(ctx, t) {
				// Guards failed: this ignore rule does not apply; return false
				// so the caller keeps searching.
				return transitionResult[S, E]{}, false
			}
		}
	}
	// Ignore the event: mark it as an internal transition with no action and no
	// state change.
	return transitionResult[S, E]{
		destination: source,
		isInternal:  true,
	}, true
}

// isGuarded reports whether guard conditions are present.
func (b *ignoredBehavior[S, E]) isGuarded() bool {
	return len(b.guards) > 0
}

// guardDescription returns a guard description.
func (b *ignoredBehavior[S, E]) guardDescription() string {
	if len(b.guards) > 0 {
		return "ignored (guarded)"
	}
	return ""
}

// validate checks the guard condition list.
func (b *ignoredBehavior[S, E]) validate() error {
	return validateGuards(b.guards)
}
