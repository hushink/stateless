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
// State and event definitions
// ---------------------------------------------------------------------------

// State defines the turnstile state type.
type State int

const (
	// Locked means the turnstile is locked and requires a coin to pass.
	Locked State = iota
	// Unlocked means the turnstile is unlocked and can be pushed open.
	Unlocked
)

// String implements fmt.Stringer for readable state names.
func (s State) String() string {
	switch s {
	case Locked:
		return "Locked(锁定)"
	case Unlocked:
		return "Unlocked(解锁)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// Event defines the turnstile event type.
type Event int

const (
	// Coin is the coin-insertion event.
	Coin Event = iota
	// Push is the push-the-gate event.
	Push
)

// String implements fmt.Stringer for readable event names.
func (e Event) String() string {
	switch e {
	case Coin:
		return "Coin(投币)"
	case Push:
		return "Push(推门)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(e))
	}
}

// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

func main() {
	debug := flag.Bool("debug", false, "enable debug logging")
	flag.Parse()

	// ======================================================================
	// Step 1: Build the state machine with the fluent Builder API
	// ======================================================================
	sm, err := stateless.NewBuilder[State, Event](Locked).
		// -- Configure the Locked state --
		Configure(Locked).
		Permit(Coin, Unlocked). // Coin -> Unlocked
		Ignore(Push).           // Push while locked -> ignored (no error, no state change)
		OnEntry(func(ctx context.Context, t stateless.Transition[State, Event]) error {
			fmt.Println("  [进入锁定状态] 旋转门已锁定，请投币")
			return nil
		}).
		OnExit(func(ctx context.Context, t stateless.Transition[State, Event]) error {
			fmt.Println("  [离开锁定状态]")
			return nil
		}).
		Done().
		// -- Configure the Unlocked state --
		Configure(Unlocked).
		Permit(Push, Locked). // Push -> Locked
		PermitReentry(Coin).  // Coin while unlocked -> reentry (exit + entry, state unchanged)
		OnEntry(func(ctx context.Context, t stateless.Transition[State, Event]) error {
			if t.IsReentry {
				fmt.Println("  [重入解锁状态] 多投了一枚硬币，谢谢！")
			} else {
				fmt.Println("  [进入解锁状态] 请通过旋转门")
			}
			return nil
		}).
		OnExit(func(ctx context.Context, t stateless.Transition[State, Event]) error {
			fmt.Println("  [离开解锁状态]")
			return nil
		}).
		Done().
		WithDebug(*debug).
		Build()
	if err != nil {
		log.Fatalf("构建状态机失败: %v", err)
	}

	ctx := context.Background()
	currentState := Locked

	// ======================================================================
	// Step 2: Inspect the initial state
	// ======================================================================
	fmt.Println("========== 旋转门状态机示例 ==========")
	fmt.Printf("当前状态: %v\n", currentState)

	// Demo PermittedTriggers: list events triggerable from the current state.
	fmt.Printf("可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// Demo CanFire: check whether a specific event can be triggered.
	fmt.Printf("能否推门(Push)? %v\n", sm.CanFire(currentState, Push))
	fmt.Printf("能否投币(Coin)? %v\n", sm.CanFire(currentState, Coin))

	// ======================================================================
	// Step 3: Push while Locked (ignored, no change)
	// ======================================================================
	fmt.Println("\n--- 尝试推门（Locked 状态下会被忽略） ---")
	currentState, err = sm.Fire(ctx, currentState, Push)
	if err != nil {
		log.Fatalf("触发事件失败: %v", err)
	}
	fmt.Printf("推门后状态: %v (未变化，因为 Ignore)\n", currentState)

	// ======================================================================
	// Step 4: Insert a coin, Locked -> Unlocked
	// ======================================================================
	fmt.Println("\n--- 投币 ---")
	currentState, err = sm.Fire(ctx, currentState, Coin)
	if err != nil {
		log.Fatalf("触发事件失败: %v", err)
	}
	fmt.Printf("投币后状态: %v\n", currentState)

	// Show the events available while Unlocked.
	fmt.Printf("可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// ======================================================================
	// Step 5: Insert another coin -- demo PermitReentry (reentry transition)
	// ======================================================================
	fmt.Println("\n--- 再次投币（重入转换） ---")
	currentState, err = sm.Fire(ctx, currentState, Coin)
	if err != nil {
		log.Fatalf("触发事件失败: %v", err)
	}
	fmt.Printf("再次投币后状态: %v (仍然解锁，但触发了 exit/entry 动作)\n", currentState)

	// ======================================================================
	// Step 6: Push through, Unlocked -> Locked
	// ======================================================================
	fmt.Println("\n--- 推门通过 ---")
	currentState, err = sm.Fire(ctx, currentState, Push)
	if err != nil {
		log.Fatalf("触发事件失败: %v", err)
	}
	fmt.Printf("推门后状态: %v\n", currentState)

	fmt.Println("\n========== 演示完成 ==========")
}
