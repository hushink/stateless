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
	"fmt"
	"sort"
	"strings"
)

// graphEdge represents a transition edge in the state graph.
type graphEdge struct {
	source string // Source state name
	target string // Target state name
	label  string // Edge label (event name and optional guard description)
}

// collectGraphEdges walks the state machine configuration and collects every
// drawable transition edge. Internal transitions and ignored behaviors produce
// no edges; dynamic transitions produce no edges because their target is unknown
// at build time. Reentry transitions produce self-loop edges. The returned edge
// list is sorted by source, target, and label for deterministic output.
func (sm *stateMachine[S, E]) collectGraphEdges() []graphEdge {
	var edges []graphEdge

	for state, repr := range sm.config {
		stateStr := fmt.Sprintf("%v", state)

		for event, behaviors := range repr.triggerBehaviors {
			eventStr := fmt.Sprintf("%v", event)

			for _, behavior := range behaviors {
				switch b := behavior.(type) {
				case *permittedBehavior[S, E]:
					// Unconditional transition: use the destination directly.
					edges = append(edges, graphEdge{
						source: stateStr,
						target: fmt.Sprintf("%v", b.destination),
						label:  eventStr,
					})

				case *guardedBehavior[S, E]:
					// Guarded transition: append the label only when a guard
					// description exists.
					label := eventStr
					if desc := b.guardDescription(); desc != "" {
						label = fmt.Sprintf("%s [%s]", eventStr, desc)
					}
					edges = append(edges, graphEdge{
						source: stateStr,
						target: fmt.Sprintf("%v", b.destination),
						label:  label,
					})

				case *reentryBehavior[S, E]:
					// Reentry transition: a self-loop whose target is the source.
					label := eventStr
					if b.isGuarded() {
						label = fmt.Sprintf("%s [%s]", eventStr, b.guardDescription())
					}
					edges = append(edges, graphEdge{
						source: stateStr,
						target: stateStr,
						label:  label,
					})

				case *dynamicBehavior[S, E]:
					// Dynamic transition: the target is unknown, so no edge is emitted.
					_ = b

				case *internalBehavior[S, E]:
					// Internal transition: no edge.
					_ = b

				case *ignoredBehavior[S, E]:
					// Ignored behavior: no edge.
					_ = b
				}
			}
		}
	}

	// Sort for deterministic output.
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].source != edges[j].source {
			return edges[i].source < edges[j].source
		}
		if edges[i].target != edges[j].target {
			return edges[i].target < edges[j].target
		}
		return edges[i].label < edges[j].label
	})

	return edges
}

// sortedStateStrings returns the string representation of every state, sorted
// alphabetically.
func (sm *stateMachine[S, E]) sortedStateStrings() []string {
	names := make([]string, 0, len(sm.config))
	for s := range sm.config {
		names = append(names, fmt.Sprintf("%v", s))
	}
	sort.Strings(names)
	return names
}

// parentChildren returns a map from parent state name to a sorted list of child
// state names. Only parents that actually have children are included.
func (sm *stateMachine[S, E]) parentChildren() map[string][]string {
	result := make(map[string][]string)
	for _, repr := range sm.config {
		if len(repr.substates) > 0 {
			parentStr := fmt.Sprintf("%v", repr.state)
			children := make([]string, 0, len(repr.substates))
			for _, sub := range repr.substates {
				children = append(children, fmt.Sprintf("%v", sub.state))
			}
			sort.Strings(children)
			result[parentStr] = children
		}
	}
	return result
}

// childToParent returns a map from child state name to parent state name.
func (sm *stateMachine[S, E]) childToParent() map[string]string {
	result := make(map[string]string)
	for _, repr := range sm.config {
		if repr.superstate != nil {
			childStr := fmt.Sprintf("%v", repr.state)
			parentStr := fmt.Sprintf("%v", repr.superstate.state)
			result[childStr] = parentStr
		}
	}
	return result
}

