package database

import (
	"github.com/cb-platform/internal/domain/models"
	"github.com/cb-platform/internal/pkg/logger"
	"gorm.io/gorm"
)

// AutoMigrate 自动迁移(开发环境使用,生产环境使用 SQL 迁移脚本)
func AutoMigrate(db *gorm.DB) error {
	if err := db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return err
	}

	tables := []interface{}{
		// 用户与供应商
		&models.User{},
		&models.PlatformAccount{},
		&models.Supplier{},
		&models.SupplierProduct{},
		// 选品
		&models.Product{},
		&models.ProductTrend{},
		&models.ProductCompetitor{},
		// 采购
		&models.InquirySheet{},
		&models.Quote{},
		&models.PurchaseOrder{},
		&models.PurchaseStatusLog{},
		&models.ReceiveRecord{},
		// 库存
		&models.Warehouse{},
		&models.Inventory{},
		&models.InventoryMovement{},
		&models.StockAlert{},
		// 对账与利润
		&models.Bill{},
		&models.BillItem{},
		&models.ProfitReport{},
		// AI
		&models.AIWorkflow{},
		&models.AIWorkflowRun{},
		&models.KnowledgeBase{},
		&models.KnowledgeDocument{},
		&models.PromptTemplate{},
		// 消息中心
		&models.Message{},
	}

	if err := db.AutoMigrate(tables...); err != nil {
		return err
	}

	if err := db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; err != nil {
		return err
	}

	// 创建索引(补充 GORM 默认索引未覆盖的场景)
	if err := createIndexes(db); err != nil {
		logger.Get().Warnf("create indexes warning: %v", err)
	}

	logger.Get().Infof("auto migrated %d tables", len(tables))
	return nil
}

func createIndexes(db *gorm.DB) error {
	indexes := []string{
		// 选品综合查询
		"CREATE INDEX idx_products_stage_score ON products(stage, ai_score DESC)",
		"CREATE INDEX idx_products_category_stage ON products(category, stage)",
		// 采购单多维度查询
		"CREATE INDEX idx_purchase_orders_status_created ON purchase_orders(status, created_at DESC)",
		"CREATE INDEX idx_purchase_orders_supplier_status ON purchase_orders(supplier_id, status)",
		// 库存查询
		"CREATE INDEX idx_inventories_sku_warehouse ON inventories(sku, warehouse_id)",
		"CREATE INDEX idx_inventory_movements_sku_created ON inventory_movements(sku, created_at DESC)",
		// 对账
		"CREATE INDEX idx_bills_status_supplier ON bills(status, supplier_id)",
		"CREATE INDEX idx_profit_reports_date_sku ON profit_reports(stat_date, sku)",
		// AI
		"CREATE INDEX idx_ai_workflow_runs_status_created ON ai_workflow_runs(status, created_at DESC)",
		"CREATE INDEX idx_ai_workflow_runs_ref ON ai_workflow_runs(ref_type, ref_id)",
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			// 索引已存在不报错
			logger.Get().Debugf("create index skipped: %v", err)
		}
	}
	return nil
}

// 注:SeedData 定义在 seed.go 中(包含 defaults + business data)
