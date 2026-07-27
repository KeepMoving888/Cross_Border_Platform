package database

import (
	"fmt"
	"time"

	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// SeedData 初始化业务数据(开发环境与首次部署使用)
// 行业:家电美容(个护电器 + 美容仪器 + 美体仪器 + 配件耗材)
func SeedData(db *gorm.DB) error {
	if err := seedDefaults(db); err != nil {
		return err
	}
	if err := seedBusinessData(db); err != nil {
		return err
	}
	if err := seedMessagesAndEnrichInventory(db); err != nil {
		return err
	}
	return nil
}

// seedMessagesAndEnrichInventory 独立幂等：消息中心 + 多仓库存补齐(已有库也能升级)
func seedMessagesAndEnrichInventory(db *gorm.DB) error {
	// 1) 多仓覆盖不足 / 断货预警占比过高时重建库存与预警
	var whCovered int64
	db.Model(&models.Inventory{}).Distinct("warehouse_id").Count(&whCovered)
	var invTotal int64
	db.Model(&models.Inventory{}).Count(&invTotal)
	var alertTotal, outOfStockTotal int64
	db.Model(&models.StockAlert{}).Where("status = ?", "pending").Count(&alertTotal)
	db.Model(&models.StockAlert{}).Where("status = ? AND type = ?", "pending", "out_of_stock").Count(&outOfStockTotal)
	var prodCount int64
	db.Model(&models.Product{}).Count(&prodCount)
	needRebuild := false
	if invTotal > 0 && whCovered < 3 {
		needRebuild = true
	}
	// 库存为空但有选品数据 → 重建
	if invTotal == 0 && prodCount > 0 {
		needRebuild = true
	}
	// 断货占比过高(>40%) 也重建，避免看板全是「无货」
	if alertTotal >= 5 && outOfStockTotal*100/alertTotal > 40 {
		needRebuild = true
	}
	if needRebuild {
		var products []models.Product
		db.Find(&products)
		if len(products) > 0 {
			db.Exec("DELETE FROM stock_alerts")
			db.Exec("DELETE FROM inventory_movements")
			db.Exec("DELETE FROM inventories")
			invs := buildInventories(products)
			if len(invs) > 0 {
				if err := db.Create(&invs).Error; err != nil {
					return err
				}
				movs := buildMovements(invs)
				if len(movs) > 0 {
					_ = db.Create(&movs).Error
				}
				alerts := buildStockAlerts(invs)
				if len(alerts) > 0 {
					_ = db.Create(&alerts).Error
				}
				logger.Get().Infof("enriched multi-warehouse inventories: %d rows, alerts: %d (wh=%d)", len(invs), len(alerts), whCovered)
			}
		}
	}

	// 1.3) 利润报表重建: 清除旧数据并用新参数重新生成(确保今日有数据 + 合理金额 + 无规律波动)
	var profitCount int64
	db.Model(&models.ProfitReport{}).Count(&profitCount)
	if profitCount > 0 {
		var todayProfit int64
		db.Model(&models.ProfitReport{}).Where("stat_date = ?", time.Now().Format("2006-01-02")).Count(&todayProfit)
		if todayProfit == 0 {
			// 旧数据不含今天, 需重建
			db.Where("1=1").Delete(&models.ProfitReport{})
			profitCount = 0
		}
	}
	if profitCount == 0 {
		var products []models.Product
		db.Find(&products)
		profits := buildProfitReports(products)
		if len(profits) > 0 {
			if err := db.Create(&profits).Error; err != nil {
				logger.Get().Warnf("rebuild profit reports failed: %v", err)
			} else {
				logger.Get().Infof("rebuilt %d profit reports (today included, realistic margins)", len(profits))
			}
		}
	}

	// 1.5) AI 执行记录补齐：表为空但工作流定义已存在 → 生成 30 天波动数据
	var aiRunCount int64
	db.Model(&models.AIWorkflowRun{}).Count(&aiRunCount)
	if aiRunCount == 0 {
		var wfCount int64
		db.Model(&models.AIWorkflow{}).Count(&wfCount)
		if wfCount > 0 {
			extRuns := buildAIWorkflowRunsExt()
			if len(extRuns) > 0 {
				if err := db.Create(&extRuns).Error; err != nil {
					logger.Get().Warnf("seed AI workflow runs ext failed: %v", err)
				} else {
					baseRuns := buildAIWorkflowRuns()
					if len(baseRuns) > 0 {
						_ = db.Create(&baseRuns).Error
					}
					logger.Get().Infof("enriched AI workflow runs: %d base + %d ext", len(baseRuns), len(extRuns))
				}
			}
		}
	}

	// 2) 消息中心种子
	var msgCount int64
	db.Model(&models.Message{}).Count(&msgCount)
	if msgCount > 0 {
		return nil
	}

	var users []models.User
	db.Find(&users)
	if len(users) == 0 {
		return nil
	}

	var alerts []models.StockAlert
	db.Where("status = ?", "pending").Order("id DESC").Limit(8).Find(&alerts)

	var bills []models.Bill
	db.Where("status IN ?", []string{"draft", "matching", "disputed"}).Limit(5).Find(&bills)

	var orders []models.PurchaseOrder
	db.Where("status IN ?", []string{"ordered", "tracking", "shipped"}).Limit(5).Find(&orders)

	whName := map[uint]string{
		1: "深圳主仓", 2: "美西海外仓", 3: "欧洲海外仓", 4: "亚马逊 FBA 仓", 5: "亚马逊 FBA 欧洲仓",
	}

	messages := make([]models.Message, 0, 40)
	now := time.Now()
	for _, u := range users {
		// 系统欢迎
		messages = append(messages, models.Message{
			UserID: u.ID, Type: "system", Level: "info",
			Title: "欢迎使用 CB-Platform 智能运营中台",
			Content: "选品 → 采购 → 库存 → 对账 全链路已就绪。请从工作台查看今日经营概览。",
			Link: "/dashboard", IsRead: false,
			BaseModel: models.BaseModel{CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-48 * time.Hour)},
		})
		// 库存预警
		for i, a := range alerts {
			if i >= 4 {
				break
			}
			level := "warning"
			title := "库存偏低预警"
			if a.Type == "out_of_stock" || a.CurrentQty == 0 {
				level = "critical"
				title = "断货预警"
			}
			messages = append(messages, models.Message{
				UserID: u.ID, Type: "stock_alert", Level: level,
				Title: title + " · " + a.SKU,
				Content: fmt.Sprintf("%s · SKU %s 当前库存 %d，安全库存 %d，请尽快补货。",
					whName[a.WarehouseID], a.SKU, a.CurrentQty, a.Threshold),
				RefType: "inventory", RefID: fmt.Sprintf("%d", a.ID),
				Link: "/inventory", IsRead: i > 1,
				BaseModel: models.BaseModel{CreatedAt: now.Add(-time.Duration(i+1) * time.Hour), UpdatedAt: now},
			})
		}
		// 采购跟进
		for i, o := range orders {
			if i >= 2 {
				break
			}
			messages = append(messages, models.Message{
				UserID: u.ID, Type: "purchase", Level: "info",
				Title: "采购单待跟进 · " + o.OrderNo,
				Content: fmt.Sprintf("%s 状态为 %s，预计到货请关注履约进度。", o.ProductName, o.Status),
				RefType: "purchase", RefID: fmt.Sprintf("%d", o.ID),
				Link: "/purchases", IsRead: false,
				BaseModel: models.BaseModel{CreatedAt: now.Add(-time.Duration(i+3) * time.Hour), UpdatedAt: now},
			})
		}
		// 对账提醒
		for i, b := range bills {
			if i >= 2 {
				break
			}
			level := "warning"
			if b.Status == "disputed" {
				level = "critical"
			}
			messages = append(messages, models.Message{
				UserID: u.ID, Type: "finance", Level: level,
				Title: "对账待处理 · " + b.BillNo,
				Content: fmt.Sprintf("账单状态 %s，应付金额请核对后完成对账/付款。", b.Status),
				RefType: "bill", RefID: fmt.Sprintf("%d", b.ID),
				Link: "/finance", IsRead: false,
				BaseModel: models.BaseModel{CreatedAt: now.Add(-time.Duration(i+5) * time.Hour), UpdatedAt: now},
			})
		}
	}

	if len(messages) > 0 {
		if err := db.Create(&messages).Error; err != nil {
			return err
		}
		logger.Get().Infof("seeded %d messages", len(messages))
	}
	return nil
}

