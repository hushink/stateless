# stateless

**English** | [简体中文](README.zh-CN.md)

A type-safe, stateless state machine library for Go, inspired by the core design principles of Spring State Machine. It uses modern Go practices (generics and `context`) and has zero external dependencies.

## Features

- **Generic type safety** — States and events are user-defined types, checked at compile time by Go generics
- **Stateless design** — The machine holds no mutable state; callers supply the current state on every operation, so a single instance is naturally safe for concurrent use
- **Builder pattern** — A fluent, declarative API with a clear separation between `Builder` (topology) and `StateConfigurator` (per-state behavior)
- **Uninterrupted chaining** — `Configure(state)` returns a `StateConfigurator`; `Done()` returns the `Builder`
- **Hierarchical states** — Parent/child relationships with automatic transition inheritance
- **Guards** — Attach guard functions to transitions for runtime conditional logic
- **Entry/exit actions** — Run actions on state entry and exit, with event-filtered variants (`OnEntryFrom`)
- **Fire with arguments** — Pass arbitrary arguments through `Fire` and access them via `Transition.Args`
- **Context-aware queries** — `CanFireContext` / `PermittedTriggersContext` expose request context and arguments to guards
- **Multiple transition kinds** — External, internal, reentry, dynamic, guarded, and ignored
- **Build-time validation** — `Build()` detects cycles, invalid initial transitions, and nil guards/selectors/actions/listeners to prevent runtime panics
- **Lifecycle listeners** — Observe transitioning, transitioned, and unhandled-trigger events
- **Graph export** — Generate Graphviz DOT and Mermaid state diagrams from a configured machine
- **Zero dependencies** — Standard library only

## Installation

```bash
go get github.com/hushink/stateless
```

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hushink/stateless"
)

type State int

const (
	Locked State = iota
	Unlocked
)

type Event int

const (
	Coin Event = iota
	Push
)

func main() {
	sm, err := stateless.NewBuilder[State, Event](Locked).
		Configure(Locked).
			Permit(Coin, Unlocked).
			Ignore(Push).
		Done().
		Configure(Unlocked).
			Permit(Push, Locked).
			PermitReentry(Coin).
			OnEntry(func(ctx context.Context, t stateless.Transition[State, Event]) error {
				fmt.Println("Gate unlocked — push to pass")
				return nil
			}).
		Build()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	current := Locked

	current, _ = sm.Fire(ctx, current, Coin) // Locked -> Unlocked
	current, _ = sm.Fire(ctx, current, Push) // Unlocked -> Locked

	fmt.Println("final state:", current) // Locked
}
```

## API Overview

The library is organized around three core interfaces with clearly separated responsibilities:

| Interface | Responsibility |
|---|---|
| **`Builder[S, E]`** | Defines machine topology: registers states, declares substate relationships, sets initial transitions, attaches listeners, and `Build()` produces an immutable `StateMachine`. |
| **`StateConfigurator[S, E]`** | Configures the behavior of a single state: transitions, guards, and entry/exit actions. `Done()` returns to the `Builder`. |
| **`StateMachine[S, E]`** | Immutable read-only runtime interface: `Fire`, `CanFire`, `PermittedTriggers`, `IsInState`, and graph export. |

### Builder Methods

```go
NewBuilder[S, E](initialState S) Builder[S, E]

