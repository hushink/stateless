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
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func newDebugBufferLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})), buf
}

func TestDebugLoggingWritesTransitionDetails(t *testing.T) {
	logger, buf := newDebugBufferLogger(slog.LevelDebug)
	sm := mustBuild(t, NewBuilder[testState, testEvent](stateA).
		Configure(stateA).Permit(eventX, stateB).Done().
		Configure(stateB).Done().
		WithDebugLogger(logger))

	if _, err := sm.Fire(context.Background(), stateA, eventX); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"msg=fire",
		"msg=transition",
		"msg=exit",
		"msg=enter",
		`msg="transition completed"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected debug output to contain %q, got:\n%s", want, out)
		}
	}
}

func TestDebugLoggingDisabled(t *testing.T) {
	buildMachine := func(t *testing.T, b Builder[testState, testEvent]) StateMachine[testState, testEvent] {
		t.Helper()
		sm, err := b.Build()
		if err != nil {
			t.Fatalf("Build() failed: %v", err)
		}
		return sm
	}

	t.Run("WithDebug(false) clears a configured logger", func(t *testing.T) {
		logger, buf := newDebugBufferLogger(slog.LevelDebug)
		sm := buildMachine(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Done().
			Configure(stateB).Done().
			WithDebugLogger(logger).
			WithDebug(false))

		if _, err := sm.Fire(context.Background(), stateA, eventX); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no debug output, got:\n%s", buf.String())
		}
	})

	t.Run("WithDebugLogger(nil) disables logging", func(t *testing.T) {
		logger, buf := newDebugBufferLogger(slog.LevelDebug)
		sm := buildMachine(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Done().
			Configure(stateB).Done().
			WithDebugLogger(logger).
			WithDebugLogger(nil))

		if _, err := sm.Fire(context.Background(), stateA, eventX); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no debug output, got:\n%s", buf.String())
		}
	})

	t.Run("logger level above debug emits nothing", func(t *testing.T) {
		logger, buf := newDebugBufferLogger(slog.LevelInfo)
		sm := buildMachine(t, NewBuilder[testState, testEvent](stateA).
			Configure(stateA).Permit(eventX, stateB).Done().
			Configure(stateB).Done().
			WithDebugLogger(logger))

		if _, err := sm.Fire(context.Background(), stateA, eventX); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected no debug output, got:\n%s", buf.String())
		}
	})
}
