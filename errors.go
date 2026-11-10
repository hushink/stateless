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
	"errors"
	"fmt"
)

// ErrTriggerNotPermitted is returned when an event has no valid transition in the
// current state. Use errors.Is to match it.
var ErrTriggerNotPermitted = errors.New("trigger not permitted")

// ErrInvalidConfiguration is returned when the state machine configuration is
// invalid (for example, a circular substate relationship). Use errors.Is to match it.
var ErrInvalidConfiguration = errors.New("invalid configuration")

// TriggerNotPermittedError provides detailed information when an event is not
// permitted in the current state, including the state and event that failed,
// to aid debugging and error handling.
type TriggerNotPermittedError[S, E comparable] struct {
	State S // Current state when the event was fired
	Event E // Event that is not permitted
}

// Error returns the string representation of the error.
func (e *TriggerNotPermittedError[S, E]) Error() string {
	return fmt.Sprintf("trigger not permitted: event %v is not configured in state %v", e.Event, e.State)
}

// Unwrap returns the underlying sentinel error, enabling
// errors.Is(err, ErrTriggerNotPermitted).
func (e *TriggerNotPermittedError[S, E]) Unwrap() error {
	return ErrTriggerNotPermitted
}

// InvalidConfigurationError provides detailed information about an invalid
// state machine configuration.
type InvalidConfigurationError struct {
	Message string // Detailed description of the configuration error
}

// Error returns the string representation of the error.
func (e *InvalidConfigurationError) Error() string {
	return fmt.Sprintf("invalid configuration: %s", e.Message)
}

// Unwrap returns the underlying sentinel error, enabling
// errors.Is(err, ErrInvalidConfiguration).
func (e *InvalidConfigurationError) Unwrap() error {
	return ErrInvalidConfiguration
}
