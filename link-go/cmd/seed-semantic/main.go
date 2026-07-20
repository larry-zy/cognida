// 工具：为电商演示库（ecommerce_demo）冷启动写入受治理的语义模型，
// 让 semantic_query 走「治理口径」主路径而非每次回退词法 NL2SQL。
//
// 用法：cd link-go && set -a && source .env && set +a && go run ./cmd/seed-semantic
// 连接的是 link 应用库（元数据库，非业务库 ecommerce_demo），经 GORM +
// SemanticRepository.UpsertBundle 全量幂等写入；固定 ID，可反复执行。
//
// 落库对象：租户 tenant_id=1（dev 用户）、status=active 的两套语义模型：
//   - 电商销售（grain=order，orders ⋈ customers）
//   - 商品销售（grain=order_item，order_items ⋈ products ⋈ categories）
// 两套均为「扇出安全」：所有 JOIN 都是从事实表出发的 to-one 关系，聚合不虚增。
//
// 前置：ecommerce_demo 已 seed（cmd/seed-ecommerce），且 link 库已 migrate-db。
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"link/internal/model/semantic"
	mysqlrepo "link/internal/repository/mysql"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// seedTenant 目标租户：dev 用户 tenant_id=1（与 users.tenant_id 修复后一致）。
const seedTenant = int64(1)

// ecommerceDatasourceID 电商演示库（ecommerce_demo）数据源 ID：写入逻辑表的
// DatabaseID 绑定，使 semantic_query 返回的治理 SQL 显式打向该外部数据源，
// 而非依赖「会话隐式选库」。默认值为 dev 环境登记的 ID，可经 env 覆盖以适配他机。
// 查询来源：SELECT id FROM data_sources WHERE name='电商演示库' AND tenant_id=1。
var ecommerceDatasourceID = env("ECOMMERCE_DATASOURCE_ID", "20260705030628ad7827ced5604d2c")

func main() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "link"),
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}

	// 冷启动自愈：确保语义表存在（幂等；正常已由 migrate-db 同步）。
	if err := db.AutoMigrate(
		&mysqlrepo.SemanticModelModel{}, &mysqlrepo.SemanticLogicalTableModel{},
		&mysqlrepo.SemanticDimensionModel{}, &mysqlrepo.SemanticMeasureModel{},
		&mysqlrepo.SemanticMetricModel{}, &mysqlrepo.SemanticRelationModel{},
	); err != nil {
		log.Fatalf("automigrate semantic tables failed: %v", err)
	}

	repo := mysqlrepo.NewSemanticRepository(db)
	ctx := context.Background()

	bundles := []*semantic.ModelBundle{salesBundle(), productBundle()}
	for _, b := range bundles {
		if err := repo.UpsertBundle(ctx, b); err != nil {
			log.Fatalf("upsert bundle %q failed: %v", b.Model.Name, err)
		}
		fmt.Printf("OK: seeded semantic model %q (tenant=%d, %d tables / %d dims / %d measures / %d metrics)\n",
			b.Model.Name, b.Model.TenantID,
			len(b.LogicalTables), len(b.Dimensions), len(b.Measures), len(b.Metrics))
	}
	fmt.Printf("DONE: %d semantic models active for tenant %d\n", len(bundles), seedTenant)
}

