-- CB-Platform 数据库初始化脚本
-- 用于生产环境手动执行(开发环境使用 AutoMigrate 自动迁移)
-- 执行:mysql -u cb -p cb_platform < migrations/001_init.sql

SET FOREIGN_KEY_CHECKS = 0;

-- ============== 用户与供应商 ==============
CREATE TABLE IF NOT EXISTS `users` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `username` VARCHAR(64) NOT NULL,
  `password` VARCHAR(128) NOT NULL,
  `real_name` VARCHAR(64),
  `email` VARCHAR(128),
  `phone` VARCHAR(32),
  `avatar` VARCHAR(255),
  `role` VARCHAR(32) DEFAULT 'staff',
  `department` VARCHAR(64),
  `status` TINYINT UNSIGNED DEFAULT 1,
  `last_login_at` TIMESTAMP NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_role` (`role`),
  KEY `idx_department` (`department`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户表';

CREATE TABLE IF NOT EXISTS `platform_accounts` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(128) NOT NULL,
  `platform` VARCHAR(32) NOT NULL,
  `region` VARCHAR(32),
  `seller_id` VARCHAR(128),
  `refresh_token` TEXT,
  `access_token` TEXT,
  `token_expire_at` TIMESTAMP NULL,
  `status` TINYINT UNSIGNED DEFAULT 1,
  `metadata` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  PRIMARY KEY (`id`),
  KEY `idx_platform` (`platform`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='平台账号';

CREATE TABLE IF NOT EXISTS `suppliers` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(128) NOT NULL,
  `code` VARCHAR(64),
  `contact_name` VARCHAR(64),
  `phone` VARCHAR(32),
  `email` VARCHAR(128),
  `address` VARCHAR(255),
  `region` VARCHAR(64),
  `payment_terms` VARCHAR(128),
  `settlement_cycle` VARCHAR(64),
  `rating` VARCHAR(8) DEFAULT 'B',
  `coop_status` VARCHAR(32) DEFAULT 'active',
  `total_amount` DECIMAL(18,2) DEFAULT 0,
  `on_time_rate` DECIMAL(5,2) DEFAULT 0,
  `quality_rate` DECIMAL(5,2) DEFAULT 0,
  `remark` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_rating` (`rating`),
  KEY `idx_coop_status` (`coop_status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='供应商';

-- ============== 选品 ==============
CREATE TABLE IF NOT EXISTS `products` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `sku` VARCHAR(64) NOT NULL,
  `asin` VARCHAR(32),
  `name` VARCHAR(255) NOT NULL,
  `image_url` VARCHAR(512),
  `category` VARCHAR(128),
  `sub_category` VARCHAR(128),
  `stage` VARCHAR(32) DEFAULT 'sourcing',
  `platform` VARCHAR(32),
  `target_market` VARCHAR(64),
  `list_price` DECIMAL(18,2),
  `est_cost_price` DECIMAL(18,4),
  `est_margin_rate` DECIMAL(5,2),
  `currency` VARCHAR(8) DEFAULT 'USD',
  `ai_score` DECIMAL(5,2) DEFAULT 0,
  `ai_insight` TEXT,
  `monthly_sales` INT DEFAULT 0,
  `review_count` INT DEFAULT 0,
  `rating` DECIMAL(3,2),
  `owner_id` BIGINT UNSIGNED,
  `supplier_id` BIGINT UNSIGNED,
  `tags` VARCHAR(512),
  `remark` TEXT,
  `decided_at` TIMESTAMP NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sku` (`sku`),
  KEY `idx_asin` (`asin`),
  KEY `idx_category` (`category`),
  KEY `idx_sub_category` (`sub_category`),
  KEY `idx_stage` (`stage`),
  KEY `idx_platform` (`platform`),
  KEY `idx_owner_id` (`owner_id`),
  KEY `idx_supplier_id` (`supplier_id`),
  KEY `idx_products_stage_score` (`stage`, `ai_score` DESC),
  KEY `idx_products_category_stage` (`category`, `stage`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='选品池';

-- ============== 采购 ==============
CREATE TABLE IF NOT EXISTS `purchase_orders` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `order_no` VARCHAR(64) NOT NULL,
  `title` VARCHAR(255),
  `inquiry_id` BIGINT UNSIGNED,
  `product_id` BIGINT UNSIGNED,
  `sku` VARCHAR(64),
  `product_name` VARCHAR(255),
  `spec` VARCHAR(255),
  `supplier_id` BIGINT UNSIGNED NOT NULL,
  `quantity` INT NOT NULL,
  `unit_price` DECIMAL(18,4) NOT NULL,
  `currency` VARCHAR(8) DEFAULT 'CNY',
  `total_amount` DECIMAL(18,2) NOT NULL,
  `payment_terms` VARCHAR(64),
  `expected_date` TIMESTAMP NULL,
  `actual_date` TIMESTAMP NULL,
  `status` VARCHAR(32) DEFAULT 'inquiry',
  `creator_id` BIGINT UNSIGNED NOT NULL,
  `logistics_no` VARCHAR(128),
  `logistics_company` VARCHAR(64),
  `remark` TEXT,
  `status_history` TEXT,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `deleted_at` TIMESTAMP NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_no` (`order_no`),
  KEY `idx_inquiry_id` (`inquiry_id`),
  KEY `idx_product_id` (`product_id`),
  KEY `idx_sku` (`sku`),
  KEY `idx_supplier_id` (`supplier_id`),
  KEY `idx_status` (`status`),
  KEY `idx_creator_id` (`creator_id`),
  KEY `idx_purchase_orders_status_created` (`status`, `created_at` DESC),
  KEY `idx_purchase_orders_supplier_status` (`supplier_id`, `status`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='采购单';

-- 注:其他表通过 AutoMigrate 自动创建,生产环境可参照此格式补全

SET FOREIGN_KEY_CHECKS = 1;

-- ============== 初始管理员账号 ==============
-- 密码: admin123 (bcrypt hash)
INSERT INTO `users` (`username`, `password`, `real_name`, `role`, `department`, `status`)
SELECT 'admin', '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', '系统管理员', 'admin', '技术部', 1
WHERE NOT EXISTS (SELECT 1 FROM `users` WHERE `username` = 'admin');
