// 工具：为「电商演示库」(ecommerce_demo) 生成一套规模更大、外键一致的演示数据。
// 仅用于开发/演示：会 TRUNCATE 现有 5 张业务表并按目标规模重建，同时新增 4 组关联表
// (product_reviews / suppliers+purchase_orders / coupons+customer_coupons / shipments)。
//
// 用法：cd link-go && set -a && source .env && set +a && go run ./cmd/seed-ecommerce
// 连接的是业务库 ecommerce_demo（非 link 应用库），用 root 账号建表+写数据；
// 数据源里配置的 ecommerce_ro 只读账号仅供 Go 后端查询，不用于本工具。
//
// 规模（中等）：客户 500 / 商品 150 / 订单 3000 / 明细 ~7000，
// 评价 ~2500 / 供应商 30 / 采购单 400 / 优惠券模板 25 / 领券记录 1500 / 发货单 ~2000。
// 所有主键显式指定（TRUNCATE 后自增归零，id 从 1 连续），因此跨表引用完全可控。
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---- 规模常量 ----
const (
	nCustomers = 500
	nProducts  = 150
	nOrders    = 3000
	nSuppliers = 30
	nPurchase  = 400
	nCoupons   = 25
	nCustCoup  = 1500
)

// ---- 随机素材池 ----
var (
	surnames  = []rune("赵钱孙李周吴郑王冯陈褚卫蒋沈韩杨朱秦尤许何吕施张孔曹严华金魏陶姜戚谢邹喻")
	givenA    = []rune("伟芳娜秀英敏静丽强磊军洋勇艳杰娟涛明超霞平刚桂香文辉力莉倩宇浩晨")
	givenB    = []rune("华建国荣春海生龙飞鹏杰昊然轩睿欣怡雅琪梓涵子萱")
	cities    = []string{"北京", "上海", "广州", "深圳", "杭州", "成都", "武汉", "南京", "西安", "苏州", "重庆", "长沙", "青岛", "天津", "郑州"}
	brands    = []string{"华为", "小米", "苹果", "三星", "联想", "戴尔", "索尼", "美的", "海尔", "格力", "耐克", "阿迪达斯", "优衣库", "兰蔻", "雅诗兰黛", "飞利浦", "九阳", "膳魔师", "安踏", "李宁"}
	carriers  = []string{"顺丰速运", "中通快递", "圆通速递", "京东物流", "韵达快递", "申通快递"}
	payMethod = []string{"alipay", "wechat", "card"}
	reviewPos = []string{"质量很好，物流很快，非常满意！", "宝贝收到了，做工精细，好评。", "用了几天，性价比很高，推荐购买。", "包装很仔细，正品无疑，会回购。", "服务态度好，产品符合预期。", "第二次购买了，一如既往地好。"}
	reviewMid = []string{"东西还行，就是发货有点慢。", "整体不错，细节还有提升空间。", "价格偏高，但质量对得起。", "包装一般，产品尚可。"}
	reviewNeg = []string{"和描述有差距，有点失望。", "物流太慢了，等了好几天。", "做工一般，性价比不高。"}
)

func randName(r *rand.Rand) string {
	n := string(surnames[r.Intn(len(surnames))])
	if r.Intn(2) == 0 {
		return n + string(givenA[r.Intn(len(givenA))])
	}
	return n + string(givenB[r.Intn(len(givenB))]) + string(givenA[r.Intn(len(givenA))])
}

// pick 按权重返回索引对应的值。
func pickWeighted(r *rand.Rand, items []string, weights []int) string {
	total := 0
	for _, w := range weights {
		total += w
	}
	x := r.Intn(total)
	for i, w := range weights {
		if x < w {
			return items[i]
		}
		x -= w
	}
	return items[len(items)-1]
}

func daysBack(r *rand.Rand, base time.Time, maxDays int) time.Time {
	d := r.Intn(maxDays)
	return base.AddDate(0, 0, -d).
		Add(time.Duration(r.Intn(24)) * time.Hour).
		Add(time.Duration(r.Intn(60)) * time.Minute).
		Add(time.Duration(r.Intn(60)) * time.Second)
}

const dtLayout = "2006-01-02 15:04:05"