// salesBundle 电商销售模型（grain=order）：事实表 orders，to-one 关联 customers。
//
// 扇出安全：orders→customers 是「一单一客」的 to-one 关系，按客户属性分组聚合
// 订单口径指标不会虚增。指标 Expr 内引用的列以度量名（pay_amount 等英文原子）
// 出现，metricTableID 据此把指标绑定到 orders 事实表。
func salesBundle() *semantic.ModelBundle {
	const (
		modelID = "seed_ecom_sales"
		ltOrder = "seed_lt_orders"
		ltCust  = "seed_lt_customers"
	)
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{
			ID: modelID, TenantID: seedTenant, Name: "电商销售",
			Description: "电商订单销售域：营收/净营收/客单价/折扣率/退款率等受治理指标，可按订单状态、支付方式、渠道、大区、省份、城市、性别、VIP 等级、注册渠道、下单日期（支持日/周/月/季/年上卷）分析。",
			Version:     1, Status: semantic.ModelStatusActive,
		},
		LogicalTables: []*semantic.LogicalTable{
			{ID: ltOrder, ModelID: modelID, Name: "orders", PhysicalTable: "orders", DatabaseID: ecommerceDatasourceID, PrimaryKey: "id", Description: "订单事实表"},
			{ID: ltCust, ModelID: modelID, Name: "customers", PhysicalTable: "customers", DatabaseID: ecommerceDatasourceID, PrimaryKey: "id", Description: "客户维表"},
		},
		Relations: []*semantic.Relation{
			{ID: "seed_rel_ord_cust", ModelID: modelID, LeftTableID: ltOrder, RightTableID: ltCust,
				JoinType: semantic.JoinLeft, JoinCondition: "orders.customer_id = customers.id",
				Description: "订单→客户（to-one，扇出安全）"},
		},
		// 度量：英文原子列，作为指标 Expr 的可解析构件（metricTableID 据名归表）。
		Measures: []*semantic.Measure{
			{ID: "seed_ms_pay", ModelID: modelID, LogicalTableID: ltOrder, Name: "pay_amount", Expr: "pay_amount", Aggregation: semantic.AggSum, Description: "实付金额"},
			{ID: "seed_ms_disc", ModelID: modelID, LogicalTableID: ltOrder, Name: "discount_amount", Expr: "discount_amount", Aggregation: semantic.AggSum, Description: "优惠金额"},
			{ID: "seed_ms_total", ModelID: modelID, LogicalTableID: ltOrder, Name: "total_amount", Expr: "total_amount", Aggregation: semantic.AggSum, Description: "订单原价总额"},
			{ID: "seed_ms_ordcnt", ModelID: modelID, LogicalTableID: ltOrder, Name: "订单数", Expr: "id", Aggregation: semantic.AggCount, Description: "订单笔数"},
			{ID: "seed_ms_custcnt", ModelID: modelID, LogicalTableID: ltOrder, Name: "客户数", Expr: "COUNT(DISTINCT orders.customer_id)", Aggregation: semantic.AggNone, Description: "去重下单客户数"},
			{ID: "seed_ms_refund", ModelID: modelID, LogicalTableID: ltOrder, Name: "退款额", Expr: "refund_amount", Aggregation: semantic.AggSum, Description: "退款金额"},
			{ID: "seed_ms_ship", ModelID: modelID, LogicalTableID: ltOrder, Name: "运费", Expr: "shipping_fee", Aggregation: semantic.AggSum, Description: "运费"},
		},
		// 指标：口径钉死在 Expr，同义词供 grounding。派生指标以 {指标/度量名} 引用其它口径，
		// 引擎递归展开（带环检测），保证跨口径一致（如净营收/退款率复用「营收」定义）。
		Metrics: []*semantic.Metric{
			{ID: "seed_mt_rev", ModelID: modelID, Name: "营收", Expr: "SUM(orders.pay_amount)",
				Caliber: "已支付金额(pay_amount)之和", Format: "currency", Synonyms: []string{"revenue", "gmv", "销售额", "成交额"}},
			{ID: "seed_mt_aov", ModelID: modelID, Name: "客单价", Expr: "SUM(orders.pay_amount)/NULLIF(COUNT(orders.id),0)",
				Caliber: "客单价 = 支付总额 / 订单数", Format: "currency", Synonyms: []string{"aov", "客单"}},
			{ID: "seed_mt_disc", ModelID: modelID, Name: "折扣率", Expr: "SUM(orders.discount_amount)/NULLIF(SUM(orders.total_amount),0)",
				Caliber: "折扣率 = 优惠额 / 订单原价总额", Format: "percent", Synonyms: []string{"discount_rate", "折扣"}},
			// ② 派生指标：引用其它指标/度量，引擎展开时钉死口径。
			{ID: "seed_mt_netrev", ModelID: modelID, Name: "净营收", Expr: "{营收} - {退款额}",
				Caliber: "净营收 = 营收 - 退款额", Format: "currency", Synonyms: []string{"net_revenue", "净收入", "净销售额"}},
			{ID: "seed_mt_refrate", ModelID: modelID, Name: "退款率", Expr: "{退款额} / NULLIF({营收}, 0)",
				Caliber: "退款率 = 退款额 / 营收", Format: "percent", Synonyms: []string{"refund_rate", "退货率", "退款占比"}},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "seed_d_status", ModelID: modelID, LogicalTableID: ltOrder, Name: "订单状态", Expr: "status", DataType: "string", Synonyms: []string{"状态", "order_status"}},
			{ID: "seed_d_pay", ModelID: modelID, LogicalTableID: ltOrder, Name: "支付方式", Expr: "payment_method", DataType: "string", Synonyms: []string{"支付渠道", "payment_method"}},
			{ID: "seed_d_channel", ModelID: modelID, LogicalTableID: ltOrder, Name: "渠道", Expr: "channel", DataType: "string", Synonyms: []string{"下单渠道", "channel", "来源"}},
			{ID: "seed_d_region", ModelID: modelID, LogicalTableID: ltOrder, Name: "大区", Expr: "region", DataType: "string", Synonyms: []string{"区域", "region", "地区"}},
			{ID: "seed_d_date", ModelID: modelID, LogicalTableID: ltOrder, Name: "下单日期", Expr: "orders.created_at", DataType: "datetime", Synonyms: []string{"日期", "date", "下单时间"}},
			{ID: "seed_d_city", ModelID: modelID, LogicalTableID: ltCust, Name: "城市", Expr: "city", DataType: "string", Synonyms: []string{"city", "所在城市"}},
			{ID: "seed_d_province", ModelID: modelID, LogicalTableID: ltCust, Name: "省份", Expr: "province", DataType: "string", Synonyms: []string{"province", "所在省份"}},
			{ID: "seed_d_gender", ModelID: modelID, LogicalTableID: ltCust, Name: "性别", Expr: "gender", DataType: "string", Synonyms: []string{"gender"}},
			{ID: "seed_d_vip", ModelID: modelID, LogicalTableID: ltCust, Name: "VIP等级", Expr: "vip_level", DataType: "string", Synonyms: []string{"vip", "vip_level", "会员等级"}},
			{ID: "seed_d_regchan", ModelID: modelID, LogicalTableID: ltCust, Name: "注册渠道", Expr: "register_channel", DataType: "string", Synonyms: []string{"register_channel", "获客渠道"}},
		},
	}
}

