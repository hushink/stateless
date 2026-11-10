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
	"testing"
)

// --- Test type definitions ---

type testState int

const (
	stateA testState = iota
	stateB
	stateC
	stateD
)

func (s testState) String() string {
	return [...]string{"A", "B", "C", "D"}[s]
}

type testEvent int

const (
	eventX testEvent = iota
	eventY
	eventZ
)

func (e testEvent) String() string {
	return [...]string{"X", "Y", "Z"}[e]
}

// mustBuild builds a state machine and calls t.Fatal on failure.
func mustBuild(t *testing.T, b Builder[testState, testEvent]) StateMachine[testState, testEvent] {
	t.Helper()
	sm, err := b.Build()
	if err != nil {
		t.Fatalf("Build() returned unexpected error: %v", err)
	}
	return sm
}

// --- TestFire: basic transitions ---
func TestFire(t *testing.T) {
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).Permit(eventX, stateB).
		Done().Configure(stateB).Permit(eventY, stateA).Done())

	t.Run("simple Permit transition succeeds", func(t *testing.T) {
		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}
	})

	t.Run("Fire returns correct new state for chained transitions", func(t *testing.T) {
		s1, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		s2, err := sm.Fire(context.Background(), s1, eventY)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s2 != stateA {
			t.Errorf("got %v, want %v", s2, stateA)
		}
	})

	t.Run("unconfigured event returns TriggerNotPermittedError", func(t *testing.T) {
		got, err := sm.Fire(context.Background(), stateA, eventZ)
		if got != stateA {
			t.Errorf("state should remain %v, got %v", stateA, got)
		}
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
		var tnpe *TriggerNotPermittedError[testState, testEvent]
		if !errors.As(err, &tnpe) {
			t.Errorf("expected TriggerNotPermittedError type, got %T", err)
		}
	})

	t.Run("unregistered state returns TriggerNotPermittedError", func(t *testing.T) {
		got, err := sm.Fire(context.Background(), stateD, eventX)
		if got != stateD {
			t.Errorf("state should remain %v, got %v", stateD, got)
		}
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
	})
}

// --- TestGuard: guard conditions ---
func TestGuard(t *testing.T) {
	t.Run("PermitIf guard passes allows transition", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, func(_ context.Context, _ Transition[testState, testEvent]) bool {
			return true
		}).Done().Configure(stateB).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}
	})

	t.Run("PermitIf guard rejects returns TriggerNotPermittedError", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, func(_ context.Context, _ Transition[testState, testEvent]) bool {
			return false
		}).Done().Configure(stateB).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
		if got != stateA {
			t.Errorf("state should remain %v, got %v", stateA, got)
		}
	})

	t.Run("multiple guards must all pass", func(t *testing.T) {
		callCount := 0
		g1 := func(_ context.Context, _ Transition[testState, testEvent]) bool { callCount++; return true }
		g2 := func(_ context.Context, _ Transition[testState, testEvent]) bool { callCount++; return false }

		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, g1, g2).
			Done().Configure(stateB).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
		if callCount != 2 {
			t.Errorf("expected 2 guard calls, got %d", callCount)
		}
	})

	t.Run("multiple PermitIf same event first match wins", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).
			PermitIf(eventX, stateB, func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }).
			PermitIf(eventX, stateC, func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }).
			Done().Configure(stateB).Done().Configure(stateC).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("expected first matching target %v, got %v", stateB, got)
		}
	})
}

// --- TestDynamic: dynamic transitions ---
func TestDynamic(t *testing.T) {
	t.Run("PermitDynamic selects target based on selector", func(t *testing.T) {
		target := stateB
		sel := func(_ context.Context, _ Transition[testState, testEvent]) testState { return target }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitDynamic(eventX, sel).
			Done().Configure(stateB).Done().Configure(stateC).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}

		target = stateC
		got, err = sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateC {
			t.Errorf("got %v, want %v", got, stateC)
		}
	})

	t.Run("PermitDynamicIf guard passes uses selector", func(t *testing.T) {
		sel := func(_ context.Context, _ Transition[testState, testEvent]) testState { return stateB }
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitDynamicIf(eventX, sel, guard).
			Done().Configure(stateB).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}
	})

	t.Run("PermitDynamicIf guard rejects does not transition", func(t *testing.T) {
		sel := func(_ context.Context, _ Transition[testState, testEvent]) testState { return stateB }
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitDynamicIf(eventX, sel, guard).
			Done().Configure(stateB).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
		if got != stateA {
			t.Errorf("state should remain %v, got %v", stateA, got)
		}
	})
}