Configure(state S) StateConfigurator[S, E]
SubstateOf(child, parent S) Builder[S, E]
InitialTransition(state, initialChild S) Builder[S, E]
WithListener(l Listener[S, E]) Builder[S, E]
Build() (StateMachine[S, E], error)
```

Notes:

- `SubstateOf`, `InitialTransition`, and the destinations of `Permit`/`PermitIf` are registered automatically; no extra `Configure` call is required.
- An `InitialTransition` target must be a descendant of the declaring state; otherwise `Build()` returns `ErrInvalidConfiguration`.
- `Build()` does not mutate the `Builder`, so it can be called repeatedly; each call returns an independently frozen `StateMachine`.
- `Build()` validates nil guards, nil dynamic selectors, nil entry/exit actions, and nil listeners, and detects substate and initial-transition cycles.

### StateConfigurator Methods

```go
Permit(event E, dest S) StateConfigurator[S, E]
PermitIf(event E, dest S, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
PermitDynamic(event E, selector DynamicSelectorFunc[S, E]) StateConfigurator[S, E]
PermitDynamicIf(event E, selector DynamicSelectorFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E]
PermitReentry(event E) StateConfigurator[S, E]
PermitReentryIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
Ignore(event E) StateConfigurator[S, E]
IgnoreIf(event E, guards ...GuardFunc[S, E]) StateConfigurator[S, E]
InternalTransition(event E, action ActionFunc[S, E]) StateConfigurator[S, E]
InternalTransitionIf(event E, action ActionFunc[S, E], guards ...GuardFunc[S, E]) StateConfigurator[S, E]
OnEntry(action ActionFunc[S, E]) StateConfigurator[S, E]
OnEntryFrom(event E, action ActionFunc[S, E]) StateConfigurator[S, E]
OnExit(action ActionFunc[S, E]) StateConfigurator[S, E]
Done() Builder[S, E]
```

### StateMachine Methods

```go
Fire(ctx context.Context, state S, event E, args ...any) (S, error)
CanFire(state S, event E, args ...any) bool
CanFireContext(ctx context.Context, state S, event E, args ...any) bool
PermittedTriggers(state S, args ...any) []E
PermittedTriggersContext(ctx context.Context, state S, args ...any) []E
InitialState() S
IsInState(state S, possibleSuperstate S) bool
ToDotGraph() string
ToMermaidGraph() string
```

Notes:

- `CanFire` / `PermittedTriggers` evaluate guards and dynamic selectors; pass `args` so argument-dependent guards are evaluated correctly.
- Use `CanFireContext` / `PermittedTriggersContext` when guards need request context (tenant, tracing information, and so on).
- `PermittedTriggers` returns events in registration order, with the state's own events before events inherited from parent states.
- `InitialState()` resolves the `InitialTransition` chain to the final substate, which helps determine where a stateless machine starts.

## Transition Kinds

| Method | Behavior |
|---|---|
| `Permit` | Unconditional external transition to the destination |
| `PermitIf` | Guarded external transition — performed only when every guard returns `true` |
| `PermitDynamic` | Destination chosen at runtime by a selector function |
| `PermitDynamicIf` | Guarded dynamic transition |
| `PermitReentry` | Re-enters the same state, running exit and entry actions |
| `PermitReentryIf` | Guarded reentry transition |
| `Ignore` | Silently ignore the event — no transition, no error |
| `IgnoreIf` | Conditionally ignore the event |
| `InternalTransition` | Runs an action without entry/exit actions; the state does not change |
| `InternalTransitionIf` | Guarded internal transition |

Additional semantics:

- `Permit(event, currentState)` is an external self-transition and, like `PermitReentry`, runs exit and entry actions. Use `InternalTransition` when you only want to run an action without entry/exit.
- `Transition.Destination` and the destination observed by listeners is the final state after resolving the `InitialTransition` chain.
- Dynamic transition targets are unknown at build time. Explicitly `Configure` possible target states so graph export and later queries are complete.

## Hierarchical States

Declare parent/child relationships with `SubstateOf`. A substate inherits the parent's transitions and can be tested with `IsInState`.

```go
sm, _ := stateless.NewBuilder[OrderState, OrderEvent](Pending).
	Configure(Pending).
		Permit(Pay, Paid).
	Done().
	Configure(Paid).
		Permit(Ship, Shipped).
	Done().
	SubstateOf(Paid, Pending).        // Paid is a substate of Pending
	InitialTransition(Pending, Paid). // Entering Pending automatically transitions to Paid
	Configure(Shipped).
		Permit(Deliver, Delivered).
	Done().
	Configure(Delivered).Done().
	Build()

// IsInState honors the hierarchy.
sm.IsInState(Paid, Pending) // true — Paid is a substate of Pending
```

## Fire with Arguments

Pass runtime arguments through `Fire` and access them in actions and guards via `Transition.Args`:

```go
sm, _ := stateless.NewBuilder[State, Event](Open).
	Configure(Open).
		Permit(Assign, InProgress).
	Done().
	Configure(InProgress).
		Permit(Resolve, Resolved).
		OnEntryFrom(Assign, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			assignee := t.Args[0].(string)
			fmt.Printf("ticket assigned to: %s\n", assignee)
			return nil
		}).
	Done().
	Configure(Resolved).Done().
	Build()