// productBundle 商品销售模型（grain=order_item）：事实表 order_items，
// to-one 关联 products，products 再 to-one 关联 categories。
//
// 扇出安全：从明细行出发，一行明细对应一个商品、一个商品对应一个品类，
// 全链 to-one，按品类/品牌分组聚合销量/销售额不虚增。
func productBundle() *semantic.ModelBundle {
	const (
		modelID = "seed_ecom_product"
		ltItem  = "seed_lt_items"
		ltProd  = "seed_lt_products"
		ltCat   = "seed_lt_categories"
	)
	return &semantic.ModelBundle{
		Model: &semantic.SemanticModel{
			ID: modelID, TenantID: seedTenant, Name: "商品销售",
			Description: "商品销售域（订单明细粒度）：销量/商品销售额/均价/商品成本/毛利/毛利率，可按品类、品牌、商品名称、商品状态分析。",
			Version:     1, Status: semantic.ModelStatusActive,
		},
		LogicalTables: []*semantic.LogicalTable{
			{ID: ltItem, ModelID: modelID, Name: "order_items", PhysicalTable: "order_items", DatabaseID: ecommerceDatasourceID, PrimaryKey: "id", Description: "订单明细事实表"},
			{ID: ltProd, ModelID: modelID, Name: "products", PhysicalTable: "products", DatabaseID: ecommerceDatasourceID, PrimaryKey: "id", Description: "商品维表"},
			{ID: ltCat, ModelID: modelID, Name: "categories", PhysicalTable: "categories", DatabaseID: ecommerceDatasourceID, PrimaryKey: "id", Description: "品类维表"},
		},
		Relations: []*semantic.Relation{
			{ID: "seed_rel_item_prod", ModelID: modelID, LeftTableID: ltItem, RightTableID: ltProd,
				JoinType: semantic.JoinLeft, JoinCondition: "order_items.product_id = products.id",
				Description: "明细→商品（to-one，扇出安全）"},
			{ID: "seed_rel_prod_cat", ModelID: modelID, LeftTableID: ltProd, RightTableID: ltCat,
				JoinType: semantic.JoinLeft, JoinCondition: "products.category_id = categories.id",
				Description: "商品→品类（to-one，扇出安全）"},
		},
		Measures: []*semantic.Measure{
			{ID: "seed_ms_qty", ModelID: modelID, LogicalTableID: ltItem, Name: "quantity", Expr: "quantity", Aggregation: semantic.AggSum, Description: "明细数量"},
			{ID: "seed_ms_sub", ModelID: modelID, LogicalTableID: ltItem, Name: "subtotal", Expr: "subtotal", Aggregation: semantic.AggSum, Description: "明细小计"},
		},
		Metrics: []*semantic.Metric{
			{ID: "seed_mt_qty", ModelID: modelID, Name: "销量", Expr: "SUM(order_items.quantity)",
				Caliber: "销量 = 明细数量之和", Synonyms: []string{"销售量", "件数", "quantity_sum"}},
			{ID: "seed_mt_sales", ModelID: modelID, Name: "商品销售额", Expr: "SUM(order_items.subtotal)",
				Caliber: "商品销售额 = 明细小计之和", Format: "currency", Synonyms: []string{"销售额", "sales"}},
			{ID: "seed_mt_avgprice", ModelID: modelID, Name: "均价", Expr: "SUM(order_items.subtotal)/NULLIF(SUM(order_items.quantity),0)",
				Caliber: "均价 = 商品销售额 / 销量", Format: "currency", Synonyms: []string{"单价", "avg_price"}},
			// ② 跨表派生：商品成本按下单数量 × 商品成本价聚合，引用 products 触发引擎自动 JOIN。
			{ID: "seed_mt_cogs", ModelID: modelID, Name: "商品成本", Expr: "SUM(order_items.quantity * products.cost)",
				Caliber: "商品成本 = Σ(明细数量 × 商品成本价)", Format: "currency", Synonyms: []string{"成本", "cogs", "销货成本"}},
			// ② 派生：毛利引用「商品销售额」「商品成本」，毛利率再引用「毛利」（派生套派生，递归展开）。
			{ID: "seed_mt_gp", ModelID: modelID, Name: "毛利", Expr: "{商品销售额} - {商品成本}",
				Caliber: "毛利 = 商品销售额 - 商品成本", Format: "currency", Synonyms: []string{"gross_profit", "毛利润"}},
			{ID: "seed_mt_gpm", ModelID: modelID, Name: "毛利率", Expr: "{毛利} / NULLIF({商品销售额}, 0)",
				Caliber: "毛利率 = 毛利 / 商品销售额", Format: "percent", Synonyms: []string{"gross_margin", "毛利润率", "毛利占比"}},
		},
		Dimensions: []*semantic.Dimension{
			{ID: "seed_d_cat", ModelID: modelID, LogicalTableID: ltCat, Name: "品类", Expr: "name", DataType: "string", Synonyms: []string{"分类", "category", "品类名"}},
			{ID: "seed_d_brand", ModelID: modelID, LogicalTableID: ltProd, Name: "品牌", Expr: "brand", DataType: "string", Synonyms: []string{"brand"}},
			{ID: "seed_d_pname", ModelID: modelID, LogicalTableID: ltProd, Name: "商品名称", Expr: "name", DataType: "string", Synonyms: []string{"商品", "product", "商品名"}},
			{ID: "seed_d_pstatus", ModelID: modelID, LogicalTableID: ltProd, Name: "商品状态", Expr: "status", DataType: "string", Synonyms: []string{"product_status", "上架状态"}},
		},
	}
}