// generateDotGraph generates the Graphviz DOT representation of the state graph.
//
// Graph features:
//   - A digraph declaration with left-to-right layout (rankdir=LR)
//   - Ellipse nodes for every state
//   - A UML-style invisible point connecting to the initial state
//   - Hierarchical states represented by nested subgraph clusters
//   - Transition edges labeled with event names and optional [guard descriptions]
func (sm *stateMachine[S, E]) generateDotGraph() string {
	var sb strings.Builder

	sb.WriteString("digraph {\n")
	sb.WriteString("    rankdir=LR;\n")
	sb.WriteString("    node [shape=ellipse];\n")
	sb.WriteString("\n")

	// Initial-state entry point (UML-style invisible point).
	initialStr := fmt.Sprintf("%v", sm.initialState)
	sb.WriteString("    __initial__ [shape=point, width=0.2, label=\"\"];\n")
	sb.WriteString(fmt.Sprintf("    __initial__ -> %q;\n", initialStr))
	sb.WriteString("\n")

	// Collect hierarchy relationships.
	pc := sm.parentChildren()
	cp := sm.childToParent()
	allStates := sm.sortedStateStrings()

	// Emit subgraph clusters for hierarchical states. Only top-level parents are
	// emitted here; nested parents are handled recursively by writeDotCluster,
	// otherwise the same subtree would be emitted more than once.
	for _, state := range allStates {
		if _, isChild := cp[state]; isChild {
			continue
		}
		if children, ok := pc[state]; ok {
			sm.writeDotCluster(&sb, state, children, pc, "    ")
		}
	}

	// Emit non-hierarchical state nodes (top-level isolated states that are
	// neither children nor parents).
	for _, state := range allStates {
		_, isChild := cp[state]
		_, isParent := pc[state]
		if !isChild && !isParent {
			// Top-level isolated state: emit the node directly.
			sb.WriteString(fmt.Sprintf("    %q;\n", state))
		}
	}

	sb.WriteString("\n")

	// Emit all transition edges.
	edges := sm.collectGraphEdges()
	for _, e := range edges {
		sb.WriteString(fmt.Sprintf("    %q -> %q [label=%q];\n", e.source, e.target, e.label))
	}

	sb.WriteString("}\n")

	return sb.String()
}