// seedDefaults 初始化基础数据(用户、仓库、AI 工作流模板)
func seedDefaults(db *gorm.DB) error {
	// 1. 内部用户(覆盖运营/采购/财务/库管等岗位)
	var userCount int64
	db.Model(&models.User{}).Count(&userCount)
	if userCount == 0 {
		hashed, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		users := []models.User{
			{Username: "admin", Password: string(hashed), RealName: "系统管理员", Role: "admin", Department: "技术部", Status: models.StatusEnabled, Email: "admin@cb-platform.example"},
			{Username: "purchaser", Password: string(hashed), RealName: "李采购", Role: "manager", Department: "采购部", Status: models.StatusEnabled, Email: "purchase@cb-platform.example"},
			{Username: "operator", Password: string(hashed), RealName: "王运营", Role: "manager", Department: "运营部", Status: models.StatusEnabled, Email: "ops@cb-platform.example"},
			{Username: "finance", Password: string(hashed), RealName: "陈财务", Role: "staff", Department: "财务部", Status: models.StatusEnabled, Email: "finance@cb-platform.example"},
			{Username: "warehouse", Password: string(hashed), RealName: "赵库管", Role: "staff", Department: "仓储部", Status: models.StatusEnabled, Email: "wh@cb-platform.example"},
		}
		for i := range users {
			if err := db.Create(&users[i]).Error; err != nil {
				logger.Get().Warnf("create user %s failed: %v", users[i].Username, err)
			}
		}
		logger.Get().Infof("default %d users created (admin/admin123)", len(users))
	}

	// 2. 仓库(覆盖国内主仓、海外仓、FBA 仓)
	var whCount int64
	db.Model(&models.Warehouse{}).Count(&whCount)
	if whCount == 0 {
		warehouses := []models.Warehouse{
			{Code: "WH-CN-01", Name: "深圳主仓", Type: "domestic", Country: "CN", Address: "深圳市宝安区西乡街道", Manager: "赵库管", Status: models.StatusEnabled},
			{Code: "WH-US-01", Name: "美西海外仓", Type: "overseas", Country: "US", Address: "Los Angeles, CA 90001", Manager: "Tom Wang", Status: models.StatusEnabled},
			{Code: "WH-EU-01", Name: "欧洲海外仓", Type: "overseas", Country: "DE", Address: "Frankfurt, Germany", Manager: "Hans Müller", Status: models.StatusEnabled},
			{Code: "WH-FBA-01", Name: "亚马逊 FBA 仓", Type: "fba", Country: "US", Manager: "Amazon FBA", Status: models.StatusEnabled},
			{Code: "WH-FBA-02", Name: "亚马逊 FBA 欧洲仓", Type: "fba", Country: "DE", Manager: "Amazon FBA EU", Status: models.StatusEnabled},
		}
		for i := range warehouses {
			if err := db.Create(&warehouses[i]).Error; err != nil {
				return err
			}
		}
		logger.Get().Infof("default %d warehouses created", len(warehouses))
	}

	// 3. 供应商(家电美容产业链,覆盖深圳/东莞/义乌/广州等产地)
	var supCount int64
	db.Model(&models.Supplier{}).Count(&supCount)
	if supCount == 0 {
		suppliers := []models.Supplier{
			{Name: "深圳市韩姿电子科技有限公司", Code: "SUP-HZ-001", ContactName: "刘华", Phone: "13800138001", Email: "liu.hua@hanzi-tech.cn", Region: "广东深圳", Address: "深圳市龙华区大浪街道", PaymentTerms: "deposit_balance", SettlementCycle: "net_30", Rating: "A", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(2860000), OnTimeRate: decimal.NewFromFloat(96.5), QualityRate: decimal.NewFromFloat(98.2)},
			{Name: "东莞市臻美电器制造有限公司", Code: "SUP-ZM-002", ContactName: "陈志强", Phone: "13900139002", Email: "chen@zhenmei-dg.com", Region: "广东东莞", Address: "东莞市长安镇厦边社区", PaymentTerms: "net_30", SettlementCycle: "net_45", Rating: "A", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(1920000), OnTimeRate: decimal.NewFromFloat(94.8), QualityRate: decimal.NewFromFloat(97.1)},
			{Name: "深圳市慕岚光电科技有限公司", Code: "SUP-ML-003", ContactName: "黄美玲", Phone: "13700137003", Email: "huang@mulan-opto.cn", Region: "广东深圳", Address: "深圳市宝安区福永街道", PaymentTerms: "deposit_balance", SettlementCycle: "net_30", Rating: "B", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(856000), OnTimeRate: decimal.NewFromFloat(91.2), QualityRate: decimal.NewFromFloat(95.8)},
			{Name: "义乌市丝蓓丽日用品有限公司", Code: "SUP-SBL-004", ContactName: "王素琴", Phone: "13600136004", Email: "wang@sbeili-yw.com", Region: "浙江义乌", Address: "义乌市北苑街道", PaymentTerms: "prepay", SettlementCycle: "net_15", Rating: "B", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(328000), OnTimeRate: decimal.NewFromFloat(98.5), QualityRate: decimal.NewFromFloat(96.4)},
			{Name: "深圳市肌秘生物科技有限公司", Code: "SUP-JM-005", ContactName: "张博士", Phone: "13500135005", Email: "zhang@jimi-bio.cn", Region: "广东深圳", Address: "深圳市南山区科技园", PaymentTerms: "deposit_balance", SettlementCycle: "net_30", Rating: "A", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(1245000), OnTimeRate: decimal.NewFromFloat(95.0), QualityRate: decimal.NewFromFloat(99.1)},
			{Name: "佛山市顺德区艾尚美电器厂", Code: "SUP-ASM-006", ContactName: "周明", Phone: "13400134006", Email: "zhou@aishangmei.cn", Region: "广东佛山", Address: "佛山市顺德区容桂街道", PaymentTerms: "net_30", SettlementCycle: "net_30", Rating: "B", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(612000), OnTimeRate: decimal.NewFromFloat(89.5), QualityRate: decimal.NewFromFloat(93.7)},
			{Name: "广州市白云区雅菲思化妆品厂", Code: "SUP-YFS-007", ContactName: "林雅", Phone: "13300133007", Email: "lin@yafisi-gz.com", Region: "广东广州", Address: "广州市白云区钟落潭镇", PaymentTerms: "prepay", SettlementCycle: "net_15", Rating: "C", CoopStatus: "suspended", TotalAmount: decimal.NewFromFloat(186000), OnTimeRate: decimal.NewFromFloat(82.0), QualityRate: decimal.NewFromFloat(88.5)},
			{Name: "宁波市鄞州凯迅电子有限公司", Code: "SUP-KX-008", ContactName: "吴凯", Phone: "13200132008", Email: "wu@kaixun-nb.cn", Region: "浙江宁波", Address: "宁波市鄞州区下应街道", PaymentTerms: "net_60", SettlementCycle: "net_60", Rating: "A", CoopStatus: "active", TotalAmount: decimal.NewFromFloat(1580000), OnTimeRate: decimal.NewFromFloat(97.8), QualityRate: decimal.NewFromFloat(98.9)},
		}
		for i := range suppliers {
			if err := db.Create(&suppliers[i]).Error; err != nil {
				return err
			}
		}
		logger.Get().Infof("default %d suppliers created", len(suppliers))
	}

	// 4. 默认 AI 工作流
	var wfCount int64
	db.Model(&models.AIWorkflow{}).Count(&wfCount)
	if wfCount == 0 {
		workflows := defaultAIWorkflows()
		for i := range workflows {
			if err := db.Create(&workflows[i]).Error; err != nil {
				return err
			}
		}
		logger.Get().Infof("default %d AI workflows created", len(workflows))
	}

	// 5. 默认 Prompt 模板
	var ptCount int64
	db.Model(&models.PromptTemplate{}).Count(&ptCount)
	if ptCount == 0 {
		templates := defaultPromptTemplates()
		for i := range templates {
			if err := db.Create(&templates[i]).Error; err != nil {
				return err
			}
		}
		logger.Get().Infof("default %d prompt templates created", len(templates))
	}

	// 6. 知识库(家电美容行业)
	var kbCount int64
	db.Model(&models.KnowledgeBase{}).Count(&kbCount)
	if kbCount == 0 {
		kbs := []models.KnowledgeBase{
			{Name: "产品说明书知识库", Code: "KB-PRODUCT-MANUAL", Description: "家电美容产品说明书与使用指引", Type: "product_manual", EmbeddingModel: "bge-large-zh", Dimension: 1024, DocumentCount: 28, Status: "enabled"},
			{Name: "采购合同知识库", Code: "KB-PURCHASE-CONTRACT", Description: "供应商合同模板与条款", Type: "purchase_contract", EmbeddingModel: "bge-large-zh", Dimension: 1024, DocumentCount: 12, Status: "enabled"},
			{Name: "客服 FAQ 知识库", Code: "KB-CS-FAQ", Description: "客服常见问题与回复模板", Type: "faq", EmbeddingModel: "bge-large-zh", Dimension: 1024, DocumentCount: 156, Status: "enabled"},
		}
		for i := range kbs {
			if err := db.Create(&kbs[i]).Error; err != nil {
				return err
			}
		}
		logger.Get().Infof("default %d knowledge bases created", len(kbs))
	}

	return nil
}

