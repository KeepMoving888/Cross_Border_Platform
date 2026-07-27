package finance

import (
	"fmt"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// matchTolerance 对账金额容差(元):差异在此范围内视为匹配
const matchTolerance = 1

// ReconciliationService 对账自动匹配服务:自动匹配账单与采购单,检测差异并标记状态
type ReconciliationService struct {
	db *gorm.DB
}

// NewReconciliationService 创建对账服务实例
func NewReconciliationService(db *gorm.DB) *ReconciliationService {
	return &ReconciliationService{db: db}
}

// AutoMatch 自动匹配账单与采购单
//  1. 查询所有待对账账单(状态为 draft/matching,即"待对账"状态)
//  2. 根据 bill.order_no 关联 purchase_orders.order_no
//  3. 比较账单应付金额与采购单总金额:
//     a. 差异 <= 1 元(容差) → 标记 matched
//     b. 差异 > 1 元        → 标记 disputed,并在消息中心创建 finance 类型预警(level=warning)
//  4. 未找到匹配采购单的账单跳过(保持待对账状态)
//  5. 返回匹配数与差异数
func (s *ReconciliationService) AutoMatch() (matched int, disputed int, err error) {
	// 1. 查询所有待对账账单(draft=草稿 / matching=对账中,均为待对账状态)
	var bills []models.Bill
	if err = s.db.Where("status IN ?", []string{"draft", "matching"}).Find(&bills).Error; err != nil {
		return 0, 0, fmt.Errorf("查询待对账账单失败: %w", err)
	}
	if len(bills) == 0 {
		logger.Get().Info("对账自动匹配:无待对账账单")
		return 0, 0, nil
	}

	// 预加载需通知用户(admin/manager)
	notifyUserIDs := s.loadNotifyUserIDs()

	tolerance := decimal.NewFromInt(matchTolerance)
	skipped := 0

	for _, bill := range bills {
		// 2. 根据账单 order_no 关联采购单;无 order_no 则跳过(保持待对账)
		if bill.OrderNo == "" {
			skipped++
			continue
		}
		var po models.PurchaseOrder
		if qErr := s.db.Where("order_no = ?", bill.OrderNo).First(&po).Error; qErr != nil {
			if qErr == gorm.ErrRecordNotFound {
				// 未找到匹配的采购单,跳过(保持待对账状态)
				skipped++
				continue
			}
			return matched, disputed, fmt.Errorf("查询采购单 %s 失败: %w", bill.OrderNo, qErr)
		}

		// 3a. 比较账单应付金额与采购单总金额
		diff := bill.PayableAmount.Sub(po.TotalAmount)
		absDiff := diff.Abs()
		now := time.Now()

		// 3b/3c. 根据差异是否超过容差,标记为 matched 或 disputed
		newStatus := "matched"
		if absDiff.GreaterThan(tolerance) {
			newStatus = "disputed"
		}

		if uErr := s.db.Model(&models.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
			"status":      newStatus,
			"diff_amount": diff,
			"matched_at":  &now,
		}).Error; uErr != nil {
			return matched, disputed, fmt.Errorf("更新账单 %s 状态失败: %w", bill.BillNo, uErr)
		}

		if newStatus == "matched" {
			matched++
		} else {
			// 差异超容差,在消息中心创建 finance 类型预警(level=warning)
			s.createDisputeMessages(bill, po, diff, notifyUserIDs)
			disputed++
		}
	}

	logger.Get().Infof("对账自动匹配完成: 匹配=%d, 差异=%d, 跳过=%d, 待对账总数=%d",
		matched, disputed, skipped, len(bills))
	return matched, disputed, nil
}

// loadNotifyUserIDs 加载需通知的用户 ID(admin/manager)
func (s *ReconciliationService) loadNotifyUserIDs() []uint {
	var ids []uint
	s.db.Model(&models.User{}).
		Where("role IN ? AND status = ?", []string{"admin", "manager"}, models.StatusEnabled).
		Pluck("id", &ids)
	return ids
}

// createDisputeMessages 为差异账单创建消息中心预警(finance 类型,warning 级别)
func (s *ReconciliationService) createDisputeMessages(bill models.Bill, po models.PurchaseOrder, diff decimal.Decimal, notifyUserIDs []uint) {
	if len(notifyUserIDs) == 0 {
		return
	}
	title := fmt.Sprintf("对账差异预警 · %s", bill.BillNo)
	content := fmt.Sprintf("账单 %s 与采购单 %s 金额存在差异:应付 %s, 采购单总额 %s, 差异 %s,请核对处理。",
		bill.BillNo, po.OrderNo, bill.PayableAmount.String(), po.TotalAmount.String(), diff.String())
	messages := make([]models.Message, 0, len(notifyUserIDs))
	for _, uid := range notifyUserIDs {
		messages = append(messages, models.Message{
			UserID:  uid,
			Type:    "finance",
			Level:   "warning",
			Title:   title,
			Content: content,
			RefType: "bill",
			RefID:   fmt.Sprintf("%d", bill.ID),
			Link:    "/finance",
			IsRead:  false,
		})
	}
	if err := s.db.Create(&messages).Error; err != nil {
		logger.Get().Warnf("创建对账差异预警消息失败 账单 %s: %v", bill.BillNo, err)
	}
}
