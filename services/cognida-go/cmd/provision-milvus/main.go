// 工具：重建 Milvus 统一 "link" collection —— 以带服务端 BM25 Function 的 schema
// 重新建表、建索引（dense HNSW/L2 + sparse BM25）、Load，并从 MySQL chunks 全量回填
// 向量（重新 embedding）。〔KB-3〕
//
// 背景：Milvus 无法向既有 collection 追加 Function，线上 "link" 是带库外建的、
// 缺 text→sparse 的 BM25 映射；要落地服务端 BM25 全文检索只能 drop 后重建。向量
// embedding 只存在于 Milvus（MySQL 仅存 chunks 文本），故重建必须重新向量化回填。
//
// 用法：cd cognida-go && set -a && source .env && set +a
//
//	go run ./cmd/provision-milvus --recreate --reindex --yes
//
// 标志：
//
//	--recreate  先 drop 既有 "link" collection 再重建（危险：清空全部向量）。
//	--reindex   重建后从 MySQL chunks 全量回填并重新 embedding。
//	--yes       跳过交互确认（用于脚本/CI）。
//	--batch     每次 embedding 请求的文本条数（DashScope 单请求上限，默认 10）。
//
// 幂等性：不带 --recreate 时若 "link" 已存在则跳过建表；回填用 chunk_id 派生的稳定
// 主键（stableVectorID），对同一 chunk 幂等（同一 chunk 落同一主键）。但 client.Insert
// 非 Upsert，对既有数据重复回填会产生重复主键，故全量回填务必配合 --recreate。
//
// 前置：Milvus / MySQL 可达，EMBEDDING_API_KEY 有效。本命令会连真实服务并写数据。
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"strings"

	"github.com/cloudwego/eino/components/embedding"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"cognida/internal/config"
	llmembedding "cognida/internal/infrastructure/llm/embedding"
	"cognida/internal/model/knowledge"
	milvusrepo "cognida/internal/repository/milvus"
	"cognida/internal/repository/milvus/retriever"
	mysqlrepo "cognida/internal/repository/mysql"
)

// linkKBID "link" 为全局统一 collection，kbID 参数在 retriever 内被忽略
//（getCollectionName 恒返回 "link"），传 0 占位。
const linkKBID = int64(0)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// stableVectorID 与 service/knowledge 一致：由全局唯一 chunk_id 派生稳定、非负 int64
// 主键（FNV-1a 64 位哈希）。同一 chunk 幂等落同一主键，避免跨文档主键冲突〔KB-2〕。
func stableVectorID(chunkID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(chunkID))
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF)
}

// float64sToFloat32s 将 embedder 返回的 []float64 转为 Milvus 稠密向量所需的 []float32。
func float64sToFloat32s(in []float64) []float32 {
	out := make([]float32, len(in))
	for i, v := range in {
		out[i] = float32(v)
	}
	return out
}

