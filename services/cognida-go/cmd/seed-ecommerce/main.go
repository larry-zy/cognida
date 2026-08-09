// 工具：为「电商演示库」(ecommerce_demo) 生成一套规模更大、结构更复杂、外键一致的演示数据。
// 仅用于开发/演示：DROP 并按增强后的 schema 重建全部 30 张表，再灌入多年跨度的关联数据。
//
// 用法：cd cognida-go && set -a && source .env && set +a && go run ./cmd/seed-ecommerce
// 连接的是业务库 ecommerce_demo（非 link 应用库），用 root 账号建表+写数据；
// 数据源里配置的 ecommerce_ro 只读账号仅供 Go 后端查询，不用于本工具（库级 GRANT 覆盖新表）。
//
// 本工具「自持完整 schema」：不依赖 seed-ecommerce-demo.sql 的历史列，DROP+CREATE 全部表，
// 因此新增列/新表无需手动 ALTER。
//
// 表结构（30 张）：
//   基础域：categories / products / customers / orders / order_items
//   交易域：product_reviews / shipments / order_returns
//   供应链：suppliers / purchase_orders / inventory_snapshots
//   营销域：coupons / customer_coupons / marketing_campaigns / ad_spend_daily
//   行为域：user_events / shopping_carts / cart_items（点击流/漏斗/加购弃购）
//   会员域：points_ledger（积分流水） / product_price_history（价格变更）
//   分群域：customer_rfm_snapshots（RFM月度快照） / membership_tier_history（会员升级轨迹）
//   实验域：ab_experiments / ab_assignments（A/B分流转化） / recommendation_events（推荐曝光点击流）
//   客服域：support_tickets（SLA时效/CSAT） / nps_surveys（NPS调研）
//   促销域：flash_sales（秒杀） / group_buys（拼团） / promotion_products（促销参与商品多对多）
//
// 数据规模：客户 2000 / 商品 500 / 订单 20000 / 明细 ~6 万，时间跨度约 3 年；
// 订单时间带「业务增长趋势 + 月度季节性 + 大促尖峰(618/双11/双12) + 周末/时段」分布，
// 支撑同比环比、漏斗转化、加购弃购、RFM/留存/Cohort、积分 LTV、价格弹性、
// 客户分群/生命周期、A/B实验、推荐CTR/CVR、客服SLA/满意度、秒杀拼团/购物篮等复杂分析场景。
// 所有主键显式指定（DROP 后自增归零，id 从 1 连续），跨表引用完全可控。
package main

import (
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
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
	nCustomers = 5000
	nProducts  = 1200
	nOrders    = 60000
	nSuppliers = 300
	nPurchase  = 15000
	nCoupons   = 300
	nCustCoup  = 40000
	nCampaigns = 400
	// 复杂场景表规模
	nExperiments = 60    // A/B 实验
	nAgents      = 80    // 客服坐席
	nTickets     = 24000 // 客服工单
	nNPS         = 12000 // NPS 调研
	nFlashSales  = 500   // 秒杀场次
	nGroupBuys   = 8000  // 拼团实例
	rfmMonths    = 18     // RFM 快照回溯月数
	// 时间跨度：订单最多回溯 ~3 年；客户注册更早，避免下单早于注册。
	spanOrderDays = 1080
	spanCustDays  = 1260
	spanProdDays  = 1000
)

// ---- 随机素材池 ----
var (
	surnames  = []rune("赵钱孙李周吴郑王冯陈褚卫蒋沈韩杨朱秦尤许何吕施张孔曹严华金魏陶姜戚谢邹喻")
	givenA    = []rune("伟芳娜秀英敏静丽强磊军洋勇艳杰娟涛明超霞平刚桂香文辉力莉倩宇浩晨")
	givenB    = []rune("华建国荣春海生龙飞鹏杰昊然轩睿欣怡雅琪梓涵子萱")
	brands    = []string{"华为", "小米", "苹果", "三星", "联想", "戴尔", "索尼", "美的", "海尔", "格力", "耐克", "阿迪达斯", "优衣库", "兰蔻", "雅诗兰黛", "飞利浦", "九阳", "膳魔师", "安踏", "李宁"}
	carriers  = []string{"顺丰速运", "中通快递", "圆通速递", "京东物流", "韵达快递", "申通快递"}
	payMethod = []string{"alipay", "wechat", "card", "unionpay"}
	channels  = []string{"web", "app", "mini_program", "offline"}
	channelW  = []int{26, 44, 22, 8}
	regChans  = []string{"organic", "ad", "referral", "social", "offline"}
	regChanW  = []int{34, 26, 18, 14, 8}
	campChans = []string{"search_ad", "feed_ad", "social", "affiliate", "edm"}
	campObjs  = []string{"拉新", "促活", "转化", "品牌曝光"}
	// campToUTM：营销活动渠道 → 订单归因流量来源(utm_source)。
	campToUTM = map[string]string{
		"search_ad": "paid_search", "feed_ad": "social", "social": "social",
		"affiliate": "affiliate", "edm": "edm",
	}
	utmSources = []string{"organic", "direct", "paid_search", "social", "affiliate", "edm"}
	utmW       = []int{30, 24, 16, 14, 10, 6}
	warehouses = []string{"华东中心仓", "华北中心仓", "华南中心仓", "西南中心仓"}
	returnReasons = []string{"质量问题", "尺寸不合适", "与描述不符", "七天无理由", "物流损坏", "发错货", "重复购买"}
	priceReasons  = []string{"促销调价", "成本上涨", "换季调整", "竞品对标", "清仓甩卖", "新品定价"}
	reviewPos = []string{"质量很好，物流很快，非常满意！", "宝贝收到了，做工精细，好评。", "用了几天，性价比很高，推荐购买。", "包装很仔细，正品无疑，会回购。", "服务态度好，产品符合预期。", "第二次购买了，一如既往地好。"}
	reviewMid = []string{"东西还行，就是发货有点慢。", "整体不错，细节还有提升空间。", "价格偏高，但质量对得起。", "包装一般，产品尚可。"}
	reviewNeg = []string{"和描述有差距，有点失望。", "物流太慢了，等了好几天。", "做工一般，性价比不高。"}
	// 会员等级（消费驱动轨迹，与 customers.vip_level 的当前运营等级解耦）
	tierNames = []string{"bronze", "silver", "gold", "platinum"}
	tierUpTh  = []float64{0, 5000, 20000, 60000} // 升入各级的累计消费门槛
	// RFM 客户分群标签
	rfmSegments = []string{"重要价值客户", "重要发展客户", "重要保持客户", "重要挽留客户",
		"一般价值客户", "一般发展客户", "一般保持客户", "一般挽留客户", "流失预警客户"}
	// A/B 实验
	abMetrics  = []string{"conversion_rate", "aov", "add_cart_rate", "ctr", "retention_7d"}
	abVariants = []string{"control", "variant_a", "variant_b"}
	abNames    = []string{"结算页按钮文案", "商详页推荐位排序", "首页 banner 样式", "购物车满减提示", "搜索结果排序算法",
		"新客首单优惠券", "价格展示方式", "商品主图轮播", "会员权益弹窗", "物流时效标签"}
	// 推荐位场景
	recScenes = []string{"首页猜你喜欢", "购物车推荐", "商详页相关推荐", "搜索结果推荐", "下单成功页推荐"}
	// 客服工单
	ticketCats     = []string{"退款咨询", "物流问题", "商品咨询", "投诉建议", "发票问题", "账户问题", "其他"}
	ticketChans    = []string{"在线客服", "电话", "邮件", "APP工单"}
	ticketPrio     = []string{"low", "medium", "high", "urgent"}
	ticketPrioW    = []int{30, 40, 22, 8}
	npsComments = []string{"体验很好会推荐给朋友", "整体满意但物流可以更快", "商品质量不错", "客服响应及时", "希望多些优惠活动", "价格偏贵", "退货流程有点繁琐", ""}
)

// cityMeta：城市 → {省份, 大区}，供 customers.province 与 orders.region 派生。
var cityMeta = map[string][2]string{
	"北京": {"北京", "华北"}, "天津": {"天津", "华北"},
	"上海": {"上海", "华东"}, "杭州": {"浙江", "华东"}, "南京": {"江苏", "华东"}, "苏州": {"江苏", "华东"}, "青岛": {"山东", "华东"},
	"广州": {"广东", "华南"}, "深圳": {"广东", "华南"},
	"成都": {"四川", "西南"}, "重庆": {"重庆", "西南"},
	"武汉": {"湖北", "华中"}, "长沙": {"湖南", "华中"}, "郑州": {"河南", "华中"},
	"西安": {"陕西", "西北"},
}

