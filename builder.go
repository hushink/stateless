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

import "fmt"

// Builder defines the public interface for building a state machine.
// Every configuration method returns either Builder or StateConfigurator to
// support uninterrupted chaining; only Build terminates the chain and returns
// a read-only StateMachine instance.
type Builder[S, E comparable] interface {
	// Configure starts configuring the behavior of the given state (transition
	// rules, entry/exit actions, and so on). If the state is not registered yet,
	// an empty state configuration is created automatically. It returns a
	// StateConfigurator for chaining the state's detailed behavior.
	Configure(state S) StateConfigurator[S, E]
	// SubstateOf declares child as a substate of parent. A substate inherits the
	// parent's transition rules; the hierarchy is resolved when Build is called.
	SubstateOf(child, parent S) Builder[S, E]
	// InitialTransition sets the initial child state that state transitions to
	// automatically upon entry. When an external transition targets state, the
	// machine recursively transitions to initialChild.
	InitialTransition(state, initialChild S) Builder[S, E]
	// WithListener registers a global listener that observes transition lifecycle
	// events. It may be called multiple times to register multiple listeners.
	WithListener(l Listener[S, E]) Builder[S, E]
	// Build terminates the configuration chain, validates the configuration, and
	// returns a read-only StateMachine. Validation failures return an error
	// wrapping the ErrInvalidConfiguration sentinel.
	Build() (StateMachine[S, E], error)
}

// NewBuilder creates a new state machine builder with initialState as the initial
// state. It returns the Builder interface; the concrete implementation is unexported.
func NewBuilder[S, E comparable](initialState S) Builder[S, E] {
	return &builder[S, E]{
		initialState:      initialState,
		config:            make(map[S]*stateRepresentation[S, E]),
		configuredStates:  make(map[S]bool),
		substateRelations: make(map[S]S),
	}
}

// builder is the unexported implementation of the Builder interface.
type builder[S, E comparable] struct {
	initialState      S                                // Initial state
	config            map[S]*stateRepresentation[S, E] // State -> state configuration
	configuredStates  map[S]bool                       // States explicitly configured via Configure
	listeners         []Listener[S, E]                 // Global listener list
	substateRelations map[S]S                          // child -> parent substate mapping
}

// getOrCreate returns the stateRepresentation for the given state, creating it if needed.
func (b *builder[S, E]) getOrCreate(state S) *stateRepresentation[S, E] {
	if repr, ok := b.config[state]; ok {
		return repr
	}
	repr := newStateRepresentation[S, E](state)
	b.config[state] = repr
	return repr
}

// Configure starts configuring the behavior of the given state (transition rules,
// entry/exit actions, and so on).
func (b *builder[S, E]) Configure(state S) StateConfigurator[S, E] {
	repr := b.getOrCreate(state)
	b.configuredStates[state] = true
	return &stateConfigurator[S, E]{
		b:     b,
		state: state,
		repr:  repr,
	}
}

// SubstateOf declares child as a substate of parent. Both states are registered
// automatically, so Build will not panic even if they were never Configured.
func (b *builder[S, E]) SubstateOf(child, parent S) Builder[S, E] {
	b.getOrCreate(child)
	b.getOrCreate(parent)
	b.substateRelations[child] = parent
	return b
}

// InitialTransition sets the initial child state that state transitions to upon
// entry. Both state and initialChild are registered automatically.
func (b *builder[S, E]) InitialTransition(state, initialChild S) Builder[S, E] {
	repr := b.getOrCreate(state)
	b.getOrCreate(initialChild)
	target := initialChild
	repr.initialTransitionTarget = &target
	return b
}

// WithListener registers a global listener.
func (b *builder[S, E]) WithListener(l Listener[S, E]) Builder[S, E] {
	b.listeners = append(b.listeners, l)
	return b
}