// writeDotCluster recursively writes a DOT subgraph cluster (supporting nested
// hierarchy levels).
func (sm *stateMachine[S, E]) writeDotCluster(sb *strings.Builder, parent string, children []string, pc map[string][]string, indent string) {
	sb.WriteString(fmt.Sprintf("%ssubgraph %q {\n", indent, "cluster_"+parent))
	sb.WriteString(fmt.Sprintf("%s    label=%q;\n", indent, parent))

	for _, child := range children {
		// If the child has children of its own, recurse into a nested cluster.
		if grandChildren, ok := pc[child]; ok {
			sm.writeDotCluster(sb, child, grandChildren, pc, indent+"    ")
		} else {
			sb.WriteString(fmt.Sprintf("%s    %q;\n", indent, child))
		}
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// generateMermaidGraph generates the Mermaid state diagram representation.
//
// Graph features:
//   - A stateDiagram-v2 declaration
//   - Initial states represented by [*] -->
//   - Transitions represented by --> with : event names
//   - Hierarchical states represented by state parent { ... } nesting, with all
//     child states declared inside their parent block
//   - State display names (which may contain parentheses, spaces, CJK, and so on)
//     preserved via `state "display name" as safeID` to avoid invalid Mermaid IDs
//   - Guarded transition labels include [guard descriptions]
func (sm *stateMachine[S, E]) generateMermaidGraph() string {
	var sb strings.Builder

	sb.WriteString("stateDiagram-v2\n")

	ids := sm.mermaidIDs()

	// Initial-state entry.
	initialStr := fmt.Sprintf("%v", sm.initialState)
	sb.WriteString(fmt.Sprintf("    [*] --> %s\n", ids[initialStr]))

	// Collect hierarchy relationships.
	pc := sm.parentChildren()
	cp := sm.childToParent()
	allStates := sm.sortedStateStrings()

	// Emit top-level state declarations and hierarchical nested blocks.
	for _, state := range allStates {
		if _, isChild := cp[state]; isChild {
			continue // Child states are declared inside their parent's block.
		}
		if children, ok := pc[state]; ok {
			sm.writeMermaidState(&sb, state, children, pc, ids, "    ")
		} else {
			sb.WriteString(fmt.Sprintf("    %s\n", mermaidStateDeclaration(state, ids[state])))
		}
	}

	// Emit all transition edges (referencing states by safe ID).
	edges := sm.collectGraphEdges()
	for _, e := range edges {
		sb.WriteString(fmt.Sprintf("    %s --> %s : %s\n",
			ids[e.source], ids[e.target], sanitizeMermaidLabel(e.label)))
	}

	return sb.String()
}

// writeMermaidState recursively writes a nested Mermaid state block.
func (sm *stateMachine[S, E]) writeMermaidState(sb *strings.Builder, parent string, children []string, pc map[string][]string, ids map[string]string, indent string) {
	sb.WriteString(fmt.Sprintf("%s%s {\n", indent, mermaidStateDeclaration(parent, ids[parent])))

	// Declare every child state inside the parent block first (including
	// recursively nested blocks).
	for _, child := range children {
		if grandChildren, ok := pc[child]; ok {
			sm.writeMermaidState(sb, child, grandChildren, pc, ids, indent+"    ")
		} else {
			sb.WriteString(fmt.Sprintf("%s    %s\n", indent, mermaidStateDeclaration(child, ids[child])))
		}
	}

	// If the parent has an initial substate target, emit the internal initial transition.
	for _, repr := range sm.config {
		parentStr := fmt.Sprintf("%v", repr.state)
		if parentStr == parent && repr.initialTransitionTarget != nil {
			targetStr := fmt.Sprintf("%v", *repr.initialTransitionTarget)
			sb.WriteString(fmt.Sprintf("%s    [*] --> %s\n", indent, ids[targetStr]))
			break
		}
	}

	sb.WriteString(fmt.Sprintf("%s}\n", indent))
}

// mermaidIDs returns a mapping from state display names to unique safe Mermaid
// identifiers. Display names may contain spaces, parentheses, CJK characters,
// and so on; using them directly as IDs would produce invalid syntax.
func (sm *stateMachine[S, E]) mermaidIDs() map[string]string {
	names := sm.sortedStateStrings()
	ids := make(map[string]string, len(names))
	used := make(map[string]bool, len(names))
	for i, name := range names {
		base := sanitizeMermaidID(name)
		if base == "" {
			base = fmt.Sprintf("state_%d", i)
		}
		id := base
		for suffix := 2; used[id]; suffix++ {
			id = fmt.Sprintf("%s_%d", base, suffix)
		}
		used[id] = true
		ids[name] = id
	}
	return ids
}

// sanitizeMermaidID converts a state display name into a Mermaid-safe identifier.
// Only ASCII letters, digits, and underscores are kept; every other character is
// replaced with an underscore.
func sanitizeMermaidID(name string) string {
	var sb strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	id := sb.String()
	// Mermaid identifiers must not start with a digit.
	if id != "" && id[0] >= '0' && id[0] <= '9' {
		id = "s" + id
	}
	return id
}

// mermaidStateDeclaration generates the declaration statement for a single state.
// When the display name equals the safe ID, the ID is used directly; otherwise an
// as-alias preserves the readable display name.
func mermaidStateDeclaration(display, id string) string {
	if display == id || display == "" {
		return "state " + id
	}
	return fmt.Sprintf("state \"%s\" as %s", escapeMermaidDescription(display), id)
}

// escapeMermaidDescription escapes Mermaid/HTML special characters and line
// breaks in a state display name.
func escapeMermaidDescription(display string) string {
	// Escape # first so entity markers inserted below are not escaped again.
	display = strings.ReplaceAll(display, "#", "#35;")
	display = strings.ReplaceAll(display, "\"", "#quot;")
	display = strings.ReplaceAll(display, "<", "#lt;")
	display = strings.ReplaceAll(display, ">", "#gt;")
	display = strings.ReplaceAll(display, "\r\n", " ")
	display = strings.ReplaceAll(display, "\n", " ")
	display = strings.ReplaceAll(display, "\r", " ")
	return display
}

// sanitizeMermaidLabel ensures a transition label cannot break Mermaid's line structure.
func sanitizeMermaidLabel(label string) string {
	label = strings.ReplaceAll(label, "\r\n", " ")
	label = strings.ReplaceAll(label, "\n", " ")
	label = strings.ReplaceAll(label, "\r", " ")
	return label
}
