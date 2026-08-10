// 工具：将转换好的评测数据集（HF 基准 + 自造场景）一键灌入 link 元数据库，
// 供「Agent 测评」创建任务时直接选用。
//
// 用法：cd cognida-go && set -a && source .env && set +a && go run ./cmd/seed/eval-datasets
//        # 加 --strict 时，若电商金标准与 ecommerce_demo 现状失配则非零退出
//        cd cognida-go && set -a && source .env && set +a && go run ./cmd/seed/eval-datasets --strict
//
// 数据来源：data/manifest.json + data/<dataset_id>.jsonl（由
// cognida-python/scripts/convert_eval_datasets.py 产出，经 //go:embed 打包进二进制，
// 故与运行 cwd 无关）。每行一条样本，字段与 QAPair 对齐：
//   question / reference_answer / relevant_pids? / expected_tools? / expected_steps?
//
// 幂等：按 dataset_id 硬删除既有数据集行与样本记录后整集重灌，反复执行不产生重复。
// 落库对象：租户 tenant_id=1（dev 用户），evaluation_type 取 manifest 标注（agent/qa）。
//
// 金标准一致性（A6）：scenario_ecommerce_agent 的 golden 是生成期从 ecommerce_demo 现算并
// 冻结进 JSONL 的。若电商库被 `cmd/seed/ecommerce`（DROP+CREATE 随机重建），冻结 golden 会与
// 新库失配、导致「Agent 答对也判错」。本工具 seed 前会对若干整库计数题现场查 ecommerce_demo
// 对拍嵌入 golden：不一致则 loud WARN；带 --strict 时直接非零退出。连不上电商库则降级跳过校验
// 并提示。重建电商库后，务必重跑：
//   cd cognida-python && .venv/bin/python scripts/convert_eval_datasets.py --only scenario_ecommerce_agent
// 重新生成 golden，再执行本 seed 工具。
//
// 前置：link 库已 migrate-db（evaluation_datasets / evaluation_dataset_records 已建表）。
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/model/evaluation"
	mysqlrepo "cognida/internal/repository/mysql"
)

//go:embed data/manifest.json data/*.jsonl
var dataFS embed.FS