// Build terminates the configuration chain, validates the configuration, and
// returns a read-only StateMachine. Build never mutates the Builder itself, so
// it can be called repeatedly and each call returns an isolated StateMachine.
func (b *builder[S, E]) Build() (StateMachine[S, E], error) {
	// 1. Verify the initial state was explicitly configured.
	if !b.configuredStates[b.initialState] {
		return nil, &InvalidConfigurationError{
			Message: fmt.Sprintf("initial state %v has not been configured", b.initialState),
		}
	}

	// 2. Deep-copy every state configuration. superstate/substates are always
	//    rebuilt on the copies, so Build never mutates the Builder's internal
	//    state and repeated Builds cannot accumulate duplicate substates.
	frozenConfig := make(map[S]*stateRepresentation[S, E], len(b.config))
	for state, repr := range b.config {
		cloned := &stateRepresentation[S, E]{
			state: state,
			// superstate and substates are left empty; step 3 rebuilds them
			// from substateRelations.
		}
		if repr.initialTransitionTarget != nil {
			target := *repr.initialTransitionTarget
			cloned.initialTransitionTarget = &target
		}
		cloned.triggerBehaviors = make(map[E][]triggerBehavior[S, E], len(repr.triggerBehaviors))
		for event, behaviors := range repr.triggerBehaviors {
			copied := make([]triggerBehavior[S, E], len(behaviors))
			copy(copied, behaviors)
			cloned.triggerBehaviors[event] = copied
		}
		if len(repr.triggerOrder) > 0 {
			cloned.triggerOrder = make([]E, len(repr.triggerOrder))
			copy(cloned.triggerOrder, repr.triggerOrder)
		}
		if len(repr.entryActions) > 0 {
			cloned.entryActions = make([]entryActionBehavior[S, E], len(repr.entryActions))
			copy(cloned.entryActions, repr.entryActions)
		}
		if len(repr.exitActions) > 0 {
			cloned.exitActions = make([]ActionFunc[S, E], len(repr.exitActions))
			copy(cloned.exitActions, repr.exitActions)
		}
		frozenConfig[state] = cloned
	}

	// 3. Establish superstate/substates relationships on the copies.
	for child, parent := range b.substateRelations {
		childRepr, ok := frozenConfig[child]
		if !ok {
			return nil, &InvalidConfigurationError{
				Message: fmt.Sprintf("substate %v has not been configured", child),
			}
		}
		parentRepr, ok := frozenConfig[parent]
		if !ok {
			return nil, &InvalidConfigurationError{
				Message: fmt.Sprintf("superstate %v has not been configured", parent),
			}
		}
		childRepr.superstate = parentRepr
		parentRepr.substates = append(parentRepr.substates, childRepr)
	}

	// 4. Detect circular substate references.
	for state := range frozenConfig {
		if err := detectSubstateCycle(frozenConfig, state); err != nil {
			return nil, err
		}
	}

	// 5. Detect InitialTransition cycles.
	for state := range frozenConfig {
		if err := detectInitialTransitionCycle(frozenConfig, state); err != nil {
			return nil, err
		}
	}

	// 6. Validate initial transition targets: they must exist and must be
	//    descendants of the state that declares them.
	for state, repr := range frozenConfig {
		if repr.initialTransitionTarget == nil {
			continue
		}
		target := *repr.initialTransitionTarget
		targetRepr, ok := frozenConfig[target]
		if !ok {
			return nil, &InvalidConfigurationError{
				Message: fmt.Sprintf("initial transition target %v of state %v has not been configured", target, state),
			}
		}
		if !isDescendantOf(targetRepr, state) {
			return nil, &InvalidConfigurationError{
				Message: fmt.Sprintf("initial transition target %v is not a substate of %v", target, state),
			}
		}
	}

	// 7. Validate trigger behaviors and actions so nil guards/selectors are
	//    reported at Build time.
	for state, repr := range frozenConfig {
		for _, event := range repr.triggerOrder {
			for _, behavior := range repr.triggerBehaviors[event] {
				if err := behavior.validate(); err != nil {
					return nil, &InvalidConfigurationError{
						Message: fmt.Sprintf("state %v, event %v: %v", state, event, err),
					}
				}
			}
		}
		for i, entryAction := range repr.entryActions {
			if entryAction.action == nil {
				return nil, &InvalidConfigurationError{
					Message: fmt.Sprintf("state %v: entry action at index %d is nil", state, i),
				}
			}
		}
		for i, exitAction := range repr.exitActions {
			if exitAction == nil {
				return nil, &InvalidConfigurationError{
					Message: fmt.Sprintf("state %v: exit action at index %d is nil", state, i),
				}
			}
		}
	}

	// 8. Validate and copy the listeners slice.
	for i, listener := range b.listeners {
		if listener == nil {
			return nil, &InvalidConfigurationError{
				Message: fmt.Sprintf("listener at index %d is nil", i),
			}
		}
	}
	frozenListeners := make([]Listener[S, E], len(b.listeners))
	copy(frozenListeners, b.listeners)

	// 9. Create the immutable stateMachine.
	return &stateMachine[S, E]{
		initialState: b.initialState,
		config:       frozenConfig,
		listeners:    frozenListeners,
	}, nil
}

// detectSubstateCycle checks the parent chain starting at state for a circular reference.
func detectSubstateCycle[S, E comparable](config map[S]*stateRepresentation[S, E], state S) error {
	visited := make(map[S]bool)
	current := state
	for {
		if visited[current] {
			return &InvalidConfigurationError{
				Message: fmt.Sprintf("circular substate hierarchy detected involving state %v", current),
			}
		}
		visited[current] = true
		repr, ok := config[current]
		if !ok || repr.superstate == nil {
			break
		}
		current = repr.superstate.state
	}
	return nil
}

// isDescendantOf reports whether target is a descendant (direct or indirect) of state.
func isDescendantOf[S, E comparable](target *stateRepresentation[S, E], state S) bool {
	for current := target.superstate; current != nil; current = current.superstate {
		if current.state == state {
			return true
		}
	}
	return false
}

// detectInitialTransitionCycle checks the initialTransitionTarget chain starting
// at state for a cycle. If one exists (for example A.InitialTransition(B),
// B.InitialTransition(A)), it returns an InvalidConfigurationError.
func detectInitialTransitionCycle[S, E comparable](config map[S]*stateRepresentation[S, E], state S) error {
	visited := make(map[S]bool)
	current := state
	for {
		repr, ok := config[current]
		if !ok || repr.initialTransitionTarget == nil {
			return nil // Chain ends; no cycle.
		}
		next := *repr.initialTransitionTarget
		if visited[next] {
			return &InvalidConfigurationError{
				Message: fmt.Sprintf("circular initial transition detected starting from state %v", state),
			}
		}
		visited[current] = true
		current = next
	}
}