// seedBusinessData 业务数据(选品、供应商商品、采购、库存、对账、利润)
func seedBusinessData(db *gorm.DB) error {
	// 选品池
	var prodCount int64
	db.Model(&models.Product{}).Count(&prodCount)
	if prodCount > 0 {
		return nil // 已有选品数据则跳过
	}

	// 获取供应商 ID 映射
	var suppliers []models.Supplier
	db.Find(&suppliers)
	supMap := make(map[string]uint)
	for _, s := range suppliers {
		supMap[s.Code] = s.ID
	}

	// 获取 admin 用户 ID
	var admin models.User
	db.Where("username = ?", "admin").First(&admin)

	// ==================== 选品池(15 个家电美容产品) ====================
	products := []models.Product{
		// 个护电器类
		{SKU: "HD-PRO-2026", ASIN: "B0D1HJ2K3L", Name: "负离子速干吹风机 1800W 旅行家用", ImageURL: "", Category: "个护电器", SubCategory: "吹风机", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(69.99), EstCostPrice: decimal.NewFromFloat(18.50), EstMarginRate: decimal.NewFromFloat(73.6), Currency: "USD", AIScore: decimal.NewFromFloat(87.5), MonthlySales: 2400, ReviewCount: 186, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ZM-002"]), Tags: "负离子,速干,1800W,旅行,恒温", AIInsight: "美国市场年度搜索增长 18%,差异化卖点集中在外观与噪音控制,头部竞品均价 USD 79.99,本 SKU 定价 69.99 具竞争力"},
		{SKU: "CF-3IN1-CURL", ASIN: "B0D2KL4M5N", Name: "三合一卷直两用器 陶瓷涂层", Category: "个护电器", SubCategory: "卷发棒", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(49.99), EstCostPrice: decimal.NewFromFloat(14.20), EstMarginRate: decimal.NewFromFloat(71.6), Currency: "USD", AIScore: decimal.NewFromFloat(82.0), MonthlySales: 1800, ReviewCount: 142, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-HZ-001"]), Tags: "三合一,卷直两用,陶瓷涂层,防烫"},
		{SKU: "HS-CER-PRO", ASIN: "B0D3MN6P7Q", Name: "陶瓷直发梳 30 秒速热", Category: "个护电器", SubCategory: "直发器", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(39.99), EstCostPrice: decimal.NewFromFloat(11.80), EstMarginRate: decimal.NewFromFloat(70.4), Currency: "EUR", AIScore: decimal.NewFromFloat(79.5), MonthlySales: 980, ReviewCount: 87, Rating: decimal.NewFromFloat(4.4), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-HZ-001"]), Tags: "陶瓷,30秒速热,LED显示"},
		{SKU: "ST-SONIC-10", ASIN: "B0D4PQ8R9S", Name: "声波电动牙刷 38000次/分 5 档模式", Category: "个护电器", SubCategory: "电动牙刷", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(29.99), EstCostPrice: decimal.NewFromFloat(7.50), EstMarginRate: decimal.NewFromFloat(75.0), Currency: "USD", AIScore: decimal.NewFromFloat(91.2), MonthlySales: 5200, ReviewCount: 482, Rating: decimal.NewFromFloat(4.7), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-KX-008"]), Tags: "声波,38000次,5档,IPX7", AIInsight: "美国市场年搜索量 480K+,长尾词'quiet sonic toothbrush'竞争度低,适合作为引流款"},
		{SKU: "WF-CORDLESS", ASIN: "B0D5RS1T2U", Name: "便携冲牙器 无线 300ML 大容量", Category: "个护电器", SubCategory: "冲牙器", Stage: models.ProductStageTesting, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(36.99), EstCostPrice: decimal.NewFromFloat(10.80), EstMarginRate: decimal.NewFromFloat(70.8), Currency: "USD", AIScore: decimal.NewFromFloat(78.0), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-KX-008"]), Tags: "便携,无线,300ML,3档"},

		// 美容仪器类
		{SKU: "FC-SONIC-10", ASIN: "B0D6TV3U4V", Name: "声波洁面仪 硅胶刷毛 15 档", Category: "美容仪器", SubCategory: "洁面仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(45.99), EstCostPrice: decimal.NewFromFloat(12.50), EstMarginRate: decimal.NewFromFloat(72.8), Currency: "USD", AIScore: decimal.NewFromFloat(85.0), MonthlySales: 1600, ReviewCount: 198, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-JM-005"]), Tags: "声波,硅胶,15档,IPX7"},
		{SKU: "IM-EXP-PRO", ASIN: "B0D7UW5V6W", Name: "导入导出美容仪 离子+微电流", Category: "美容仪器", SubCategory: "导入导出仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(89.99), EstCostPrice: decimal.NewFromFloat(26.80), EstMarginRate: decimal.NewFromFloat(70.2), Currency: "EUR", AIScore: decimal.NewFromFloat(81.5), MonthlySales: 720, ReviewCount: 86, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-JM-005"]), Tags: "离子,微电流,导入导出,9V电池"},
		{SKU: "RF-MULTI-5", ASIN: "B0D8VX7W8X", Name: "多极射频美容仪 RF+EMS+LED", Category: "美容仪器", SubCategory: "射频仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(149.99), EstCostPrice: decimal.NewFromFloat(48.50), EstMarginRate: decimal.NewFromFloat(67.7), Currency: "USD", AIScore: decimal.NewFromFloat(88.7), MonthlySales: 420, ReviewCount: 64, Rating: decimal.NewFromFloat(4.4), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-JM-005"]), Tags: "多极射频,EMS,LED红光,抗衰老", AIInsight: "抗衰老品类年增长率 24%,客单价高(>USD 100),复购率低但毛利可观,适合主推利润款"},
		{SKU: "LED-LIGHT-7", ASIN: "B0D9WY8X9Y", Name: "七色 LED 光疗面罩 152 颗灯珠", Category: "美容仪器", SubCategory: "LED面罩", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(129.00), EstCostPrice: decimal.NewFromFloat(42.00), EstMarginRate: decimal.NewFromFloat(67.4), Currency: "USD", AIScore: decimal.NewFromFloat(86.0), MonthlySales: 580, ReviewCount: 73, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ML-003"]), Tags: "七色,152灯珠,红光蓝光,抗痘抗老"},
		{SKU: "EMS-MICRO", ASIN: "B0DAXZ9Y0Z", Name: "微电流提拉美容仪 颈部面部两用", Category: "美容仪器", SubCategory: "微电流仪", Stage: models.ProductStageSourcing, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(79.99), EstCostPrice: decimal.NewFromFloat(22.00), EstMarginRate: decimal.NewFromFloat(72.5), Currency: "USD", AIScore: decimal.NewFromFloat(74.5), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ML-003"]), Tags: "微电流,提拉,颈部面部,无创"},

		// 美体仪器类
		{SKU: "MS-NECK-HEAT", ASIN: "B0DBYA1Z2A", Name: "颈部按摩仪 恒温热敷 6 模式", Category: "美体仪器", SubCategory: "按摩仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(59.99), EstCostPrice: decimal.NewFromFloat(17.80), EstMarginRate: decimal.NewFromFloat(70.3), Currency: "USD", AIScore: decimal.NewFromFloat(83.2), MonthlySales: 1200, ReviewCount: 156, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "颈部,恒温热敷,6模式,折叠"},
		{SKU: "SLIM-RF-PRO", ASIN: "B0DCZB2A3B", Name: "射频瘦身仪 多功能燃脂", Category: "美体仪器", SubCategory: "瘦身仪", Stage: models.ProductStageTesting, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(119.00), EstCostPrice: decimal.NewFromFloat(36.50), EstMarginRate: decimal.NewFromFloat(69.3), Currency: "EUR", AIScore: decimal.NewFromFloat(76.8), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "射频,瘦身,多功能,LED显示"},
		{SKU: "IPL-HAIR-PRO", ASIN: "B0DDAC4B5C", Name: "光子脱毛仪 IPL 50万次闪光", Category: "美体仪器", SubCategory: "脱毛仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(99.99), EstCostPrice: decimal.NewFromFloat(31.20), EstMarginRate: decimal.NewFromFloat(68.8), Currency: "USD", AIScore: decimal.NewFromFloat(89.5), MonthlySales: 1850, ReviewCount: 248, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-HZ-001"]), Tags: "IPL,50万次,5档,冰点", AIInsight: "脱毛品类 Q4 季节性爆发,年同比 +32%,头部品牌 Tria / Braun 均价 USD 199+,本 SKU 价格优势明显"},

		// 配件耗材类
		{SKU: "BR-REPLACE-3", ASIN: "B0DEBD6C7D", Name: "电动牙刷替换刷头 3 支装 通用", Category: "配件耗材", SubCategory: "牙刷替换头", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(12.99), EstCostPrice: decimal.NewFromFloat(2.80), EstMarginRate: decimal.NewFromFloat(78.4), Currency: "USD", AIScore: decimal.NewFromFloat(85.5), MonthlySales: 3800, ReviewCount: 612, Rating: decimal.NewFromFloat(4.7), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-SBL-004"]), Tags: "替换刷头,3支,通用,软毛"},
		{SKU: "HD-TRAVEL-MINI", ASIN: "B0DFCE8D9E", Name: "迷你折叠吹风机 800W 旅行便携", Category: "个护电器", SubCategory: "吹风机", Stage: models.ProductStageRejected, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(24.99), EstCostPrice: decimal.NewFromFloat(9.50), EstMarginRate: decimal.NewFromFloat(62.0), Currency: "USD", AIScore: decimal.NewFromFloat(58.0), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ZM-002"]), Tags: "迷你,折叠,800W,旅行", AIInsight: "800W 功率在美国市场无明显差异化,且与主推 HD-PRO-2026 内部竞争,建议否决"},

		// ==================== 第二批:丰富家电美容品类(15 个新品) ====================
		// 个护电器扩展(剃须刀/理发器/美容棒/洁面仪/蒸汽眼罩)
		{SKU: "SH-ROTARY-3D", ASIN: "B0E1AB2C3D", Name: "3D浮动旋转剃须刀 IPX7防水", Category: "个护电器", SubCategory: "剃须刀", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(45.99), EstCostPrice: decimal.NewFromFloat(13.20), EstMarginRate: decimal.NewFromFloat(71.3), Currency: "USD", AIScore: decimal.NewFromFloat(86.2), MonthlySales: 2100, ReviewCount: 312, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-KX-008"]), Tags: "3D浮动,旋转,IPX7,USB充电", AIInsight: "男士个护品类年增长 14%,旋转式在欧美市场接受度高,头部品牌 Philips/Remington 均价 USD 60+,本 SKU 性价比突出"},
		{SKU: "CLIPPER-CORD", ASIN: "B0E2BC3D4E", Name: "专业理发器套装 不锈钢刀头", Category: "个护电器", SubCategory: "理发器", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(39.99), EstCostPrice: decimal.NewFromFloat(11.50), EstMarginRate: decimal.NewFromFloat(71.2), Currency: "USD", AIScore: decimal.NewFromFloat(83.0), MonthlySales: 1450, ReviewCount: 198, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "专业,不锈钢刀头,4档限位梳,有线"},
		{SKU: "BTY-BAR-7C", ASIN: "B0E3CD4E5F", Name: "7色美容棒 微振动导入仪", Category: "个护电器", SubCategory: "美容棒", Stage: models.ProductStageTesting, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(29.99), EstCostPrice: decimal.NewFromFloat(8.80), EstMarginRate: decimal.NewFromFloat(70.6), Currency: "USD", AIScore: decimal.NewFromFloat(76.5), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ML-003"]), Tags: "7色,微振动,导入,便携"},
		{SKU: "FC-BRUSH-SONIC", ASIN: "B0E4DE5F6G", Name: "声波洁面刷 硅胶亲肤 5档", Category: "个护电器", SubCategory: "洁面刷", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(34.99), EstCostPrice: decimal.NewFromFloat(9.90), EstMarginRate: decimal.NewFromFloat(71.7), Currency: "EUR", AIScore: decimal.NewFromFloat(82.8), MonthlySales: 880, ReviewCount: 124, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-SBL-004"]), Tags: "声波,硅胶,5档,IPX7"},
		{SKU: "EYE-STEAM-HEAT", ASIN: "B0E5EF6G7H", Name: "蒸汽热敷眼罩 4档控温", Category: "个护电器", SubCategory: "眼部按摩仪", Stage: models.ProductStageSourcing, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(49.99), EstCostPrice: decimal.NewFromFloat(15.30), EstMarginRate: decimal.NewFromFloat(69.4), Currency: "USD", AIScore: decimal.NewFromFloat(78.0), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "蒸汽,热敷,4档,蓝牙音乐"},

		// 美容仪器扩展(激光生发帽/化妆刷清洁器/指甲仪/喷雾仪)
		{SKU: "CAP-LASER-HAIR", ASIN: "B0E6FG7H8I", Name: "激光生发帽 272颗LED 医疗级", Category: "美容仪器", SubCategory: "生发仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(299.00), EstCostPrice: decimal.NewFromFloat(98.00), EstMarginRate: decimal.NewFromFloat(67.2), Currency: "USD", AIScore: decimal.NewFromFloat(90.5), MonthlySales: 180, ReviewCount: 42, Rating: decimal.NewFromFloat(4.7), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-JM-005"]), Tags: "激光,272颗LED,医疗级,生发", AIInsight: "脱发护理市场年规模 USD 3.5B,LLLT 低能量激光疗法临床有效,iRestore 等头部品牌均价 USD 499+,本 SKU 价格优势显著且毛利可观"},
		{SKU: "BRUSH-CLEAN-UV", ASIN: "B0E7GH8I9J", Name: "化妆刷清洁器 UV杀菌 旋转甩干", Category: "美容仪器", SubCategory: "化妆刷清洁器", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(35.99), EstCostPrice: decimal.NewFromFloat(10.20), EstMarginRate: decimal.NewFromFloat(71.7), Currency: "USD", AIScore: decimal.NewFromFloat(81.0), MonthlySales: 980, ReviewCount: 156, Rating: decimal.NewFromFloat(4.4), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-SBL-004"]), Tags: "UV杀菌,旋转甩干,8套支架,USB"},
		{SKU: "NAIL-DRY-LED", ASIN: "B0E8HI9J0K", Name: "美甲烤灯 48W LED双光源", Category: "美容仪器", SubCategory: "美甲仪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(26.99), EstCostPrice: decimal.NewFromFloat(7.20), EstMarginRate: decimal.NewFromFloat(73.3), Currency: "USD", AIScore: decimal.NewFromFloat(84.5), MonthlySales: 2600, ReviewCount: 421, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ML-003"]), Tags: "48W,LED双光源,4档定时,自动感应"},
		{SKU: "MIST-FACE-COOL", ASIN: "B0E9IJ0K1L", Name: "冷喷纳米喷雾仪 便携补水", Category: "美容仪器", SubCategory: "喷雾仪", Stage: models.ProductStageTesting, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(22.99), EstCostPrice: decimal.NewFromFloat(6.50), EstMarginRate: decimal.NewFromFloat(71.7), Currency: "EUR", AIScore: decimal.NewFromFloat(75.5), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-SBL-004"]), Tags: "冷喷,纳米,便携,USB-C"},

		// 美体仪器扩展(按摩枪/体脂秤/足浴盆/按摩靠垫)
		{SKU: "GUN-MASSAGE-4", ASIN: "B0EAJK1L2M", Name: "筋膜枪 4档振幅 静音电机", Category: "美体仪器", SubCategory: "按摩枪", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(59.99), EstCostPrice: decimal.NewFromFloat(18.80), EstMarginRate: decimal.NewFromFloat(68.6), Currency: "USD", AIScore: decimal.NewFromFloat(87.0), MonthlySales: 3200, ReviewCount: 538, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ZM-002"]), Tags: "4档,静音,4头,USB-C", AIInsight: "运动康复品类 Q1/Q4 双高峰,Theragon 均价 USD 329+,本 SKU 走性价比路线,月销 3000+ 验证市场需求"},
		{SKU: "SCALE-FAT-IF", ASIN: "B0EBKL2M3N", Name: "体脂秤 16电极 精准测量", Category: "美体仪器", SubCategory: "体脂秤", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(32.99), EstCostPrice: decimal.NewFromFloat(9.20), EstMarginRate: decimal.NewFromFloat(72.1), Currency: "USD", AIScore: decimal.NewFromFloat(85.5), MonthlySales: 2800, ReviewCount: 396, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-KX-008"]), Tags: "16电极,蓝牙,APP同步,USB-C"},
		{SKU: "FOOT-BATH-HEAT", ASIN: "B0ECLM3N4O", Name: "电动足浴盆 恒温按摩 电动滚轮", Category: "美体仪器", SubCategory: "足浴盆", Stage: models.ProductStageSourcing, Platform: "amazon", TargetMarket: "DE", ListPrice: decimal.NewFromFloat(89.99), EstCostPrice: decimal.NewFromFloat(28.50), EstMarginRate: decimal.NewFromFloat(68.3), Currency: "EUR", AIScore: decimal.NewFromFloat(77.5), MonthlySales: 0, ReviewCount: 0, Rating: decimal.NewFromFloat(0), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "恒温,电动滚轮,4档,LED显示"},
		{SKU: "CUSHION-MASSAGE", ASIN: "B0EDMN4O5P", Name: "汽车办公按摩靠垫 揉捏热敷", Category: "美体仪器", SubCategory: "按摩靠垫", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(54.99), EstCostPrice: decimal.NewFromFloat(16.80), EstMarginRate: decimal.NewFromFloat(69.4), Currency: "USD", AIScore: decimal.NewFromFloat(82.5), MonthlySales: 1650, ReviewCount: 287, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-ASM-006"]), Tags: "揉捏,热敷,车载办公,12V/USB"},

		// 配件耗材扩展(剃须刀网罩/美容仪导电凝胶/美甲油/化妆棉/清洁布)
		{SKU: "SH-NET-REPLACE", ASIN: "B0EENO5P6Q", Name: "剃须刀网罩刀头套装 通用 2 支装", Category: "配件耗材", SubCategory: "剃须刀网罩", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(15.99), EstCostPrice: decimal.NewFromFloat(3.50), EstMarginRate: decimal.NewFromFloat(78.1), Currency: "USD", AIScore: decimal.NewFromFloat(83.0), MonthlySales: 2200, ReviewCount: 298, Rating: decimal.NewFromFloat(4.6), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-KX-008"]), Tags: "网罩,刀头,2支装,通用"},
		{SKU: "GEL-RF-CONDUCT", ASIN: "B0EFOP6Q7R", Name: "射频仪导电凝胶 100ml 4 瓶装", Category: "配件耗材", SubCategory: "导电凝胶", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(19.99), EstCostPrice: decimal.NewFromFloat(4.20), EstMarginRate: decimal.NewFromFloat(79.0), Currency: "USD", AIScore: decimal.NewFromFloat(80.5), MonthlySales: 1450, ReviewCount: 178, Rating: decimal.NewFromFloat(4.4), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-JM-005"]), Tags: "导电凝胶,100ml,4瓶装,RF专用"},
		{SKU: "COTTON-MASK-30", ASIN: "B0EGPQ7R8S", Name: "一次性压缩面膜纸 30 粒装 便携", Category: "配件耗材", SubCategory: "面膜纸", Stage: models.ProductStageApproved, Platform: "amazon", TargetMarket: "US", ListPrice: decimal.NewFromFloat(9.99), EstCostPrice: decimal.NewFromFloat(2.10), EstMarginRate: decimal.NewFromFloat(78.9), Currency: "USD", AIScore: decimal.NewFromFloat(78.0), MonthlySales: 3100, ReviewCount: 482, Rating: decimal.NewFromFloat(4.5), OwnerID: admin.ID, SupplierID: uIntPtr(supMap["SUP-SBL-004"]), Tags: "压缩面膜,30粒,便携,纯棉"},
	}
	if err := db.Create(&products).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d products", len(products))

	// 设置已通过/否决产品的决策时间
	now := time.Now()
	for i := range products {
		if products[i].Stage == models.ProductStageApproved || products[i].Stage == models.ProductStageRejected {
			decidedAt := now.AddDate(0, -1, -i)
			products[i].DecidedAt = &decidedAt
			db.Model(&products[i]).Update("decided_at", decidedAt)
		}
	}

	// ==================== 供应商可供货商品 ====================
	supProducts := []models.SupplierProduct{
		{SupplierID: supMap["SUP-HZ-001"], SKU: "CF-3IN1-CURL", ProductName: "三合一卷直两用器 陶瓷涂层", Spec: "32mm 卷筒 + 直板,110V/220V", Category: "个护电器", Unit: "台", MOQ: 500, LeadTime: 25, CostPrice: decimal.NewFromFloat(14.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-HZ-001"], SKU: "HS-CER-PRO", ProductName: "陶瓷直发梳 30 秒速热", Spec: "PTC 陶瓷,LED 显示,220V", Category: "个护电器", Unit: "台", MOQ: 500, LeadTime: 22, CostPrice: decimal.NewFromFloat(11.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-HZ-001"], SKU: "IPL-HAIR-PRO", ProductName: "光子脱毛仪 IPL 50万次", Spec: "5 档,冰点,USB 充电", Category: "美体仪器", Unit: "台", MOQ: 300, LeadTime: 30, CostPrice: decimal.NewFromFloat(31.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-ZM-002"], SKU: "HD-PRO-2026", ProductName: "负离子速干吹风机 1800W", Spec: "1800W,负离子,恒温,110V/220V", Category: "个护电器", Unit: "台", MOQ: 1000, LeadTime: 28, CostPrice: decimal.NewFromFloat(18.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ZM-002"], SKU: "HD-TRAVEL-MINI", ProductName: "迷你折叠吹风机 800W", Spec: "800W,折叠,USB-C", Category: "个护电器", Unit: "台", MOQ: 1000, LeadTime: 25, CostPrice: decimal.NewFromFloat(9.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ML-003"], SKU: "LED-LIGHT-7", ProductName: "七色 LED 光疗面罩", Spec: "152 颗灯珠,7 色,遥控", Category: "美容仪器", Unit: "台", MOQ: 300, LeadTime: 35, CostPrice: decimal.NewFromFloat(42.00), Currency: "CNY"},
		{SupplierID: supMap["SUP-ML-003"], SKU: "EMS-MICRO", ProductName: "微电流提拉美容仪", Spec: "颈部面部,USB 充电", Category: "美容仪器", Unit: "台", MOQ: 300, LeadTime: 32, CostPrice: decimal.NewFromFloat(22.00), Currency: "CNY"},
		{SupplierID: supMap["SUP-SBL-004"], SKU: "BR-REPLACE-3", ProductName: "电动牙刷替换刷头 3 支装", Spec: "软毛,通用接口", Category: "配件耗材", Unit: "盒", MOQ: 2000, LeadTime: 15, CostPrice: decimal.NewFromFloat(2.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-JM-005"], SKU: "FC-SONIC-10", ProductName: "声波洁面仪 硅胶刷毛", Spec: "15 档,IPX7,USB 磁吸充电", Category: "美容仪器", Unit: "台", MOQ: 500, LeadTime: 28, CostPrice: decimal.NewFromFloat(12.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-JM-005"], SKU: "IM-EXP-PRO", ProductName: "导入导出美容仪", Spec: "离子+微电流,9V 电池", Category: "美容仪器", Unit: "台", MOQ: 500, LeadTime: 30, CostPrice: decimal.NewFromFloat(26.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-JM-005"], SKU: "RF-MULTI-5", ProductName: "多极射频美容仪", Spec: "RF+EMS+LED,3 合 1", Category: "美容仪器", Unit: "台", MOQ: 300, LeadTime: 35, CostPrice: decimal.NewFromFloat(48.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "MS-NECK-HEAT", ProductName: "颈部按摩仪", Spec: "恒温热敷,6 模式,折叠", Category: "美体仪器", Unit: "台", MOQ: 500, LeadTime: 25, CostPrice: decimal.NewFromFloat(17.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "SLIM-RF-PRO", ProductName: "射频瘦身仪", Spec: "多功能,LED 显示", Category: "美体仪器", Unit: "台", MOQ: 300, LeadTime: 32, CostPrice: decimal.NewFromFloat(36.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-KX-008"], SKU: "ST-SONIC-10", ProductName: "声波电动牙刷", Spec: "38000 次/分,5 档,IPX7", Category: "个护电器", Unit: "台", MOQ: 1000, LeadTime: 20, CostPrice: decimal.NewFromFloat(7.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-KX-008"], SKU: "WF-CORDLESS", ProductName: "便携冲牙器 无线", Spec: "300ML,3 档,USB-C", Category: "个护电器", Unit: "台", MOQ: 800, LeadTime: 22, CostPrice: decimal.NewFromFloat(10.80), Currency: "CNY"},

		// 第二批:新增产品供应商可供货记录
		{SupplierID: supMap["SUP-KX-008"], SKU: "SH-ROTARY-3D", ProductName: "3D浮动旋转剃须刀", Spec: "3D浮动,IPX7,USB充电", Category: "个护电器", Unit: "台", MOQ: 800, LeadTime: 25, CostPrice: decimal.NewFromFloat(13.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "CLIPPER-CORD", ProductName: "专业理发器套装", Spec: "不锈钢刀头,4档限位梳,有线", Category: "个护电器", Unit: "台", MOQ: 600, LeadTime: 22, CostPrice: decimal.NewFromFloat(11.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ML-003"], SKU: "BTY-BAR-7C", ProductName: "7色美容棒", Spec: "微振动,7色LED,USB", Category: "个护电器", Unit: "台", MOQ: 1000, LeadTime: 20, CostPrice: decimal.NewFromFloat(8.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-SBL-004"], SKU: "FC-BRUSH-SONIC", ProductName: "声波洁面刷", Spec: "硅胶,5档,IPX7", Category: "个护电器", Unit: "台", MOQ: 800, LeadTime: 18, CostPrice: decimal.NewFromFloat(9.90), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "EYE-STEAM-HEAT", ProductName: "蒸汽热敷眼罩", Spec: "4档控温,蓝牙音乐", Category: "个护电器", Unit: "台", MOQ: 500, LeadTime: 28, CostPrice: decimal.NewFromFloat(15.30), Currency: "CNY"},
		{SupplierID: supMap["SUP-JM-005"], SKU: "CAP-LASER-HAIR", ProductName: "激光生发帽", Spec: "272颗LED,医疗级", Category: "美容仪器", Unit: "台", MOQ: 200, LeadTime: 40, CostPrice: decimal.NewFromFloat(98.00), Currency: "CNY"},
		{SupplierID: supMap["SUP-SBL-004"], SKU: "BRUSH-CLEAN-UV", ProductName: "化妆刷清洁器", Spec: "UV杀菌,旋转甩干,8套支架", Category: "美容仪器", Unit: "台", MOQ: 600, LeadTime: 20, CostPrice: decimal.NewFromFloat(10.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-ML-003"], SKU: "NAIL-DRY-LED", ProductName: "美甲烤灯 48W", Spec: "LED双光源,4档定时,自动感应", Category: "美容仪器", Unit: "台", MOQ: 1000, LeadTime: 18, CostPrice: decimal.NewFromFloat(7.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-SBL-004"], SKU: "MIST-FACE-COOL", ProductName: "冷喷纳米喷雾仪", Spec: "便携,USB-C,纳米雾化", Category: "美容仪器", Unit: "台", MOQ: 1000, LeadTime: 15, CostPrice: decimal.NewFromFloat(6.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ZM-002"], SKU: "GUN-MASSAGE-4", ProductName: "筋膜枪 4档", Spec: "静音电机,4头,USB-C", Category: "美体仪器", Unit: "台", MOQ: 800, LeadTime: 25, CostPrice: decimal.NewFromFloat(18.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-KX-008"], SKU: "SCALE-FAT-IF", ProductName: "体脂秤 16电极", Spec: "蓝牙,APP同步,USB-C", Category: "美体仪器", Unit: "台", MOQ: 1000, LeadTime: 22, CostPrice: decimal.NewFromFloat(9.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "FOOT-BATH-HEAT", ProductName: "电动足浴盆", Spec: "恒温,电动滚轮,4档,LED", Category: "美体仪器", Unit: "台", MOQ: 400, LeadTime: 30, CostPrice: decimal.NewFromFloat(28.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-ASM-006"], SKU: "CUSHION-MASSAGE", ProductName: "汽车办公按摩靠垫", Spec: "揉捏,热敷,12V/USB", Category: "美体仪器", Unit: "台", MOQ: 600, LeadTime: 25, CostPrice: decimal.NewFromFloat(16.80), Currency: "CNY"},
		{SupplierID: supMap["SUP-KX-008"], SKU: "SH-NET-REPLACE", ProductName: "剃须刀网罩刀头套装", Spec: "2支装,通用", Category: "配件耗材", Unit: "盒", MOQ: 2000, LeadTime: 18, CostPrice: decimal.NewFromFloat(3.50), Currency: "CNY"},
		{SupplierID: supMap["SUP-JM-005"], SKU: "GEL-RF-CONDUCT", ProductName: "射频仪导电凝胶", Spec: "100ml,4瓶装,RF专用", Category: "配件耗材", Unit: "盒", MOQ: 1500, LeadTime: 15, CostPrice: decimal.NewFromFloat(4.20), Currency: "CNY"},
		{SupplierID: supMap["SUP-SBL-004"], SKU: "COTTON-MASK-30", ProductName: "一次性压缩面膜纸", Spec: "30粒装,纯棉,便携", Category: "配件耗材", Unit: "包", MOQ: 3000, LeadTime: 12, CostPrice: decimal.NewFromFloat(2.10), Currency: "CNY"},
	}
	if err := db.Create(&supProducts).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d supplier products", len(supProducts))

	// ==================== 采购单(覆盖各状态) ====================
	purchaseOrders := buildPurchaseOrders(products, supMap, admin.ID)
	if err := db.Create(&purchaseOrders).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d purchase orders", len(purchaseOrders))

	// ==================== 库存 ====================
	inventories := buildInventories(products)
	if err := db.Create(&inventories).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d inventory records", len(inventories))

	// ==================== 库存流水(入库记录) ====================
	movements := buildMovements(inventories)
	if err := db.Create(&movements).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d inventory movements", len(movements))

	// ==================== 库存预警 ====================
	alerts := buildStockAlerts(inventories)
	if err := db.Create(&alerts).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d stock alerts", len(alerts))

	// ==================== 对账账单 ====================
	bills := buildBills(purchaseOrders, supMap)
	if err := db.Create(&bills).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d bills", len(bills))

	// ==================== 利润报表(近 30 天) ====================
	profits := buildProfitReports(products)
	if err := db.Create(&profits).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d profit reports", len(profits))

	// ==================== AI 工作流执行记录(基础 6 条 + 扩展 60 条) ====================
	runs := buildAIWorkflowRuns()
	if err := db.Create(&runs).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d AI workflow runs (base)", len(runs))

	extRuns := buildAIWorkflowRunsExt()
	if err := db.Create(&extRuns).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d AI workflow runs (extended)", len(extRuns))

	// ==================== 产品市场趋势(近 30 天,每个 SKU) ====================
	trends := buildProductTrends(products)
	if err := db.Create(&trends).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d product trends", len(trends))

	// ==================== 竞品监控(每个畅销品 2-3 个竞品) ====================
	competitors := buildProductCompetitors(products)
	if err := db.Create(&competitors).Error; err != nil {
		return err
	}
	logger.Get().Infof("seeded %d product competitors", len(competitors))

	return nil
}

// uIntPtr 安全转 *uint
func uIntPtr(v uint) *uint {
	return &v
}

// buildPurchaseOrders 构造覆盖全状态的采购单
func buildPurchaseOrders(products []models.Product, supMap map[string]uint, creatorID uint) []models.PurchaseOrder {
	now := time.Now()
	orders := make([]models.PurchaseOrder, 0, 10)

	// 取已通过且关联供应商的产品,生成已结算/已入库/已发货等不同状态订单
	type orderTpl struct {
		idx    int
		status string
		qty    int
		daysAgo int
		logistics string
		company string
	}
	templates := []orderTpl{
		{idx: 0, status: models.PurchaseStatusSettled, qty: 3000, daysAgo: 75, logistics: "SF1234567890CN", company: "顺丰国际"},
		{idx: 4, status: models.PurchaseStatusSettled, qty: 5000, daysAgo: 60, logistics: "YT9876543210US", company: "云途物流"},
		{idx: 5, status: models.PurchaseStatusReceived, qty: 2000, daysAgo: 30, logistics: "DHL5566778899DE", company: "DHL"},
		{idx: 8, status: models.PurchaseStatusReconciling, qty: 800, daysAgo: 40, logistics: "FEDEX110223344US", company: "FedEx"},
		{idx: 11, status: models.PurchaseStatusShipped, qty: 2000, daysAgo: 12, logistics: "SF9988776655CN", company: "顺丰国际"},
		{idx: 6, status: models.PurchaseStatusOrdered, qty: 1500, daysAgo: 5, logistics: "", company: ""},
		{idx: 9, status: models.PurchaseStatusInquiry, qty: 1000, daysAgo: 2, logistics: "", company: ""},
		{idx: 12, status: models.PurchaseStatusQuoting, qty: 600, daysAgo: 3, logistics: "", company: ""},
	}

	for _, t := range templates {
		if t.idx >= len(products) {
			continue
		}
		p := products[t.idx]
		if p.SupplierID == nil {
			continue
		}
		createdAt := now.AddDate(0, 0, -t.daysAgo)
		expected := createdAt.AddDate(0, 0, 30)
		var actual *time.Time
		if t.status == models.PurchaseStatusReceived || t.status == models.PurchaseStatusReconciling || t.status == models.PurchaseStatusSettled {
			a := expected.AddDate(0, 0, -2)
			actual = &a
		}
		order := models.PurchaseOrder{
			OrderNo:      "PO-" + now.Format("2006") + fmt.Sprintf("%04d", t.idx+1),
			Title:        "采购 " + p.Name,
			ProductID:    &p.ID,
			SKU:          p.SKU,
			ProductName:  p.Name,
			Spec:         "标准包装",
			SupplierID:   *p.SupplierID,
			Quantity:     t.qty,
			UnitPrice:    p.EstCostPrice,
			Currency:     "CNY",
			TotalAmount:  p.EstCostPrice.Mul(decimal.NewFromInt(int64(t.qty))),
			PaymentTerms: "deposit_balance",
			ExpectedDate: &expected,
			ActualDate:   actual,
			Status:       t.status,
			CreatorID:    creatorID,
			LogisticsNo:  t.logistics,
			LogisticsCompany: t.company,
			StatusHistory: `[{"status":"` + t.status + `","at":"` + createdAt.Format(time.RFC3339) + `","operator":"admin"}]`,
		}
		order.CreatedAt = createdAt
		order.UpdatedAt = now
		orders = append(orders, order)
	}
	return orders
}

// buildInventories 构造库存(分主仓+海外仓)
func buildInventories(products []models.Product) []models.Inventory {
	// 预设库存模板：控制 out_of_stock 比例，避免预警全是无货
	// 多数健康库存 + 少量低库存 + 极少数断货
	availTpl := []int{3200, 2400, 1200, 4800, 85, 1800, 600, 320, 480, 0, 1500, 90, 1500, 3600, 120, 2800, 1800, 70, 1100, 0, 240, 1200, 3200, 95, 4200, 3500, 60, 2100, 2700, 110, 3900}
	lockedTpl := []int{300, 200, 100, 800, 20, 200, 60, 80, 100, 0, 300, 15, 250, 600, 30, 350, 180, 10, 120, 0, 30, 150, 400, 25, 520, 300, 12, 210, 340, 20, 480}
	transitTpl := []int{1000, 0, 0, 2000, 200, 500, 0, 0, 0, 500, 800, 100, 0, 2000, 300, 1200, 600, 150, 400, 0, 0, 500, 1000, 200, 1500, 800, 100, 700, 900, 250, 1300}
	safetyTpl := []int{500, 400, 200, 1000, 200, 400, 150, 100, 150, 100, 400, 200, 400, 1000, 250, 600, 350, 150, 250, 100, 80, 300, 500, 200, 700, 500, 120, 400, 450, 180, 650}

	// 多仓分布：深圳主仓全量、美西/欧洲/FBA 按销量与类目分流
	invs := make([]models.Inventory, 0, len(products)*3)
	for i, p := range products {
		if p.SupplierID == nil {
			continue
		}
		idx := i % len(availTpl)
		available := availTpl[idx]
		locked := lockedTpl[idx]
		transit := transitTpl[idx]
		safety := safetyTpl[idx]
		last := time.Now().AddDate(0, 0, -i-2)

		// 1) 深圳主仓
		invs = append(invs, models.Inventory{
			WarehouseID: 1, SKU: p.SKU,
			AvailableQty: available, LockedQty: locked, InTransitQty: transit, SafetyStock: safety,
			UnitCost: p.EstCostPrice, Currency: "CNY", LastInboundAt: &last,
		})

		// 2) 美西海外仓：中高销量
		if p.MonthlySales >= 600 {
			avail2 := available / 3
			if available == 0 {
				avail2 = 0
			} else if avail2 < 40 {
				avail2 = 40 + (i % 50)
			}
			// 部分 SKU 在海外仓偏低
			if i%7 == 0 {
				avail2 = 30 + (i % 20)
			}
			invs = append(invs, models.Inventory{
				WarehouseID: 2, SKU: p.SKU,
				AvailableQty: avail2, LockedQty: locked / 3, InTransitQty: transit / 3, SafetyStock: safety / 2,
				UnitCost: p.EstCostPrice, Currency: "CNY", LastInboundAt: &last,
			})
		}

		// 3) 欧洲海外仓：平台/市场相关
		if p.TargetMarket == "EU" || (p.Platform == "amazon" && i%3 == 0) {
			avail3 := available / 4
			if available == 0 {
				avail3 = 0
			} else if avail3 < 30 {
				avail3 = 35 + (i % 40)
			}
			if i%5 == 1 {
				avail3 = 25 // 低库存预警
			}
			invs = append(invs, models.Inventory{
				WarehouseID: 3, SKU: p.SKU,
				AvailableQty: avail3, LockedQty: locked / 4, InTransitQty: transit / 4, SafetyStock: safety / 2,
				UnitCost: p.EstCostPrice, Currency: "CNY", LastInboundAt: &last,
			})
		}

		// 4) 亚马逊 FBA 仓：畅销品
		if p.MonthlySales >= 1200 && (p.Platform == "amazon" || p.Platform == "temu") {
			avail4 := available / 5
			if available == 0 {
				avail4 = 0
			} else if avail4 < 50 {
				avail4 = 55 + (i % 30)
			}
			// 少数 FBA 断货案例
			if i%11 == 0 {
				avail4 = 0
			} else if i%8 == 0 {
				avail4 = 20 // 低库存
			}
			invs = append(invs, models.Inventory{
				WarehouseID: 4, SKU: p.SKU,
				AvailableQty: avail4, LockedQty: locked / 5, InTransitQty: transit / 5, SafetyStock: safety / 3,
				UnitCost: p.EstCostPrice, Currency: "CNY", LastInboundAt: &last,
			})
		}

		// 5) FBA 欧洲仓：部分欧盟市场
		if p.TargetMarket == "EU" && p.MonthlySales >= 800 {
			avail5 := available / 6
			if available == 0 {
				avail5 = 15
			} else if avail5 < 40 {
				avail5 = 45 + (i % 25)
			}
			if i%9 == 0 {
				avail5 = 18
			}
			invs = append(invs, models.Inventory{
				WarehouseID: 5, SKU: p.SKU,
				AvailableQty: avail5, LockedQty: locked / 6, InTransitQty: transit / 6, SafetyStock: safety / 3,
				UnitCost: p.EstCostPrice, Currency: "CNY", LastInboundAt: &last,
			})
		}
	}
	return invs
}

// buildMovements 构造库存流水
func buildMovements(invs []models.Inventory) []models.InventoryMovement {
	movs := make([]models.InventoryMovement, 0, len(invs)*2)
	for i, inv := range invs {
		if inv.AvailableQty <= 0 {
			continue
		}
		// 入库记录
		createdAt := time.Now().AddDate(0, 0, -i-5)
		movs = append(movs, models.InventoryMovement{
			WarehouseID: inv.WarehouseID, SKU: inv.SKU,
			Type: "inbound", Quantity: inv.AvailableQty + inv.LockedQty,
			BeforeQty: 0, AfterQty: inv.AvailableQty + inv.LockedQty,
			RefType: "purchase", RefID: fmt.Sprintf("PO-%04d", i+1),
			OperatorID: 1, Remark: "采购入库",
			BaseModel: models.BaseModel{CreatedAt: createdAt, UpdatedAt: createdAt},
		})
		// 锁定记录
		if inv.LockedQty > 0 {
			movs = append(movs, models.InventoryMovement{
				WarehouseID: inv.WarehouseID, SKU: inv.SKU,
				Type: "lock", Quantity: inv.LockedQty,
				BeforeQty: inv.AvailableQty + inv.LockedQty, AfterQty: inv.AvailableQty,
				RefType: "sale", RefID: fmt.Sprintf("SO-%04d", i+100),
				OperatorID: 1, Remark: "订单锁定",
				BaseModel: models.BaseModel{CreatedAt: createdAt.Add(time.Hour), UpdatedAt: createdAt.Add(time.Hour)},
			})
		}
	}
	return movs
}

// buildStockAlerts 构造库存预警
func buildStockAlerts(invs []models.Inventory) []models.StockAlert {
	alerts := make([]models.StockAlert, 0, 4)
	now := time.Now()
	for _, inv := range invs {
		if inv.AvailableQty == 0 {
			alerts = append(alerts, models.StockAlert{
				WarehouseID: inv.WarehouseID, SKU: inv.SKU,
				Type: "out_of_stock", CurrentQty: 0, Threshold: inv.SafetyStock,
				Status: "pending",
				BaseModel: models.BaseModel{CreatedAt: now.AddDate(0, 0, -1), UpdatedAt: now.AddDate(0, 0, -1)},
			})
		} else if inv.AvailableQty < inv.SafetyStock {
			alerts = append(alerts, models.StockAlert{
				WarehouseID: inv.WarehouseID, SKU: inv.SKU,
				Type: "low_stock", CurrentQty: inv.AvailableQty, Threshold: inv.SafetyStock,
				Status: "pending",
				BaseModel: models.BaseModel{CreatedAt: now.AddDate(0, 0, -2), UpdatedAt: now.AddDate(0, 0, -2)},
			})
		}
	}
	return alerts
}

// buildBills 构造账单
func buildBills(orders []models.PurchaseOrder, supMap map[string]uint) []models.Bill {
	bills := make([]models.Bill, 0, len(orders))
	now := time.Now()
	for i, o := range orders {
		if o.Status != models.PurchaseStatusReconciling &&
			o.Status != models.PurchaseStatusSettled &&
			o.Status != models.PurchaseStatusReceived {
			continue
		}
		status := "draft"
		var paidAt, matchedAt *time.Time
		diff := decimal.Zero
		if o.Status == models.PurchaseStatusSettled {
			status = "paid"
			t := o.ActualDate.AddDate(0, 0, 15)
			paidAt = &t
			matchedAt = o.ActualDate
		} else if o.Status == models.PurchaseStatusReconciling {
			status = "disputed"
			diff = o.TotalAmount.Mul(decimal.NewFromFloat(0.03)) // 3% 差异
		} else if o.Status == models.PurchaseStatusReceived {
			status = "matching"
		}
		bill := models.Bill{
			BillNo:          fmt.Sprintf("BILL-%04d", i+1),
			OrderID:         &o.ID,
			OrderNo:         o.OrderNo,
			SupplierID:      o.SupplierID,
			Type:            "purchase",
			PeriodStart:     &o.CreatedAt,
			PeriodEnd:       o.ExpectedDate,
			PayableAmount:   o.TotalAmount,
			PaidAmount:      o.TotalAmount.Sub(diff),
			DiffAmount:      diff,
			Currency:        "CNY",
			Status:          status,
			SettlementMethod: o.PaymentTerms,
			PayeeName:       supplierNameByCode(supMap, o.SupplierID),
			PayeeAccount:    fmt.Sprintf("6222-0000-%04d-%04d", o.SupplierID*1111, o.SupplierID*2222),
			CreatorID:       o.CreatorID,
			MatchedAt:       matchedAt,
			PaidAt:          paidAt,
			Remark:          "采购 " + o.ProductName,
		}
		bill.CreatedAt = o.CreatedAt.AddDate(0, 0, 20)
		bill.UpdatedAt = now
		bills = append(bills, bill)
	}
	return bills
}

// buildProfitReports 构造 30 天利润报表(仅畅销品)
func buildProfitReports(products []models.Product) []models.ProfitReport {
	reports := make([]models.ProfitReport, 0, 30*4)
	now := time.Now()
	// 无规律波动系数(30 天, 每天一个 0.6-1.4 的随机因子, 无周期性)
	dailyFactors := []float64{1.2, 0.7, 1.35, 0.85, 1.1, 0.65, 1.25, 0.9, 1.05, 0.75, 1.3, 0.8, 1.15, 0.6, 1.4, 0.95, 1.0, 0.7, 1.2, 0.85, 1.1, 0.65, 1.3, 0.9, 1.05, 0.75, 1.25, 0.8, 1.15, 0.95}
	// 价格波动系数(无规律)
	priceFactors := []float64{1.02, 0.98, 1.05, 0.96, 1.01, 1.03, 0.97, 1.04, 0.99, 1.0, 1.06, 0.95, 1.02, 0.98, 1.03, 1.01, 0.97, 1.04, 0.99, 1.05, 0.96, 1.02, 1.0, 0.98, 1.03, 0.97, 1.01, 1.04, 0.99, 1.02}
	for _, p := range products {
		if p.MonthlySales == 0 {
			continue
		}
		// 实际日报表取预估月销量的 35%, 体现真实经营节奏
		dailySales := p.MonthlySales * 35 / 100 / 30
		if dailySales == 0 {
			dailySales = 1
		}
		// 汇率(本币 -> CNY 报表口径),仅用于报表展示口径统一
		exchangeRate := decimal.NewFromFloat(7.2)
		listPriceCNY := p.ListPrice.Mul(exchangeRate)
		costPriceCNY := p.EstCostPrice.Mul(exchangeRate)
		for d := 0; d < 30; d++ {
			statDate := now.AddDate(0, 0, -d)
			// 无规律销量波动
			qty := int64(float64(dailySales) * dailyFactors[d])
			if qty < 0 {
				qty = 0
			}
			// 收入(CNY) - 价格也有无规律波动
			revenue := decimal.NewFromInt(qty).Mul(listPriceCNY).Mul(decimal.NewFromFloat(priceFactors[d]))
			// 货物成本(CNY)
			goodsCost := decimal.NewFromInt(qty).Mul(costPriceCNY)
			freightCost := goodsCost.Mul(decimal.NewFromFloat(0.12))
			platformFee := revenue.Mul(decimal.NewFromFloat(0.18))
			adCost := revenue.Mul(decimal.NewFromFloat(0.16))
			taxCost := revenue.Mul(decimal.NewFromFloat(0.08))
			refundCost := revenue.Mul(decimal.NewFromFloat(0.05))
			otherCost := revenue.Mul(decimal.NewFromFloat(0.03))
			totalCost := goodsCost.Add(freightCost).Add(platformFee).Add(adCost).Add(taxCost).Add(refundCost).Add(otherCost)
			netProfit := revenue.Sub(totalCost)
			marginRate := decimal.Zero
			roi := decimal.Zero
			if revenue.GreaterThan(decimal.Zero) {
				marginRate = netProfit.Div(revenue).Mul(decimal.NewFromInt(100))
			}
			if goodsCost.GreaterThan(decimal.Zero) {
				roi = netProfit.Div(goodsCost).Mul(decimal.NewFromInt(100))
			}
			reports = append(reports, models.ProfitReport{
				Period:      "day",
				StatDate:    statDate,
				SKU:         p.SKU,
				Platform:    p.Platform,
				Market:      p.TargetMarket,
				Revenue:     revenue,
				Qty:         qty,
				GoodsCost:   goodsCost,
				FreightCost: freightCost,
				PlatformFee: platformFee,
				AdCost:      adCost,
				TaxCost:     taxCost,
				RefundCost:  refundCost,
				OtherCost:   otherCost,
				ExchangeRate: decimal.NewFromFloat(7.2),
				Currency:    p.Currency,
				GrossProfit: revenue.Sub(goodsCost).Sub(freightCost),
				NetProfit:   netProfit,
				MarginRate:  marginRate,
				ROI:         roi,
			})
		}
	}
	return reports
}

// buildAIWorkflowRuns 构造 AI 工作流执行记录
func buildAIWorkflowRuns() []models.AIWorkflowRun {
	runs := make([]models.AIWorkflowRun, 0, 6)
	now := time.Now()
	type runTpl struct {
		code    string
		status  string
		dur     int64
		tokens  int
		cost    float64
		trigger string
		operID  *uint
		refType string
		refID   string
		output  string
	}
	templates := []runTpl{
		{code: "wf_product_analysis", status: "success", dur: 3200, tokens: 1850, cost: 0.0123, trigger: "manual", operID: uIntPtr(1), refType: "product", refID: "HD-PRO-2026", output: `{"score":87.5,"recommendation":"推荐进入","reasons":["美国市场年度搜索增长18%","差异化卖点集中在恒温与噪音控制","客单价空间充裕"],"risks":["头部品牌竞争激烈","需差异化包装"]}`},
		{code: "wf_product_analysis", status: "success", dur: 2850, tokens: 1640, cost: 0.0109, trigger: "manual", operID: uIntPtr(1), refType: "product", refID: "IPL-HAIR-PRO", output: `{"score":89.5,"recommendation":"主推","reasons":["Q4季节性爆发","头部品牌Tria/Braun均价USD199+","本SKU价格优势明显"],"risks":["FDA合规要求","需提供护目镜"]}`},
		{code: "wf_product_analysis", status: "success", dur: 2680, tokens: 1520, cost: 0.0101, trigger: "manual", operID: uIntPtr(1), refType: "product", refID: "HD-TRAVEL-MINI", output: `{"score":58.0,"recommendation":"不建议","reasons":["800W功率无明显差异化","与主推HD-PRO-2026内部竞争"],"risks":["内部SKU冲突","利润率偏低"]}`},
		{code: "wf_purchase_assistant", status: "success", dur: 4100, tokens: 2120, cost: 0.0141, trigger: "event", refType: "purchase", refID: "PO-0001", output: `{"price_reasonable":true,"market_avg":18.50,"quote_price":18.50,"delivery_risk":"low","negotiation":"价格持平市场均价,建议锁定2个月长单以换取3%折扣"}`},
		{code: "wf_customer_service", status: "success", dur: 1850, tokens: 980, cost: 0.0065, trigger: "event", refType: "ticket", refID: "TICKET-1024", output: `{"reply":"Hi,thanks for your question. The HD-PRO-2026 supports dual voltage 110V/220V, suitable for global travel. Best regards","language":"en","confidence":0.92}`},
		{code: "wf_data_analysis", status: "failed", dur: 1200, tokens: 0, cost: 0, trigger: "scheduled", output: ``, refType: "report", refID: "daily-sales"},
	}
	for i, t := range templates {
		started := now.AddDate(0, 0, -i-1)
		completed := started.Add(time.Duration(t.dur) * time.Millisecond)
		var operID *uint
		if t.operID != nil {
			operID = t.operID
		}
		runs = append(runs, models.AIWorkflowRun{
			WorkflowCode: t.code,
			TriggerType:  t.trigger,
			Input:        `{}`,
			Output:       t.output,
			Status:       t.status,
			Error: func(s string) string {
				if s == "failed" {
					return "LLM provider timeout (glm-4-plus, 30s exceeded)"
				}
				return ""
			}(t.status),
			Duration:          t.dur,
			PromptTokens:      t.tokens - 200,
			CompletionTokens:  200,
			TotalTokens:       t.tokens,
			Cost:              decimal.NewFromFloat(t.cost),
			OperatorID:        operID,
			RefType:           t.refType,
			RefID:             t.refID,
			StartedAt:         &started,
			CompletedAt:       &completed,
		})
	}
	return runs
}

func supplierNameByCode(supMap map[string]uint, id uint) string {
	for code, sid := range supMap {
		if sid == id {
			return code
		}
	}
	return ""
}

// buildProductTrends 构造近 30 天市场趋势数据(每个 SKU 一条/天)
func buildProductTrends(products []models.Product) []models.ProductTrend {
	trends := make([]models.ProductTrend, 0, len(products)*30)
	now := time.Now()
	// 类目平均价(USD),用于生成竞品均价基准
	categoryAvgPrice := map[string]float64{
		"个护电器": 49.99, "美容仪器": 109.99, "美体仪器": 79.99, "配件耗材": 14.99,
	}
	for _, p := range products {
		avgPrice := categoryAvgPrice[p.Category]
		if avgPrice == 0 {
			avgPrice = 50.0
		}
		baseSearch := p.MonthlySales * 8 // 搜索量约为销量的 8 倍
		if baseSearch == 0 {
			baseSearch = 300
		}
		baseComp := 25 // 默认 25 个竞品
		for d := 0; d < 30; d++ {
			statDate := now.AddDate(0, 0, -d-1)
			// 搜索量 ±15% 波动,周末略高
			weekendBoost := 1.0
			if statDate.Weekday() == time.Saturday || statDate.Weekday() == time.Sunday {
				weekendBoost = 1.18
			}
			searchVol := int(float64(baseSearch) * weekendBoost * (1 + float64(d%7-3)*0.04))
			salesVol := p.MonthlySales / 30
			if salesVol == 0 {
				salesVol = 2
			}
			salesVol = int(float64(salesVol) * (1 + float64(d%5-2)*0.08))
			compCount := baseComp + d%6
			avgPriceDecimal := decimal.NewFromFloat(avgPrice * (1 + float64(d%5-2)*0.02))
			reviewGrowth := salesVol / 8
			trends = append(trends, models.ProductTrend{
				ProductID:       p.ID,
				SKU:             p.SKU,
				StatDate:        statDate,
				Platform:        p.Platform,
				Market:          p.TargetMarket,
				SearchVolume:    searchVol,
				SalesVolume:     salesVol,
				CompetitorCount: compCount,
				AvgPrice:        avgPriceDecimal,
				ReviewGrowth:    reviewGrowth,
			})
		}
	}
	return trends
}

// buildProductCompetitors 构造竞品监控(畅销品各 2-3 个,长尾品 1 个)
func buildProductCompetitors(products []models.Product) []models.ProductCompetitor {
	competitors := make([]models.ProductCompetitor, 0, len(products)*2)
	// 头部品牌(按品类)
	brandByCategory := map[string][]string{
		"个护电器": {"Philips", "Panasonic", "Braun", "Remington", "Dyson"},
		"美容仪器": {"FOREO", "NuFACE", "TriPollar", "Dr.Arrivo", "YA-MAN"},
		"美体仪器": {"Therabody", "Hyperice", "Renpho", "Beurer", "Compex"},
		"配件耗材": {"Oral-B", "Crest", "Philips Sonicare", "Waterpik", "Panasonic"},
	}
	now := time.Now()
	for _, p := range products {
		brands := brandByCategory[p.Category]
		if len(brands) == 0 {
			continue
		}
		compCount := 2
		if p.MonthlySales >= 2000 {
			compCount = 3
		} else if p.MonthlySales == 0 {
			compCount = 1
		}
		for i := 0; i < compCount && i < len(brands); i++ {
			brand := brands[(int(p.ID)+i)%len(brands)]
			// 竞品均价略高于本 SKU(差异化定位)
			compPrice := p.ListPrice.Mul(decimal.NewFromFloat(1.15 + float64(i)*0.08))
			compSales := p.MonthlySales * (60 - i*15) / 100
			if compSales == 0 {
				compSales = 50 * (3 - i)
			}
			compReview := p.ReviewCount * (80 - i*20) / 100
			if compReview == 0 {
				compReview = 120 - i*30
			}
			rating := p.Rating
			if rating.IsZero() {
				rating = decimal.NewFromFloat(4.3)
			}
			rating = rating.Sub(decimal.NewFromFloat(float64(i) * 0.1))
			asin := fmt.Sprintf("B0COMP%04d%01d", p.ID, i+1)
			competitors = append(competitors, models.ProductCompetitor{
				ProductID:      p.ID,
				CompetitorASIN: asin,
				CompetitorSKU:  fmt.Sprintf("%s-COMP-%d", p.SKU, i+1),
				Brand:          brand,
				Price:          compPrice,
				SalesEst:       compSales,
				ReviewCount:    compReview,
				Rating:         rating,
				ListingURL:     fmt.Sprintf("https://www.amazon.com/dp/%s", asin),
				BaseModel:      models.BaseModel{CreatedAt: now.AddDate(0, 0, -i-1), UpdatedAt: now},
			})
		}
	}
	return competitors
}

// buildAIWorkflowRunsExt 扩展 AI 工作流执行记录(30 天,每日 1-6 条波动,共约 110 条)
func buildAIWorkflowRunsExt() []models.AIWorkflowRun {
	runs := make([]models.AIWorkflowRun, 0, 130)
	now := time.Now()
	scenes := []struct {
		code    string
		trigger string
		refType string
	}{
		{"wf_product_analysis", "manual", "product"},
		{"wf_purchase_assistant", "event", "purchase"},
		{"wf_customer_service", "event", "ticket"},
		{"wf_data_analysis", "scheduled", "report"},
		{"wf_listing_generator", "manual", "listing"},
	}
	// 每日记录数无规律波动(1-7条，无周期性)
	dailyCounts := []int{3, 7, 1, 5, 2, 6, 4, 2, 5, 1, 7, 3, 4, 6, 1, 2, 5, 7, 3, 4, 1, 6, 2, 5, 3, 7, 4, 1, 3, 2}
	for d := 0; d < 30; d++ {
		cnt := dailyCounts[d]
		for k := 0; k < cnt; k++ {
			scene := scenes[(d+k)%len(scenes)]
			started := now.AddDate(0, 0, -d).Add(time.Duration(8+k*2) * time.Hour)
			if k >= 4 {
				started = started.Add(time.Duration(14+(k-4)*3) * time.Hour)
			}
			dur := int64(1200 + (d*73+k*421)%2600)
			tokens := 700 + (d*43+k*191)%1600
			status := "success"
			errMsg := ""
			if d%9 == 0 && k == cnt-1 {
				status = "failed"
				errMsg = "LLM provider timeout (glm-4-plus, 30s exceeded)"
				dur = 30000
				tokens = 0
			}
			completed := started.Add(time.Duration(dur) * time.Millisecond)
			refID := fmt.Sprintf("%s-%04d", scene.refType, 1000+d*10+k)
			if scene.code == "wf_product_analysis" {
				refID = fmt.Sprintf("SKU-%04d", 100+d*10+k)
			}
			runs = append(runs, models.AIWorkflowRun{
				WorkflowCode:     scene.code,
				TriggerType:      scene.trigger,
				Input:            `{}`,
				Output:           `{"status":"ok","summary":"AI 处理完成"}`,
				Status:           status,
				Error:            errMsg,
				Duration:         dur,
				PromptTokens:     tokens - 200,
				CompletionTokens: 200,
				TotalTokens:      tokens,
				Cost:             decimal.NewFromFloat(float64(tokens) * 0.000002),
				OperatorID:       uIntPtr(1),
				RefType:          scene.refType,
				RefID:            refID,
				StartedAt:        &started,
				CompletedAt:      &completed,
				BaseModel:        models.BaseModel{CreatedAt: started, UpdatedAt: completed},
			})
		}
	}
	return runs
}

// defaultAIWorkflows 默认 AI 工作流模板(覆盖选品/采购/客服/数据分析/内容生成 5 大场景)
func defaultAIWorkflows() []models.AIWorkflow {
	return []models.AIWorkflow{
		{
			Code: "wf_product_analysis", Name: "选品 AI 分析",
			Description: "基于市场数据、竞品情况、成本结构,输出选品评分与建议",
			Type: "agent", Scene: "product_analysis",
			Definition: `{"nodes":[{"id":"input","type":"input"},{"id":"llm","type":"llm","config":{"system_prompt":"你是跨境电商选品专家","user_prompt":"分析以下选品:{{.sku}} {{.name}} 类目:{{.category}} 售价:{{.list_price}} 成本:{{.est_cost_price}} 月销:{{.monthly_sales}}"}},{"id":"output","type":"output"}],"edges":[{"from":"input","to":"llm"},{"from":"llm","to":"output"}]}`,
			PromptTemplate: "分析以下选品:{{.sku}} {{.name}} 类目:{{.category}} 售价:{{.list_price}} 成本:{{.est_cost_price}} 月销:{{.monthly_sales}}",
			InputSchema:  `{"type":"object","properties":{"sku":{"type":"string"},"name":{"type":"string"},"category":{"type":"string"},"list_price":{"type":"number"},"est_cost_price":{"type":"number"},"monthly_sales":{"type":"integer"}}}`,
			OutputSchema: `{"type":"object","properties":{"score":{"type":"number"},"recommendation":{"type":"string"},"reasons":{"type":"array"},"risks":{"type":"array"}}}`,
			Provider: "glm", Model: "glm-4-plus",
			Temperature: decimal.NewFromFloat(0.30), MaxTokens: 2000,
			Status: "enabled", Version: 1,
		},
		{
			Code: "wf_purchase_assistant", Name: "采购助手",
			Description: "评估供应商报价合理性、交付风险,给出谈判建议",
			Type: "automation", Scene: "purchase_assistant",
			Definition: `{"nodes":[{"id":"input","type":"input"},{"id":"llm","type":"llm","config":{"system_prompt":"你是跨境电商采购谈判专家"}},{"id":"output","type":"output"}],"edges":[{"from":"input","to":"llm"},{"from":"llm","to":"output"}]}`,
			PromptTemplate: "评估采购单 {{.order_no}} 供应商报价 {{.unit_price}} 数量 {{.quantity}}",
			InputSchema:  `{"type":"object","properties":{"order_no":{"type":"string"},"unit_price":{"type":"number"},"quantity":{"type":"integer"}}}`,
			OutputSchema: `{"type":"object","properties":{"price_reasonable":{"type":"boolean"},"market_avg":{"type":"number"},"delivery_risk":{"type":"string"},"negotiation":{"type":"string"}}}`,
			Provider: "glm", Model: "glm-4-plus",
			Temperature: decimal.NewFromFloat(0.20), MaxTokens: 1500,
			Status: "enabled", Version: 1,
		},
		{
			Code: "wf_customer_service", Name: "智能客服",
			Description: "自动回复客户咨询,支持多语言",
			Type: "rag", Scene: "customer_service",
			Definition: `{"nodes":[{"id":"input","type":"input"},{"id":"rag","type":"rag","config":{"knowledge_base_id":3,"top_k":3}},{"id":"llm","type":"llm","config":{"system_prompt":"You are a professional customer service agent for home & beauty appliances. Reply in the user's language."}},{"id":"output","type":"output"}],"edges":[{"from":"input","to":"rag"},{"from":"rag","to":"llm"},{"from":"llm","to":"output"}]}`,
			PromptTemplate: "用户问题:{{.question}}\n相关文档:{{.rag_context}}",
			InputSchema:  `{"type":"object","properties":{"question":{"type":"string"},"language":{"type":"string"}}}`,
			OutputSchema: `{"type":"object","properties":{"reply":{"type":"string"},"language":{"type":"string"},"confidence":{"type":"number"}}}`,
			Provider: "claude", Model: "claude-sonnet-4-5-20250929",
			Temperature: decimal.NewFromFloat(0.50), MaxTokens: 1000,
			Status: "enabled", Version: 1,
		},
		{
			Code: "wf_data_analysis", Name: "数据分析助手",
			Description: "自然语言转 SQL,生成销售/库存/利润报表",
			Type: "text2sql", Scene: "data_analysis",
			Definition: `{"nodes":[{"id":"input","type":"input"},{"id":"text2sql","type":"text2sql"},{"id":"sql_execute","type":"sql_execute"},{"id":"llm","type":"llm","config":{"system_prompt":"你是数据分析专家,基于 SQL 查询结果给出业务洞察"}},{"id":"output","type":"output"}],"edges":[{"from":"input","to":"text2sql"},{"from":"text2sql","to":"sql_execute"},{"from":"sql_execute","to":"llm"},{"from":"llm","to":"output"}]}`,
			PromptTemplate: "用户问题:{{.question}}\nSQL 结果:{{.result}}",
			InputSchema:  `{"type":"object","properties":{"question":{"type":"string"}}}`,
			OutputSchema: `{"type":"object","properties":{"sql":{"type":"string"},"result":{"type":"array"},"insight":{"type":"string"}}}`,
			Provider: "glm", Model: "glm-4-plus",
			Temperature: decimal.NewFromFloat(0.10), MaxTokens: 1500,
			Status: "enabled", Version: 1,
		},
		{
			Code: "wf_content_generation", Name: "内容生成",
			Description: "生成产品 Listing 标题、五点描述、A+ 页面文案",
			Type: "agent", Scene: "content_generation",
			Definition: `{"nodes":[{"id":"input","type":"input"},{"id":"llm","type":"llm","config":{"system_prompt":"你是亚马逊 Listing 优化专家,精通 SEO 与转化文案"}},{"id":"output","type":"output"}],"edges":[{"from":"input","to":"llm"},{"from":"llm","to":"output"}]}`,
			PromptTemplate: "为产品 {{.name}} (SKU: {{.sku}}, 类目: {{.category}}) 生成亚马逊 Listing",
			InputSchema:  `{"type":"object","properties":{"sku":{"type":"string"},"name":{"type":"string"},"category":{"type":"string"}}}`,
			OutputSchema: `{"type":"object","properties":{"title":{"type":"string"},"bullets":{"type":"array"},"description":{"type":"string"}}}`,
			Provider: "claude", Model: "claude-sonnet-4-5-20250929",
			Temperature: decimal.NewFromFloat(0.70), MaxTokens: 2500,
			Status: "enabled", Version: 1,
		},
	}
}

// defaultPromptTemplates 默认 Prompt 模板
func defaultPromptTemplates() []models.PromptTemplate {
	return []models.PromptTemplate{
		{
			Code: "pt_product_score", Name: "选品评分 Prompt",
			Scene: "product_analysis",
			SystemPrompt: "你是跨境电商选品专家,擅长家电美容品类。基于市场数据、竞品情况、成本结构,输出选品评分(0-100)与建议。",
			UserPrompt: `请分析以下选品并输出 JSON:
- SKU: {{.sku}}
- 产品名: {{.name}}
- 类目: {{.category}} / {{.sub_category}}
- 售价: {{.list_price}} {{.currency}}
- 预估成本: {{.est_cost_price}}
- 预估毛利率: {{.est_margin_rate}}%
- 月销量: {{.monthly_sales}}
- 评分: {{.rating}} ({{.review_count}} reviews)
- 平台: {{.platform}} / {{.target_market}}

输出格式:
{
  "score": 0-100,
  "recommendation": "推荐进入|谨慎进入|不建议",
  "reasons": ["..."],
  "risks": ["..."],
  "suggestion": "..."
}`,
			Variables:    `{"sku":"string","name":"string","category":"string","sub_category":"string","list_price":"number","currency":"string","est_cost_price":"number","est_margin_rate":"number","monthly_sales":"integer","rating":"number","review_count":"integer","platform":"string","target_market":"string"}`,
			OutputFormat: "json", Version: 1, Status: "enabled",
		},
		{
			Code: "pt_purchase_eval", Name: "采购报价评估 Prompt",
			Scene: "purchase_assistant",
			SystemPrompt: "你是跨境电商采购谈判专家,熟悉中国家电美容产业链供应商生态,能精准评估报价合理性与交付风险。",
			UserPrompt: `评估以下采购单:
- 采购单号: {{.order_no}}
- SKU: {{.sku}}
- 供应商: {{.supplier_name}} (评级 {{.supplier_rating}})
- 报价: {{.unit_price}} {{.currency}}
- 采购数量: {{.quantity}}
- 交货周期: {{.lead_time}} 天

输出 JSON:
{
  "price_reasonable": true/false,
  "market_avg": 0.00,
  "delivery_risk": "low|medium|high",
  "negotiation": "谈判建议"
}`,
			Variables:    `{"order_no":"string","sku":"string","supplier_name":"string","supplier_rating":"string","unit_price":"number","currency":"string","quantity":"integer","lead_time":"integer"}`,
			OutputFormat: "json", Version: 1, Status: "enabled",
		},
		{
			Code: "pt_cs_reply", Name: "客服回复 Prompt",
			Scene: "customer_service",
			SystemPrompt: "You are a professional customer service agent for home & beauty appliances brand. Reply in the user's language. Be concise, friendly and accurate.",
			UserPrompt: `Customer question: {{.question}}

Related knowledge:
{{.rag_context}}

Reply:`,
			Variables:    `{"question":"string","rag_context":"string"}`,
			OutputFormat: "text", Version: 1, Status: "enabled",
		},
		{
			Code: "pt_listing_gen", Name: "Listing 生成 Prompt",
			Scene: "content_generation",
			SystemPrompt: "你是亚马逊 Listing 优化专家,精通 SEO 与转化文案,擅长家电美容品类。",
			UserPrompt: `为以下产品生成亚马逊 Listing:
- 产品: {{.name}}
- SKU: {{.sku}}
- 类目: {{.category}}
- 卖点: {{.tags}}

输出 JSON:
{
  "title": "200 字符以内的 SEO 标题",
  "bullets": ["5 条五点描述"],
  "description": "A+ 页面描述",
  "search_terms": "后台关键词"
}`,
			Variables:    `{"name":"string","sku":"string","category":"string","tags":"string"}`,
			OutputFormat: "json", Version: 1, Status: "enabled",
		},
	}
}
