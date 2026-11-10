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
	"strings"
	"testing"
)

// --- Automatic registration of references at build time ---

func TestBuildAutoRegistersReferencedStates(t *testing.T) {
	t.Run("SubstateOf without Configure does not panic", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Done().
			SubstateOf(stateB, stateA))

		if !sm.IsInState(stateB, stateA) {
			t.Error("expected auto-registered substate B to be in A")
		}
	})

	t.Run("Permit destination without Configure appears in graph", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Done())

		dot := sm.ToDotGraph()
		if !strings.Contains(dot, `"B";`) {
			t.Errorf("expected auto-registered destination B as graph node, got:\n%s", dot)
		}
	})

	t.Run("InitialTransition target without Configure is registered", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Done().
			InitialTransition(stateA, stateB).SubstateOf(stateB, stateA).
			Configure(stateB).Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if !errors.Is(err, ErrTriggerNotPermitted) {
			t.Fatalf("expected TriggerNotPermitted for unconfigured event, got %v", err)
		}
		if got != stateA {
			t.Errorf("state should remain %v, got %v", stateA, got)
		}
	})
}

// --- Build is repeatable and does not mutate the Builder ---

func TestBuildIsRepeatable(t *testing.T) {
	t.Run("repeated Build does not duplicate substates", func(t *testing.T) {
		b := NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).Done()

		if _, err := b.Build(); err != nil {
			t.Fatalf("first Build failed: %v", err)
		}
		sm, err := b.Build()
		if err != nil {
			t.Fatalf("second Build failed: %v", err)
		}

		dot := sm.ToDotGraph()
		if n := countExactLines(dot, `"B";`); n != 1 {
			t.Errorf("expected substate B exactly once in DOT graph, got %d:\n%s", n, dot)
		}
		if n := strings.Count(dot, "cluster_A"); n != 1 {
			t.Errorf("expected cluster_A exactly once in DOT graph, got %d:\n%s", n, dot)
		}
	})

	t.Run("hierarchy changes after Build do not affect earlier StateMachine", func(t *testing.T) {
		b := NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Done().
			Configure(stateB).Done().
			SubstateOf(stateB, stateA)

		sm1, err := b.Build()
		if err != nil {
			t.Fatalf("first Build failed: %v", err)
		}

		b.Configure(stateC).Done()
		b.SubstateOf(stateC, stateA)
		sm2, err := b.Build()
		if err != nil {
			t.Fatalf("second Build failed: %v", err)
		}

		if sm1.IsInState(stateC, stateA) {
			t.Error("state C added after first Build must not exist in sm1")
		}
		if !sm2.IsInState(stateC, stateA) {
			t.Error("expected state C to be a substate of A in sm2")
		}
	})
}

// --- External self-transitions ---

func TestExternalSelfTransition(t *testing.T) {
	t.Run("Permit to same state executes exit and entry actions", func(t *testing.T) {
		var actions []string
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).
			Permit(eventX, stateA).
			OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "exit-A")
				return nil
			}).
			OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "entry-A")
				return nil
			}).
			Done())

		got, err := sm.Fire(context.Background(), stateA, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateA {
			t.Errorf("got %v, want %v", got, stateA)
		}
		if len(actions) != 2 || actions[0] != "exit-A" || actions[1] != "entry-A" {
			t.Errorf("expected [exit-A, entry-A], got %v", actions)
		}
	})

	t.Run("self transition reports IsReentry to listeners", func(t *testing.T) {
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateA).Done().
			WithListener(listener))

		if _, err := sm.Fire(context.Background(), stateA, eventX); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(listener.transitioning) != 1 || !listener.transitioning[0].IsReentry {
			t.Errorf("expected IsReentry=true, got %+v", listener.transitioning)
		}
	})

	t.Run("inherited PermitReentry reenters the actual current state", func(t *testing.T) {
		var actions []string
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).PermitReentry(eventX).Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).
			OnExit(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "exit-B")
				return nil
			}).
			OnEntry(func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, "entry-B")
				return nil
			}).
			Done())

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v (reentry should keep current state)", got, stateB)
		}
		if len(actions) != 2 || actions[0] != "exit-B" || actions[1] != "entry-B" {
			t.Errorf("expected [exit-B, entry-B], got %v", actions)
		}
	})
}

// --- Deep initial substates ---

