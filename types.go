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

// GuardFunc defines a guard condition function. It is invoked before a transition
// executes; returning true permits the transition and returning false rejects it.
// Guard functions must not have side effects.
type GuardFunc[S, E comparable] func(ctx context.Context, t Transition[S, E]) bool

// ActionFunc defines an action function executed during a transition or on state
// entry/exit. Returning an error aborts the current transition and propagates the
// error to the caller.
type ActionFunc[S, E comparable] func(ctx context.Context, t Transition[S, E]) error

// DynamicSelectorFunc defines a dynamic target selection function, used to model
// the behavior of a Choice pseudostate. The target state is decided at runtime
// from the current context.
type DynamicSelectorFunc[S, E comparable] func(ctx context.Context, t Transition[S, E]) S

// Transition describes the complete context of a state transition.
type Transition[S, E comparable] struct {
	Source      S     // Source state
	Destination S     // Target state (final state after resolving InitialTransition chains)
	Event       E     // Triggering event
	Args        []any // Additional arguments
	IsReentry   bool  // Whether this is a reentry transition (final destination == source)
}