// --- TestInternalTransition: internal transitions ---
func TestInternalTransition(t *testing.T) {
	noop := func(_ context.Context, _ Transition[testState, testEvent]) error { return nil }

	t.Run("does not change state", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).InternalTransition(eventX, noop).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
	})

	t.Run("does not trigger entry/exit actions", func(t *testing.T) {
		var actions []string
		act := func(_ context.Context, _ Transition[testState, testEvent]) error {
			actions = append(actions, "internal")
			return nil
		}
		entry := func(_ context.Context, _ Transition[testState, testEvent]) error {
			actions = append(actions, "entry-A")
			return nil
		}
		exit := func(_ context.Context, _ Transition[testState, testEvent]) error {
			actions = append(actions, "exit-A")
			return nil
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).InternalTransition(eventX, act).OnEntry(entry).OnExit(exit).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if len(actions) != 1 || actions[0] != "internal" {
			t.Errorf("expected [internal], got %v", actions)
		}
	})

	t.Run("action is executed", func(t *testing.T) {
		executed := false
		act := func(_ context.Context, _ Transition[testState, testEvent]) error {
			executed = true
			return nil
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).InternalTransition(eventX, act).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if !executed {
			t.Error("internal action was not executed")
		}
	})

	t.Run("InternalTransitionIf guard controls execution", func(t *testing.T) {
		executed := false
		act := func(_ context.Context, _ Transition[testState, testEvent]) error {
			executed = true
			return nil
		}
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).InternalTransitionIf(eventX, act, guard).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
		if executed {
			t.Error("action should not execute when guard rejects")
		}
	})
}

// --- TestReentry: reentry transitions ---
func TestReentry(t *testing.T) {
	t.Run("PermitReentry returns same state", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitReentry(eventX).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
	})

	t.Run("PermitReentry triggers exit and entry actions", func(t *testing.T) {
		var actions []string
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitReentry(eventX).
			OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "exit-A")
				return nil
			}).
			OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "entry-A")
				return nil
			}).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if len(actions) != 2 || actions[0] != "exit-A" || actions[1] != "entry-A" {
			t.Errorf("expected [exit-A, entry-A], got %v", actions)
		}
	})

	t.Run("PermitReentryIf guard controls reentry", func(t *testing.T) {
		allow := false
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return allow }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitReentryIf(eventX, guard).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}

		allow = true
		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
	})
}

// --- TestIgnore: ignored events ---
func TestIgnore(t *testing.T) {
	t.Run("Ignore keeps state unchanged no error", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Ignore(eventX).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
	})

	t.Run("IgnoreIf guard passes ignores event", func(t *testing.T) {
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).IgnoreIf(eventX, guard).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
	})

	t.Run("IgnoreIf guard rejects returns error", func(t *testing.T) {
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).IgnoreIf(eventX, guard).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Errorf("expected ErrTriggerNotPermitted, got %v", err)
		}
	})
}