var cities = []string{"北京", "上海", "广州", "深圳", "杭州", "成都", "武汉", "南京", "西安", "苏州", "重庆", "长沙", "青岛", "天津", "郑州"}

func randName(r *rand.Rand) string {
	n := string(surnames[r.Intn(len(surnames))])
	if r.Intn(2) == 0 {
		return n + string(givenA[r.Intn(len(givenA))])
	}
	return n + string(givenB[r.Intn(len(givenB))]) + string(givenA[r.Intn(len(givenA))])
}

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
	// 日内抖动一律「往前减」，确保 d==0 时结果仍严格早于 base（不越过 now 落到未来）。
	return base.AddDate(0, 0, -d).
		Add(-time.Duration(r.Intn(24)) * time.Hour).
		Add(-time.Duration(r.Intn(60)) * time.Minute).
		Add(-time.Duration(r.Intn(60)) * time.Second)
}

// notFuture 把「基准时间 + 正向偏移」得到的子记录时间钳制在 now 之内，
// 避免评价/退货/发货/收货等衍生事件因基准接近 now 而落到未来。
func notFuture(t, now time.Time) time.Time {
	if t.After(now) {
		return now
	}
	return t
}

// ---- 订单时间分布：业务增长 + 月度季节性 + 大促尖峰 + 周末 ----

// promoBoost 返回大促日的下单倍率（1.0 为平日）。
func promoBoost(d time.Time) float64 {
	m, day := d.Month(), d.Day()
	switch {
	case m == time.November && day == 11:
		return 6.0 // 双11
	case m == time.November && day >= 9 && day <= 13:
		return 3.0
	case m == time.June && day == 18:
		return 5.0 // 618
	case m == time.June && day >= 15 && day <= 19:
		return 2.6
	case m == time.December && day == 12:
		return 3.2 // 双12
	case m == time.December && day >= 20 && day <= 31:
		return 1.8 // 元旦季
	case m == time.September && day == 9:
		return 1.6 // 99 划算节
	case m == time.March && day == 8:
		return 1.5 // 女神节
	}
	return 1.0
}

// monthMul 月度季节性倍率。
var monthMul = map[time.Month]float64{
	time.January: 0.9, time.February: 0.7, time.March: 0.95, time.April: 1.0,
	time.May: 1.05, time.June: 1.4, time.July: 1.0, time.August: 1.05,
	time.September: 1.1, time.October: 1.05, time.November: 1.6, time.December: 1.3,
}

// buildDayWeights 预计算过去 days 天每日下单权重的前缀和，供 pickDay 采样。
// index 0 = 最早，days-1 = 今天；权重 = 增长趋势 × 月度季节性 × 周末 × 大促。
func buildDayWeights(now time.Time, days int) ([]time.Time, []float64) {
	dates := make([]time.Time, days)
	cum := make([]float64, days)
	total := 0.0
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		growth := 0.45 + 0.55*float64(i)/float64(days-1) // 三年内业务由弱到强
		w := growth * monthMul[d.Month()]
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			w *= 1.18
		}
		w *= promoBoost(d)
		total += w
		dates[i] = d
		cum[i] = total
	}
	return dates, cum
}

// hourOfDay 返回一天中带「晚间高峰」偏好的小时（0-23）。
func hourOfDay(r *rand.Rand) int {
	h := pickWeightedInt(r, []int{0, 3, 6, 9, 12, 15, 18, 21}, []int{2, 1, 4, 11, 12, 11, 17, 14})
	return h + r.Intn(3)
}

// pickDay 按预计算权重抽一天，并叠加时段分布，返回钳制在 now 内的时间点。
func pickDay(r *rand.Rand, dates []time.Time, cum []float64, now time.Time) time.Time {
	x := r.Float64() * cum[len(cum)-1]
	lo, hi := 0, len(cum)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if cum[mid] < x {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	d := dates[lo]
	t := time.Date(d.Year(), d.Month(), d.Day(), hourOfDay(r), r.Intn(60), r.Intn(60), 0, d.Location())
	return notFuture(t, now)
}

// spread 在 [start,end] 上生成 n 个单调递增时间点（末点=end），供会话事件时间线使用。
func spread(start, end time.Time, n int) []time.Time {
	if n <= 1 {
		return []time.Time{end}
	}
	if !end.After(start) {
		start = end.Add(-time.Duration(n) * time.Minute)
	}
	span := end.Sub(start)
	out := make([]time.Time, n)
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		out[i] = start.Add(time.Duration(float64(span) * frac))
	}
	out[n-1] = end
	return out
}

const dtLayout = "2006-01-02 15:04:05"
const dateLayout = "2006-01-02"

// itemLine 一条订单明细的内存视图（供后续退货/发货/购物车引用）。
type itemLine struct {
	pid, qty   int
	unit, cost float64
	sub        float64
}

