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
	"testing"
)

// --- TestSubstateOf: substate inheritance ---
func TestSubstateOf(t *testing.T) {
	t.Run("child inherits parent transitions", func(t *testing.T) {
		// stateB is child of stateA. stateA has eventX->stateC.
		// When in stateB and fire eventX, should transition to stateC via inherited rule.
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Permit(eventX, stateC).
			Done().SubstateOf(stateB, stateA).
			Configure(stateB).Done().
			Configure(stateC).Done())

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateC {
			t.Errorf("got %v, want %v", got, stateC)
		}
	})

	t.Run("child own transition takes priority over parent", func(t *testing.T) {
		// stateB is child of stateA. Both handle eventX but go to different targets.
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Permit(eventX, stateC).
			Done().SubstateOf(stateB, stateA).
			Configure(stateB).Permit(eventX, stateD).
			Done().Configure(stateC).Done().Configure(stateD).Done())

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateD {
			t.Errorf("got %v, want %v (child should override parent)", got, stateD)
		}
	})

	t.Run("sibling transition executes exit/entry correctly", func(t *testing.T) {
		// stateB and stateC are children of stateA. Transition B->C.
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).OnEntry(mkAction("entry-A")).OnExit(mkAction("exit-A")).
			Done().SubstateOf(stateB, stateA).SubstateOf(stateC, stateA).
			Configure(stateB).Permit(eventX, stateC).
			OnEntry(mkAction("entry-B")).OnExit(mkAction("exit-B")).
			Done().Configure(stateC).
			OnEntry(mkAction("entry-C")).OnExit(mkAction("exit-C")).Done())

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateC {
			t.Errorf("got %v, want %v", got, stateC)
		}
		// Within same parent: exit-B, entry-C. Parent A should NOT exit/enter.
		if len(actions) != 2 {
			t.Fatalf("expected 2 actions, got %v", actions)
		}
		if actions[0] != "exit-B" || actions[1] != "entry-C" {
			t.Errorf("expected [exit-B, entry-C], got %v", actions)
		}
	})
}

// --- TestIsInState: hierarchy checks with IsInState ---
func TestIsInState(t *testing.T) {
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
		Configure(stateA).Done().
		SubstateOf(stateB, stateA).SubstateOf(stateC, stateB).
		Configure(stateB).Done().
		Configure(stateC).Done().
		Configure(stateD).Done())

	tests := []struct {
		name               string
		state              testState
		possibleSuperstate testState
		want               bool
	}{
		{"same state returns true", stateA, stateA, true},
		{"child IsInState parent returns true", stateB, stateA, true},
		{"unrelated state returns false", stateD, stateA, false},
		{"grandchild IsInState grandparent returns true", stateC, stateA, true},
		{"grandchild IsInState parent returns true", stateC, stateB, true},
		{"parent is not in child", stateA, stateB, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.IsInState(tt.state, tt.possibleSuperstate)
			if got != tt.want {
				t.Errorf("IsInState(%v, %v) = %v, want %v", tt.state, tt.possibleSuperstate, got, tt.want)
			}
		})
	}
}

// --- TestHierarchicalEntryExit: entry/exit ordering in hierarchical states ---
func TestHierarchicalEntryExit(t *testing.T) {
	t.Run("entering child from outside: parent entry then child entry", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		// stateD -> stateB (child of stateA)
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateD).
			Configure(stateA).OnEntry(mkAction("entry-A")).OnExit(mkAction("exit-A")).
			Done().SubstateOf(stateB, stateA).
			Configure(stateB).OnEntry(mkAction("entry-B")).OnExit(mkAction("exit-B")).
			Done().Configure(stateD).Permit(eventX, stateB).Done())

		got, err := sm.Fire(context.Background(), stateD, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}
		// Should enter parent A first, then child B
		if len(actions) != 2 || actions[0] != "entry-A" || actions[1] != "entry-B" {
			t.Errorf("expected [entry-A, entry-B], got %v", actions)
		}
	})

	t.Run("exiting child to outside: child exit then parent exit", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		// stateB (child of stateA) -> stateD
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).OnEntry(mkAction("entry-A")).OnExit(mkAction("exit-A")).
			Done().SubstateOf(stateB, stateA).
			Configure(stateB).Permit(eventX, stateD).
			OnEntry(mkAction("entry-B")).OnExit(mkAction("exit-B")).
			Done().Configure(stateD).OnEntry(mkAction("entry-D")).Done())

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateD {
			t.Errorf("got %v, want %v", got, stateD)
		}
		// Should exit child B first, then parent A, then enter D
		if len(actions) != 3 || actions[0] != "exit-B" || actions[1] != "exit-A" || actions[2] != "entry-D" {
			t.Errorf("expected [exit-B, exit-A, entry-D], got %v", actions)
		}
	})

	t.Run("sibling transition under same parent does not exit/enter parent", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).OnEntry(mkAction("entry-A")).OnExit(mkAction("exit-A")).
			Done().SubstateOf(stateB, stateA).SubstateOf(stateC, stateA).
			Configure(stateB).Permit(eventX, stateC).
			OnExit(mkAction("exit-B")).
			Done().Configure(stateC).
			OnEntry(mkAction("entry-C")).Done())

		sm.Fire(context.Background(), stateB, eventX)
		// Parent should not be involved
		for _, a := range actions {
			if a == "entry-A" || a == "exit-A" {
				t.Errorf("parent action %q should not execute for sibling transition, actions=%v", a, actions)
			}
		}
		if len(actions) != 2 || actions[0] != "exit-B" || actions[1] != "entry-C" {
			t.Errorf("expected [exit-B, entry-C], got %v", actions)
		}
	})
}

// --- TestInitialTransition: InitialTransition behavior ---
func TestInitialTransition(t *testing.T) {
	t.Run("entering composite state auto-transitions to initial child", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		// stateD -> stateA (composite, initial=stateB)
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateD).
			Configure(stateA).OnEntry(mkAction("entry-A")).Done().
			InitialTransition(stateA, stateB).SubstateOf(stateB, stateA).
			Configure(stateB).OnEntry(mkAction("entry-B")).Done().
			Configure(stateD).Permit(eventX, stateA).Done())

		got, err := sm.Fire(context.Background(), stateD, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should end up in stateB (initial child of stateA)
		if got != stateB {
			t.Errorf("got %v, want %v (initial transition target)", got, stateB)
		}
		// Entry order: stateA first, then stateB
		if len(actions) != 2 || actions[0] != "entry-A" || actions[1] != "entry-B" {
			t.Errorf("expected [entry-A, entry-B], got %v", actions)
		}
	})

	t.Run("nested initial transitions chain correctly", func(t *testing.T) {
		// stateA has initial->stateB, stateB has initial->stateC
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateD).
			Configure(stateA).Done().
			InitialTransition(stateA, stateB).SubstateOf(stateB, stateA).
			InitialTransition(stateB, stateC).SubstateOf(stateC, stateB).
			Configure(stateB).Done().
			Configure(stateC).Done().
			Configure(stateD).Permit(eventX, stateA).Done())

		got, err := sm.Fire(context.Background(), stateD, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateC {
			t.Errorf("got %v, want %v (nested initial transition)", got, stateC)
		}
	})
}