func TestDeepInitialTransition(t *testing.T) {
	t.Run("enters all intermediate ancestors in parent-to-child order", func(t *testing.T) {
		var actions []string
		mkAction := func(name string) ActionFunc[testState, testEvent] {
			return func(_ context.Context, _ Transition[testState, testEvent]) error {
				actions = append(actions, name)
				return nil
			}
		}
		// stateC is a substate of stateB, and stateB is a substate of stateA.
		// stateA's initial transition points directly to its grandchild stateC.
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateD).
			Configure(stateA).OnEntry(mkAction("entry-A")).Done().
			InitialTransition(stateA, stateC).
			SubstateOf(stateB, stateA).
			SubstateOf(stateC, stateB).
			Configure(stateB).OnEntry(mkAction("entry-B")).Done().
			Configure(stateC).OnEntry(mkAction("entry-C")).Done().
			Configure(stateD).Permit(eventX, stateA).Done())

		got, err := sm.Fire(context.Background(), stateD, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateC {
			t.Errorf("got %v, want %v", got, stateC)
		}
		expected := []string{"entry-A", "entry-B", "entry-C"}
		if len(actions) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, actions)
		}
		for i := range expected {
			if actions[i] != expected[i] {
				t.Errorf("actions[%d] = %q, want %q (all: %v)", i, actions[i], expected[i], actions)
			}
		}
	})

	t.Run("InitialState resolves the initial transition chain", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Done().
			InitialTransition(stateA, stateB).
			SubstateOf(stateB, stateA).
			Configure(stateB).Done().
			InitialTransition(stateB, stateC).
			SubstateOf(stateC, stateB).
			Configure(stateC).Done())

		if got := sm.InitialState(); got != stateC {
			t.Errorf("InitialState() = %v, want %v", got, stateC)
		}
	})
}

// --- Inherited behaviors see the actual source state ---

func TestInheritedBehaviorSeesActualSource(t *testing.T) {
	t.Run("guard receives actual current state as Source", func(t *testing.T) {
		var guardSource testState
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).PermitIf(eventX, stateC,
			func(_ context.Context, tr Transition[testState, testEvent]) bool {
				guardSource = tr.Source
				return true
			}).Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).Done().
			Configure(stateC).Done())

		if _, err := sm.Fire(context.Background(), stateB, eventX); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if guardSource != stateB {
			t.Errorf("guard Source = %v, want %v (actual current state)", guardSource, stateB)
		}
	})

	t.Run("inherited internal transition reports actual source", func(t *testing.T) {
		var actionSource testState
		listener := &testListener{}
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).
			InternalTransition(eventX, func(_ context.Context, tr Transition[testState, testEvent]) error {
				actionSource = tr.Source
				return nil
			}).
			Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).Done().
			WithListener(listener))

		got, err := sm.Fire(context.Background(), stateB, eventX)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != stateB {
			t.Errorf("got %v, want %v", got, stateB)
		}
		if actionSource != stateB {
			t.Errorf("action Source = %v, want %v", actionSource, stateB)
		}
		if len(listener.transitioned) != 1 || listener.transitioned[0].Destination != stateB {
			t.Errorf("expected internal transition destination B, got %+v", listener.transitioned)
		}
	})
}

// --- CanFire / PermittedTriggers support args and context ---

func TestCanFireWithArgsAndContext(t *testing.T) {
	type ctxKey string
	const tenantKey ctxKey = "tenant"

	// eventX's guard depends on args; eventY's guard depends on context.
	argsGuard := func(_ context.Context, tr Transition[testState, testEvent]) bool {
		return len(tr.Args) > 0 && tr.Args[0] == "ok"
	}
	ctxGuard := func(ctx context.Context, _ Transition[testState, testEvent]) bool {
		return ctx.Value(tenantKey) == "acme"
	}
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).
		PermitIf(eventX, stateB, argsGuard).
		PermitIf(eventY, stateC, ctxGuard).
		Done().
		Configure(stateB).Done().
		Configure(stateC).Done())

	t.Run("CanFire passes args", func(t *testing.T) {
		if sm.CanFire(stateA, eventX) {
			t.Error("expected CanFire=false without args")
		}
		if !sm.CanFire(stateA, eventX, "ok") {
			t.Error("expected CanFire=true with matching args")
		}
	})

	t.Run("CanFireContext passes context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), tenantKey, "acme")
		if !sm.CanFireContext(ctx, stateA, eventY) {
			t.Error("expected CanFireContext=true with matching context")
		}
		if sm.CanFireContext(context.Background(), stateA, eventY) {
			t.Error("expected CanFireContext=false without context value")
		}
	})

	t.Run("PermittedTriggers passes args", func(t *testing.T) {
		if got := sm.PermittedTriggers(stateA); len(got) != 0 {
			t.Errorf("expected no triggers without args/context, got %v", got)
		}
		got := sm.PermittedTriggers(stateA, "ok")
		if len(got) != 1 || got[0] != eventX {
			t.Errorf("expected [X] with matching args, got %v", got)
		}
	})

	t.Run("PermittedTriggersContext passes context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), tenantKey, "acme")
		got := sm.PermittedTriggersContext(ctx, stateA)
		if len(got) != 1 || got[0] != eventY {
			t.Errorf("expected [Y] with matching context, got %v", got)
		}
	})
}

