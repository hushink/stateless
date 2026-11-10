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

// Package stateless provides a stateless state machine library for Go, inspired
// by the core design principles of Spring State Machine.
//
// The stateless design means the machine itself does not hold the current state;
// callers supply the current state on every operation. This makes a single machine
// instance safe to share across goroutines and well suited to highly concurrent use.
//
// Key features:
//   - Stateless design: configuration is separated from state storage and is naturally concurrency-safe
//   - Generic type safety: states and events are user-defined and checked at compile time
//   - Fluent Builder configuration: a declarative API for states, transitions, guards, and actions
//   - Hierarchical states: parent/child relationships with automatic transition inheritance
//   - Multiple transition kinds: external, internal, reentry, dynamic, and ignored events
//   - Guards: transitions can be guarded by functions evaluated at runtime
//   - Entry/exit actions: custom actions on state entry and exit, optionally filtered by event
//   - Context-aware queries: CanFireContext and PermittedTriggersContext expose request context to guards
//   - Build-time validation: Build detects cycles, invalid initial transitions, and nil guards/selectors/actions/listeners
//   - Graph export: Graphviz DOT and Mermaid state diagrams
//   - Zero external dependencies: standard library only
package stateless