func main() {
	r := rand.New(rand.NewSource(20260705))
	now := time.Now()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/ecommerce_demo?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true",
		env("DB_USER", "root"), env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"))

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open failed: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping failed: %v (库 ecommerce_demo 是否存在?)", err)
	}

	must(db, "SET FOREIGN_KEY_CHECKS=0")
	createNewTables(db)
	// 清空全部相关表（自增归零，主键从 1 连续）。
	for _, t := range []string{"categories", "products", "customers", "orders", "order_items",
		"product_reviews", "suppliers", "purchase_orders", "coupons", "customer_coupons", "shipments"} {
		must(db, "TRUNCATE TABLE `"+t+"`")
	}

	// ---------- categories（6 顶级 + 若干二级） ----------
	type cat struct {
		id       int
		name     string
		parentID *int
	}
	tops := []string{"电子数码", "家用电器", "服饰鞋包", "美妆个护", "食品生鲜", "运动户外"}
	subMap := map[string][]string{
		"电子数码": {"手机通讯", "笔记本电脑", "平板电脑", "智能穿戴", "影音设备"},
		"家用电器": {"厨房电器", "生活电器", "大家电"},
		"服饰鞋包": {"男装", "女装", "运动鞋", "箱包"},
		"美妆个护": {"护肤", "彩妆", "个人清洁"},
		"食品生鲜": {"休闲零食", "粮油调味", "生鲜果蔬"},
		"运动户外": {"健身器材", "户外装备"},
	}
	var cats []cat
	cid := 0
	topID := map[string]int{}
	for _, t := range tops {
		cid++
		cats = append(cats, cat{cid, t, nil})
		topID[t] = cid
	}
	var leafIDs []int // 只有叶子分类挂商品
	for _, t := range tops {
		p := topID[t]
		for _, s := range subMap[t] {
			cid++
			pid := p
			cats = append(cats, cat{cid, s, &pid})
			leafIDs = append(leafIDs, cid)
		}
	}
	{
		cols := []string{"id", "name", "parent_id"}
		rows := make([][]interface{}, 0, len(cats))
		for _, c := range cats {
			var pv interface{}
			if c.parentID != nil {
				pv = *c.parentID
			}
			rows = append(rows, []interface{}{c.id, c.name, pv})
		}
		batchInsert(db, "categories", cols, rows)
	}

	// ---------- customers ----------
	custCity := make([]string, nCustomers+1)
	custReg := make([]time.Time, nCustomers+1)
	{
		cols := []string{"id", "name", "gender", "city", "vip_level", "email", "phone", "registered_at"}
		rows := make([][]interface{}, 0, nCustomers)
		for id := 1; id <= nCustomers; id++ {
			gender := "male"
			if r.Intn(2) == 0 {
				gender = "female"
			}
			city := cities[r.Intn(len(cities))]
			vip := pickWeightedInt(r, []int{0, 1, 2, 3}, []int{50, 28, 15, 7})
			reg := daysBack(r, now, 1000)
			custCity[id] = city
			custReg[id] = reg
			email := fmt.Sprintf("user%04d@demo.com", id)
			phone := fmt.Sprintf("1%d%08d", 3+r.Intn(6), r.Intn(100000000))
			rows = append(rows, []interface{}{id, randName(r), gender, city, vip, email, phone, reg.Format(dtLayout)})
		}
		batchInsert(db, "customers", cols, rows)
	}

	// ---------- products ----------
	prodPrice := make([]float64, nProducts+1)
	prodCost := make([]float64, nProducts+1)
	{
		cols := []string{"id", "category_id", "name", "brand", "price", "cost", "stock", "status", "created_at"}
		rows := make([][]interface{}, 0, nProducts)
		for id := 1; id <= nProducts; id++ {
			catID := leafIDs[r.Intn(len(leafIDs))]
			brand := brands[r.Intn(len(brands))]
			price := float64(r.Intn(499000)+1000) / 100.0 // 10 ~ 5000
			cost := round2(price * (0.45 + r.Float64()*0.3))
			price = round2(price)
			stock := r.Intn(2000)
			status := "on_sale"
			if r.Intn(100) < 12 {
				status = "off_shelf"
			}
			prodPrice[id] = price
			prodCost[id] = cost
			name := fmt.Sprintf("%s %s%02d款", brand, cats[catID-1].name, r.Intn(99)+1)
			rows = append(rows, []interface{}{id, catID, name, brand, price, cost, stock, status, daysBack(r, now, 730).Format(dtLayout)})
		}
		batchInsert(db, "products", cols, rows)
	}

	// ---------- orders + order_items ----------
	orderStatus := make([]string, nOrders+1)
	orderCust := make([]int, nOrders+1)
	orderCreated := make([]time.Time, nOrders+1)
	// 记录每个订单买了哪些商品（供评价引用）。
	orderProducts := make([][]int, nOrders+1)
	statusOpts := []string{"completed", "shipped", "paid", "cancelled", "refunded"}
	statusW := []int{55, 12, 10, 13, 10}
	{
		oCols := []string{"id", "order_no", "customer_id", "status", "total_amount", "discount_amount", "pay_amount", "payment_method", "created_at", "paid_at"}
		iCols := []string{"id", "order_id", "product_id", "quantity", "unit_price", "subtotal"}
		oRows := make([][]interface{}, 0, nOrders)
		iRows := make([][]interface{}, 0, nOrders*3)
		itemID := 0
		for id := 1; id <= nOrders; id++ {
			cust := r.Intn(nCustomers) + 1
			// 下单时间不早于客户注册时间，集中在最近一年、偏向近月。
			maxDays := 365
			created := daysBack(r, now, maxDays)
			if created.Before(custReg[cust]) {
				created = custReg[cust].Add(time.Duration(r.Intn(72)) * time.Hour)
			}
			status := pickWeighted(r, statusOpts, statusW)

			// 1~5 个不同商品
			nItems := 1 + r.Intn(5)
			seen := map[int]bool{}
			var total float64
			var prods []int
			for k := 0; k < nItems; k++ {
				pid := r.Intn(nProducts) + 1
				if seen[pid] {
					continue
				}
				seen[pid] = true
				qty := 1 + r.Intn(4)
				unit := prodPrice[pid]
				sub := round2(unit * float64(qty))
				total = round2(total + sub)
				itemID++
				iRows = append(iRows, []interface{}{itemID, id, pid, qty, unit, sub})
				prods = append(prods, pid)
			}
			orderProducts[id] = prods

			// 折扣 0 / 5% / 10% / 满减
			discount := 0.0
			switch r.Intn(4) {
			case 1:
				discount = round2(total * 0.05)
			case 2:
				discount = round2(total * 0.10)
			case 3:
				if total > 200 {
					discount = 20
				}
			}
			pay := round2(total - discount)

			var paidAt interface{}
			if status != "cancelled" {
				pt := created.Add(time.Duration(r.Intn(120)+1) * time.Minute)
				paidAt = pt.Format(dtLayout)
			}
			orderStatus[id] = status
			orderCust[id] = cust
			orderCreated[id] = created
			orderNo := fmt.Sprintf("SO%s%05d", created.Format("20060102"), id)
			oRows = append(oRows, []interface{}{id, orderNo, cust, status, total, discount, pay,
				payMethod[r.Intn(len(payMethod))], created.Format(dtLayout), paidAt})
		}
		batchInsert(db, "orders", oCols, oRows)
		batchInsert(db, "order_items", iCols, iRows)
		log.Printf("orders=%d order_items=%d", len(oRows), len(iRows))
	}

	// ---------- product_reviews（仅 completed 订单的一部分商品被评价） ----------
	{
		cols := []string{"id", "product_id", "customer_id", "order_id", "rating", "content", "created_at"}
		rows := make([][]interface{}, 0, 2500)
		rid := 0
		for oid := 1; oid <= nOrders; oid++ {
			if orderStatus[oid] != "completed" {
				continue
			}
			if r.Intn(100) >= 60 { // ~60% 已完成订单产生评价
				continue
			}
			for _, pid := range orderProducts[oid] {
				if r.Intn(100) >= 70 { // 订单内约 70% 商品被评
					continue
				}
				rating := pickWeightedInt(r, []int{5, 4, 3, 2, 1}, []int{55, 25, 10, 5, 5})
				var content string
				switch {
				case rating >= 4:
					content = reviewPos[r.Intn(len(reviewPos))]
				case rating == 3:
					content = reviewMid[r.Intn(len(reviewMid))]
				default:
					content = reviewNeg[r.Intn(len(reviewNeg))]
				}
				rid++
				ct := orderCreated[oid].AddDate(0, 0, r.Intn(15)+1)
				rows = append(rows, []interface{}{rid, pid, orderCust[oid], oid, rating, content, ct.Format(dtLayout)})
			}
		}
		batchInsert(db, "product_reviews", cols, rows)
		log.Printf("product_reviews=%d", len(rows))
	}

	// ---------- suppliers + purchase_orders ----------
	{
		cols := []string{"id", "name", "contact", "phone", "city", "rating", "created_at"}
		rows := make([][]interface{}, 0, nSuppliers)
		for id := 1; id <= nSuppliers; id++ {
			name := brands[r.Intn(len(brands))] + "授权供应商" + fmt.Sprintf("%02d", id)
			rating := round1(3.0 + r.Float64()*2.0)
			rows = append(rows, []interface{}{id, name, randName(r), fmt.Sprintf("1%d%08d", 3+r.Intn(6), r.Intn(100000000)),
				cities[r.Intn(len(cities))], rating, daysBack(r, now, 900).Format(dtLayout)})
		}
		batchInsert(db, "suppliers", cols, rows)
	}
	{
		cols := []string{"id", "po_no", "supplier_id", "product_id", "quantity", "unit_cost", "total_cost", "status", "created_at", "received_at"}
		rows := make([][]interface{}, 0, nPurchase)
		for id := 1; id <= nPurchase; id++ {
			pid := r.Intn(nProducts) + 1
			qty := (r.Intn(20) + 1) * 10
			unit := round2(prodCost[pid] * (0.9 + r.Float64()*0.2))
			total := round2(unit * float64(qty))
			status := pickWeighted(r, []string{"received", "pending", "cancelled"}, []int{70, 22, 8})
			created := daysBack(r, now, 700)
			var recv interface{}
			if status == "received" {
				recv = created.AddDate(0, 0, r.Intn(10)+1).Format(dtLayout)
			}
			poNo := fmt.Sprintf("PO%s%04d", created.Format("20060102"), id)
			rows = append(rows, []interface{}{id, poNo, r.Intn(nSuppliers) + 1, pid, qty, unit, total, status, created.Format(dtLayout), recv})
		}
		batchInsert(db, "purchase_orders", cols, rows)
		log.Printf("suppliers=%d purchase_orders=%d", nSuppliers, len(rows))
	}

	// ---------- coupons + customer_coupons ----------
	{
		cols := []string{"id", "code", "name", "type", "discount_value", "min_spend", "valid_from", "valid_to", "status", "created_at"}
		rows := make([][]interface{}, 0, nCoupons)
		for id := 1; id <= nCoupons; id++ {
			isPercent := r.Intn(2) == 0
			typ := "fixed"
			var val, minSpend float64
			if isPercent {
				typ = "percent"
				val = float64(r.Intn(4)+1) * 5 // 折扣百分比: 5/10/15/20 (%)
				minSpend = float64((r.Intn(5) + 1) * 100)
			} else {
				val = float64((r.Intn(10) + 1) * 10) // 10~100
				minSpend = val * float64(r.Intn(5)+5) // 门槛远高于面额
			}
			vf := daysBack(r, now, 400)
			vt := vf.AddDate(0, 0, 30+r.Intn(90))
			status := "active"
			if vt.Before(now) {
				status = "expired"
			}
			rows = append(rows, []interface{}{id, fmt.Sprintf("CPN%06d", id*7+1000), couponName(typ, val),
				typ, round2(val), round2(minSpend), vf.Format(dtLayout), vt.Format(dtLayout), status, vf.Format(dtLayout)})
		}
		batchInsert(db, "coupons", cols, rows)
	}
	{
		cols := []string{"id", "customer_id", "coupon_id", "order_id", "status", "received_at", "used_at"}
		rows := make([][]interface{}, 0, nCustCoup)
		for id := 1; id <= nCustCoup; id++ {
			cust := r.Intn(nCustomers) + 1
			coup := r.Intn(nCoupons) + 1
			recv := daysBack(r, now, 300)
			roll := r.Intn(100)
			var status string
			var orderID, usedAt interface{}
			switch {
			case roll < 45: // 已使用：绑定该客户的一笔订单
				status = "used"
				if oid := findCustomerOrder(orderCust, cust, r); oid > 0 {
					orderID = oid
					usedAt = orderCreated[oid].Format(dtLayout)
				} else {
					usedAt = recv.AddDate(0, 0, r.Intn(20)+1).Format(dtLayout)
				}
			case roll < 80:
				status = "unused"
			default:
				status = "expired"
			}
			rows = append(rows, []interface{}{id, cust, coup, orderID, status, recv.Format(dtLayout), usedAt})
		}
		batchInsert(db, "customer_coupons", cols, rows)
		log.Printf("coupons=%d customer_coupons=%d", nCoupons, len(rows))
	}

	// ---------- shipments（已发货/已完成订单各一单） ----------
	{
		cols := []string{"id", "order_id", "tracking_no", "carrier", "status", "shipped_at", "delivered_at", "receiver_city"}
		rows := make([][]interface{}, 0, 2200)
		sid := 0
		for oid := 1; oid <= nOrders; oid++ {
			st := orderStatus[oid]
			if st != "completed" && st != "shipped" {
				continue
			}
			sid++
			shipped := orderCreated[oid].Add(time.Duration(r.Intn(48)+2) * time.Hour)
			var delivered interface{}
			shipStatus := "in_transit"
			if st == "completed" {
				dt := shipped.AddDate(0, 0, r.Intn(5)+1)
				delivered = dt.Format(dtLayout)
				shipStatus = "delivered"
			}
			carrier := carriers[r.Intn(len(carriers))]
			tracking := fmt.Sprintf("SF%013d", r.Int63n(1e13))
			rows = append(rows, []interface{}{sid, oid, tracking, carrier, shipStatus,
				shipped.Format(dtLayout), delivered, custCity[orderCust[oid]]})
		}
		batchInsert(db, "shipments", cols, rows)
		log.Printf("shipments=%d", len(rows))
	}

	must(db, "SET FOREIGN_KEY_CHECKS=1")
	fmt.Println("OK: ecommerce_demo 重建完成（5 张表重灌 + 6 张新表）")
}

