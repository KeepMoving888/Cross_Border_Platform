package purchase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cb-platform/internal/domain/models"
	"github.com/looplab/fsm"
)

// 状态机事件
const (
	EventInquire      = "inquire"       // 询价
	EventQuote        = "quote"         // 报价
	EventSelectQuote  = "select_quote"  // 选定报价
	EventOrder        = "order"         // 下单
	EventShip         = "ship"          // 发货
	EventReceive      = "receive"       // 入库
	EventQC           = "qc"            // 质检
	EventReconcile    = "reconcile"     // 对账
	EventSettle       = "settle"        // 结算
	EventCancel       = "cancel"        // 取消
	EventReopen       = "reopen"        // 重新开启(从已取消恢复到询价)
)

// 状态转换规则(描述业务流程的合法路径)
var stateTransitions = fsm.Events{
	{Name: EventInquire, Src: []string{""}, Dst: models.PurchaseStatusInquiry},
	{Name: EventQuote, Src: []string{models.PurchaseStatusInquiry}, Dst: models.PurchaseStatusQuoting},
	{Name: EventSelectQuote, Src: []string{models.PurchaseStatusQuoting, models.PurchaseStatusInquiry}, Dst: models.PurchaseStatusOrdered},
	{Name: EventOrder, Src: []string{models.PurchaseStatusQuoting, models.PurchaseStatusInquiry}, Dst: models.PurchaseStatusOrdered},
	{Name: EventShip, Src: []string{models.PurchaseStatusOrdered}, Dst: models.PurchaseStatusShipped},
	{Name: EventReceive, Src: []string{models.PurchaseStatusOrdered, models.PurchaseStatusShipped, models.PurchaseStatusTracking}, Dst: models.PurchaseStatusReceived},
	{Name: EventQC, Src: []string{models.PurchaseStatusReceived}, Dst: models.PurchaseStatusQC},
	{Name: EventReconcile, Src: []string{models.PurchaseStatusQC, models.PurchaseStatusReceived}, Dst: models.PurchaseStatusReconciling},
	{Name: EventSettle, Src: []string{models.PurchaseStatusReconciling}, Dst: models.PurchaseStatusSettled},
	{Name: EventCancel, Src: []string{models.PurchaseStatusInquiry, models.PurchaseStatusQuoting, models.PurchaseStatusOrdered, models.PurchaseStatusTracking, models.PurchaseStatusShipped}, Dst: models.PurchaseStatusCancelled},
	{Name: EventReopen, Src: []string{models.PurchaseStatusCancelled}, Dst: models.PurchaseStatusInquiry},
}

// 状态机管理器(每个订单一个独立实例)
type StateMachine struct {
	fsm *fsm.FSM
}

var (
	fsmOnce     sync.Once
	sharedFSM   *fsm.FSM
	callbacks   = fsm.Callbacks{}
)

// NewStateMachine 创建订单状态机
func NewStateMachine(currentState string) *StateMachine {
	fsmOnce.Do(func() {
		sharedFSM = fsm.NewFSM(models.PurchaseStatusInquiry, stateTransitions, callbacks)
	})
	// 每个订单独立状态机,基于当前状态
	f := fsm.NewFSM(currentState, stateTransitions, fsm.Callbacks{})
	return &StateMachine{fsm: f}
}

// CanTransition 判断事件是否可触发
func (sm *StateMachine) CanTransition(event string) bool {
	return sm.fsm.Can(event)
}

// Transition 触发状态转换
func (sm *StateMachine) Transition(event string) error {
	if !sm.fsm.Can(event) {
		current := sm.fsm.Current()
		allowed := sm.allowedEventsFromState(current)
		return fmt.Errorf("状态 %s 不允许事件 %s,允许的事件: %s",
			current, event, strings.Join(allowed, ", "))
	}
	err := sm.fsm.Event(context.Background(), event)
	if err != nil {
		return fmt.Errorf("状态转换失败: %w", err)
	}
	return nil
}

// Current 当前状态
func (sm *StateMachine) Current() string {
	return sm.fsm.Current()
}

// allowedEventsFromState 获取状态允许的事件
func (sm *StateMachine) allowedEventsFromState(state string) []string {
	events := []string{}
	for _, e := range stateTransitions {
		for _, s := range e.Src {
			if s == state {
				events = append(events, e.Name+" -> "+e.Dst)
			}
		}
	}
	return events
}

// EventToStatus 事件对应的目标状态
func EventToStatus(event string) (string, error) {
	for _, e := range stateTransitions {
		if e.Name == event {
			return e.Dst, nil
		}
	}
	return "", fmt.Errorf("未知事件: %s", event)
}

// AllEvents 所有事件列表
func AllEvents() []string {
	events := make([]string, 0, len(stateTransitions))
	for _, e := range stateTransitions {
		events = append(events, e.Name)
	}
	return events
}

// AllStates 所有状态列表
func AllStates() []string {
	states := map[string]bool{
		models.PurchaseStatusInquiry:    true,
		models.PurchaseStatusQuoting:     true,
		models.PurchaseStatusOrdered:     true,
		models.PurchaseStatusTracking:    true,
		models.PurchaseStatusShipped:     true,
		models.PurchaseStatusReceived:    true,
		models.PurchaseStatusQC:          true,
		models.PurchaseStatusReconciling: true,
		models.PurchaseStatusSettled:     true,
		models.PurchaseStatusCancelled:   true,
	}
	result := make([]string, 0, len(states))
	for s := range states {
		result = append(result, s)
	}
	return result
}

// StatusLabel 状态中文标签
func StatusLabel(status string) string {
	labels := map[string]string{
		models.PurchaseStatusInquiry:    "询价中",
		models.PurchaseStatusQuoting:     "比价中",
		models.PurchaseStatusOrdered:     "已下单",
		models.PurchaseStatusTracking:    "跟单中",
		models.PurchaseStatusShipped:     "已发货",
		models.PurchaseStatusReceived:    "已入库",
		models.PurchaseStatusQC:          "质检中",
		models.PurchaseStatusReconciling: "对账中",
		models.PurchaseStatusSettled:     "已结算",
		models.PurchaseStatusCancelled:   "已取消",
	}
	if l, ok := labels[status]; ok {
		return l
	}
	return status
}