func main() {
	var (
		recreate  = flag.Bool("recreate", false, "drop 既有 link collection 后重建（危险：清空全部向量）")
		reindex   = flag.Bool("reindex", false, "重建后从 MySQL chunks 全量回填并重新 embedding")
		assumeYes = flag.Bool("yes", false, "跳过交互确认")
		batchSize = flag.Int("batch", 10, "每次 embedding 请求的文本条数（DashScope 单请求上限）")
	)
	flag.Parse()

	if !*recreate && !*reindex {
		log.Fatalf("无操作：请至少指定 --recreate 或 --reindex（通常 --recreate --reindex 一起用）")
	}
	// 〔#6〕单独 --reindex（不带 --recreate）会向既有数据重复回填：回填走 client.Insert
	// 而非 Upsert，stableVectorID 对同一 chunk 落同一主键，重跑即产生重复主键/重复向量。
	// 故全量回填必须先 --recreate 清空既有集合，禁止 reindex-only。
	if *reindex && !*recreate {
		log.Fatalf("--reindex 必须与 --recreate 同用：回填用 client.Insert（非 Upsert），单独 reindex 会对既有数据产生重复主键。如需全量回填请执行 --recreate --reindex")
	}
	if *batchSize <= 0 {
		log.Fatalf("--batch 必须为正整数，当前值: %d", *batchSize)
	}

	// 危险操作确认：--recreate 会清空线上全部向量。
	if *recreate && !*assumeYes {
		fmt.Print("⚠️  --recreate 将 DROP 既有 'link' collection 并清空全部向量，不可恢复。\n输入 yes 继续: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(line) != "yes" {
			log.Fatalf("已取消")
		}
	}

	ctx := context.Background()

	// 1. 初始化 Milvus 客户端
	if err := milvusrepo.InitMilvus(config.LoadMilvusConfig()); err != nil {
		log.Fatalf("init milvus failed: %v", err)
	}
	defer func() { _ = milvusrepo.CloseMilvus() }()

	// 2. 构造 embedder + retriever + adapter
	embedder, err := llmembedding.NewEmbedder(config.LoadEmbeddingConfig())
	if err != nil {
		log.Fatalf("init embedder failed: %v", err)
	}
	vr, err := retriever.NewVectorRetriever(embedder)
	if err != nil {
		log.Fatalf("init vector retriever failed: %v", err)
	}
	adapter := retriever.NewVectorRepositoryAdapter(vr)

	// 3. 探测向量维度（embedding provider 无静态维度声明，运行时探测）
	probe, err := embedder.EmbedStrings(ctx, []string{"probe"})
	if err != nil {
		log.Fatalf("probe embedding failed: %v", err)
	}
	if len(probe) == 0 || len(probe[0]) == 0 {
		log.Fatalf("probe embedding 返回空向量，无法确定维度")
	}
	dimension := len(probe[0])
	log.Printf("[provision] embedding 维度探测 = %d", dimension)

	// 4. --recreate：drop 既有 collection（幂等，不存在则跳过）
	if *recreate {
		if err := vr.DropKnowledgeBase(ctx, linkKBID); err != nil {
			log.Fatalf("drop collection failed: %v", err)
		}
	}

	// 5. 建表（带 BM25 Function + sparse BM25 索引，见 adapter.CreateCollection）。
	//    已存在则内部跳过——保证幂等。
	collOpts := knowledge.DefaultCollectionOptions()
	if err := adapter.CreateCollection(ctx, linkKBID, dimension, collOpts); err != nil {
		log.Fatalf("create collection failed: %v", err)
	}
	log.Printf("[provision] collection 'link' 就绪（BM25 Function + sparse 索引已建）")

	// 6. 稠密向量索引（dense_vector, HNSW/L2）。adapter.CreateCollection 只建 BM25 索引，
	//    稠密索引须单独创建，否则 Load 会因稠密向量字段缺索引而失败。
	if err := adapter.CreateIndex(ctx, linkKBID, "dense_vector",
		knowledge.IndexTypeHnsw, knowledge.MetricTypeL2,
		map[string]string{"M": "16", "efConstruction": "256"}); err != nil {
		log.Fatalf("create dense index failed: %v", err)
	}
	log.Printf("[provision] dense_vector 索引就绪（HNSW/L2）")

	// 7. Load collection 到内存（同步等待），使其可检索。
	if err := adapter.LoadCollection(ctx, linkKBID, false); err != nil {
		log.Fatalf("load collection failed: %v", err)
	}
	log.Printf("[provision] collection 'link' 已 Load")

	// 8. --reindex：从 MySQL chunks 全量回填
	if *reindex {
		total, err := backfill(ctx, adapter, embedder, dimension, *batchSize)
		if err != nil {
			log.Fatalf("backfill failed: %v", err)
		}
		log.Printf("[provision] 回填完成，共写入 %d 个 chunk 向量", total)
	}

	log.Printf("[provision] DONE")
}

