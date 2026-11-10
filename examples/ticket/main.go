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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/hushink/stateless"
)

// ---------------------------------------------------------------------------
// State definitions
// ---------------------------------------------------------------------------

// State defines the ticket system state type.
type State int

const (
	// Open is a newly created ticket waiting for an assignee.
	Open State = iota
	// InProgress is a ticket that has been assigned and is being worked on.
	InProgress
	// Resolved is a ticket that has been resolved and awaits closure.
	Resolved
	// Closed is a closed ticket (terminal state).
	Closed
)

// String implements fmt.Stringer for readable state names.
func (s State) String() string {
	switch s {
	case Open:
		return "Open(待处理)"
	case InProgress:
		return "InProgress(处理中)"
	case Resolved:
		return "Resolved(已解决)"
	case Closed:
		return "Closed(已关闭)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// ---------------------------------------------------------------------------
// Event definitions
// ---------------------------------------------------------------------------

// Event defines the ticket system event type.
type Event int

const (
	// Assign assigns the ticket; args: assignee string (name of the assignee).
	Assign Event = iota
	// Resolve resolves the ticket; args: resolution string (resolution notes).
	Resolve
	// Close closes the ticket; args: score int (satisfaction score 1-5).
	Close
	// Reopen reopens the ticket; args: reason string (reason for reopening).
	Reopen
)

// String implements fmt.Stringer for readable event names.
func (e Event) String() string {
	switch e {
	case Assign:
		return "Assign(分配)"
	case Resolve:
		return "Resolve(解决)"
	case Close:
		return "Close(关闭)"
	case Reopen:
		return "Reopen(重开)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(e))
	}
}

// ---------------------------------------------------------------------------
// Helper: fire an event with arguments and print the result
// ---------------------------------------------------------------------------

// fireEvent triggers an event and prints the state before and after the
// transition; on error it keeps the original state.
func fireEvent(ctx context.Context, sm stateless.StateMachine[State, Event], state State, event Event, args ...any) State {
	fmt.Printf("\n--- 触发事件: %v (当前状态: %v, 参数: %v) ---\n", event, state, args)
	newState, err := sm.Fire(ctx, state, event, args...)
	if err != nil {
		fmt.Printf("  错误: %v\n", err)
		return state // Keep the original state on error.
	}
	fmt.Printf("  新状态: %v\n", newState)
	return newState
}

// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

func main() {
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// ======================================================================
	// Step 1: Build the ticket state machine with the fluent Builder API
	// ======================================================================
	sm, err := stateless.NewBuilder[State, Event](Open).
		// -- Configure the Open (pending) state --
		Configure(Open).
		// Assign -> InProgress
		Permit(Assign, InProgress).
		OnEntry(func(ctx context.Context, t stateless.Transition[State, Event]) error {
			fmt.Println("  [Entry] 工单已创建，等待分配处理人")
			return nil
		}).
		Done().
		// -- Configure the InProgress state --
		Configure(InProgress).
		// Resolve -> Resolved
		Permit(Resolve, Resolved).
		// Use OnEntryFrom to extract the assignee only when entering via Assign.
		OnEntryFrom(Assign, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			// Extract the assignee name passed with the Assign event from t.Args[0].
			assignee := t.Args[0].(string)
			fmt.Printf("  [OnEntryFrom Assign] 工单已分配给: %s\n", assignee)
			return nil
		}).
		// Extract the reopen reason when re-entering via Reopen.
		OnEntryFrom(Reopen, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			reason := t.Args[0].(string)
			fmt.Printf("  [OnEntryFrom Reopen] 工单被重新打开，原因: %s\n", reason)
			return nil
		}).
		Done().
		// -- Configure the Resolved state --
		Configure(Resolved).
		// Close -> Closed (guarded: the score must be within 1-5)
		PermitIf(Close, Closed, func(ctx context.Context, t stateless.Transition[State, Event]) bool {
			if len(t.Args) == 0 {
				fmt.Println("  [Guard] 关闭失败: 未提供满意度评分")
				return false
			}
			score, ok := t.Args[0].(int)
			if !ok {
				fmt.Println("  [Guard] 关闭失败: 评分参数类型错误")
				return false
			}
			valid := score >= 1 && score <= 5
			fmt.Printf("  [Guard] 检查满意度评分: %d (有效范围 1-5) -> %v\n", score, valid)
			return valid
		}).
		// Use OnEntryFrom to extract the resolution only when entering via Resolve.
		OnEntryFrom(Resolve, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			resolution := t.Args[0].(string)
			fmt.Printf("  [OnEntryFrom Resolve] 解决方案: %s\n", resolution)
			return nil
		}).
		Done().
		// -- Configure the Closed state --
		Configure(Closed).
		// Reopen -> InProgress
		Permit(Reopen, InProgress).
		// Extract the satisfaction score when entering the Closed state.
		OnEntryFrom(Close, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			score := t.Args[0].(int)
			fmt.Printf("  [OnEntryFrom Close] 工单已关闭，满意度评分: %d/5\n", score)
			return nil
		}).
		Done().
		WithDebug(*debug).
		Build()
	if err != nil {
		log.Fatalf("构建状态机失败: %v", err)
	}

	ctx := context.Background()

	// ======================================================================
	// Scenario 1: Full ticket lifecycle -- demonstrates Fire with args
	// ======================================================================
	fmt.Println("==========================================================")
	fmt.Println("  场景一：完整工单生命周期（Fire 带 args 参数）")
	fmt.Println("==========================================================")

	currentState := Open
	fmt.Printf("初始状态: %v\n", currentState)

	// Demo PermittedTriggers: list events triggerable from the current state.
	fmt.Printf("可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// Demo CanFire: check whether a specific event can be triggered.
	fmt.Printf("能否分配(Assign)? %v\n", sm.CanFire(currentState, Assign))
	fmt.Printf("能否直接解决(Resolve)? %v\n", sm.CanFire(currentState, Resolve))

	// 1. Assign the ticket -- pass the assignee through Fire.
	currentState = fireEvent(ctx, sm, currentState, Assign, "Alice")

	// Show events available while InProgress.
	fmt.Printf("\n可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// 2. Resolve the ticket -- pass the resolution through Fire.
	currentState = fireEvent(ctx, sm, currentState, Resolve, "已修复数据库连接问题")

	// 3. Close the ticket -- pass the satisfaction score (scenario ends here;
	//    the result is not used further).
	_ = fireEvent(ctx, sm, currentState, Close, 5)

	// ======================================================================
	// Scenario 2: Guard interception -- score out of range
	// ======================================================================
	fmt.Println("\n==========================================================")
	fmt.Println("  场景二：Guard 守卫条件拦截（评分超出范围）")
	fmt.Println("==========================================================")

	currentState = Resolved
	fmt.Printf("当前状态: %v\n", currentState)

	// Try to close with an invalid score -- the guard should reject it.
	fmt.Println("\n--- 尝试用评分 0 关闭（应被 Guard 拒绝） ---")
	_, err = sm.Fire(ctx, currentState, Close, 0)
	if err != nil {
		fmt.Printf("  预期的错误: %v\n", err)
	}

	// Try to close with an out-of-range score.
	fmt.Println("\n--- 尝试用评分 10 关闭（应被 Guard 拒绝） ---")
	_, err = sm.Fire(ctx, currentState, Close, 10)
	if err != nil {
		fmt.Printf("  预期的错误: %v\n", err)
	}

	// Close with a valid score.
	currentState = fireEvent(ctx, sm, currentState, Close, 3)

	// ======================================================================
	// Scenario 3: Reopen a closed ticket -- demonstrates Reopen argument passing
	// ======================================================================
	fmt.Println("\n==========================================================")
	fmt.Println("  场景三：重新打开已关闭的工单")
	fmt.Println("==========================================================")

	fmt.Printf("当前状态: %v\n", currentState)

	// Demo CanFire: a closed ticket can be reopened.
	fmt.Printf("能否重开(Reopen)? %v\n", sm.CanFire(currentState, Reopen))
	fmt.Printf("能否分配(Assign)? %v\n", sm.CanFire(currentState, Assign))

	// Reopen the ticket -- pass the reopen reason through Fire.
	currentState = fireEvent(ctx, sm, currentState, Reopen, "问题复现")

	// Show events available after reopening.
	fmt.Printf("\n可触发事件: %v\n", sm.PermittedTriggers(currentState))

	fmt.Println("\n==========================================================")
	fmt.Println("  所有场景演示完成")
	fmt.Println("==========================================================")
}