func main() {
	// #nosec G404 -- 播种测试数据非加密用途
	r := rand.New(rand.NewSource(20260705))
	now := time.Now()
	dayDates, dayCum := buildDayWeights(now, spanOrderDays)

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
	createSchema(db)

	// ---------- categories（6 顶级 + 若干二级，带 level/sort_order） ----------
	type cat struct {
		id       int
		name     string
		parentID *int
		level    int
		sort     int
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
	for i, t := range tops {
		cid++
		cats = append(cats, cat{cid, t, nil, 1, i + 1})
		topID[t] = cid
	}
	var leafIDs []int
	for _, t := range tops {
		p := topID[t]
		for i, s := range subMap[t] {
			cid++
			pid := p
			cats = append(cats, cat{cid, s, &pid, 2, i + 1})
			leafIDs = append(leafIDs, cid)
		}
	}
	{
		cols := []string{"id", "name", "parent_id", "level", "sort_order"}
		rows := make([][]interface{}, 0, len(cats))
		for _, c := range cats {
			var pv interface{}
			if c.parentID != nil {
				pv = *c.parentID
			}
			rows = append(rows, []interface{}{c.id, c.name, pv, c.level, c.sort})
		}
		batchInsert(db, "categories", cols, rows)
	}

	// ---------- customers（+ age / province / register_channel） ----------
	custCity := make([]string, nCustomers+1)
	custReg := make([]time.Time, nCustomers+1)
	{
		cols := []string{"id", "name", "gender", "age", "city", "province", "vip_level", "register_channel", "email", "phone", "registered_at"}
		rows := make([][]interface{}, 0, nCustomers)
		for id := 1; id <= nCustomers; id++ {
			gender := "male"
			if r.Intn(2) == 0 {
				gender = "female"
			}
			city := cities[r.Intn(len(cities))]
			province := cityMeta[city][0]
			age := 18 + pickWeightedInt(r, []int{0, 12, 25, 37}, []int{20, 40, 28, 12}) + r.Intn(12)
			vip := pickWeightedInt(r, []int{0, 1, 2, 3}, []int{50, 28, 15, 7})
			regCh := pickWeighted(r, regChans, regChanW)
			reg := daysBack(r, now, spanCustDays)
			custCity[id] = city
			custReg[id] = reg
			email := fmt.Sprintf("user%04d@demo.com", id)
			phone := fmt.Sprintf("1%d%08d", 3+r.Intn(6), r.Intn(100000000))
			rows = append(rows, []interface{}{id, randName(r), gender, age, city, province, vip, regCh, email, phone, reg.Format(dtLayout)})
		}
		batchInsert(db, "customers", cols, rows)
	}

	// ---------- products（+ sku / weight_kg / launch_date） ----------
	prodPrice := make([]float64, nProducts+1)
	prodCost := make([]float64, nProducts+1)
	prodLaunch := make([]time.Time, nProducts+1)
	{
		cols := []string{"id", "category_id", "name", "brand", "sku", "price", "cost", "weight_kg", "stock", "status", "launch_date", "created_at"}
		rows := make([][]interface{}, 0, nProducts)
		for id := 1; id <= nProducts; id++ {
			catID := leafIDs[r.Intn(len(leafIDs))]
			brand := brands[r.Intn(len(brands))]
			price := float64(r.Intn(499000)+1000) / 100.0 // 10 ~ 5000
			cost := round2(price * (0.45 + r.Float64()*0.3))
			price = round2(price)
			weight := round2(0.1 + r.Float64()*19.9)
			stock := r.Intn(2000)
			status := "on_sale"
			if r.Intn(100) < 12 {
				status = "off_shelf"
			}
			prodPrice[id] = price
			prodCost[id] = cost
			launch := daysBack(r, now, spanProdDays)
			prodLaunch[id] = launch
			name := fmt.Sprintf("%s %s%02d款", brand, cats[catID-1].name, r.Intn(99)+1)
			sku := fmt.Sprintf("SKU-%03d-%05d", catID, id)
			rows = append(rows, []interface{}{id, catID, name, brand, sku, price, cost, weight, stock, status,
				launch.Format(dateLayout), launch.Format(dtLayout)})
		}
		batchInsert(db, "products", cols, rows)
	}

	// ---------- marketing_campaigns + ad_spend_daily（先于订单生成，供订单归因引用） ----------
	campStart := make([]time.Time, nCampaigns+1)
	campEnd := make([]time.Time, nCampaigns+1)
	campChan := make([]string, nCampaigns+1)
	{
		cCols := []string{"id", "name", "channel", "objective", "budget", "start_date", "end_date", "status", "created_at"}
		aCols := []string{"id", "campaign_id", "stat_date", "impressions", "clicks", "cost", "conversions", "revenue"}
		cRows := make([][]interface{}, 0, nCampaigns)
		aRows := make([][]interface{}, 0, nCampaigns*45)
		adID := 0
		for id := 1; id <= nCampaigns; id++ {
			ch := campChans[r.Intn(len(campChans))]
			obj := campObjs[r.Intn(len(campObjs))]
			budget := float64((r.Intn(19) + 1) * 5000) // 5k ~ 100k
			start := daysBack(r, now, spanOrderDays)
			durDays := 14 + r.Intn(76) // 14~90 天
			end := start.AddDate(0, 0, durDays)
			status := "ended"
			if end.After(now) {
				status = "running"
			}
			campStart[id] = start
			campEnd[id] = end
			campChan[id] = ch
			name := fmt.Sprintf("%s-%s-第%02d期", obj, ch, id)
			cRows = append(cRows, []interface{}{id, name, ch, obj, round2(budget),
				start.Format(dateLayout), end.Format(dateLayout), status, start.Format(dtLayout)})

			// 日花费：最多采样 45 天，累计不超预算；点击/转化/回流金额按典型漏斗生成。
			day := start
			spent := 0.0
			dayCap := durDays
			if dayCap > 45 {
				dayCap = 45
			}
			for d := 0; d < dayCap; d++ {
				if day.After(now) {
					break
				}
				impr := 2000 + r.Intn(48000)
				ctr := 0.008 + r.Float64()*0.04
				clicks := int(float64(impr) * ctr)
				cost := round2(float64(clicks) * (0.4 + r.Float64()*2.6))
				if spent+cost > budget {
					cost = round2(budget - spent)
				}
				spent = round2(spent + cost)
				cvr := 0.01 + r.Float64()*0.06
				conv := int(float64(clicks) * cvr)
				revenue := round2(float64(conv) * (80 + r.Float64()*420))
				adID++
				aRows = append(aRows, []interface{}{adID, id, day.Format(dateLayout), impr, clicks, cost, conv, revenue})
				day = day.AddDate(0, 0, 1)
				if spent >= budget {
					break
				}
			}
		}
		batchInsert(db, "marketing_campaigns", cCols, cRows)
		batchInsert(db, "ad_spend_daily", aCols, aRows)
		log.Printf("marketing_campaigns=%d ad_spend_daily=%d", len(cRows), len(aRows))
	}

	// ---------- coupons 券模板（先于订单，供订单券归因引用） ----------
	coupMin := make([]float64, nCoupons+1)
	coupFrom := make([]time.Time, nCoupons+1)
	coupTo := make([]time.Time, nCoupons+1)
	{
		cols := []string{"id", "code", "name", "type", "discount_value", "min_spend", "valid_from", "valid_to", "status", "created_at"}
		rows := make([][]interface{}, 0, nCoupons)
		for id := 1; id <= nCoupons; id++ {
			isPercent := r.Intn(2) == 0
			typ := "fixed"
			var val, minSpend float64
			if isPercent {
				typ = "percent"
				val = float64(r.Intn(4)+1) * 5
				minSpend = float64((r.Intn(5) + 1) * 100)
			} else {
				val = float64((r.Intn(10) + 1) * 10)
				minSpend = val * float64(r.Intn(5)+5)
			}
			vf := daysBack(r, now, 600)
			vt := vf.AddDate(0, 0, 30+r.Intn(90))
			status := "active"
			if vt.Before(now) {
				status = "expired"
			}
			coupMin[id] = round2(minSpend)
			coupFrom[id] = vf
			coupTo[id] = vt
			rows = append(rows, []interface{}{id, fmt.Sprintf("CPN%06d", id*7+1000), couponName(typ, val),
				typ, round2(val), round2(minSpend), vf.Format(dtLayout), vt.Format(dtLayout), status, vf.Format(dtLayout)})
		}
		batchInsert(db, "coupons", cols, rows)
	}

	// ---------- orders + order_items + order_returns（同一趟生成，退货依赖明细行） ----------
	orderStatus := make([]string, nOrders+1)
	orderCust := make([]int, nOrders+1)
	orderCreated := make([]time.Time, nOrders+1)
	orderLines := make([][]itemLine, nOrders+1)
	orderChannel := make([]string, nOrders+1)
	orderPay := make([]float64, nOrders+1)
	orderCampaign := make([]int, nOrders+1) // 0 表示无归因
	statusOpts := []string{"completed", "shipped", "paid", "cancelled", "refunded"}
	statusW := []int{55, 12, 10, 13, 10}
	var retRows [][]interface{}
	retID := 0
	{
		oCols := []string{"id", "order_no", "customer_id", "status", "channel", "region",
			"campaign_id", "coupon_id", "utm_source", "is_first_order",
			"total_amount", "discount_amount", "shipping_fee", "tax_amount", "refund_amount", "pay_amount",
			"payment_method", "created_at", "paid_at"}
		iCols := []string{"id", "order_id", "product_id", "quantity", "unit_price", "cost_price", "subtotal"}
		oRows := make([][]interface{}, 0, nOrders)
		iRows := make([][]interface{}, 0, nOrders*3)
		itemID := 0
		for id := 1; id <= nOrders; id++ {
			cust := r.Intn(nCustomers) + 1
			created := pickDay(r, dayDates, dayCum, now)
			if created.Before(custReg[cust]) {
				created = notFuture(custReg[cust].Add(time.Duration(r.Intn(72))*time.Hour), now)
			}
			status := pickWeighted(r, statusOpts, statusW)
			region := cityMeta[custCity[cust]][1]
			channel := pickWeighted(r, channels, channelW)

			nItems := 1 + r.Intn(5)
			seen := map[int]bool{}
			var total float64
			var lines []itemLine
			for k := 0; k < nItems; k++ {
				pid := r.Intn(nProducts) + 1
				if seen[pid] {
					continue
				}
				seen[pid] = true
				qty := 1 + r.Intn(4)
				unit := prodPrice[pid]
				cst := prodCost[pid]
				sub := round2(unit * float64(qty))
				total = round2(total + sub)
				itemID++
				iRows = append(iRows, []interface{}{itemID, id, pid, qty, unit, cst, sub})
				lines = append(lines, itemLine{pid: pid, qty: qty, unit: unit, cost: cst, sub: sub})
			}
			orderLines[id] = lines

			// 折扣 + 券归因：折扣命中时尽量关联一张当日有效、门槛达标的券模板。
			discount := 0.0
			var couponID interface{}
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
			if discount > 0 {
				if c := pickCoupon(r, coupMin, coupFrom, coupTo, total, created, nCoupons); c > 0 {
					couponID = c
				}
			}
			pay := round2(total - discount)
			// 运费：满 99 包邮，否则按区域小额运费；税额按 13% 内含价税分离。
			shipping := 0.0
			if pay < 99 {
				shipping = float64([]int{8, 10, 12, 15}[r.Intn(4)])
			}
			tax := round2(pay - pay/1.13)

			// 归因：约 40% 订单归因到当日在投的营销活动，utm_source 随之派生。
			var campaignID interface{}
			utm := pickWeighted(r, utmSources, utmW)
			if r.Intn(100) < 40 {
				if c := pickCampaign(r, campStart, campEnd, created, nCampaigns); c > 0 {
					campaignID = c
					orderCampaign[id] = c
					utm = campToUTM[campChan[c]]
				}
			}

			// 退款：refunded 全额退，completed 有小概率部分退。
			refund := 0.0
			switch status {
			case "refunded":
				refund = pay
				ratio := 1.0
				if total > 0 {
					ratio = pay / total
				}
				for _, ln := range lines {
					retID++
					ct := notFuture(created.AddDate(0, 0, r.Intn(20)+1), now)
					retRows = append(retRows, []interface{}{retID, id, ln.pid, ln.qty,
						returnReasons[r.Intn(len(returnReasons))], round2(ln.sub * ratio), "refunded", ct.Format(dtLayout)})
				}
			case "completed":
				if len(lines) > 0 && r.Intn(100) < 8 {
					ln := lines[r.Intn(len(lines))]
					refund = round2(ln.unit) // 退一件
					retID++
					ct := notFuture(created.AddDate(0, 0, r.Intn(20)+1), now)
					retRows = append(retRows, []interface{}{retID, id, ln.pid, 1,
						returnReasons[r.Intn(len(returnReasons))], refund, "approved", ct.Format(dtLayout)})
				}
			}

			var paidAt interface{}
			if status != "cancelled" {
				pt := notFuture(created.Add(time.Duration(r.Intn(120)+1)*time.Minute), now)
				paidAt = pt.Format(dtLayout)
			}
			orderStatus[id] = status
			orderCust[id] = cust
			orderCreated[id] = created
			orderChannel[id] = channel
			orderPay[id] = pay
			orderNo := fmt.Sprintf("SO%s%06d", created.Format("20060102"), id)
			oRows = append(oRows, []interface{}{id, orderNo, cust, status, channel, region,
				campaignID, couponID, utm, 0,
				total, discount, shipping, tax, refund, pay,
				payMethod[r.Intn(len(payMethod))], created.Format(dtLayout), paidAt})
		}

		// 首单标记：每个客户按下单时间最早的一笔置 is_first_order=1（该列在行内索引 9）。
		firstOrder := map[int]int{}
		for id := 1; id <= nOrders; id++ {
			c := orderCust[id]
			if f, ok := firstOrder[c]; !ok || orderCreated[id].Before(orderCreated[f]) {
				firstOrder[c] = id
			}
		}
		for _, oid := range firstOrder {
			oRows[oid-1][9] = 1
		}

		batchInsert(db, "orders", oCols, oRows)
		batchInsert(db, "order_items", iCols, iRows)
		batchInsert(db, "order_returns",
			[]string{"id", "order_id", "product_id", "quantity", "reason", "refund_amount", "status", "created_at"}, retRows)
		log.Printf("orders=%d order_items=%d order_returns=%d", len(oRows), len(iRows), len(retRows))
	}

	// ---------- product_reviews（仅 completed 订单的一部分商品被评价） ----------
	{
		cols := []string{"id", "product_id", "customer_id", "order_id", "rating", "content", "created_at"}
		rows := make([][]interface{}, 0, 3000)
		rid := 0
		for oid := 1; oid <= nOrders; oid++ {
			if orderStatus[oid] != "completed" {
				continue
			}
			if r.Intn(100) >= 60 {
				continue
			}
			for _, ln := range orderLines[oid] {
				if r.Intn(100) >= 70 {
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
				ct := notFuture(orderCreated[oid].AddDate(0, 0, r.Intn(15)+1), now)
				rows = append(rows, []interface{}{rid, ln.pid, orderCust[oid], oid, rating, content, ct.Format(dtLayout)})
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
				cities[r.Intn(len(cities))], rating, daysBack(r, now, 1200).Format(dtLayout)})
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
			created := daysBack(r, now, spanOrderDays)
			var recv interface{}
			if status == "received" {
				recv = notFuture(created.AddDate(0, 0, r.Intn(10)+1), now).Format(dtLayout)
			}
			poNo := fmt.Sprintf("PO%s%04d", created.Format("20060102"), id)
			rows = append(rows, []interface{}{id, poNo, r.Intn(nSuppliers) + 1, pid, qty, unit, total, status, created.Format(dtLayout), recv})
		}
		batchInsert(db, "purchase_orders", cols, rows)
		log.Printf("suppliers=%d purchase_orders=%d", nSuppliers, len(rows))
	}

	// ---------- inventory_snapshots（每个商品每月月初一张快照，覆盖近 24 个月） ----------
	{
		const invMonths = 24
		cols := []string{"id", "product_id", "snapshot_date", "stock_on_hand", "stock_reserved", "stock_inbound", "warehouse"}
		rows := make([][]interface{}, 0, nProducts*invMonths)
		sid := 0
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		for pid := 1; pid <= nProducts; pid++ {
			wh := warehouses[pid%len(warehouses)]
			onHand := 200 + r.Intn(1800)
			for m := invMonths - 1; m >= 0; m-- {
				snap := monthStart.AddDate(0, -m, 0)
				onHand += r.Intn(600) - 300
				if onHand < 0 {
					onHand = r.Intn(120)
				}
				reserved := r.Intn(onHand/4 + 1)
				inbound := r.Intn(400)
				sid++
				rows = append(rows, []interface{}{sid, pid, snap.Format(dateLayout), onHand, reserved, inbound, wh})
			}
		}
		batchInsert(db, "inventory_snapshots", cols, rows)
		log.Printf("inventory_snapshots=%d", len(rows))
	}

	// ---------- customer_coupons（领券记录，用券关联真实订单） ----------
	{
		cols := []string{"id", "customer_id", "coupon_id", "order_id", "status", "received_at", "used_at"}
		rows := make([][]interface{}, 0, nCustCoup)
		for id := 1; id <= nCustCoup; id++ {
			cust := r.Intn(nCustomers) + 1
			coup := r.Intn(nCoupons) + 1
			recv := daysBack(r, now, 500)
			roll := r.Intn(100)
			var status string
			var orderID, usedAt interface{}
			switch {
			case roll < 45:
				status = "used"
				if oid := findCustomerOrder(orderCust, cust, r); oid > 0 {
					orderID = oid
					usedAt = orderCreated[oid].Format(dtLayout)
				} else {
					usedAt = notFuture(recv.AddDate(0, 0, r.Intn(20)+1), now).Format(dtLayout)
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
		rows := make([][]interface{}, 0, 4000)
		sid := 0
		for oid := 1; oid <= nOrders; oid++ {
			st := orderStatus[oid]
			if st != "completed" && st != "shipped" {
				continue
			}
			sid++
			shipped := notFuture(orderCreated[oid].Add(time.Duration(r.Intn(48)+2)*time.Hour), now)
			var delivered interface{}
			shipStatus := "in_transit"
			if st == "completed" {
				dt := notFuture(shipped.AddDate(0, 0, r.Intn(5)+1), now)
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

	// ---------- user_events + shopping_carts + cart_items（点击流/漏斗/加购弃购） ----------
	{
		eCols := []string{"id", "session_id", "customer_id", "event_type", "product_id", "channel", "campaign_id", "occurred_at"}
		cCols := []string{"id", "customer_id", "status", "item_count", "total_amount", "converted_order_id", "created_at", "updated_at"}
		ciCols := []string{"id", "cart_id", "product_id", "quantity", "unit_price", "added_at"}
		eRows := make([][]interface{}, 0, nOrders*6)
		cRows := make([][]interface{}, 0, nOrders*2)
		ciRows := make([][]interface{}, 0, nOrders*4)
		evID, sessID, cartID, ciID := 0, 0, 0, 0

		emit := func(sid string, cust interface{}, typ string, pid interface{}, ch string, camp interface{}, at time.Time) {
			evID++
			eRows = append(eRows, []interface{}{evID, sid, cust, typ, pid, ch, camp, at.Format(dtLayout)})
		}

		// 转化会话：每笔非取消订单一条完整漏斗 view→add_cart→checkout→purchase，并生成 converted 购物车。
		for oid := 1; oid <= nOrders; oid++ {
			if orderStatus[oid] == "cancelled" {
				continue
			}
			lines := orderLines[oid]
			if len(lines) == 0 {
				continue
			}
			sessID++
			sid := fmt.Sprintf("SES%08d", sessID)
			ch := orderChannel[oid]
			var custV interface{} = orderCust[oid]
			var campV interface{}
			if orderCampaign[oid] > 0 {
				campV = orderCampaign[oid]
			}
			created := orderCreated[oid]
			start := created.Add(-time.Duration(5+r.Intn(55)) * time.Minute)

			// 浏览商品 = 明细商品 + 0~2 个随机商品；事件数 = 浏览 + 加购(每条明细) + checkout + purchase。
			viewPids := make([]int, 0, len(lines)+2)
			for _, ln := range lines {
				viewPids = append(viewPids, ln.pid)
			}
			for extra := r.Intn(3); extra > 0; extra-- {
				viewPids = append(viewPids, r.Intn(nProducts)+1)
			}
			nEv := len(viewPids) + len(lines) + 2
			ts := spread(start, created, nEv)
			k := 0
			for _, p := range viewPids {
				emit(sid, custV, "view", p, ch, campV, ts[k])
				k++
			}
			for _, ln := range lines {
				emit(sid, custV, "add_cart", ln.pid, ch, campV, ts[k])
				k++
			}
			emit(sid, custV, "checkout", nil, ch, campV, ts[k])
			k++
			emit(sid, custV, "purchase", lines[0].pid, ch, campV, ts[k])

			cartID++
			cRows = append(cRows, []interface{}{cartID, orderCust[oid], "converted", len(lines), orderPay[oid],
				oid, start.Format(dtLayout), created.Format(dtLayout)})
			for _, ln := range lines {
				ciID++
				ciRows = append(ciRows, []interface{}{ciID, cartID, ln.pid, ln.qty, ln.unit, start.Format(dtLayout)})
			}
		}

		// 未转化会话：跳出(view) / 弃购(add_cart) / 弃结算(checkout)，其中加购及以上生成 abandoned 购物车。
		nAband := nOrders / 2
		for a := 0; a < nAband; a++ {
			sessID++
			sid := fmt.Sprintf("SES%08d", sessID)
			ch := pickWeighted(r, channels, channelW)
			anon := r.Intn(100) < 18
			var custV interface{}
			custID := 0
			if !anon {
				custID = r.Intn(nCustomers) + 1
				custV = custID
			}
			when := pickDay(r, dayDates, dayCum, now)
			if custID > 0 && when.Before(custReg[custID]) {
				when = notFuture(custReg[custID].Add(time.Duration(r.Intn(240))*time.Hour), now)
			}
			var campV interface{}
			if r.Intn(100) < 30 {
				if c := pickCampaign(r, campStart, campEnd, when, nCampaigns); c > 0 {
					campV = c
				}
			}
			depth := pickWeightedInt(r, []int{1, 2, 3}, []int{45, 38, 17}) // 1 跳出 / 2 弃购 / 3 弃结算

			nView := 1 + r.Intn(3)
			cartLines := 0
			if depth >= 2 {
				cartLines = 1 + r.Intn(3)
			}
			nEv := nView + cartLines
			if depth >= 3 {
				nEv++
			}
			end := notFuture(when.Add(time.Duration(2+r.Intn(40))*time.Minute), now)
			ts := spread(when, end, nEv)
			k := 0
			var cartPids []int
			var cartQtys []int
			var total float64
			for v := 0; v < nView; v++ {
				p := r.Intn(nProducts) + 1
				emit(sid, custV, "view", p, ch, campV, ts[k])
				k++
				if v < cartLines {
					cartPids = append(cartPids, p)
				}
			}
			for _, p := range cartPids {
				emit(sid, custV, "add_cart", p, ch, campV, ts[k])
				k++
				q := 1 + r.Intn(3)
				cartQtys = append(cartQtys, q)
				total = round2(total + prodPrice[p]*float64(q))
			}
			if depth >= 3 {
				emit(sid, custV, "checkout", nil, ch, campV, ts[k])
			}
			if len(cartPids) > 0 {
				cartID++
				var custCol interface{}
				if custID > 0 {
					custCol = custID
				}
				cRows = append(cRows, []interface{}{cartID, custCol, "abandoned", len(cartPids), total,
					nil, when.Format(dtLayout), end.Format(dtLayout)})
				for i, p := range cartPids {
					ciID++
					ciRows = append(ciRows, []interface{}{ciID, cartID, p, cartQtys[i], prodPrice[p], ts[nView+i].Format(dtLayout)})
				}
			}
		}
		batchInsert(db, "user_events", eCols, eRows)
		batchInsert(db, "shopping_carts", cCols, cRows)
		batchInsert(db, "cart_items", ciCols, ciRows)
		log.Printf("user_events=%d shopping_carts=%d cart_items=%d", len(eRows), len(cRows), len(ciRows))
	}

	// ---------- points_ledger（会员积分流水：按客户时间序，1 元=1 分，含偶发兑换） ----------
	{
		type oref struct {
			t   time.Time
			pay float64
			oid int
		}
		byCust := make([][]oref, nCustomers+1)
		for id := 1; id <= nOrders; id++ {
			st := orderStatus[id]
			if st == "cancelled" || st == "refunded" {
				continue
			}
			c := orderCust[id]
			byCust[c] = append(byCust[c], oref{orderCreated[id], orderPay[id], id})
		}
		cols := []string{"id", "customer_id", "order_id", "change_type", "points", "balance_after", "created_at"}
		rows := make([][]interface{}, 0, nOrders)
		pid := 0
		for cust := 1; cust <= nCustomers; cust++ {
			list := byCust[cust]
			if len(list) == 0 {
				continue
			}
			sort.Slice(list, func(i, j int) bool { return list[i].t.Before(list[j].t) })
			bal := 0
			for _, o := range list {
				earn := int(o.pay)
				if earn <= 0 {
					continue
				}
				bal += earn
				pid++
				rows = append(rows, []interface{}{pid, cust, o.oid, "earn", earn, bal, o.t.Format(dtLayout)})
				// 余额充足时偶发兑换（每满千分抵扣，兑换记为负分）。
				if bal >= 1000 && r.Intn(100) < 22 {
					redeem := (bal / 1000) * 500
					if redeem > bal {
						redeem = bal
					}
					bal -= redeem
					pid++
					rt := notFuture(o.t.Add(time.Duration(r.Intn(72)+1)*time.Hour), now)
					rows = append(rows, []interface{}{pid, cust, nil, "redeem", -redeem, bal, rt.Format(dtLayout)})
				}
			}
		}
		batchInsert(db, "points_ledger", cols, rows)
		log.Printf("points_ledger=%d", len(rows))
	}

	// ---------- product_price_history（约 55% 商品有 1~4 次调价，末次价=当前售价） ----------
	{
		cols := []string{"id", "product_id", "old_price", "new_price", "reason", "changed_at"}
		rows := make([][]interface{}, 0, nProducts*2)
		hid := 0
		for pid := 1; pid <= nProducts; pid++ {
			if r.Intn(100) >= 55 {
				continue
			}
			k := 1 + r.Intn(4)
			// 反推价格链：末价=当前售价，往前每次按 0.8~1.15 倍缩放。
			prices := make([]float64, k+1)
			prices[k] = prodPrice[pid]
			for j := k - 1; j >= 0; j-- {
				prices[j] = round2(prices[j+1] * (0.8 + r.Float64()*0.35))
			}
			// 调价时间：launch 与 now 之间取 k 个升序时刻。
			maxDays := int(now.Sub(prodLaunch[pid]).Hours()/24) - 1
			if maxDays < k+1 {
				maxDays = k + 1
			}
			times := make([]time.Time, k)
			for j := 0; j < k; j++ {
				times[j] = notFuture(prodLaunch[pid].AddDate(0, 0, r.Intn(maxDays)+1), now)
			}
			sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
			for j := 1; j <= k; j++ {
				hid++
				rows = append(rows, []interface{}{hid, pid, prices[j-1], prices[j],
					priceReasons[r.Intn(len(priceReasons))], times[j-1].Format(dtLayout)})
			}
		}
		batchInsert(db, "product_price_history", cols, rows)
		log.Printf("product_price_history=%d", len(rows))
	}

	// ========== 复杂分析场景（客户分群 / A+B 实验 / 售后客服 / 促销玩法） ==========

	// 客户订单聚合（按时间升序），供 RFM 快照与会员等级轨迹复用。
	type custOrder struct {
		t   time.Time
		pay float64
	}
	custOrders := make([][]custOrder, nCustomers+1)
	for oid := 1; oid <= nOrders; oid++ {
		if st := orderStatus[oid]; st == "cancelled" || st == "refunded" {
			continue
		}
		c := orderCust[oid]
		custOrders[c] = append(custOrders[c], custOrder{orderCreated[oid], orderPay[oid]})
	}
	for c := 1; c <= nCustomers; c++ {
		sort.Slice(custOrders[c], func(i, j int) bool { return custOrders[c][i].t.Before(custOrders[c][j].t) })
	}

	// ---------- customer_rfm_snapshots（近 12 个月末对活跃客户做 RFM 打标分群） ----------
	{
		cols := []string{"id", "customer_id", "snapshot_month", "recency_days", "frequency", "monetary",
			"r_score", "f_score", "m_score", "segment", "created_at"}
		rows := make([][]interface{}, 0, nCustomers*rfmMonths/2)
		sid := 0
		firstOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		for m := rfmMonths - 1; m >= 0; m-- {
			monStart := firstOfMonth.AddDate(0, -m, 0)
			cutoff := monStart.AddDate(0, 1, 0).Add(-time.Second) // 该月最后一秒
			if cutoff.After(now) {
				cutoff = now
			}
			for c := 1; c <= nCustomers; c++ {
				list := custOrders[c]
				if len(list) == 0 || list[0].t.After(cutoff) {
					continue
				}
				freq := 0
				var mon float64
				var last time.Time
				for _, o := range list {
					if o.t.After(cutoff) {
						break
					}
					freq++
					mon = round2(mon + o.pay)
					last = o.t
				}
				if freq == 0 {
					continue
				}
				recency := int(cutoff.Sub(last).Hours() / 24)
				// 阈值按真实数据规模标定，使 R/F/M 得分在客户间充分区分（累计口径）。
				rs := scoreBand(float64(recency), []float64{7, 30, 90, 180}, true) // 越近分越高
				fs := scoreBand(float64(freq), []float64{1, 2, 3, 6}, false)       // 越多分越高
				ms := scoreBand(mon, []float64{40000, 90000, 140000, 200000}, false)
				sid++
				rows = append(rows, []interface{}{sid, c, monStart.Format(dateLayout), recency, freq, mon,
					rs, fs, ms, rfmSegment(rs, fs, ms), cutoff.Format(dtLayout)})
			}
		}
		batchInsert(db, "customer_rfm_snapshots", cols, rows)
		log.Printf("customer_rfm_snapshots=%d", len(rows))
	}

	// ---------- membership_tier_history（消费驱动的会员等级升级轨迹） ----------
	{
		cols := []string{"id", "customer_id", "from_tier", "to_tier", "reason", "cumulative_spend", "changed_at"}
		rows := make([][]interface{}, 0, nCustomers)
		hid := 0
		for c := 1; c <= nCustomers; c++ {
			list := custOrders[c]
			if len(list) == 0 {
				continue
			}
			level := 0
			var cum float64
			for _, o := range list {
				cum = round2(cum + o.pay)
				for level < 3 && cum >= tierUpTh[level+1] {
					level++
					reason := "消费升级"
					if r.Intn(100) < 15 {
						reason = "年度评级"
					}
					hid++
					rows = append(rows, []interface{}{hid, c, tierNames[level-1], tierNames[level], reason, cum, o.t.Format(dtLayout)})
				}
			}
		}
		batchInsert(db, "membership_tier_history", cols, rows)
		log.Printf("membership_tier_history=%d", len(rows))
	}

	// ---------- ab_experiments + ab_assignments（实验分流与转化，variant 带提升效应） ----------
	expStart := make([]time.Time, nExperiments+1)
	expEnd := make([]time.Time, nExperiments+1)
	{
		cols := []string{"id", "exp_key", "name", "hypothesis", "primary_metric", "status", "start_date", "end_date", "created_at"}
		rows := make([][]interface{}, 0, nExperiments)
		for id := 1; id <= nExperiments; id++ {
			start := daysBack(r, now, spanOrderDays)
			end := start.AddDate(0, 0, 14+r.Intn(46))
			status := "completed"
			if end.After(now) {
				status = "running"
			}
			expStart[id] = start
			expEnd[id] = end
			name := abNames[r.Intn(len(abNames))] + "实验"
			metric := abMetrics[r.Intn(len(abMetrics))]
			hyp := fmt.Sprintf("优化「%s」可提升 %s", name, metric)
			rows = append(rows, []interface{}{id, fmt.Sprintf("EXP-%04d", id), name, hyp, metric, status,
				start.Format(dateLayout), end.Format(dateLayout), start.Format(dtLayout)})
		}
		batchInsert(db, "ab_experiments", cols, rows)
	}
	{
		cols := []string{"id", "experiment_id", "customer_id", "variant", "assigned_at", "converted", "conversion_value"}
		rows := make([][]interface{}, 0, nExperiments*1200)
		aid := 0
		for e := 1; e <= nExperiments; e++ {
			participants := 400 + r.Intn(1600)
			nv := 2
			if r.Intn(2) == 0 {
				nv = 3
			}
			winHours := int(expEnd[e].Sub(expStart[e]).Hours())
			if winHours < 1 {
				winHours = 1
			}
			seen := map[int]bool{}
			for k := 0; k < participants; k++ {
				c := r.Intn(nCustomers) + 1
				if seen[c] {
					continue
				}
				seen[c] = true
				vIdx := r.Intn(nv)
				assigned := notFuture(expStart[e].Add(time.Duration(r.Intn(winHours))*time.Hour), now)
				cvr := 0.08 + 0.06*float64(vIdx) // control 8% / a 14% / b 20%
				converted := 0
				var cval interface{}
				if r.Float64() < cvr {
					converted = 1
					cval = round2(50 + r.Float64()*450)
				}
				aid++
				rows = append(rows, []interface{}{aid, e, c, abVariants[vIdx], assigned.Format(dtLayout), converted, cval})
			}
		}
		batchInsert(db, "ab_assignments", cols, rows)
		log.Printf("ab_experiments=%d ab_assignments=%d", nExperiments, len(rows))
	}

	// ---------- recommendation_events（推荐位曝光→点击→加购→购买事件流） ----------
	{
		cols := []string{"id", "session_id", "customer_id", "product_id", "scene", "rank", "action", "occurred_at"}
		rows := make([][]interface{}, 0, 110000)
		eid, sess := 0, 0
		emit := func(sid string, cust interface{}, pid int, scene string, rank int, action string, at time.Time) {
			eid++
			rows = append(rows, []interface{}{eid, sid, cust, pid, scene, rank, action, at.Format(dtLayout)})
		}
		const nShows = 90000
		for s := 0; s < nShows; s++ {
			sess++
			sid := fmt.Sprintf("RS%08d", sess)
			scene := recScenes[r.Intn(len(recScenes))]
			pid := r.Intn(nProducts) + 1
			rank := r.Intn(20) + 1
			custID := 0
			var custV interface{}
			if r.Intn(100) >= 25 {
				custID = r.Intn(nCustomers) + 1
				custV = custID
			}
			when := pickDay(r, dayDates, dayCum, now)
			if custID > 0 && when.Before(custReg[custID]) {
				when = notFuture(custReg[custID].Add(time.Duration(r.Intn(240))*time.Hour), now)
			}
			emit(sid, custV, pid, scene, rank, "impression", when)
			ctr := 0.18 / (1 + float64(rank)/5.0) // 排名越靠后点击率越低
			if r.Float64() < ctr {
				when = notFuture(when.Add(time.Duration(r.Intn(120)+5)*time.Second), now)
				emit(sid, custV, pid, scene, rank, "click", when)
				if r.Float64() < 0.35 {
					when = notFuture(when.Add(time.Duration(r.Intn(300)+10)*time.Second), now)
					emit(sid, custV, pid, scene, rank, "add_cart", when)
					if r.Float64() < 0.4 {
						when = notFuture(when.Add(time.Duration(r.Intn(600)+30)*time.Second), now)
						emit(sid, custV, pid, scene, rank, "purchase", when)
					}
				}
			}
		}
		batchInsert(db, "recommendation_events", cols, rows)
		log.Printf("recommendation_events=%d", len(rows))
	}

	// ---------- support_tickets（客服工单：SLA 时效 + CSAT 满意度） ----------
	{
		cols := []string{"id", "ticket_no", "customer_id", "order_id", "category", "channel", "priority", "status",
			"first_response_minutes", "resolution_minutes", "csat_score", "agent_id", "created_at", "resolved_at"}
		rows := make([][]interface{}, 0, nTickets)
		frBase := map[string][2]int{"urgent": {5, 30}, "high": {15, 90}, "medium": {30, 240}, "low": {60, 480}}
		for id := 1; id <= nTickets; id++ {
			cust := r.Intn(nCustomers) + 1
			cat := ticketCats[r.Intn(len(ticketCats))]
			var orderCol interface{}
			if cat == "退款咨询" || cat == "物流问题" || cat == "发票问题" {
				if oid := findCustomerOrder(orderCust, cust, r); oid > 0 {
					orderCol = oid
				}
			}
			prio := pickWeighted(r, ticketPrio, ticketPrioW)
			created := daysBack(r, now, spanOrderDays)
			if created.Before(custReg[cust]) {
				created = notFuture(custReg[cust].Add(time.Duration(r.Intn(240))*time.Hour), now)
			}
			fb := frBase[prio]
			frMin := fb[0] + r.Intn(fb[1]-fb[0]+1)
			status := pickWeighted(r, []string{"resolved", "closed", "pending"}, []int{56, 32, 12})
			agent := r.Intn(nAgents) + 1
			var resMinCol, resolvedCol, csatCol interface{}
			if status == "pending" {
				// 未解决：无解决时长/CSAT
			} else {
				resMin := frMin + r.Intn(2880) + 30 // 首响后再到解决，最多 ~2 天
				resMinCol = resMin
				resolvedAt := notFuture(created.Add(time.Duration(resMin)*time.Minute), now)
				resolvedCol = resolvedAt.Format(dtLayout)
				if r.Intn(100) < 68 { // 约 68% 工单有 CSAT 评分
					csatCol = pickWeightedInt(r, []int{5, 4, 3, 2, 1}, []int{48, 27, 13, 7, 5})
				}
			}
			rows = append(rows, []interface{}{id, fmt.Sprintf("TK%s%05d", created.Format("20060102"), id), cust, orderCol,
				cat, ticketChans[r.Intn(len(ticketChans))], prio, status, frMin, resMinCol, csatCol, agent,
				created.Format(dtLayout), resolvedCol})
		}
		batchInsert(db, "support_tickets", cols, rows)
		log.Printf("support_tickets=%d", len(rows))
	}

	// ---------- nps_surveys（NPS 调研，分推荐者/中立/贬损） ----------
	{
		cols := []string{"id", "customer_id", "score", "category", "comment", "surveyed_at"}
		rows := make([][]interface{}, 0, nNPS)
		for id := 1; id <= nNPS; id++ {
			cust := r.Intn(nCustomers) + 1
			score := pickWeightedInt(r, []int{10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
				[]int{22, 20, 15, 10, 8, 6, 5, 5, 4, 3, 2})
			var category string
			switch {
			case score >= 9:
				category = "promoter"
			case score >= 7:
				category = "passive"
			default:
				category = "detractor"
			}
			surveyed := daysBack(r, now, 540)
			if surveyed.Before(custReg[cust]) {
				surveyed = notFuture(custReg[cust].Add(time.Duration(r.Intn(240))*time.Hour), now)
			}
			rows = append(rows, []interface{}{id, cust, score, category, npsComments[r.Intn(len(npsComments))], surveyed.Format(dtLayout)})
		}
		batchInsert(db, "nps_surveys", cols, rows)
		log.Printf("nps_surveys=%d", len(rows))
	}

	// ---------- flash_sales（秒杀场次，成交价远低于原价，售罄率不一） ----------
	flashProduct := make([]int, nFlashSales+1)
	flashStart := make([]time.Time, nFlashSales+1)
	{
		cols := []string{"id", "name", "product_id", "original_price", "flash_price", "discount_rate",
			"stock_limit", "sold_count", "start_at", "end_at", "status", "created_at"}
		rows := make([][]interface{}, 0, nFlashSales)
		durOpts := []int{2, 4, 6, 12, 24}
		for id := 1; id <= nFlashSales; id++ {
			pid := r.Intn(nProducts) + 1
			orig := prodPrice[pid]
			rate := 0.5 + r.Float64()*0.35 // 成交价 = 原价的 50%~85%
			flash := round2(orig * rate)
			stockLimit := (r.Intn(20) + 1) * 50
			start := daysBack(r, now, spanOrderDays)
			end := start.Add(time.Duration(durOpts[r.Intn(len(durOpts))]) * time.Hour)
			status := "ended"
			sold := r.Intn(stockLimit + 1)
			if end.After(now) {
				status = "active"
				sold = r.Intn(stockLimit/2 + 1)
			}
			flashProduct[id] = pid
			flashStart[id] = start
			name := fmt.Sprintf("%d点场·限时秒杀%03d", start.Hour(), id)
			rows = append(rows, []interface{}{id, name, pid, orig, flash, round2(rate), stockLimit, sold,
				start.Format(dtLayout), end.Format(dtLayout), status, start.Format(dtLayout)})
		}
		batchInsert(db, "flash_sales", cols, rows)
		log.Printf("flash_sales=%d", len(rows))
	}

	// ---------- group_buys（拼团实例，成团/进行中/失败，含成员数与到期时间） ----------
	{
		cols := []string{"id", "group_no", "product_id", "initiator_customer_id", "group_size", "current_members",
			"original_price", "group_price", "status", "created_at", "expires_at", "completed_at"}
		rows := make([][]interface{}, 0, nGroupBuys)
		sizeOpts := []int{2, 3, 5}
		for id := 1; id <= nGroupBuys; id++ {
			pid := r.Intn(nProducts) + 1
			orig := prodPrice[pid]
			gsize := sizeOpts[r.Intn(len(sizeOpts))]
			gprice := round2(orig * (0.6 + r.Float64()*0.25))
			initiator := r.Intn(nCustomers) + 1
			created := pickDay(r, dayDates, dayCum, now)
			if created.Before(custReg[initiator]) {
				created = notFuture(custReg[initiator].Add(time.Duration(r.Intn(240))*time.Hour), now)
			}
			expires := created.Add(time.Duration(24+r.Intn(48)) * time.Hour) // 到期(可能在未来，进行中团合理)
			var status string
			var members int
			var completedCol interface{}
			switch {
			case expires.After(now) && r.Intn(100) < 45:
				status = "pending"
				members = 1 + r.Intn(gsize-1) // 未满
			case r.Intn(100) < 60:
				status = "success"
				members = gsize
				completedCol = notFuture(created.Add(time.Duration(r.Intn(24)+1)*time.Hour), now).Format(dtLayout)
			default:
				status = "failed"
				members = 1 + r.Intn(gsize-1)
			}
			rows = append(rows, []interface{}{id, fmt.Sprintf("GB%s%05d", created.Format("20060102"), id), pid, initiator,
				gsize, members, orig, gprice, status, created.Format(dtLayout), expires.Format(dtLayout), completedCol})
		}
		batchInsert(db, "group_buys", cols, rows)
		log.Printf("group_buys=%d", len(rows))
	}

	// ---------- promotion_products（促销↔商品多对多：营销活动/优惠券/秒杀 参与商品） ----------
	{
		cols := []string{"id", "promotion_type", "promotion_id", "product_id", "special_price", "created_at"}
		rows := make([][]interface{}, 0, nCampaigns*6+nCoupons*4+nFlashSales)
		pid := 0
		for c := 1; c <= nCampaigns; c++ {
			for j, k := 0, 3+r.Intn(6); j < k; j++ {
				pid++
				rows = append(rows, []interface{}{pid, "campaign", c, r.Intn(nProducts) + 1, nil, campStart[c].Format(dtLayout)})
			}
		}
		for c := 1; c <= nCoupons; c++ {
			for j, k := 0, 2+r.Intn(4); j < k; j++ {
				pid++
				rows = append(rows, []interface{}{pid, "coupon", c, r.Intn(nProducts) + 1, nil, coupFrom[c].Format(dtLayout)})
			}
		}
		for f := 1; f <= nFlashSales; f++ {
			prod := flashProduct[f]
			pid++
			rows = append(rows, []interface{}{pid, "flash_sale", f, prod, round2(prodPrice[prod] * (0.6 + r.Float64()*0.25)),
				flashStart[f].Format(dtLayout)})
		}
		batchInsert(db, "promotion_products", cols, rows)
		log.Printf("promotion_products=%d", len(rows))
	}

	must(db, "SET FOREIGN_KEY_CHECKS=1")
	fmt.Println("OK: ecommerce_demo 重建完成（30 张表全量重灌，时间跨度约 3 年，含点击流/购物车/积分/RFM分群/AB实验/客服/促销）")
}

// scoreBand 把数值按升序阈值映射到 1~5 分；reverse=true 表示数值越小分越高（如 recency）。
func scoreBand(v float64, th []float64, reverse bool) int {
	score := 1
	for _, t := range th {
		if v > t {
			score++
		}
	}
	if reverse {
		return 6 - score
	}
	return score
}

// rfmSegment 由 R/F/M 三个 1~5 分映射到客户分群标签。
func rfmSegment(rs, fs, ms int) string {
	rHigh, fHigh, mHigh := rs >= 4, fs >= 4, ms >= 4
	var idx int
	switch {
	case rHigh && fHigh && mHigh:
		idx = 0 // 重要价值
	case rHigh && !fHigh && mHigh:
		idx = 1 // 重要发展
	case !rHigh && fHigh && mHigh:
		idx = 2 // 重要保持
	case !rHigh && !fHigh && mHigh:
		idx = 3 // 重要挽留
	case rHigh && fHigh && !mHigh:
		idx = 4 // 一般价值
	case rHigh && !fHigh && !mHigh:
		idx = 5 // 一般发展
	case !rHigh && fHigh && !mHigh:
		idx = 6 // 一般保持
	case rs <= 2 && !fHigh && !mHigh:
		idx = 8 // 流失预警
	default:
		idx = 7 // 一般挽留
	}
	return rfmSegments[idx]
}

// pickCampaign 随机找一个在 date 当日在投的营销活动 id（找不到返回 0）。
func pickCampaign(r *rand.Rand, starts, ends []time.Time, date time.Time, n int) int {
	for t := 0; t < 6; t++ {
		c := r.Intn(n) + 1
		if !date.Before(starts[c]) && !date.After(ends[c]) {
			return c
		}
	}
	return 0
}

// pickCoupon 随机找一张 date 当日有效、门槛不超过 total 的券模板 id（找不到返回 0）。
func pickCoupon(r *rand.Rand, mins []float64, froms, tos []time.Time, total float64, date time.Time, n int) int {
	for t := 0; t < 6; t++ {
		c := r.Intn(n) + 1
		if total >= mins[c] && !date.Before(froms[c]) && !date.After(tos[c]) {
			return c
		}
	}
	return 0
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

// createSchema DROP 并重建全部 20 张表（自持完整增强 schema，风格统一 utf8mb4_unicode_ci + 注释 + 索引）。
func createSchema(db *sql.DB) {
	tables := []string{
		"promotion_products", "group_buys", "flash_sales", "nps_surveys", "support_tickets",
		"recommendation_events", "ab_assignments", "ab_experiments", "membership_tier_history",
		"customer_rfm_snapshots",
		"product_price_history", "points_ledger", "cart_items", "shopping_carts", "user_events",
		"ad_spend_daily", "marketing_campaigns", "inventory_snapshots", "order_returns",
		"shipments", "customer_coupons", "coupons", "purchase_orders", "suppliers",
		"product_reviews", "order_items", "orders", "products", "customers", "categories",
	}
	for _, t := range tables {
		must(db, "DROP TABLE IF EXISTS `"+t+"`")
	}
	stmts := []string{
		`CREATE TABLE categories (
			id INT NOT NULL AUTO_INCREMENT,
			name VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '分类名称',
			parent_id INT DEFAULT NULL COMMENT '父分类ID(顶级为NULL)',
			level TINYINT NOT NULL DEFAULT 1 COMMENT '层级 1顶级/2二级',
			sort_order INT NOT NULL DEFAULT 0 COMMENT '同级排序',
			PRIMARY KEY (id),
			KEY idx_parent (parent_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品分类表'`,
		`CREATE TABLE customers (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户表'`,
		`CREATE TABLE products (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品表'`,
		`CREATE TABLE orders (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单表'`,
		`CREATE TABLE order_items (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单明细表'`,
		`CREATE TABLE product_reviews (
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
		`CREATE TABLE suppliers (
			id INT NOT NULL AUTO_INCREMENT,
			name VARCHAR(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '供应商名称',
			contact VARCHAR(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系人',
			phone VARCHAR(20) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '联系电话',
			city VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '所在城市',
			rating DECIMAL(2,1) NOT NULL DEFAULT '0.0' COMMENT '合作评级 0-5',
			created_at DATETIME NOT NULL COMMENT '合作起始时间',
			PRIMARY KEY (id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='供应商表'`,
		`CREATE TABLE purchase_orders (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='采购单表'`,
		`CREATE TABLE coupons (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='优惠券模板表'`,
		`CREATE TABLE customer_coupons (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户领券记录表'`,
		`CREATE TABLE shipments (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='发货物流表'`,
		`CREATE TABLE order_returns (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='订单退货表'`,
		`CREATE TABLE inventory_snapshots (
			id INT NOT NULL AUTO_INCREMENT,
			product_id INT NOT NULL COMMENT '商品ID',
			snapshot_date DATE NOT NULL COMMENT '快照日期(月初)',
			stock_on_hand INT NOT NULL COMMENT '在库库存',
			stock_reserved INT NOT NULL COMMENT '锁定库存',
			stock_inbound INT NOT NULL COMMENT '在途入库',
			warehouse VARCHAR(32) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '仓库',
			PRIMARY KEY (id),
			KEY idx_product_date (product_id, snapshot_date)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='库存月度快照表'`,
		`CREATE TABLE marketing_campaigns (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='营销活动表'`,
		`CREATE TABLE ad_spend_daily (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='广告日花费表'`,
		`CREATE TABLE user_events (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户行为事件表(点击流/漏斗)'`,
		`CREATE TABLE shopping_carts (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='购物车表'`,
		`CREATE TABLE cart_items (
			id INT NOT NULL AUTO_INCREMENT,
			cart_id INT NOT NULL COMMENT '购物车ID',
			product_id INT NOT NULL COMMENT '商品ID',
			quantity INT NOT NULL COMMENT '数量',
			unit_price DECIMAL(10,2) NOT NULL COMMENT '加购时单价',
			added_at DATETIME NOT NULL COMMENT '加购时间',
			PRIMARY KEY (id),
			KEY idx_cart (cart_id),
			KEY idx_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='购物车明细表'`,
		`CREATE TABLE points_ledger (
			id BIGINT NOT NULL AUTO_INCREMENT,
			customer_id INT NOT NULL COMMENT '客户ID',
			order_id INT DEFAULT NULL COMMENT '关联订单ID(兑换/过期为NULL)',
			change_type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型 earn获取/redeem兑换/expire过期',
			points INT NOT NULL COMMENT '积分变动(正为增,负为减)',
			balance_after INT NOT NULL COMMENT '变动后余额',
			created_at DATETIME NOT NULL COMMENT '发生时间',
			PRIMARY KEY (id),
			KEY idx_customer_time (customer_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会员积分流水表'`,
		`CREATE TABLE product_price_history (
			id INT NOT NULL AUTO_INCREMENT,
			product_id INT NOT NULL COMMENT '商品ID',
			old_price DECIMAL(10,2) NOT NULL COMMENT '调整前售价',
			new_price DECIMAL(10,2) NOT NULL COMMENT '调整后售价',
			reason VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '调价原因',
			changed_at DATETIME NOT NULL COMMENT '调价时间',
			PRIMARY KEY (id),
			KEY idx_product_time (product_id, changed_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='商品价格变更历史表'`,
		`CREATE TABLE customer_rfm_snapshots (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客户RFM分群月度快照表'`,
		`CREATE TABLE membership_tier_history (
			id INT NOT NULL AUTO_INCREMENT,
			customer_id INT NOT NULL COMMENT '客户ID',
			from_tier VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '原等级',
			to_tier VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '新等级',
			reason VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '变更原因',
			cumulative_spend DECIMAL(12,2) NOT NULL COMMENT '变更时累计消费',
			changed_at DATETIME NOT NULL COMMENT '变更时间',
			PRIMARY KEY (id),
			KEY idx_customer_time (customer_id, changed_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会员等级升级轨迹表'`,
		`CREATE TABLE ab_experiments (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B实验表'`,
		`CREATE TABLE ab_assignments (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='A/B实验分流与转化表'`,
		`CREATE TABLE recommendation_events (
			id BIGINT NOT NULL AUTO_INCREMENT,
			session_id VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '会话ID',
			customer_id INT DEFAULT NULL COMMENT '客户ID(匿名为NULL)',
			product_id INT NOT NULL COMMENT '推荐商品ID',
			scene VARCHAR(24) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '推荐场景',
			` + "`rank`" + ` TINYINT NOT NULL COMMENT '推荐位次',
			action VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '行为 impression/click/add_cart/purchase',
			occurred_at DATETIME NOT NULL COMMENT '发生时间',
			PRIMARY KEY (id),
			KEY idx_scene_action (scene, action),
			KEY idx_session (session_id),
			KEY idx_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='推荐位曝光点击事件流表'`,
		`CREATE TABLE support_tickets (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='客服工单表'`,
		`CREATE TABLE nps_surveys (
			id INT NOT NULL AUTO_INCREMENT,
			customer_id INT NOT NULL COMMENT '客户ID',
			score TINYINT NOT NULL COMMENT 'NPS评分 0-10',
			category VARCHAR(12) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类别 promoter/passive/detractor',
			comment VARCHAR(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT '开放反馈',
			surveyed_at DATETIME NOT NULL COMMENT '调研时间',
			PRIMARY KEY (id),
			KEY idx_customer (customer_id),
			KEY idx_cat_time (category, surveyed_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='NPS调研表'`,
		`CREATE TABLE flash_sales (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限时秒杀场次表'`,
		`CREATE TABLE group_buys (
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
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='拼团实例表'`,
		`CREATE TABLE promotion_products (
			id INT NOT NULL AUTO_INCREMENT,
			promotion_type VARCHAR(16) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '促销类型 campaign/coupon/flash_sale',
			promotion_id INT NOT NULL COMMENT '促销活动ID(对应类型主键)',
			product_id INT NOT NULL COMMENT '参与商品ID',
			special_price DECIMAL(10,2) DEFAULT NULL COMMENT '专享价(无则NULL)',
			created_at DATETIME NOT NULL COMMENT '创建时间',
			PRIMARY KEY (id),
			KEY idx_type_promo (promotion_type, promotion_id),
			KEY idx_product (product_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='促销活动参与商品表(多对多)'`,
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
