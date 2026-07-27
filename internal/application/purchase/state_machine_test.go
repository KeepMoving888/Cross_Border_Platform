package purchase

import (
	"testing"

	"github.com/cb-platform/internal/domain/models"
)

func TestNewStateMachine_DefaultState(t *testing.T) {
	sm := NewStateMachine(models.PurchaseStatusInquiry)
	if sm.Current() != models.PurchaseStatusInquiry {
		t.Errorf("expected %s, got %s", models.PurchaseStatusInquiry, sm.Current())
	}
}

func TestStateMachine_LegalTransition(t *testing.T) {
	tests := []struct {
		name        string
		from        string
		event       string
		expectTo    string
		expectError bool
	}{
		{"询价->比价", models.PurchaseStatusInquiry, EventQuote, models.PurchaseStatusQuoting, false},
		{"比价->下单", models.PurchaseStatusQuoting, EventSelectQuote, models.PurchaseStatusOrdered, false},
		{"下单->发货", models.PurchaseStatusOrdered, EventShip, models.PurchaseStatusShipped, false},
		{"发货->入库", models.PurchaseStatusShipped, EventReceive, models.PurchaseStatusReceived, false},
		{"入库->质检", models.PurchaseStatusReceived, EventQC, models.PurchaseStatusQC, false},
		{"质检->对账", models.PurchaseStatusQC, EventReconcile, models.PurchaseStatusReconciling, false},
		{"对账->结算", models.PurchaseStatusReconciling, EventSettle, models.PurchaseStatusSettled, false},
		{"下单->入库(跨状态合法)", models.PurchaseStatusOrdered, EventReceive, models.PurchaseStatusReceived, false},
		{"询价->结算(非法)", models.PurchaseStatusInquiry, EventSettle, "", true},
		{"已结算->发货(非法)", models.PurchaseStatusSettled, EventShip, "", true},
		{"已取消->恢复询价", models.PurchaseStatusCancelled, EventReopen, models.PurchaseStatusInquiry, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine(tt.from)
			err := sm.Transition(tt.event)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for %s from %s, but got nil", tt.event, tt.from)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if sm.Current() != tt.expectTo {
				t.Errorf("expected %s, got %s", tt.expectTo, sm.Current())
			}
		})
	}
}

func TestStateMachine_CancelFromAnyActiveState(t *testing.T) {
	activeStates := []string{
		models.PurchaseStatusInquiry,
		models.PurchaseStatusQuoting,
		models.PurchaseStatusOrdered,
		models.PurchaseStatusTracking,
		models.PurchaseStatusShipped,
	}
	for _, s := range activeStates {
		sm := NewStateMachine(s)
		if !sm.CanTransition(EventCancel) {
			t.Errorf("state %s should allow cancel", s)
		}
	}
}

func TestStateMachine_SettleOnlyFromReconciling(t *testing.T) {
	// 已结算状态不能再结算
	sm := NewStateMachine(models.PurchaseStatusSettled)
	if sm.CanTransition(EventSettle) {
		t.Error("settled state should not allow settle again")
	}
}

func TestStateMachine_AllEventsHaveTarget(t *testing.T) {
	events := AllEvents()
	if len(events) == 0 {
		t.Error("expected non-empty events list")
	}
	for _, e := range events {
		_, err := EventToStatus(e)
		if err != nil {
			t.Errorf("event %s has no target status: %v", e, err)
		}
	}
}

func TestStateMachine_StatusLabel(t *testing.T) {
	tests := []struct {
		status string
		label  string
	}{
		{models.PurchaseStatusInquiry, "询价中"},
		{models.PurchaseStatusOrdered, "已下单"},
		{models.PurchaseStatusSettled, "已结算"},
		{models.PurchaseStatusCancelled, "已取消"},
		{"unknown_status", "unknown_status"},
	}
	for _, tt := range tests {
		if got := StatusLabel(tt.status); got != tt.label {
			t.Errorf("status %s: expected %s, got %s", tt.status, tt.label, got)
		}
	}
}

func TestStateMachine_AllStates(t *testing.T) {
	states := AllStates()
	if len(states) != 10 {
		t.Errorf("expected 10 states, got %d", len(states))
	}
}