// findCustomerOrder 随机找该客户的一笔订单（简单线性抽样，找不到返回 0）。
func findCustomerOrder(orderCust []int, cust int, r *rand.Rand) int {
	for tries := 0; tries < 8; tries++ {
		oid := r.Intn(len(orderCust)-1) + 1
		if orderCust[oid] == cust {
			return oid
		}
	}
	return 0
}

func couponName(typ string, val float64) string {
	if typ == "percent" {
		return fmt.Sprintf("满减券·%.0f%%off", val)
	}
	return fmt.Sprintf("现金券·减%.0f元", val)
}

// createNewTables 建 6 张新表（风格与现有表一致：utf8mb4_unicode_ci + 注释 + 索引）。
func createNewTables(db *sql.DB) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS product_reviews (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品评价表'`,
		`CREATE TABLE IF NOT EXISTS suppliers (
			id INT NOT NULL AUTO_INCREMENT,
			name VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '供应商名称',
			contact VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系人',
			phone VARCHAR(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系电话',
			city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '所在城市',
			rating DECIMAL(2,1) NOT NULL DEFAULT '0.0' COMMENT '合作评级 0-5',
			created_at DATETIME NOT NULL COMMENT '合作起始时间',
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='供应商表'`,
		`CREATE TABLE IF NOT EXISTS purchase_orders (
			id INT NOT NULL AUTO_INCREMENT,
			po_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '采购单号',
			supplier_id INT NOT NULL COMMENT '供应商ID',
			product_id INT NOT NULL COMMENT '商品ID',
			quantity INT NOT NULL COMMENT '采购数量',
			unit_cost DECIMAL(10,2) NOT NULL COMMENT '采购单价',
			total_cost DECIMAL(12,2) NOT NULL COMMENT '采购总额',
			status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'pending' COMMENT '状态: received入库/pending在途/cancelled取消',
			created_at DATETIME NOT NULL COMMENT '下单时间',
			received_at DATETIME DEFAULT NULL COMMENT '入库时间',
			PRIMARY KEY (id),
			UNIQUE KEY po_no (po_no),
			KEY idx_supplier (supplier_id),
			KEY idx_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='采购单表'`,
		`CREATE TABLE IF NOT EXISTS coupons (
			id INT NOT NULL AUTO_INCREMENT,
			code VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '券码',
			name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '券名称',
			type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型: fixed现金券/percent折扣券',
			discount_value DECIMAL(10,2) NOT NULL COMMENT '面额(现金券为金额, 折扣券为百分比数值)',
			min_spend DECIMAL(10,2) NOT NULL DEFAULT '0.00' COMMENT '使用门槛',
			valid_from DATETIME NOT NULL COMMENT '生效时间',
			valid_to DATETIME NOT NULL COMMENT '失效时间',
			status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'active' COMMENT '状态: active有效/expired过期',
			created_at DATETIME NOT NULL COMMENT '创建时间',
			PRIMARY KEY (id),
			UNIQUE KEY code (code)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='优惠券模板表'`,
		`CREATE TABLE IF NOT EXISTS customer_coupons (
			id INT NOT NULL AUTO_INCREMENT,
			customer_id INT NOT NULL COMMENT '客户ID',
			coupon_id INT NOT NULL COMMENT '优惠券ID',
			order_id INT DEFAULT NULL COMMENT '使用的订单ID(未使用为NULL)',
			status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'unused' COMMENT '状态: unused未用/used已用/expired过期',
			received_at DATETIME NOT NULL COMMENT '领取时间',
			used_at DATETIME DEFAULT NULL COMMENT '使用时间',
			PRIMARY KEY (id),
			KEY idx_customer (customer_id),
			KEY idx_coupon (coupon_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户领券记录表'`,
		`CREATE TABLE IF NOT EXISTS shipments (
			id INT NOT NULL AUTO_INCREMENT,
			order_id INT NOT NULL COMMENT '订单ID',
			tracking_no VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '快递单号',
			carrier VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '承运商',
			status VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '状态: delivered已签收/in_transit运输中/returned退回',
			shipped_at DATETIME NOT NULL COMMENT '发货时间',
			delivered_at DATETIME DEFAULT NULL COMMENT '签收时间',
			receiver_city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '收货城市',
			PRIMARY KEY (id),
			UNIQUE KEY uk_order (order_id),
			KEY idx_status (status)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='发货物流表'`,
	}
	for _, s := range stmts {
		must(db, s)
	}
}