// countExactLines counts lines whose trimmed content equals want.
func countExactLines(s, want string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == want {
			count++
		}
	}
	return count
}

func TestPermittedTriggersOrderIsDeterministic(t *testing.T) {
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).Permit(eventX, stateB).Permit(eventY, stateC).Done().
		SubstateOf(stateA, stateD).
		Configure(stateD).Permit(eventZ, stateC).Done().
		Configure(stateB).Done().
		Configure(stateC).Done())

	want := []testEvent{eventX, eventY, eventZ}
	for i := 0; i < 20; i++ {
		got := sm.PermittedTriggers(stateA)
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("iteration %d: got %v, want %v (child events first, registration order)", i, got, want)
			}
		}
	}
}

// --- Build-time configuration validation ---

func TestBuildValidationOfNilConfig(t *testing.T) {
	var nilGuard GuardFunc[testState, testEvent]
	var nilSelector DynamicSelectorFunc[testState, testEvent]
	var nilAction ActionFunc[testState, testEvent]

	tests := []struct {
		name  string
		build func() (StateMachine[testState, testEvent], error)
	}{
		{
			name: "nil guard",
			build: func() (StateMachine[testState, testEvent], error) {
				return NewBuilder[testState, testEvent](stateA).
					Configure(stateA).PermitIf(eventX, stateB, nilGuard).Done().
					Build()
			},
		},
		{
			name: "nil dynamic selector",
			build: func() (StateMachine[testState, testEvent], error) {
				return NewBuilder[testState, testEvent](stateA).
					Configure(stateA).PermitDynamic(eventX, nilSelector).Done().
					Build()
			},
		},
		{
			name: "nil entry action",
			build: func() (StateMachine[testState, testEvent], error) {
				return NewBuilder[testState, testEvent](stateA).
					Configure(stateA).OnEntry(nilAction).Done().
					Build()
			},
		},
		{
			name: "nil exit action",
			build: func() (StateMachine[testState, testEvent], error) {
				return NewBuilder[testState, testEvent](stateA).
					Configure(stateA).OnExit(nilAction).Done().
					Build()
			},
		},
		{
			name: "nil listener",
			build: func() (StateMachine[testState, testEvent], error) {
				return NewBuilder[testState, testEvent](stateA).
					Configure(stateA).Done().
					WithListener(nil).
					Build()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm, err := tt.build()
			if err == nil {
				t.Fatalf("expected error, got StateMachine %v", sm)
			}
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Errorf("expected ErrInvalidConfiguration, got %v", err)
			}
		})
	}
}

func TestInitialTransitionTargetMustBeSubstate(t *testing.T) {
	_, err := NewBuilder[testState, testEvent](stateA).
		Configure(stateA).Done().
		InitialTransition(stateA, stateB).
		Configure(stateB).Done().
		Build()

	if err == nil {
		t.Fatal("expected error for initial target that is not a substate")
	}
	if !errors.Is(err, ErrInvalidConfiguration) {
		t.Errorf("expected ErrInvalidConfiguration, got %v", err)
	}
}

// --- Configuration immutability ---

func TestGuardsAreCopiedAtConfiguration(t *testing.T) {
	guards := []GuardFunc[testState, testEvent]{
		func(_ context.Context, _ Transition[testState, testEvent]) bool { return true },
	}
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).PermitIf(eventX, stateB, guards...).Done().
		Configure(stateB).Done())

	// Mutating the caller's slice must not affect the built StateMachine.
	guards[0] = func(_ context.Context, _ Transition[testState, testEvent]) bool { return false }

	if !sm.CanFire(stateA, eventX) {
		t.Error("mutating the caller's guard slice must not affect the built StateMachine")
	}
}

// --- Graph export fixes ---

// displayState verifies that display names with parentheses and CJK characters
// can be safely exported to a Mermaid graph.
type displayState int

const (
	displayPending displayState = iota
	displayPaid
	displayShipped
)

func (s displayState) String() string {
	return [...]string{"Pending(待付款)", "Paid(已付款)", "Shipped(已发货)"}[s]
}

type displayEvent int

const (
	displayPay displayEvent = iota
	displayShip
)

func (e displayEvent) String() string {
	return [...]string{"Pay(付款)", "Ship(发货)"}[e]
}

// trickyState verifies that IDs are automatically de-duplicated when distinct
// display names sanitize to the same identifier.
type trickyState int

func (s trickyState) String() string {
	return [...]string{"A B", "A-B"}[s]
}

