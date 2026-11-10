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

// OrderState defines the order state type.
type OrderState int

const (
	// Pending is the unpaid state (initial state).
	Pending OrderState = iota
	// Paid is the paid state (a substate of Pending, demonstrating hierarchy).
	Paid
	// Shipped is the shipped state.
	Shipped
	// Delivered is the delivered state (terminal state).
	Delivered
	// Cancelled is the cancelled state (terminal state).
	Cancelled
)

// String implements fmt.Stringer.
func (s OrderState) String() string {
	switch s {
	case Pending:
		return "Pending(待付款)"
	case Paid:
		return "Paid(已付款)"
	case Shipped:
		return "Shipped(已发货)"
	case Delivered:
		return "Delivered(已签收)"
	case Cancelled:
		return "Cancelled(已取消)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// ---------------------------------------------------------------------------
// Event definitions
// ---------------------------------------------------------------------------

// OrderEvent defines the order event type.
type OrderEvent int

const (
	// Pay is the payment event.
	Pay OrderEvent = iota
	// Ship is the shipping event.
	Ship
	// Deliver is the delivery event.
	Deliver
	// Cancel is the cancellation event.
	Cancel
)

// String implements fmt.Stringer.
func (e OrderEvent) String() string {
	switch e {
	case Pay:
		return "Pay(付款)"
	case Ship:
		return "Ship(发货)"
	case Deliver:
		return "Deliver(签收)"
	case Cancel:
		return "Cancel(取消)"
	default:
		return fmt.Sprintf("Unknown(%d)", int(e))
	}
}

// ---------------------------------------------------------------------------
// OrderListener -- demonstrates the Listener interface (embedded ListenerAdapter)
// ---------------------------------------------------------------------------

// OrderListener implements a custom listener that overrides only the methods it
// cares about.
type OrderListener struct {
	stateless.ListenerAdapter[OrderState, OrderEvent] // Embedded no-op implementation; override only what you need.
}

// OnTransitioning logs a message just before a transition occurs.
func (l *OrderListener) OnTransitioning(_ context.Context, t stateless.Transition[OrderState, OrderEvent]) {
	fmt.Printf("  [Listener] 即将转换: %v --(%v)--> %v\n", t.Source, t.Event, t.Destination)
}

// OnTransitioned logs a message after a transition completes.
func (l *OrderListener) OnTransitioned(_ context.Context, t stateless.Transition[OrderState, OrderEvent]) {
	fmt.Printf("  [Listener] 转换完成: %v --(%v)--> %v\n", t.Source, t.Event, t.Destination)
}

// OnUnhandledTrigger logs a warning when an event is not handled.
func (l *OrderListener) OnUnhandledTrigger(_ context.Context, state OrderState, event OrderEvent) {
	fmt.Printf("  [Listener] 警告: 状态 %v 下事件 %v 未被处理\n", state, event)
}

// ---------------------------------------------------------------------------
// Helper: fire an event and print the result
// ---------------------------------------------------------------------------

func fireEvent(ctx context.Context, sm stateless.StateMachine[OrderState, OrderEvent], state OrderState, event OrderEvent) OrderState {
	fmt.Printf("\n--- 触发事件: %v (当前状态: %v) ---\n", event, state)
	newState, err := sm.Fire(ctx, state, event)
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

	// Simulated order amount (used by the guard condition).
	orderAmount := 199.0

	// ======================================================================
	// Step 1: Build the order state machine with the fluent Builder API
	// ======================================================================
	sm, err := stateless.NewBuilder[OrderState, OrderEvent](Pending).
		// -- Configure the Pending (unpaid) state --
		Configure(Pending).
		// Pay -> Paid
		Permit(Pay, Paid).
		// Cancel -> Cancelled (guarded: cancellation is allowed only when amount > 0)
		PermitIf(Cancel, Cancelled, func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) bool {
			allowed := orderAmount > 0
			fmt.Printf("  [Guard] 检查取消条件: 订单金额 %.2f > 0 ? %v\n", orderAmount, allowed)
			return allowed
		}).
		// Action executed when entering the Pending state.
		OnEntry(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Println("  [Entry] 订单已创建，等待付款...")
			return nil
		}).
		Done().
		// -- Configure the Paid state --
		Configure(Paid).
		// Ship -> Shipped
		Permit(Ship, Shipped).
		// Cancel after payment -> Cancelled (the refund is handled via OnEntryFrom)
		Permit(Cancel, Cancelled).
		// Generic entry action.
		OnEntry(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Printf("  [Entry] 订单已付款，金额: %.2f 元\n", orderAmount)
			return nil
		}).
		// Action executed when leaving the Paid state.
		OnExit(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Println("  [Exit] 离开已付款状态")
			return nil
		}).
		Done().
		// Demonstrate SubstateOf: Paid is a substate of Pending (hierarchical state).
		SubstateOf(Paid, Pending).
		// -- Configure the Shipped state --
		Configure(Shipped).
		// Deliver -> Delivered
		Permit(Deliver, Delivered).
		OnEntry(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Println("  [Entry] 订单已发货，快递运输中...")
			return nil
		}).
		Done().
		// -- Configure the Delivered state --
		Configure(Delivered).
		OnEntry(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Println("  [Entry] 订单已签收，交易完成！")
			return nil
		}).
		Done().
		// -- Configure the Cancelled state --
		Configure(Cancelled).
		// Generic entry action.
		OnEntry(func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			fmt.Println("  [Entry] 订单已取消")
			return nil
		}).
		// Demonstrate OnEntryFrom: issue a refund only when entering from Paid
		// because of the Cancel event.
		OnEntryFrom(Cancel, func(ctx context.Context, t stateless.Transition[OrderState, OrderEvent]) error {
			if t.Source == Paid {
				fmt.Printf("  [OnEntryFrom Cancel] 已从 Paid 状态取消，退款 %.2f 元\n", orderAmount)
			}
			return nil
		}).
		Done().
		// -- Register the global listener --
		WithListener(&OrderListener{}).
		// -- Enable built-in debug logging when -debug is set --
		WithDebug(*debug).
		Build()
	if err != nil {
		log.Fatalf("构建状态机失败: %v", err)
	}

	ctx := context.Background()

	// ======================================================================
	// Scenario 1: Normal order flow Pending -> Paid -> Shipped -> Delivered
	// ======================================================================
	fmt.Println("==========================================================")
	fmt.Println("  场景一：正常订单流程")
	fmt.Println("==========================================================")

	currentState := Pending
	fmt.Printf("初始状态: %v\n", currentState)

	// Demo PermittedTriggers: list events triggerable from the unpaid state.
	fmt.Printf("可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// Demo CanFire: check whether shipping is possible (unpaid orders cannot ship).
	fmt.Printf("能否直接发货? %v\n", sm.CanFire(currentState, Ship))
	fmt.Printf("能否付款? %v\n", sm.CanFire(currentState, Pay))

	// Pay.
	currentState = fireEvent(ctx, sm, currentState, Pay)

	// Demo IsInState (hierarchy query): Paid is a substate of Pending.
	fmt.Printf("\n[IsInState] Paid 是否处于 Pending 中? %v (因为 Paid 是 Pending 的子状态)\n",
		sm.IsInState(currentState, Pending))
	fmt.Printf("[IsInState] Paid 是否处于 Paid 中? %v\n",
		sm.IsInState(currentState, Paid))
	fmt.Printf("[IsInState] Paid 是否处于 Shipped 中? %v\n",
		sm.IsInState(currentState, Shipped))

	// Ship.
	currentState = fireEvent(ctx, sm, currentState, Ship)

	// Deliver.
	currentState = fireEvent(ctx, sm, currentState, Deliver)

	// Demo: attempting to ship again while delivered (should fail).
	fmt.Println("\n--- 尝试在已签收状态下发货（应失败） ---")
	_, err = sm.Fire(ctx, currentState, Ship)
	if err != nil {
		fmt.Printf("  预期的错误: %v\n", err)
	}

	// ======================================================================
	// Scenario 2: Cancel the order after payment (triggers a refund)
	// ======================================================================
	fmt.Println("\n==========================================================")
	fmt.Println("  场景二：已付款后取消订单（触发退款）")
	fmt.Println("==========================================================")

	currentState = Pending
	fmt.Printf("初始状态: %v\n", currentState)

	// Pay.
	currentState = fireEvent(ctx, sm, currentState, Pay)

	// Show events available after payment.
	fmt.Printf("\n可触发事件: %v\n", sm.PermittedTriggers(currentState))

	// Cancel the order -- cancelling from Paid triggers the OnEntryFrom refund action.
	_ = fireEvent(ctx, sm, currentState, Cancel)

	// ======================================================================
	// Scenario 3: Cancel directly from Pending (guard condition verification)
	// ======================================================================
	fmt.Println("\n==========================================================")
	fmt.Println("  场景三：待付款时直接取消（Guard 守卫条件）")
	fmt.Println("==========================================================")

	currentState = Pending
	fmt.Printf("初始状态: %v\n", currentState)

	// CanFire also evaluates guard conditions.
	fmt.Printf("能否取消? %v (Guard 会检查订单金额 > 0)\n", sm.CanFire(currentState, Cancel))

	// Cancel.
	_ = fireEvent(ctx, sm, currentState, Cancel)

	fmt.Println("\n==========================================================")
	fmt.Println("  所有场景演示完成")
	fmt.Println("==========================================================")
}