// Pass "Alice" as an argument — accessible via t.Args[0].
current, _ := sm.Fire(ctx, Open, Assign, "Alice")
```

## Listeners

Implement the `Listener` interface, or embed `ListenerAdapter` and override only the methods you need:

```go
type Listener[S, E comparable] interface {
	OnTransitioning(ctx context.Context, t Transition[S, E])
	OnTransitioned(ctx context.Context, t Transition[S, E])
	OnUnhandledTrigger(ctx context.Context, state S, event E)
}
```

Usage:

```go
type AuditListener struct {
	stateless.ListenerAdapter[MyState, MyEvent]
}

func (l *AuditListener) OnTransitioned(_ context.Context, t stateless.Transition[MyState, MyEvent]) {
	fmt.Printf("transitioned: %v -> %v via %v\n", t.Source, t.Destination, t.Event)
}

sm, _ := stateless.NewBuilder[MyState, MyEvent](Initial).
	// ... state configuration ...
	WithListener(&AuditListener{}).
	Build()
```

## Graph Export

Generate a visual representation of the state machine:

```go
sm, _ := builder.Build()

// Graphviz DOT format
dot := sm.ToDotGraph()
fmt.Println(dot)

// Mermaid state diagram format
mermaid := sm.ToMermaidGraph()
fmt.Println(mermaid)
```

DOT output supports hierarchical cluster nesting, a UML-style initial-state entry point, and guard-condition labels. Mermaid output uses `stateDiagram-v2` syntax and nested state blocks; display names containing parentheses, spaces, CJK characters, and so on are converted to safe IDs and preserved through `state "display name" as id`.

## Examples

The `examples/` directory contains three complete runnable programs:

| Example | Description |
|---|---|
| [`turnstile`](examples/turnstile/main.go) | Classic turnstile — demonstrates `Permit`, `Ignore`, `PermitReentry`, and `OnEntry`/`OnExit` actions |
| [`order`](examples/order/main.go) | Order workflow — demonstrates `PermitIf` guards, `SubstateOf` hierarchical states, `OnEntryFrom`, `IsInState`, and `Listener` |
| [`ticket`](examples/ticket/main.go) | Ticket system — demonstrates `Fire` with `args`, guard validation over `Transition.Args`, and per-event `OnEntryFrom` actions |

Run any example:

```bash
go run ./examples/turnstile
go run ./examples/order
go run ./examples/ticket
```

## Design Philosophy

- **Inspired by Spring State Machine** — Adopts core concepts (states, events, transitions, guards, actions, hierarchical states) in an idiomatic Go way
- **Interface-oriented** — The entire public API surface consists of interfaces (`Builder`, `StateConfigurator`, `StateMachine`, `Listener`); implementations are unexported
- **Immutable after Build** — `Build()` deep-copies and freezes all configuration; the resulting `StateMachine` is safe to share across goroutines
- **Stateless** — No internal mutable state; callers own and pass the current state, enabling natural concurrency without locks
- **Zero external dependencies** — Standard library only

## License

Apache-2.0