type trickyEvent int

func (e trickyEvent) String() string { return "go" }

func TestMermaidIDsAreUnique(t *testing.T) {
	sm, err := NewBuilder[trickyState, trickyEvent](0).
		Configure(0).Permit(0, 1).Done().
		Configure(1).Done().
		Build()
	if err != nil {
		t.Fatalf("Build() returned unexpected error: %v", err)
	}

	mermaid := sm.ToMermaidGraph()
	// Both "A B" and "A-B" sanitize to A_B; the second must receive a suffix.
	if !strings.Contains(mermaid, `state "A B" as A_B`) {
		t.Errorf("expected A_B for first state:\n%s", mermaid)
	}
	if !strings.Contains(mermaid, `state "A-B" as A_B_2`) {
		t.Errorf("expected A_B_2 for colliding second state:\n%s", mermaid)
	}
}

func TestMermaidGraphSanitizesDisplayNames(t *testing.T) {
	sm, err := NewBuilder[displayState, displayEvent](displayPending).
		Configure(displayPending).Permit(displayPay, displayPaid).Done().
		SubstateOf(displayPaid, displayPending).
		Configure(displayPaid).Permit(displayShip, displayShipped).Done().
		Configure(displayShipped).Done().
		Build()
	if err != nil {
		t.Fatalf("Build() returned unexpected error: %v", err)
	}

	mermaid := sm.ToMermaidGraph()

	// Display names must not be used directly as Mermaid identifiers.
	if strings.Contains(mermaid, "--> Pending(") {
		t.Errorf("display name with parentheses must not be used as Mermaid ID:\n%s", mermaid)
	}
	if strings.Contains(mermaid, "state Pending(") {
		t.Errorf("display name with parentheses must not be used in state declaration:\n%s", mermaid)
	}
	// Display names must be preserved via an as-alias.
	if !strings.Contains(mermaid, `state "Pending(待付款)" as Pending`) {
		t.Errorf("expected Pending display name in state declaration:\n%s", mermaid)
	}
	// Leaf substates must be declared inside the parent block.
	if !strings.Contains(mermaid, `state "Paid(已付款)" as Paid`) {
		t.Errorf("expected nested Paid state declaration:\n%s", mermaid)
	}
	// Top-level leaf states must also be declared.
	if !strings.Contains(mermaid, `state "Shipped(已发货)" as Shipped`) {
		t.Errorf("expected Shipped state declaration:\n%s", mermaid)
	}
}

func TestDotGraphNoDuplicateClusters(t *testing.T) {
	// A is the parent of B, and B is the parent of C.
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateC).
		Configure(stateA).Done().
		SubstateOf(stateB, stateA).
		SubstateOf(stateC, stateB).
		Configure(stateB).Done().
		Configure(stateC).Done())

	dot := sm.ToDotGraph()
	if n := strings.Count(dot, "cluster_A"); n != 1 {
		t.Errorf("expected cluster_A exactly once, got %d:\n%s", n, dot)
	}
	if n := strings.Count(dot, "cluster_B"); n != 1 {
		t.Errorf("expected cluster_B exactly once, got %d:\n%s", n, dot)
	}
}

func TestGuardedTransitionWithoutDescriptionHasNoEmptyBrackets(t *testing.T) {
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).PermitIf(eventX, stateB,
		func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }).
		Done().Configure(stateB).Done())

	if dot := sm.ToDotGraph(); strings.Contains(dot, "[]") {
		t.Errorf("DOT graph should not contain empty guard brackets:\n%s", dot)
	}
	if mermaid := sm.ToMermaidGraph(); strings.Contains(mermaid, "[]") {
		t.Errorf("Mermaid graph should not contain empty guard brackets:\n%s", mermaid)
	}
}

// --- Listeners see the final destination state ---

func TestListenerSeesResolvedDestination(t *testing.T) {
	listener := &testListener{}
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateD).
		Configure(stateA).Done().
		InitialTransition(stateA, stateB).
		SubstateOf(stateB, stateA).
		Configure(stateB).Done().
		Configure(stateD).Permit(eventX, stateA).Done().
		WithListener(listener))

	if _, err := sm.Fire(context.Background(), stateD, eventX); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, transitions := range map[string][]Transition[testState, testEvent]{
		"OnTransitioning": listener.transitioning,
		"OnTransitioned":  listener.transitioned,
	} {
		if len(transitions) != 1 {
			t.Fatalf("%s: expected 1 call, got %d", name, len(transitions))
		}
		if transitions[0].Destination != stateB {
			t.Errorf("%s: destination = %v, want resolved %v", name, transitions[0].Destination, stateB)
		}
	}
}