// --- TestActions: entry/exit actions ---
func TestActions(t *testing.T) {
	t.Run("OnEntry executes when entering state", func(t *testing.T) {
		entered := false
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
			entered = true
			return nil
		}).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if !entered {
			t.Error("OnEntry was not executed")
		}
	})

	t.Run("OnExit executes when exiting state", func(t *testing.T) {
		exited := false
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
			exited = true
			return nil
		}).Done().Configure(stateB).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if !exited {
			t.Error("OnExit was not executed")
		}
	})

	t.Run("exit executes before entry", func(t *testing.T) {
		var actions []string
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
			actions = append(actions, "exit-A")
			return nil
		}).Done().Configure(stateB).OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
			actions = append(actions, "entry-B")
			return nil
		}).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if len(actions) != 2 || actions[0] != "exit-A" || actions[1] != "entry-B" {
			t.Errorf("expected [exit-A, entry-B], got %v", actions)
		}
	})

	t.Run("multiple entry/exit actions execute in order", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			OnExit(mkAction("exit-A-1")).OnExit(mkAction("exit-A-2")).
			Done().Configure(stateB).OnEntry(mkAction("entry-B-1")).OnEntry(mkAction("entry-B-2")).Done())

		sm.Fire(context.Background(), stateA, eventX)
		expected := []string{"exit-A-1", "exit-A-2", "entry-B-1", "entry-B-2"}
		if len(actions) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, actions)
		}
		for i := range expected {
			if actions[i] != expected[i] {
				t.Errorf("actions[%d] = %q, want %q", i, actions[i], expected[i])
			}
		}
	})

	t.Run("OnEntryFrom fires only for specific event", func(t *testing.T) {
		var actions []string
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Permit(eventY, stateB).
			Done().Configure(stateB).
			OnEntryFrom(eventX, func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "entry-from-X")
				return nil
			}).
			OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "entry-B")
				return nil
			}).Done())

		sm.Fire(context.Background(), stateA, eventX)
		if len(actions) != 2 || actions[0] != "entry-from-X" || actions[1] != "entry-B" {
			t.Errorf("from eventX: expected [entry-from-X, entry-B], got %v", actions)
		}

		actions = nil
		sm.Fire(context.Background(), stateA, eventY)
		if len(actions) != 1 || actions[0] != "entry-B" {
			t.Errorf("from eventY: expected [entry-B], got %v", actions)
		}
	})

	t.Run("entry action error propagates to Fire", func(t *testing.T) {
		actionErr := fmt.Errorf("action failed")
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
			return actionErr
		}).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, actionErr) {
			t.Errorf("expected action error, got %v", err)
		}
	})

	t.Run("exit action error propagates to Fire", func(t *testing.T) {
		exitErr := fmt.Errorf("exit failed")
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
			return exitErr
		}).Done().Configure(stateB).Done())

		_, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, exitErr) {
			t.Errorf("expected exit error, got %v", err)
		}
	})
}

// --- TestCanFire: CanFire ---
func TestCanFire(t *testing.T) {
	rejectGuard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).Permit(eventX, stateB).PermitIf(eventY, stateC, rejectGuard).
		Done().Configure(stateB).Done().Configure(stateC).Done())

	tests := []struct {
		name  string
		state testState
		event testEvent
		want  bool
	}{
		{"configured event returns true", stateA, eventX, true},
		{"unconfigured event returns false", stateA, eventZ, false},
		{"guard rejecting affects CanFire", stateA, eventY, false},
		{"unconfigured state returns false", stateD, eventX, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.CanFire(tt.state, tt.event)
			if got != tt.want {
				t.Errorf("CanFire(%v, %v) = %v, want %v", tt.state, tt.event, got, tt.want)
			}
		})
	}
}

// --- TestPermittedTriggers: PermittedTriggers ---
func TestPermittedTriggers(t *testing.T) {
	t.Run("returns all triggerable events", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Permit(eventY, stateC).
			Done().Configure(stateB).Done().Configure(stateC).Done())

		triggers := sm.PermittedTriggers(stateA)
		found := map[testEvent]bool{}
		for _, e := range triggers {
			found[e] = true
		}
		if !found[eventX] || !found[eventY] {
			t.Errorf("expected eventX and eventY, got %v", triggers)
		}
	})

	t.Run("includes events with passing guards", func(t *testing.T) {
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, guard).Done().Configure(stateB).Done())

		triggers := sm.PermittedTriggers(stateA)
		if len(triggers) != 1 || triggers[0] != eventX {
			t.Errorf("expected [eventX], got %v", triggers)
		}
	})

	t.Run("excludes events with rejecting guards", func(t *testing.T) {
		guard := func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, guard).Done().Configure(stateB).Done())

		triggers := sm.PermittedTriggers(stateA)
		if len(triggers) != 0 {
			t.Errorf("expected 0 triggers, got %v", triggers)
		}
	})

	t.Run("unconfigured state returns nil", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).Configure(stateA).Done())
		triggers := sm.PermittedTriggers(stateD)
		if triggers != nil {
			t.Errorf("expected nil, got %v", triggers)
		}
	})
}

