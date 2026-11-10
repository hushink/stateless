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
	"strings"
	"testing"
)

// --- TestToDotGraph: DOT graph export ---
func TestToDotGraph(t *testing.T) {
	t.Run("simple state machine generates valid DOT format", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Permit(eventY, stateA).Done())

		dot := sm.ToDotGraph()

		// Basic structure checks
		if !strings.Contains(dot, "digraph {") {
			t.Error("DOT output should contain 'digraph {'")
		}
		if !strings.Contains(dot, "rankdir=LR") {
			t.Error("DOT output should contain 'rankdir=LR'")
		}
	})

	t.Run("contains initial state pointer", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Done())

		dot := sm.ToDotGraph()

		if !strings.Contains(dot, "__initial__") {
			t.Error("DOT output should contain __initial__ node")
		}
		if !strings.Contains(dot, `__initial__ -> "A"`) {
			t.Errorf("DOT output should point initial to A, got:\n%s", dot)
		}
	})

	t.Run("contains transition edges with labels", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Done())

		dot := sm.ToDotGraph()

		if !strings.Contains(dot, `"A" -> "B"`) {
			t.Errorf("DOT output should contain A->B edge, got:\n%s", dot)
		}
		if !strings.Contains(dot, `label="X"`) {
			t.Errorf("DOT output should contain label X, got:\n%s", dot)
		}
	})

	t.Run("guarded transition label includes guard description", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitIf(eventX, stateB,
			func(_ context.Context, _ Transition[testState, testEvent]) bool { return true }).
			Done().Configure(stateB).Done())

		dot := sm.ToDotGraph()

		// guardedBehavior with empty description produces "X []"
		if !strings.Contains(dot, `"A" -> "B"`) {
			t.Errorf("DOT output should contain edge A->B, got:\n%s", dot)
		}
	})

	t.Run("reentry produces self-loop edge", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).PermitReentry(eventX).Done())

		dot := sm.ToDotGraph()

		if !strings.Contains(dot, `"A" -> "A"`) {
			t.Errorf("DOT output should contain self-loop A->A, got:\n%s", dot)
		}
	})

	t.Run("hierarchical state uses subgraph cluster", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).Done())

		dot := sm.ToDotGraph()

		if !strings.Contains(dot, "subgraph") {
			t.Errorf("DOT output should contain subgraph for hierarchy, got:\n%s", dot)
		}
		if !strings.Contains(dot, "cluster_") {
			t.Errorf("DOT output should contain cluster_ prefix, got:\n%s", dot)
		}
	})
}

// --- TestToMermaidGraph: Mermaid graph export ---
func TestToMermaidGraph(t *testing.T) {
	t.Run("simple state machine generates valid Mermaid format", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Permit(eventY, stateA).Done())

		mermaid := sm.ToMermaidGraph()

		if !strings.Contains(mermaid, "stateDiagram-v2") {
			t.Error("Mermaid output should contain 'stateDiagram-v2'")
		}
	})

	t.Run("contains initial state pointer", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Done())

		mermaid := sm.ToMermaidGraph()

		if !strings.Contains(mermaid, "[*] --> A") {
			t.Errorf("Mermaid output should contain '[*] --> A', got:\n%s", mermaid)
		}
	})

	t.Run("contains transitions with labels", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).
			Done().Configure(stateB).Done())

		mermaid := sm.ToMermaidGraph()

		if !strings.Contains(mermaid, "A --> B : X") {
			t.Errorf("Mermaid output should contain 'A --> B : X', got:\n%s", mermaid)
		}
	})

	t.Run("hierarchical state uses nested state block", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Done().
			SubstateOf(stateB, stateA).
			Configure(stateB).Done())

		mermaid := sm.ToMermaidGraph()

		if !strings.Contains(mermaid, "state A {") {
			t.Errorf("Mermaid output should contain 'state A {', got:\n%s", mermaid)
		}
	})

	t.Run("initial transition shown in nested state", func(t *testing.T) {
		sm := mustBuild(t, NewBuilder[testState, testEvent](stateB).
			Configure(stateA).Done().
			InitialTransition(stateA, stateB).SubstateOf(stateB, stateA).
			Configure(stateB).Done())

		mermaid := sm.ToMermaidGraph()

		// Check for the nested [*] --> B inside state A
		if !strings.Contains(mermaid, "[*] --> B") {
			t.Errorf("Mermaid output should contain nested '[*] --> B', got:\n%s", mermaid)
		}
	})
}
