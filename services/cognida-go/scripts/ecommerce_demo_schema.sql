-- ============================================================
-- 电商演示库 (ecommerce_demo) 表结构脚本 —— 30 张表
-- ============================================================
-- 用途：开发/演示用「电商演示库」的建表 DDL，供手工建库/查阅。
-- 目标库：ecommerce_demo（业务演示库，非 link 应用库；不走 migrations）。
-- 用法：mysql -h<host> -P<port> -u<root_user> -p ecommerce_demo < ecommerce_demo_schema.sql
--
-- 注意：本脚本是 cmd/seed/ecommerce 的 schema 镜像（纯文档/建表用）。
--       真正灌数据仍用 `go run ./cmd/seed/ecommerce`（DROP+CREATE 全表并生成关联数据）。
--       改表结构时两处需同步：cmd/seed/ecommerce/main.go createSchema() 与本文件。
-- ============================================================

SET NAMES utf8mb4;

-- ---- DROP（无外键约束，纯 KEY 索引；顺序沿用工具口径）----
DROP TABLE IF EXISTS `promotion_products`;
DROP TABLE IF EXISTS `group_buys`;
DROP TABLE IF EXISTS `flash_sales`;
DROP TABLE IF EXISTS `nps_surveys`;
DROP TABLE IF EXISTS `support_tickets`;
DROP TABLE IF EXISTS `recommendation_events`;
DROP TABLE IF EXISTS `ab_assignments`;
DROP TABLE IF EXISTS `ab_experiments`;
DROP TABLE IF EXISTS `membership_tier_history`;
DROP TABLE IF EXISTS `customer_rfm_snapshots`;
DROP TABLE IF EXISTS `product_price_history`;
DROP TABLE IF EXISTS `points_ledger`;
DROP TABLE IF EXISTS `cart_items`;
DROP TABLE IF EXISTS `shopping_carts`;
DROP TABLE IF EXISTS `user_events`;
DROP TABLE IF EXISTS `ad_spend_daily`;
DROP TABLE IF EXISTS `marketing_campaigns`;
DROP TABLE IF EXISTS `inventory_snapshots`;
DROP TABLE IF EXISTS `order_returns`;
DROP TABLE IF EXISTS `shipments`;
DROP TABLE IF EXISTS `customer_coupons`;
DROP TABLE IF EXISTS `coupons`;
DROP TABLE IF EXISTS `purchase_orders`;
DROP TABLE IF EXISTS `suppliers`;
DROP TABLE IF EXISTS `product_reviews`;
DROP TABLE IF EXISTS `order_items`;
DROP TABLE IF EXISTS `orders`;
DROP TABLE IF EXISTS `products`;
DROP TABLE IF EXISTS `customers`;
DROP TABLE IF EXISTS `categories`;