// --- TestListener: listeners ---

type testListener struct {
	ListenerAdapter[testState, testEvent]
	transitioning []Transition[testState, testEvent]
	transitioned  []Transition[testState, testEvent]
	unhandled     []testEvent
}

func (l *testListener) OnTransitioning(_ context.Context, tr Transition[testState, testEvent]) {
	l.transitioning = append(l.transitioning, tr)
}
func (l *testListener) OnTransitioned(_ context.Context, tr Transition[testState, testEvent]) {
	l.transitioned = append(l.transitioned, tr)
}
func (l *testListener) OnUnhandledTrigger(_ context.Context, _ testState, e testEvent) {
	l.unhandled = append(l.unhandled, e)
}

func TestListener(t *testing.T) {
	t.Run("OnTransitioning called before transition", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().WithListener(listener))

		sm.Fire(context.Background(), stateA, eventX)
		if len(listener.transitioning) != 1 {
			t.Fatalf("expected 1 OnTransitioning call, got %d", len(listener.transitioning))
		}
		if listener.transitioning[0].Source != stateA || listener.transitioning[0].Event != eventX {
			t.Errorf("unexpected transitioning: %+v", listener.transitioning[0])
		}
	})

	t.Run("OnTransitioned called after transition", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().WithListener(listener))

		sm.Fire(context.Background(), stateA, eventX)
		if len(listener.transitioned) != 1 {
			t.Fatalf("expected 1 OnTransitioned call, got %d", len(listener.transitioned))
		}
		if listener.transitioned[0].Destination != stateB {
			t.Errorf("expected destination %v, got %v", stateB, listener.transitioned[0].Destination)
		}
	})

	t.Run("OnUnhandledTrigger called on unhandled event", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).
			Done().WithListener(listener))

		sm.Fire(context.Background(), stateA, eventX)
		if len(listener.unhandled) != 1 || listener.unhandled[0] != eventX {
			t.Errorf("expected unhandled [eventX], got %v", listener.unhandled)
		}
	})

	t.Run("multiple listeners all notified", func(t *testing.T) {
		l1, l2 := &testListener{}, &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).
			Done().WithListener(l1).WithListener(l2))

		sm.Fire(context.Background(), stateA, eventX)
		if len(l1.transitioned) != 1 || len(l2.transitioned) != 1 {
			t.Errorf("expected both listeners notified, l1=%d l2=%d",
				len(l1.transitioned), len(l2.transitioned))
		}
	})
}

// --- TestFireWithArgs: Fire with arguments ---
func TestFireWithArgs(t *testing.T) {
	t.Run("args passed to guard and entry action", func(t *testing.T) {
		var guardArgs, actionArgs []any
		guard := func(_ context.Context, tr Transition[testState, testEvent]) bool {
			guardArgs = tr.Args
			return true
		}
		entry := func(_ context.Context, tr Transition[testState, testEvent]) error {
			actionArgs = tr.Args
			return nil
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB, guard).
			Done().Configure(stateB).OnEntry(entry).Done())

		sm.Fire(context.Background(), stateA, eventX, "hello", 42)

		if len(guardArgs) != 2 || guardArgs[0] != "hello" || guardArgs[1] != 42 {
			t.Errorf("guard args = %v, want [hello 42]", guardArgs)
		}
		if len(actionArgs) != 2 || actionArgs[0] != "hello" || actionArgs[1] != 42 {
			t.Errorf("action args = %v, want [hello 42]", actionArgs)
		}
	})
}
