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

// StateConfigurator defines the pure behavior configuration interface for a single
// state. It provides transition and action configuration methods; every method
// returns StateConfigurator for chaining. Call Done to return to the Builder and
// continue configuring other states or build the machine.
type StateConfigurator[S, E comparable] interface {
	// --- Transition configuration ---

	// Permit configures an unconditional transition: when the given event fires,
	// the machine moves from the current state to the destination state.
	Permit(event E, dest S) StateConfigurator[S, E]
	// PermitIf configures a guarded transition: it is allowed only when every
	// guard function returns true.
	PermitIf(event E, dest S, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
	// PermitDynamic configures a dynamic transition: the destination is decided
	// at runtime by the selector function.
	PermitDynamic(event E, selector DynamicSelectorFunc[S, E]) StateConfigurator[S, E]
	// PermitDynamicIf configures a guarded dynamic transition: the selector is
	// invoked only after the guards pass.
	PermitDynamicIf(event E, selector DynamicSelectorFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E]
	// PermitReentry configures a reentry transition: the state does not change,
	// but exit and entry actions are executed.
	PermitReentry(event E) StateConfigurator[S, E]
	// PermitReentryIf configures a guarded reentry transition.
	PermitReentryIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
	// Ignore configures the given event to be ignored: no transition and no error.
	Ignore(event E) StateConfigurator[S, E]
	// IgnoreIf configures conditional ignoring: the event is ignored only when the
	// guard condition is satisfied.
	IgnoreIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
	// InternalTransition configures an internal transition: the action executes
	// without triggering entry/exit actions, and the state does not change.
	InternalTransition(event E, action ActionFunc[S, E]) StateConfigurator[S, E]
	// InternalTransitionIf configures a guarded internal transition.
	InternalTransitionIf(event E, action ActionFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E]

	// --- Action configuration ---

	// OnEntry adds an entry action executed every time the state is entered,
	// regardless of the triggering event.
	OnEntry(action ActionFunc[S, E]) StateConfigurator[S, E]
	// OnEntryFrom adds an event-filtered entry action executed only when the state
	// is entered because of the specified event.
	OnEntryFrom(event E, action ActionFunc[S, E]) StateConfigurator[S, E]
	// OnExit adds an exit action executed every time the state is left.
	OnExit(action ActionFunc[S, E]) StateConfigurator[S, E]

	// --- Back to Builder ---

	// Done ends configuration of the current state and returns the Builder to
	// continue configuring other states or build the machine.
	Done() Builder[S, E]
}

// stateConfigurator is the unexported implementation of the StateConfigurator interface.
type stateConfigurator[S, E comparable] struct {
	b     *builder[S, E]             // Owning builder
	state S                          // State currently being configured
	repr  *stateRepresentation[S, E] // Internal representation of the current state
}

// Permit configures an unconditional transition. The destination state is
// registered automatically, so it appears in graph exports even without an
// explicit Configure call.
func (sc *stateConfigurator[S, E]) Permit(event E, dest S) StateConfigurator[S, E] {
	sc.b.getOrCreate(dest)
	sc.repr.addTriggerBehavior(event, newPermittedBehavior[S, E](dest))
	return sc
}

// PermitIf configures a guarded transition. The destination state is registered
// automatically, so it appears in graph exports even without an explicit
// Configure call.
func (sc *stateConfigurator[S, E]) PermitIf(event E, dest S, guards ...GuardFunc[S, E]) StateConfigurator[S, E] {
	sc.b.getOrCreate(dest)
	sc.repr.addTriggerBehavior(event, newGuardedBehavior[S, E](event, dest, guards, ""))
	return sc
}

// PermitDynamic configures a dynamic transition without guards.
func (sc *stateConfigurator[S, E]) PermitDynamic(event E, selector DynamicSelectorFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newDynamicBehavior[S, E](event, selector, nil))
	return sc
}

// PermitDynamicIf configures a guarded dynamic transition.
func (sc *stateConfigurator[S, E]) PermitDynamicIf(event E, selector DynamicSelectorFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newDynamicBehavior[S, E](event, selector, guards))
	return sc
}

// PermitReentry configures an unconditional reentry transition.
func (sc *stateConfigurator[S, E]) PermitReentry(event E) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newReentryBehavior[S, E](event, nil))
	return sc
}

// PermitReentryIf configures a guarded reentry transition.
func (sc *stateConfigurator[S, E]) PermitReentryIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newReentryBehavior[S, E](event, guards))
	return sc
}

// Ignore configures the given event to be ignored.
func (sc *stateConfigurator[S, E]) Ignore(event E) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newIgnoredBehavior[S, E](event, nil))
	return sc
}

// IgnoreIf configures conditional ignoring of an event.
func (sc *stateConfigurator[S, E]) IgnoreIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newIgnoredBehavior[S, E](event, guards))
	return sc
}

// InternalTransition configures an internal transition.
func (sc *stateConfigurator[S, E]) InternalTransition(event E, action ActionFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newInternalBehavior[S, E](event, action, nil))
	return sc
}

// InternalTransitionIf configures a guarded internal transition.
func (sc *stateConfigurator[S, E]) InternalTransitionIf(event E, action ActionFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addTriggerBehavior(event, newInternalBehavior[S, E](event, action, guards))
	return sc
}

// OnEntry adds a generic entry action.
func (sc *stateConfigurator[S, E]) OnEntry(action ActionFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addEntryAction(action)
	return sc
}

// OnEntryFrom adds an event-filtered entry action.
func (sc *stateConfigurator[S, E]) OnEntryFrom(event E, action ActionFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addEntryActionFrom(event, action)
	return sc
}

// OnExit adds an exit action.
func (sc *stateConfigurator[S, E]) OnExit(action ActionFunc[S, E]) StateConfigurator[S, E] {
	sc.repr.addExitAction(action)
	return sc
}

// Done ends configuration of the current state and returns the owning Builder.
func (sc *stateConfigurator[S, E]) Done() Builder[S, E] {
	return sc.b
}
