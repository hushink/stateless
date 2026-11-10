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
	"testing"
)

// --- TestBuilderChaining: fluent chaining ---
func TestBuilderChaining(t *testing.T) {
	t.Run("full chain NewBuilder->Configure->Permit->Configure->Permit->Build", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Permit(eventY, stateC).
			Done().Configure(stateC).Done())

		// Verify the built state machine works correctly
		s, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != stateB {
			t.Errorf("got %v, want %v", s, stateB)
		}

		s, err = sm.Fire(context.Background(), s, eventY)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != stateC {
			t.Errorf("got %v, want %v", s, stateC)
		}
	})

	t.Run("Configure from StateConfigurator delegates to Builder", func(t *testing.T) {
		// Using Configure on StateConfigurator to switch to another state
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Permit(eventX, stateC).
			Done().Configure(stateC).Done())

		s, _ := sm.Fire(context.Background(), stateA, eventX)
		s, _ = sm.Fire(context.Background(), s, eventX)
		if s != stateC {
			t.Errorf("got %v, want %v", s, stateC)
		}
	})
}

// --- TestBuilderValidation: configuration validation ---
func TestBuilderValidation(t *testing.T) {
	t.Run("Build succeeds for valid configuration", func(t *testing.T) {
		_, err := NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Build returns usable StateMachine", func(t *testing.T) {
		sm, err := NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sm == nil {
			t.Fatal("Build() returned nil StateMachine")
		}
		if !sm.CanFire(stateA, eventX) {
			t.Error("expected CanFire to be true")
		}
	})

	t.Run("circular substate hierarchy detected", func(t *testing.T) {
		// A substate of B, B substate of A → circular
		_, err := NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Done().SubstateOf(stateA, stateB).
			Configure(stateB).Done().SubstateOf(stateB, stateA).
			Build()
		if err == nil {
			t.Fatal("expected error for circular substate")
		}
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Errorf("expected ErrInvalidConfiguration, got %v", err)
		}
		var ice *InvalidConfigurationError
		if !errors.As(err, &ice) {
			t.Errorf("expected InvalidConfigurationError type, got %T", err)
		}
	})

	t.Run("initial state not configured returns error", func(t *testing.T) {
		// Build without configuring the initial state
		_, err := NewBuilder[testState, testEvent](stateA).
			Configure(stateB).Permit(eventX, stateC).
			Done().Configure(stateC).
			Done().Build()
		if err == nil {
			t.Fatal("expected error for unconfigured initial state")
		}
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Errorf("expected ErrInvalidConfiguration, got %v", err)
		}
	})
}

// --- TestBuilderIsolation: Builder changes after Build do not affect an existing StateMachine ---
func TestBuilderIsolation(t *testing.T) {
	t.Run("modifying builder after Build does not affect StateMachine", func(t *testing.T) {
		// 1. Build a Builder and create a StateMachine from it.
		b := NewBuilder[testState, testEvent](stateA)
		b.Configure(stateA).Permit(eventX, stateB)
		b.Configure(stateB)

		sm, err := b.Build()
		if err != nil {
			t.Fatalf("Build() returned unexpected error: %v", err)
		}

		// 2. Keep modifying the Builder after Build: add a new transition for stateB.
		b.Configure(stateB).Permit(eventY, stateC)
		b.Configure(stateC)

		// 3. Verify the existing StateMachine behavior is unaffected:
		//    stateA -> eventX -> stateB still works.
		s, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s != stateB {
			t.Errorf("got %v, want %v", s, stateB)
		}

		//    stateB must not handle eventY in the original StateMachine.
		if sm.CanFire(stateB, eventY) {
			t.Error("expected CanFire(stateB, eventY) to be false on original StateMachine after builder modification")
		}
	})
}

// --- TestInitialTransitionCycleDetection: InitialTransition cycle detection ---
func TestInitialTransitionCycleDetection(t *testing.T) {
	t.Run("circular initial transition A->B->A returns error", func(t *testing.T) {
		_, err := NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Done().InitialTransition(stateA, stateB).
			Configure(stateB).Done().InitialTransition(stateB, stateA).
			Build()
		if err == nil {
			t.Fatal("expected error for circular initial transition")
		}
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Errorf("expected ErrInvalidConfiguration, got %v", err)
		}
		var ice *InvalidConfigurationError
		if !errors.As(err, &ice) {
			t.Errorf("expected InvalidConfigurationError type, got %T", err)
		}
	})

	t.Run("non-circular initial transition succeeds", func(t *testing.T) {
		_, err := NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Done().InitialTransition(stateA, stateB).
			Configure(stateB).Done().SubstateOf(stateB, stateA).
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- TestBuilderWithListener: WithListener ---
func TestBuilderWithListener(t *testing.T) {
	t.Run("listener added via Builder.WithListener is notified on Fire", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			WithListener(listener).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if len(listener.transitioned) != 1 {
			t.Errorf("expected 1 OnTransitioned call, got %d", len(listener.transitioned))
		}
	})

	t.Run("listener added via StateConfigurator.WithListener is notified", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().WithListener(listener))

		sm.Fire(context.Background(), stateA, eventX)
		if len(listener.transitioned) != 1 {
			t.Errorf("expected 1 OnTransitioned call, got %d", len(listener.transitioned))
		}
	})

	t.Run("multiple listeners added via different paths all notified", func(t *testing.T) {
		l1, l2 := &testListener{}, &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			WithListener(l1).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().WithListener(l2))

		sm.Fire(context.Background(), stateA, eventX)
		if len(l1.transitioned) != 1 {
			t.Errorf("l1: expected 1 call, got %d", len(l1.transitioned))
		}
		if len(l2.transitioned) != 1 {
			t.Errorf("l2: expected 1 call, got %d", len(l2.transitioned))
		}
	})
}
