// 工具：将转换好的评测数据集（HF 基准 + 自造场景）一键灌入 link 元数据库，
// 供「Agent 测评」创建任务时直接选用。
//
// 用法：cd link-go && set -a && source .env && set +a && go run ./cmd/seed-eval-datasets
//
// 数据来源：data/manifest.json + data/<dataset_id>.jsonl（由
// link-python/scripts/convert_eval_datasets.py 产出，经 //go:embed 打包进二进制，
// 故与运行 cwd 无关）。每行一条样本，字段与 QAPair 对齐：
//   question / reference_answer / relevant_pids? / expected_tools? / expected_steps?
//
// 幂等：按 dataset_id 硬删除既有数据集行与样本记录后整集重灌，反复执行不产生重复。
// 落库对象：租户 tenant_id=1（dev 用户），evaluation_type 取 manifest 标注（agent/qa）。
//
// 前置：link 库已 migrate-db（evaluation_datasets / evaluation_dataset_records 已建表）。
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"link/internal/model/evaluation"
	mysqlrepo "link/internal/repository/mysql"
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
		if err := seedDataset(ctx, db, e, records); err != nil {
			log.Fatalf("seed dataset %q failed: %v", e.DatasetID, err)
		}
		total += len(records)
		fmt.Printf("OK: seeded %-28s %-6s %3d 条样本\n", e.DatasetID, e.EvaluationType, len(records))
	}
	fmt.Printf("DONE: %d 个数据集，共 %d 条样本 → 租户 %d\n", len(entries), total, seedTenant)
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