// backfill 从 MySQL chunks（未软删）全量读取，分批重新 embedding 并写入 Milvus。
// 返回成功写入的 chunk 数。DB 分页用 FindInBatches，embedding 再按 batchSize 二次分批
//（DashScope 单请求把全部文本一次性发出，须调用方控制批量）。
func backfill(
	ctx context.Context,
	adapter *retriever.VectorRepositoryAdapter,
	embedder embeddingEmbedder,
	dimension, batchSize int,
) (int, error) {
	db, err := openDB()
	if err != nil {
		return 0, err
	}

	written := 0
	// DB 每批取 500 条，批内再按 batchSize 切分 embedding。
	batchErr := db.WithContext(ctx).
		Model(&mysqlrepo.ChunkModel{}).
		Where("deleted_at IS NULL").
		FindInBatches(&[]mysqlrepo.ChunkModel{}, 500, func(tx *gorm.DB, batchNo int) error {
			chunks, ok := tx.Statement.Dest.(*[]mysqlrepo.ChunkModel)
			if !ok {
				return fmt.Errorf("unexpected batch dest type %T", tx.Statement.Dest)
			}
			n, err := insertChunkBatch(ctx, adapter, embedder, dimension, batchSize, *chunks)
			if err != nil {
				return fmt.Errorf("batch %d failed: %w", batchNo, err)
			}
			written += n
			log.Printf("[provision] 已回填 %d 个 chunk", written)
			return nil
		}).Error
	if batchErr != nil {
		return written, batchErr
	}
	return written, nil
}

// insertChunkBatch 将一组 chunks 按 embedBatch 切分向量化并写入 Milvus。
func insertChunkBatch(
	ctx context.Context,
	adapter *retriever.VectorRepositoryAdapter,
	embedder embeddingEmbedder,
	dimension, embedBatch int,
	chunks []mysqlrepo.ChunkModel,
) (int, error) {
	written := 0
	for start := 0; start < len(chunks); start += embedBatch {
		end := start + embedBatch
		if end > len(chunks) {
			end = len(chunks)
		}
		sub := chunks[start:end]

		texts := make([]string, len(sub))
		for i, c := range sub {
			texts[i] = c.Content
		}

		vecs, err := embedder.EmbedStrings(ctx, texts)
		if err != nil {
			return written, fmt.Errorf("embed failed: %w", err)
		}
		if len(vecs) != len(sub) {
			return written, fmt.Errorf("embedding 数量不符：期望 %d 得到 %d", len(sub), len(vecs))
		}

		docs := make([]*knowledge.VectorDocument, len(sub))
		for i, c := range sub {
			if len(vecs[i]) != dimension {
				return written, fmt.Errorf("chunk %s 向量维度 %d 与集合维度 %d 不符", c.ID, len(vecs[i]), dimension)
			}
			tokenCount := int64(0)
			if c.TokenCount != nil {
				tokenCount = int64(*c.TokenCount)
			}
			docs[i] = &knowledge.VectorDocument{
				ID:              stableVectorID(c.ID),
				DenseVector:     float64sToFloat32s(vecs[i]),
				Text:            c.Content, // 服务端 BM25 Function 据此生成 sparse
				ChunkID:         c.ID,
				KnowledgeID:     c.KnowledgeID,
				KnowledgeBaseID: c.KnowledgeBaseID,
				TenantID:        c.TenantID,
				ChunkIndex:      c.ChunkIndex,
				Content:         c.Content,
				IsEnabled:       c.IsEnabled,
				StartAt:         int64(c.StartAt),
				EndAt:           int64(c.EndAt),
				TokenCount:      tokenCount,
			}
		}

		if err := adapter.Insert(ctx, linkKBID, docs); err != nil {
			return written, fmt.Errorf("insert failed: %w", err)
		}
		written += len(docs)
	}
	return written, nil
}

// embeddingEmbedder 即 eino Embedder（NewEmbedder 的返回类型），别名以便内部传递。
type embeddingEmbedder = embedding.Embedder

// openDB 打开 link 应用库（元数据库）连接，env 惯例与其它 cmd 工具一致。
func openDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		env("DB_USER", "root"),
		env("DB_PASSWORD", ""),
		env("DB_HOST", "127.0.0.1"),
		env("DB_PORT", "3306"),
		env("DB_NAME", "cognida"),
	)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect mysql failed: %w", err)
	}
	return db, nil
}