// seed 目标：dev 用户 tenant_id=1 / user_id=1（与既有 seed 工具一致）。
const (
	seedTenant = int64(1)
	seedUser   = int64(1)
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// manifestEntry 对齐 convert_eval_datasets.py 产出的 manifest.json 条目。
type manifestEntry struct {
	DatasetID          string `json:"dataset_id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	EvaluationType     string `json:"evaluation_type"`
	SupportsTrajectory bool   `json:"supports_trajectory"`
	RecordsFile        string `json:"records_file"`
	Count              int    `json:"count"`
}

// recordLine 对齐每行 JSONL 样本（QAPair 子集）。
type recordLine struct {
	Question        string   `json:"question"`
	ReferenceAnswer string   `json:"reference_answer"`
	RelevantPIDs    []string `json:"relevant_pids,omitempty"`
	ExpectedTools   []string `json:"expected_tools,omitempty"`
	ExpectedSteps   []string `json:"expected_steps,omitempty"`
}

func main() {
	strict := flag.Bool("strict", false, "电商金标准与 ecommerce_demo 现状失配时非零退出（否则仅告警）")
	flag.Parse()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "cognida"),
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect failed: %v", err)
	}

	entries, err := loadManifest()
	if err != nil {
		log.Fatalf("load manifest failed: %v", err)
	}

	ctx := context.Background()
	total := 0
	for _, e := range entries {
		records, err := loadRecords(e.RecordsFile)
		if err != nil {
			log.Fatalf("load records %q failed: %v", e.RecordsFile, err)
		}
		// A6：灌库前校验电商金标准是否与 ecommerce_demo 现状一致（防 reseed 后静默失配）。
		if e.DatasetID == "scenario_ecommerce_agent" {
			if mismatch := verifyEcommerceGolden(records); mismatch && *strict {
				log.Fatalf("电商金标准与 ecommerce_demo 现状失配（--strict）；请重跑 "+
					"convert_eval_datasets.py --only %s 重新生成 golden 再 seed", e.DatasetID)
			}
		}
		if err := seedDataset(ctx, db, e, records); err != nil {
			log.Fatalf("seed dataset %q failed: %v", e.DatasetID, err)
		}
		total += len(records)
		fmt.Printf("OK: seeded %-28s %-6s %3d 条样本\n", e.DatasetID, e.EvaluationType, len(records))
	}
	fmt.Printf("DONE: %d 个数据集，共 %d 条样本 → 租户 %d\n", len(entries), total, seedTenant)
}

// ecommerceCheck 描述一条「整库计数题」对拍规则：用 question 子串定位嵌入样本，
// 从其 reference_answer 抽首个整数当冻结 golden，与 SQL 现算 COUNT 比对。
type ecommerceCheck struct {
	label       string // 人类可读标签（日志用）
	questionSub string // 定位样本的 question 子串
	sql         string // 现算 COUNT 的整库计数 SQL
}

// 仅选整库计数题（结果为确定性标量，不含随机名称/金额），便于稳定对拍。
var ecommerceChecks = []ecommerceCheck{
	{label: "订单总数", questionSub: "系统里目前一共有多少笔订单", sql: "SELECT COUNT(*) FROM orders"},
	{label: "已取消订单数", questionSub: "已取消（cancelled）", sql: "SELECT COUNT(*) FROM orders WHERE status='cancelled'"},
}

var intTokenRE = regexp.MustCompile(`[0-9][0-9,]*`)

// firstIntToken 抽取字符串中首个整数（容忍千分位逗号）。用于从 golden 答案文本取计数值。
func firstIntToken(s string) (int64, bool) {
	m := intTokenRE.FindString(s)
	if m == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.ReplaceAll(m, ",", ""), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// verifyEcommerceGolden 现场查 ecommerce_demo，对若干整库计数题比对嵌入 golden。
// 返回 true 表示检测到失配。连不上库时降级跳过（返回 false 并提示），不阻断 seed。
func verifyEcommerceGolden(records []recordLine) (mismatch bool) {
	edb, err := openEcommerceDB()
	if err != nil {
		fmt.Printf("  [SKIP] 电商金标准一致性校验：连不上 ecommerce_demo（%v）\n", err)
		return false
	}
	sqlDB, err := edb.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	for _, c := range ecommerceChecks {
		golden, ok := findGoldenCount(records, c.questionSub)
		if !ok {
			fmt.Printf("  [WARN] 一致性校验：未在种子集中找到「%s」题（子串 %q），跳过\n", c.label, c.questionSub)
			continue
		}
		var live int64
		if err := edb.Raw(c.sql).Scan(&live).Error; err != nil {
			fmt.Printf("  [SKIP] 一致性校验「%s」：查询失败（%v）\n", c.label, err)
			continue
		}
		if live != golden {
			fmt.Printf("  [WARN] 金标准失配「%s」：golden=%d 但 ecommerce_demo 现算=%d —— 疑似 reseed 后未重生成 golden\n",
				c.label, golden, live)
			mismatch = true
		} else {
			fmt.Printf("  [OK] 一致性校验「%s」：golden=%d == 现算=%d\n", c.label, golden, live)
		}
	}
	return mismatch
}

// findGoldenCount 按 question 子串定位样本并从其 reference_answer 抽首个整数。
func findGoldenCount(records []recordLine, questionSub string) (int64, bool) {
	for _, r := range records {
		if strings.Contains(r.Question, questionSub) {
			return firstIntToken(r.ReferenceAnswer)
		}
	}
	return 0, false
}

// openEcommerceDB 连接电商演示库：优先 ECOMMERCE_DB_*，回落 DB_*，库名默认 ecommerce_demo。
func openEcommerceDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		firstEnv("ECOMMERCE_DB_USER", "DB_USER", "root"),
		firstEnv("ECOMMERCE_DB_PASSWORD", "DB_PASSWORD", ""),
		firstEnv("ECOMMERCE_DB_HOST", "DB_HOST", "127.0.0.1"),
		firstEnv("ECOMMERCE_DB_PORT", "DB_PORT", "3306"),
		env("ECOMMERCE_DB_NAME", "ecommerce_demo"),
	)
	return gorm.Open(mysql.Open(dsn), &gorm.Config{})
}

// firstEnv 依次取第一个非空环境变量，都为空时用 def。
func firstEnv(primary, fallback, def string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	if v := os.Getenv(fallback); v != "" {
		return v
	}
	return def
}

// loadManifest 解析嵌入的 manifest.json。
func loadManifest() ([]manifestEntry, error) {
	raw, err := dataFS.ReadFile("data/manifest.json")
	if err != nil {
		return nil, err
	}
	var entries []manifestEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// loadRecords 逐行解析嵌入的样本 JSONL。
func loadRecords(file string) ([]recordLine, error) {
	raw, err := dataFS.ReadFile("data/" + file)
	if err != nil {
		return nil, err
	}
	var records []recordLine
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 放宽单行上限，容纳长样本
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec recordLine
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("解析样本行失败: %w", err)
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

// seedDataset 幂等灌入单个数据集：先按 dataset_id 硬删旧数据，再整集重建。
func seedDataset(ctx context.Context, db *gorm.DB, e manifestEntry, records []recordLine) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 硬删除既有数据集与样本（Unscoped 绕过软删，保证反复执行无残留/重复）。
		if err := tx.Unscoped().
			Where("dataset_id = ? AND tenant_id = ?", e.DatasetID, seedTenant).
			Delete(&mysqlrepo.DatasetRecordModel{}).Error; err != nil {
			return fmt.Errorf("清理旧样本: %w", err)
		}
		if err := tx.Unscoped().
			Where("dataset_id = ? AND tenant_id = ?", e.DatasetID, seedTenant).
			Delete(&mysqlrepo.DatasetModel{}).Error; err != nil {
			return fmt.Errorf("清理旧数据集: %w", err)
		}

		now := time.Now()
		ds := &mysqlrepo.DatasetModel{
			DatasetID:      e.DatasetID,
			TenantID:       seedTenant,
			UserID:         seedUser,
			Name:           e.Name,
			Description:    e.Description,
			Type:           string(evaluation.DatasetTypeDatabase),
			EvaluationType: string(evaluation.EvaluationType(e.EvaluationType).Normalize()),
			QACount:        len(records),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(ds).Error; err != nil {
			return fmt.Errorf("创建数据集: %w", err)
		}

		if len(records) == 0 {
			return nil
		}
		models := make([]*mysqlrepo.DatasetRecordModel, len(records))
		for i, r := range records {
			// 经领域实体走 FromDomainDatasetRecord，复用其 expected_* JSON 编码。
			domain := &evaluation.DatasetRecord{
				DatasetID:       e.DatasetID,
				TenantID:        seedTenant,
				Question:        r.Question,
				ReferenceAnswer: r.ReferenceAnswer,
				RelevantPIDs:    r.RelevantPIDs,
				ExpectedTools:   r.ExpectedTools,
				ExpectedSteps:   r.ExpectedSteps,
				CreatedAt:       now,
			}
			models[i] = mysqlrepo.FromDomainDatasetRecord(domain)
		}
		if err := tx.CreateInBatches(models, 100).Error; err != nil {
			return fmt.Errorf("写入样本: %w", err)
		}
		return nil
	})
}