// ---- 工具函数 ----

func must(db *sql.DB, q string) {
	if _, err := db.Exec(q); err != nil {
		log.Fatalf("exec failed: %v\nSQL: %.120s", err, q)
	}
}

// batchInsert 分批参数化写入，避免单条 SQL 过长。
func batchInsert(db *sql.DB, table string, cols []string, rows [][]interface{}) {
	if len(rows) == 0 {
		return
	}
	const batch = 400
	colList := "`" + strings.Join(cols, "`,`") + "`"
	ph := "(" + strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"
	for start := 0; start < len(rows); start += batch {
		end := start + batch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		var b strings.Builder
		b.WriteString("INSERT INTO `" + table + "` (" + colList + ") VALUES ")
		args := make([]interface{}, 0, len(chunk)*len(cols))
		for i, row := range chunk {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(ph)
			args = append(args, row...)
		}
		if _, err := db.Exec(b.String(), args...); err != nil {
			log.Fatalf("batch insert %s failed: %v", table, err)
		}
	}
}

func pickWeightedInt(r *rand.Rand, items []int, weights []int) int {
	total := 0
	for _, w := range weights {
		total += w
	}
	x := r.Intn(total)
	for i, w := range weights {
		if x < w {
			return items[i]
		}
		x -= w
	}
	return items[len(items)-1]
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100.0
}
func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10.0
}