-- ---- 基础域 ----
CREATE TABLE categories (
	id INT NOT NULL AUTO_INCREMENT,
	name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '分类名称',
	parent_id INT DEFAULT NULL COMMENT '父分类ID(顶级为NULL)',
	level TINYINT NOT NULL DEFAULT 1 COMMENT '层级 1顶级/2二级',
	sort_order INT NOT NULL DEFAULT 0 COMMENT '同级排序',
	PRIMARY KEY (id),
	KEY idx_parent (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品分类表';

CREATE TABLE customers (
	id INT NOT NULL AUTO_INCREMENT,
	name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '客户姓名',
	gender VARCHAR(8) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '性别 male/female',
	age INT DEFAULT NULL COMMENT '年龄',
	city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '所在城市',
	province VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '所在省份',
	vip_level TINYINT NOT NULL DEFAULT 0 COMMENT 'VIP等级 0-3',
	register_channel VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'organic' COMMENT '注册渠道',
	email VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '邮箱',
	phone VARCHAR(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '手机号',
	registered_at DATETIME NOT NULL COMMENT '注册时间',
	PRIMARY KEY (id),
	KEY idx_city (city),
	KEY idx_vip (vip_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户表';

CREATE TABLE products (
	id INT NOT NULL AUTO_INCREMENT,
	category_id INT NOT NULL COMMENT '分类ID',
	name VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '商品名称',
	brand VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '品牌',
	sku VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'SKU编码',
	price DECIMAL(10,2) NOT NULL COMMENT '售价',
	cost DECIMAL(10,2) NOT NULL COMMENT '成本价',
	weight_kg DECIMAL(6,2) NOT NULL DEFAULT '0.00' COMMENT '重量(kg)',
	stock INT NOT NULL DEFAULT 0 COMMENT '库存',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'on_sale' COMMENT '状态 on_sale在售/off_shelf下架',
	launch_date DATE DEFAULT NULL COMMENT '上市日期',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	UNIQUE KEY uk_sku (sku),
	KEY idx_category (category_id),
	KEY idx_brand (brand)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品表';

CREATE TABLE orders (
	id INT NOT NULL AUTO_INCREMENT,
	order_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '订单号',
	customer_id INT NOT NULL COMMENT '客户ID',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 completed/shipped/paid/cancelled/refunded',
	channel VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'app' COMMENT '下单渠道 web/app/mini_program/offline',
	region VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '大区(按客户省份归并)',
	campaign_id INT DEFAULT NULL COMMENT '归因营销活动ID(NULL为自然流量)',
	coupon_id INT DEFAULT NULL COMMENT '使用的优惠券ID(NULL为未用券)',
	utm_source VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'direct' COMMENT '流量来源',
	is_first_order TINYINT NOT NULL DEFAULT 0 COMMENT '是否该客户首单 1是/0否',
	total_amount DECIMAL(12,2) NOT NULL COMMENT '订单原价总额',
	discount_amount DECIMAL(12,2) NOT NULL DEFAULT '0.00' COMMENT '优惠金额',
	shipping_fee DECIMAL(10,2) NOT NULL DEFAULT '0.00' COMMENT '运费',
	tax_amount DECIMAL(10,2) NOT NULL DEFAULT '0.00' COMMENT '税额(内含13%价税分离)',
	refund_amount DECIMAL(12,2) NOT NULL DEFAULT '0.00' COMMENT '退款金额',
	pay_amount DECIMAL(12,2) NOT NULL COMMENT '实付金额(原价-优惠)',
	payment_method VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '支付方式',
	created_at DATETIME NOT NULL COMMENT '下单时间',
	paid_at DATETIME DEFAULT NULL COMMENT '支付时间',
	PRIMARY KEY (id),
	UNIQUE KEY uk_order_no (order_no),
	KEY idx_customer (customer_id),
	KEY idx_status (status),
	KEY idx_created (created_at),
	KEY idx_region (region),
	KEY idx_channel (channel),
	KEY idx_campaign (campaign_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单表';

CREATE TABLE order_items (
	id INT NOT NULL AUTO_INCREMENT,
	order_id INT NOT NULL COMMENT '订单ID',
	product_id INT NOT NULL COMMENT '商品ID',
	quantity INT NOT NULL COMMENT '数量',
	unit_price DECIMAL(10,2) NOT NULL COMMENT '成交单价',
	cost_price DECIMAL(10,2) NOT NULL DEFAULT '0.00' COMMENT '成本单价(下单时快照)',
	subtotal DECIMAL(12,2) NOT NULL COMMENT '小计',
	PRIMARY KEY (id),
	KEY idx_order (order_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单明细表';

-- ---- 交易域 ----
CREATE TABLE product_reviews (
	id INT NOT NULL AUTO_INCREMENT,
	product_id INT NOT NULL COMMENT '商品ID',
	customer_id INT NOT NULL COMMENT '客户ID',
	order_id INT NOT NULL COMMENT '订单ID',
	rating TINYINT NOT NULL COMMENT '评分 1-5',
	content VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '评价内容',
	created_at DATETIME NOT NULL COMMENT '评价时间',
	PRIMARY KEY (id),
	KEY idx_product (product_id),
	KEY idx_customer (customer_id),
	KEY idx_order (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品评价表';

-- ---- 供应链 ----
CREATE TABLE suppliers (
	id INT NOT NULL AUTO_INCREMENT,
	name VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '供应商名称',
	contact VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系人',
	phone VARCHAR(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系电话',
	city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '所在城市',
	rating DECIMAL(2,1) NOT NULL DEFAULT '0.0' COMMENT '合作评级 0-5',
	created_at DATETIME NOT NULL COMMENT '合作起始时间',
	PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='供应商表';

CREATE TABLE purchase_orders (
	id INT NOT NULL AUTO_INCREMENT,
	po_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '采购单号',
	supplier_id INT NOT NULL COMMENT '供应商ID',
	product_id INT NOT NULL COMMENT '商品ID',
	quantity INT NOT NULL COMMENT '采购数量',
	unit_cost DECIMAL(10,2) NOT NULL COMMENT '采购单价',
	total_cost DECIMAL(12,2) NOT NULL COMMENT '采购总额',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT '状态 received入库/pending在途/cancelled取消',
	created_at DATETIME NOT NULL COMMENT '下单时间',
	received_at DATETIME DEFAULT NULL COMMENT '入库时间',
	PRIMARY KEY (id),
	UNIQUE KEY po_no (po_no),
	KEY idx_supplier (supplier_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='采购单表';

-- ---- 营销域 ----
CREATE TABLE coupons (
	id INT NOT NULL AUTO_INCREMENT,
	code VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '券码',
	name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '券名称',
	type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型 fixed现金券/percent折扣券',
	discount_value DECIMAL(10,2) NOT NULL COMMENT '面额(现金券为金额,折扣券为百分比数值)',
	min_spend DECIMAL(10,2) NOT NULL DEFAULT '0.00' COMMENT '使用门槛',
	valid_from DATETIME NOT NULL COMMENT '生效时间',
	valid_to DATETIME NOT NULL COMMENT '失效时间',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT '状态 active有效/expired过期',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	UNIQUE KEY code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='优惠券模板表';

CREATE TABLE customer_coupons (
	id INT NOT NULL AUTO_INCREMENT,
	customer_id INT NOT NULL COMMENT '客户ID',
	coupon_id INT NOT NULL COMMENT '优惠券ID',
	order_id INT DEFAULT NULL COMMENT '使用的订单ID(未使用为NULL)',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'unused' COMMENT '状态 unused未用/used已用/expired过期',
	received_at DATETIME NOT NULL COMMENT '领取时间',
	used_at DATETIME DEFAULT NULL COMMENT '使用时间',
	PRIMARY KEY (id),
	KEY idx_customer (customer_id),
	KEY idx_coupon (coupon_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户领券记录表';

-- ---- 物流/退货 ----
CREATE TABLE shipments (
	id INT NOT NULL AUTO_INCREMENT,
	order_id INT NOT NULL COMMENT '订单ID',
	tracking_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '快递单号',
	carrier VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '承运商',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 delivered已签收/in_transit运输中/returned退回',
	shipped_at DATETIME NOT NULL COMMENT '发货时间',
	delivered_at DATETIME DEFAULT NULL COMMENT '签收时间',
	receiver_city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '收货城市',
	PRIMARY KEY (id),
	UNIQUE KEY uk_order (order_id),
	KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='发货物流表';

CREATE TABLE order_returns (
	id INT NOT NULL AUTO_INCREMENT,
	order_id INT NOT NULL COMMENT '订单ID',
	product_id INT NOT NULL COMMENT '退货商品ID',
	quantity INT NOT NULL COMMENT '退货数量',
	reason VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '退货原因',
	refund_amount DECIMAL(12,2) NOT NULL COMMENT '退款金额',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 refunded已退款/approved已批准/processing处理中',
	created_at DATETIME NOT NULL COMMENT '退货申请时间',
	PRIMARY KEY (id),
	KEY idx_order (order_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单退货表';

-- ---- 库存 ----
CREATE TABLE inventory_snapshots (
	id INT NOT NULL AUTO_INCREMENT,
	product_id INT NOT NULL COMMENT '商品ID',
	snapshot_date DATE NOT NULL COMMENT '快照日期(月初)',
	stock_on_hand INT NOT NULL COMMENT '在库库存',
	stock_reserved INT NOT NULL COMMENT '锁定库存',
	stock_inbound INT NOT NULL COMMENT '在途入库',
	warehouse VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '仓库',
	PRIMARY KEY (id),
	KEY idx_product_date (product_id, snapshot_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='库存月度快照表';

CREATE TABLE marketing_campaigns (
	id INT NOT NULL AUTO_INCREMENT,
	name VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '活动名称',
	channel VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '投放渠道',
	objective VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '投放目标',
	budget DECIMAL(12,2) NOT NULL COMMENT '预算',
	start_date DATE NOT NULL COMMENT '开始日期',
	end_date DATE NOT NULL COMMENT '结束日期',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 running进行中/ended已结束',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	KEY idx_channel (channel)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='营销活动表';

CREATE TABLE ad_spend_daily (
	id INT NOT NULL AUTO_INCREMENT,
	campaign_id INT NOT NULL COMMENT '营销活动ID',
	stat_date DATE NOT NULL COMMENT '统计日期',
	impressions INT NOT NULL COMMENT '曝光量',
	clicks INT NOT NULL COMMENT '点击量',
	cost DECIMAL(12,2) NOT NULL COMMENT '花费',
	conversions INT NOT NULL COMMENT '转化数',
	revenue DECIMAL(12,2) NOT NULL COMMENT '归因回流金额',
	PRIMARY KEY (id),
	KEY idx_campaign_date (campaign_id, stat_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告日花费表';

-- ---- 行为域 ----
CREATE TABLE user_events (
	id BIGINT NOT NULL AUTO_INCREMENT,
	session_id VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '会话ID',
	customer_id INT DEFAULT NULL COMMENT '客户ID(匿名会话为NULL)',
	event_type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '事件类型 view浏览/add_cart加购/checkout结算/purchase下单',
	product_id INT DEFAULT NULL COMMENT '商品ID(结算事件为NULL)',
	channel VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '渠道',
	campaign_id INT DEFAULT NULL COMMENT '归因营销活动ID',
	occurred_at DATETIME NOT NULL COMMENT '事件发生时间',
	PRIMARY KEY (id),
	KEY idx_session (session_id),
	KEY idx_customer (customer_id),
	KEY idx_type_time (event_type, occurred_at),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户行为事件表(点击流/漏斗)';

CREATE TABLE shopping_carts (
	id INT NOT NULL AUTO_INCREMENT,
	customer_id INT DEFAULT NULL COMMENT '客户ID(匿名为NULL)',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 converted已转化/abandoned已弃购/active活跃',
	item_count INT NOT NULL DEFAULT 0 COMMENT '加购商品种类数',
	total_amount DECIMAL(12,2) NOT NULL DEFAULT '0.00' COMMENT '购物车金额',
	converted_order_id INT DEFAULT NULL COMMENT '转化订单ID(弃购为NULL)',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	updated_at DATETIME NOT NULL COMMENT '最后更新时间',
	PRIMARY KEY (id),
	KEY idx_customer (customer_id),
	KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='购物车表';

CREATE TABLE cart_items (
	id INT NOT NULL AUTO_INCREMENT,
	cart_id INT NOT NULL COMMENT '购物车ID',
	product_id INT NOT NULL COMMENT '商品ID',
	quantity INT NOT NULL COMMENT '数量',
	unit_price DECIMAL(10,2) NOT NULL COMMENT '加购时单价',
	added_at DATETIME NOT NULL COMMENT '加购时间',
	PRIMARY KEY (id),
	KEY idx_cart (cart_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='购物车明细表';

-- ---- 会员域 ----
CREATE TABLE points_ledger (
	id BIGINT NOT NULL AUTO_INCREMENT,
	customer_id INT NOT NULL COMMENT '客户ID',
	order_id INT DEFAULT NULL COMMENT '关联订单ID(兑换/过期为NULL)',
	change_type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型 earn获取/redeem兑换/expire过期',
	points INT NOT NULL COMMENT '积分变动(正为增,负为减)',
	balance_after INT NOT NULL COMMENT '变动后余额',
	created_at DATETIME NOT NULL COMMENT '发生时间',
	PRIMARY KEY (id),
	KEY idx_customer_time (customer_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会员积分流水表';

CREATE TABLE product_price_history (
	id INT NOT NULL AUTO_INCREMENT,
	product_id INT NOT NULL COMMENT '商品ID',
	old_price DECIMAL(10,2) NOT NULL COMMENT '调整前售价',
	new_price DECIMAL(10,2) NOT NULL COMMENT '调整后售价',
	reason VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '调价原因',
	changed_at DATETIME NOT NULL COMMENT '调价时间',
	PRIMARY KEY (id),
	KEY idx_product_time (product_id, changed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品价格变更历史表';

-- ---- 分群域 ----
CREATE TABLE customer_rfm_snapshots (
	id INT NOT NULL AUTO_INCREMENT,
	customer_id INT NOT NULL COMMENT '客户ID',
	snapshot_month DATE NOT NULL COMMENT '快照月份(当月1号)',
	recency_days INT NOT NULL COMMENT 'R:距最近一次消费天数',
	frequency INT NOT NULL COMMENT 'F:累计消费次数',
	monetary DECIMAL(12,2) NOT NULL COMMENT 'M:累计消费金额',
	r_score TINYINT NOT NULL COMMENT 'R得分 1-5',
	f_score TINYINT NOT NULL COMMENT 'F得分 1-5',
	m_score TINYINT NOT NULL COMMENT 'M得分 1-5',
	segment VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'RFM客户分群',
	created_at DATETIME NOT NULL COMMENT '快照生成时点',
	PRIMARY KEY (id),
	KEY idx_month_seg (snapshot_month, segment),
	KEY idx_customer (customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户RFM分群月度快照表';

CREATE TABLE membership_tier_history (
	id INT NOT NULL AUTO_INCREMENT,
	customer_id INT NOT NULL COMMENT '客户ID',
	from_tier VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '原等级',
	to_tier VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '新等级',
	reason VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '变更原因',
	cumulative_spend DECIMAL(12,2) NOT NULL COMMENT '变更时累计消费',
	changed_at DATETIME NOT NULL COMMENT '变更时间',
	PRIMARY KEY (id),
	KEY idx_customer_time (customer_id, changed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会员等级升级轨迹表';

-- ---- 实验域 ----
CREATE TABLE ab_experiments (
	id INT NOT NULL AUTO_INCREMENT,
	exp_key VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '实验编号',
	name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '实验名称',
	hypothesis VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '实验假设',
	primary_metric VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '核心指标',
	status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 running/completed',
	start_date DATE NOT NULL COMMENT '开始日期',
	end_date DATE NOT NULL COMMENT '结束日期',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	UNIQUE KEY uk_exp_key (exp_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B实验表';

CREATE TABLE ab_assignments (
	id BIGINT NOT NULL AUTO_INCREMENT,
	experiment_id INT NOT NULL COMMENT '实验ID',
	customer_id INT NOT NULL COMMENT '客户ID',
	variant VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '分组 control/variant_a/variant_b',
	assigned_at DATETIME NOT NULL COMMENT '分流时间',
	converted TINYINT NOT NULL DEFAULT 0 COMMENT '是否转化 0/1',
	conversion_value DECIMAL(10,2) DEFAULT NULL COMMENT '转化金额(未转化为NULL)',
	PRIMARY KEY (id),
	KEY idx_exp_variant (experiment_id, variant),
	KEY idx_customer (customer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B实验分流与转化表';

CREATE TABLE recommendation_events (
	id BIGINT NOT NULL AUTO_INCREMENT,
	session_id VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '会话ID',
	customer_id INT DEFAULT NULL COMMENT '客户ID(匿名为NULL)',
	product_id INT NOT NULL COMMENT '推荐商品ID',
	scene VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '推荐场景',
	`rank` TINYINT NOT NULL COMMENT '推荐位次',
	action VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '行为 impression/click/add_cart/purchase',
	occurred_at DATETIME NOT NULL COMMENT '发生时间',
	PRIMARY KEY (id),
	KEY idx_scene_action (scene, action),
	KEY idx_session (session_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='推荐位曝光点击事件流表';

-- ---- 客服域 ----
CREATE TABLE support_tickets (
	id INT NOT NULL AUTO_INCREMENT,
	ticket_no VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '工单号',
	customer_id INT NOT NULL COMMENT '客户ID',
	order_id INT DEFAULT NULL COMMENT '关联订单ID(可空)',
	category VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '工单分类',
	channel VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '来源渠道',
	priority VARCHAR(8) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '优先级 low/medium/high/urgent',
	status VARCHAR(12) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 resolved/closed/pending',
	first_response_minutes INT NOT NULL COMMENT '首次响应时长(分钟)',
	resolution_minutes INT DEFAULT NULL COMMENT '解决时长(分钟,未解决为NULL)',
	csat_score TINYINT DEFAULT NULL COMMENT 'CSAT满意度 1-5(未评价为NULL)',
	agent_id INT NOT NULL COMMENT '处理坐席ID',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	resolved_at DATETIME DEFAULT NULL COMMENT '解决时间(未解决为NULL)',
	PRIMARY KEY (id),
	UNIQUE KEY uk_ticket_no (ticket_no),
	KEY idx_customer (customer_id),
	KEY idx_cat_status (category, status),
	KEY idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客服工单表';

CREATE TABLE nps_surveys (
	id INT NOT NULL AUTO_INCREMENT,
	customer_id INT NOT NULL COMMENT '客户ID',
	score TINYINT NOT NULL COMMENT 'NPS评分 0-10',
	category VARCHAR(12) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类别 promoter/passive/detractor',
	comment VARCHAR(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '开放反馈',
	surveyed_at DATETIME NOT NULL COMMENT '调研时间',
	PRIMARY KEY (id),
	KEY idx_customer (customer_id),
	KEY idx_cat_time (category, surveyed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='NPS调研表';

-- ---- 促销域 ----
CREATE TABLE flash_sales (
	id INT NOT NULL AUTO_INCREMENT,
	name VARCHAR(48) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '场次名称',
	product_id INT NOT NULL COMMENT '秒杀商品ID',
	original_price DECIMAL(10,2) NOT NULL COMMENT '原价',
	flash_price DECIMAL(10,2) NOT NULL COMMENT '秒杀价',
	discount_rate DECIMAL(5,2) NOT NULL COMMENT '折扣率(成交价/原价)',
	stock_limit INT NOT NULL COMMENT '限量库存',
	sold_count INT NOT NULL COMMENT '已售数量',
	start_at DATETIME NOT NULL COMMENT '开始时间',
	end_at DATETIME NOT NULL COMMENT '结束时间',
	status VARCHAR(12) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 active/ended',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	KEY idx_product (product_id),
	KEY idx_start (start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限时秒杀场次表';

CREATE TABLE group_buys (
	id INT NOT NULL AUTO_INCREMENT,
	group_no VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '拼团编号',
	product_id INT NOT NULL COMMENT '拼团商品ID',
	initiator_customer_id INT NOT NULL COMMENT '发起人客户ID',
	group_size TINYINT NOT NULL COMMENT '成团所需人数',
	current_members TINYINT NOT NULL COMMENT '当前成员数',
	original_price DECIMAL(10,2) NOT NULL COMMENT '原价',
	group_price DECIMAL(10,2) NOT NULL COMMENT '拼团价',
	status VARCHAR(12) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态 pending/success/failed',
	created_at DATETIME NOT NULL COMMENT '发起时间',
	expires_at DATETIME NOT NULL COMMENT '到期时间(进行中团可为未来)',
	completed_at DATETIME DEFAULT NULL COMMENT '成团时间(未成团为NULL)',
	PRIMARY KEY (id),
	UNIQUE KEY uk_group_no (group_no),
	KEY idx_product (product_id),
	KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='拼团实例表';

CREATE TABLE promotion_products (
	id INT NOT NULL AUTO_INCREMENT,
	promotion_type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '促销类型 campaign/coupon/flash_sale',
	promotion_id INT NOT NULL COMMENT '促销活动ID(对应类型主键)',
	product_id INT NOT NULL COMMENT '参与商品ID',
	special_price DECIMAL(10,2) DEFAULT NULL COMMENT '专享价(无则NULL)',
	created_at DATETIME NOT NULL COMMENT '创建时间',
	PRIMARY KEY (id),
	KEY idx_type_promo (promotion_type, promotion_id),
	KEY idx_product (product_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='促销活动参与商品表(多对多)';
