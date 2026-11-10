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

// Listener defines the event listener interface for the state machine. It observes
// transition lifecycle events, including callbacks before and after transitions
// and for unhandled events.
type Listener[S, E comparable] interface {
	// OnTransitioning is called just before a transition occurs (before entry/exit actions run).
	OnTransitioning(ctx context.Context, t Transition[S, E])
	// OnTransitioned is called after a transition completes (after all entry/exit actions run).
	OnTransitioned(ctx context.Context, t Transition[S, E])
	// OnUnhandledTrigger is called when an event is not handled by any transition.
	OnUnhandledTrigger(ctx context.Context, state S, event E)
}

// ListenerAdapter provides a no-op implementation of the Listener interface.
// Embed this type and override only the methods you care about instead of
// implementing every interface method.
type ListenerAdapter[S, E comparable] struct{}

// OnTransitioning is a no-op implementation.
func (a ListenerAdapter[S, E]) OnTransitioning(_ context.Context, _ Transition[S, E]) {}

// OnTransitioned is a no-op implementation.
func (a ListenerAdapter[S, E]) OnTransitioned(_ context.Context, _ Transition[S, E]) {}

// OnUnhandledTrigger is a no-op implementation.
func (a ListenerAdapter[S, E]) OnUnhandledTrigger(_ context.Context, _ S, _ E) {}
