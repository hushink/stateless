# stateless

[English](README.md) | **简体中文**

一个类型安全、无状态的状态机库，专为 Go 语言设计，灵感来源于 Spring State Machine 的核心设计理念。采用现代 Go 实践（泛型、context），零外部依赖。

## 特性

- **泛型类型安全** — 状态和事件类型由用户定义，通过 Go 泛型实现编译时类型检查
- **无状态设计** — 状态机不保存可变状态；调用者在每次操作时提供当前状态，使实例天然支持并发使用
- **Builder 模式** — 流畅的声明式 API，`Builder`（拓扑结构）和 `StateConfigurator`（每个状态的行为）之间职责清晰分离
- **链式配置** — `Configure(state)` 返回 `StateConfigurator`；`Done()` 返回 `Builder`，实现不间断的链式调用
- **层次化状态** — 父子状态关系，支持自动转换继承
- **守卫条件** — 为转换附加守卫函数，实现运行时条件逻辑
- **进入/退出动作** — 在状态进入和退出时执行动作，支持事件过滤变体（`OnEntryFrom`）
- **带参数的 Fire** — 通过 `Fire` 传递任意参数，并通过 `Transition.Args` 访问
- **上下文感知查询** — `CanFireContext` / `PermittedTriggersContext` 让守卫条件访问请求上下文与参数
- **多种转换类型** — 外部、内部、重入、动态、条件和忽略
- **构建期校验** — `Build()` 检测环、非法初始转换和 nil 守卫/选择器/动作/监听器，避免运行时 panic
- **生命周期监听器** — 观察转换中、已转换和未处理触发事件
- **可选调试日志** — 基于 `slog` 的内置调试日志，用于开发与测试，生产默认完全关闭
- **图导出** — 从配置的状态机生成 Graphviz DOT 和 Mermaid 状态图
- **零依赖** — 仅使用 Go 标准库

## 安装

```bash
go get github.com/hushink/stateless
```

## 快速开始

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
				fmt.Println("门已解锁 — 请推门通过")
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

	fmt.Println("最终状态:", current) // Locked
}
```

## API 概览

该库围绕三个核心接口组织，职责清晰分离：

| 接口 | 职责 |
|---|---|
| **`Builder[S, E]`** | 定义状态机拓扑：注册状态、声明子状态关系、设置初始转换、附加监听器，以及 `Build()` 生成不可变的 `StateMachine`。 |
| **`StateConfigurator[S, E]`** | 配置单个状态的行为：转换、守卫、进入/退出动作。调用 `Done()` 返回 `Builder`。 |
| **`StateMachine[S, E]`** | 不可变的只读运行时接口：`Fire`、`CanFire`、`PermittedTriggers`、`IsInState` 和图导出。 |

### Builder 方法

```go
NewBuilder[S, E](initialState S) Builder[S, E]

Configure(state S) StateConfigurator[S, E]
SubstateOf(child, parent S) Builder[S, E]
InitialTransition(state, initialChild S) Builder[S, E]
WithListener(l Listener[S, E]) Builder[S, E]
WithDebug(enabled bool) Builder[S, E]
WithDebugLogger(logger *slog.Logger) Builder[S, E]
Build() (StateMachine[S, E], error)
```

说明：

- `SubstateOf`、`InitialTransition` 以及 `Permit`/`PermitIf` 的目标状态会被自动注册，无需额外 `Configure`。
- `InitialTransition` 的目标必须是 `state` 的后代状态，否则 `Build()` 返回 `ErrInvalidConfiguration`。
- `Build()` 不会修改 `Builder`，可安全地重复调用；每次返回独立冻结的 `StateMachine`。
- `Build()` 会校验 nil 守卫、nil 动态选择器、nil 进入/退出动作与 nil 监听器，并检测子状态环与初始转换环。
- 调试日志默认关闭。`WithDebug(true)` 以 debug 级别向 stderr 输出转换决策；`WithDebugLogger` 可传入自定义 `*slog.Logger`。

### StateConfigurator 方法

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

### StateMachine 方法

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

说明：

- `CanFire` / `PermittedTriggers` 会执行守卫与动态选择器；传入 `args` 可让依赖参数的守卫正确求值。
- 需要请求上下文（如租户、追踪信息）时使用 `CanFireContext` / `PermittedTriggersContext`。
- `PermittedTriggers` 按事件注册顺序返回，子状态自身的事件优先于从父状态继承的事件。
- `InitialState()` 会沿 `InitialTransition` 链解析到最终子状态，便于确定无状态状态机的起始状态。

## 转换类型

| 方法 | 行为 |
|---|---|
| `Permit` | 无条件外部转换到目标状态 |
| `PermitIf` | 带守卫的外部转换 — 仅当所有守卫返回 `true` 时才进行 |
| `PermitDynamic` | 目标由选择器函数在运行时决定 |
| `PermitDynamicIf` | 带守卫的动态转换 |
| `PermitReentry` | 重新进入相同状态，触发退出和进入动作 |
| `PermitReentryIf` | 带守卫的重入转换 |
| `Ignore` | 静默忽略事件 — 无转换，无错误 |
| `IgnoreIf` | 条件性忽略事件 |
| `InternalTransition` | 执行动作而不触发进入/退出动作；状态不变 |
| `InternalTransitionIf` | 带守卫的内部转换 |

补充语义：

- `Permit(event, 当前状态)` 属于外部自转换，与 `PermitReentry` 一样会执行退出和进入动作；若只需要执行动作而不触发 entry/exit，请使用 `InternalTransition`。
- `Transition.Destination` 与监听器看到的目标状态是沿 `InitialTransition` 链解析后的最终状态。
- 动态转换的目标在构建期未知，建议显式 `Configure` 可能的目标状态，以便图导出和后续查询完整。

## 层次化状态

使用 `SubstateOf` 声明父子关系。子状态继承父状态的转换，并可以使用 `IsInState` 进行测试。

```go
sm, _ := stateless.NewBuilder[OrderState, OrderEvent](Pending).
	Configure(Pending).
		Permit(Pay, Paid).
	Done().
	Configure(Paid).
		Permit(Ship, Shipped).
	Done().
	SubstateOf(Paid, Pending).       // Paid 是 Pending 的子状态
	InitialTransition(Pending, Paid). // 进入 Pending 时自动转换到 Paid
	Configure(Shipped).
		Permit(Deliver, Delivered).
	Done().
	Configure(Delivered).Done().
	Build()

// IsInState 尊重层次结构
sm.IsInState(Paid, Pending) // true — Paid 是 Pending 的子状态
```

## 带参数的 Fire

通过 `Fire` 传递运行时参数，并在动作和守卫中通过 `Transition.Args` 访问：

```go
sm, _ := stateless.NewBuilder[State, Event](Open).
	Configure(Open).
		Permit(Assign, InProgress).
	Done().
	Configure(InProgress).
		Permit(Resolve, Resolved).
		OnEntryFrom(Assign, func(ctx context.Context, t stateless.Transition[State, Event]) error {
			assignee := t.Args[0].(string)
			fmt.Printf("工单已分配给: %s\n", assignee)
			return nil
		}).
	Done().
	Configure(Resolved).Done().
	Build()

// 传递 "Alice" 作为参数 — 可通过 t.Args[0] 访问
current, _ := sm.Fire(ctx, Open, Assign, "Alice")
```

## 监听器

实现 `Listener` 接口（或嵌入 `ListenerAdapter` 以仅覆盖你需要的方法）：

```go
type Listener[S, E comparable] interface {
	OnTransitioning(ctx context.Context, t Transition[S, E])
	OnTransitioned(ctx context.Context, t Transition[S, E])
	OnUnhandledTrigger(ctx context.Context, state S, event E)
}
```

用法：

```go
type AuditListener struct {
	stateless.ListenerAdapter[MyState, MyEvent]
}

func (l *AuditListener) OnTransitioned(_ context.Context, t stateless.Transition[MyState, MyEvent]) {
	fmt.Printf("已转换: %v -> %v 通过 %v\n", t.Source, t.Destination, t.Event)
}

sm, _ := stateless.NewBuilder[MyState, MyEvent](Initial).
	// ... 状态配置 ...
	WithListener(&AuditListener{}).
	Build()
```

## 调试日志

调试日志**默认关闭**，仅用于开发与测试。通过 `WithDebug(true)` 以 debug 级别向 stderr 输出转换决策：

```go
sm, err := stateless.NewBuilder[State, Event](Locked).
	// ... 状态配置 ...
	WithDebug(true).
	Build()
```

如需自定义输出，可传入 `*slog.Logger`；日志级别由该 logger 自身控制：

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

sm, err := stateless.NewBuilder[State, Event](Locked).
	// ... 状态配置 ...
	WithDebugLogger(logger).
	Build()
```

生产环境请省略这些选项，或传入级别为 `slog.LevelInfo` 及以上的 logger，此时不会产生任何调试输出。记录的日志事件包括 `fire`、`unhandled trigger`、`transition`、`exit`、`enter`、`run transition action` 和 `transition completed`。日志不会记录参数值，只记录参数个数。

所有示例都支持 `-debug` 开关：

```bash
go run ./examples/turnstile -debug
```

## 图导出

生成状态机的可视化表示：

```go
sm, _ := builder.Build()

// Graphviz DOT 格式
dot := sm.ToDotGraph()
fmt.Println(dot)

// Mermaid 状态图格式
mermaid := sm.ToMermaidGraph()
fmt.Println(mermaid)
```

DOT 输出支持层次化集群嵌套、UML 样式初始状态入口点和守卫条件标签。Mermaid 输出使用 `stateDiagram-v2` 语法和嵌套状态块；含括号、空格、中文等字符的状态名会生成安全 ID，并通过 `state "显示名" as id` 保留可读名称。

## 示例

`examples/` 目录包含三个完整的可运行程序：

| 示例 | 描述 |
|---|---|
| [`turnstile`](examples/turnstile/main.go) | 经典闸机 — 演示 `Permit`、`Ignore`、`PermitReentry`、`OnEntry`/`OnExit` 动作 |
| [`order`](examples/order/main.go) | 订单工作流 — 演示 `PermitIf` 守卫、`SubstateOf` 层次化状态、`OnEntryFrom`、`IsInState` 和 `Listener` |
| [`ticket`](examples/ticket/main.go) | 工单系统 — 演示带 `args` 参数传递的 `Fire`、`Transition.Args` 上的守卫验证，以及 `OnEntryFrom` 每个事件的动作 |

运行任何示例（添加 `-debug` 可同时输出库的调试日志）：

```bash
go run ./examples/turnstile
go run ./examples/order
go run ./examples/ticket

go run ./examples/turnstile -debug
```

## 设计理念

- **灵感来源于 Spring State Machine** — 采用核心概念（状态、事件、转换、守卫、动作、层次化状态），以地道的 Go 方式实现
- **面向接口** — 所有公共 API 表面都是接口（`Builder`、`StateConfigurator`、`StateMachine`、`Listener`）；实现未导出
- **构建后不可变** — `Build()` 深度复制并冻结所有配置；生成的 `StateMachine` 可安全地在 goroutine 间共享
- **无状态设计** — 无内部可变状态；调用者拥有并传递当前状态，实现自然的并发而无需锁
- **零外部依赖** — 仅使用 Go 标准库

## 许可证

Apache-2.0
