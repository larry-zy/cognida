// Command milvus-export 是一次性运维工具：盘点/逻辑导出本地 Milvus 的 collection。
//
// 目的：在清卷重启前，趁实例还活着，把「真能读出来的」collection 逻辑导出成可迁移格式
// （schema.json + data.jsonl），以便导入任意新 Milvus 实例；对读不出来的（损坏）如实标记。
//
// 用法：
//
//	go run ./cmd/milvus-export                 # 盘点：只走元数据，不触发加载，安全
//	go run ./cmd/milvus-export -export DIR      # 逐库尝试加载+全量查询导出，每库有超时兜底
//
// 连接参数取自环境变量 MILVUS_HOST（默认 localhost:19530）/ MILVUS_TOKEN。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	exportDir := flag.String("export", "", "导出目录；为空则仅盘点")
	perColl := flag.Duration("timeout", 60*time.Second, "单库加载/查询超时")
	flag.Parse()

	addr := envOr("MILVUS_HOST", "localhost:19530")
	token := os.Getenv("MILVUS_TOKEN")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cli, err := milvusclient.New(ctx, &milvusclient.ClientConfig{Address: addr, APIKey: token})
	if err != nil {
		fmt.Printf("连接失败 %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer cli.Close(context.Background())

	names, err := cli.ListCollections(ctx, milvusclient.NewListCollectionOption())
	if err != nil {
		fmt.Printf("ListCollections 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("共 %d 个 collection\n\n", len(names))

	for _, name := range names {
		printInventory(ctx, cli, name)
		if *exportDir != "" {
			exportOne(cli, *exportDir, name, *perColl)
		}
		fmt.Println()
	}
}

// printInventory 只走元数据接口，不触发段加载。
func printInventory(ctx context.Context, cli *milvusclient.Client, name string) {
	fmt.Printf("── collection: %s\n", name)

	desc, err := cli.DescribeCollection(ctx, milvusclient.NewDescribeCollectionOption(name))
	if err != nil {
		fmt.Printf("   DescribeCollection 失败: %v\n", err)
		return
	}
	fmt.Printf("   ID=%d  字段: ", desc.ID)
	for i, f := range desc.Schema.Fields {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%s(%s)", f.Name, f.DataType.Name())
	}
	fmt.Println()

	stats, err := cli.GetCollectionStats(ctx, milvusclient.NewGetCollectionStatsOption(name))
	if err != nil {
		fmt.Printf("   行数: 取不到 (%v)\n", err)
	} else {
		fmt.Printf("   行数(row_count): %s\n", stats["row_count"])
	}

	ls, err := cli.GetLoadState(ctx, milvusclient.NewGetLoadStateOption(name))
	if err != nil {
		fmt.Printf("   加载态: 取不到 (%v)\n", err)
	} else {
		fmt.Printf("   加载态: %v\n", ls.State)
	}
}

// exportOne 尝试加载并全量查询导出；损坏库会在超时后被跳过并标记。
func exportOne(cli *milvusclient.Client, dir, name string, timeout time.Duration) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Printf("   [导出] 建目录失败: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 写 schema
	desc, err := cli.DescribeCollection(ctx, milvusclient.NewDescribeCollectionOption(name))
	if err != nil {
		fmt.Printf("   [导出] 跳过：describe 失败 %v\n", err)
		return
	}
	schemaPath := filepath.Join(dir, name+".schema.json")
	if b, e := json.MarshalIndent(desc, "", "  "); e == nil {
		_ = os.WriteFile(schemaPath, b, 0o644)
	}

	// 尝试加载（损坏库会卡在这里直到超时）
	task, err := cli.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(name))
	if err != nil {
		fmt.Printf("   [导出] 跳过：LoadCollection 失败（疑似损坏）%v\n", err)
		return
	}
	if err := task.Await(ctx); err != nil {
		fmt.Printf("   [导出] 跳过：加载未完成（疑似损坏/超时）%v\n", err)
		return
	}

	// 全量查询（主键 > 0 或空 filter，输出所有字段）
	pk := ""
	for _, f := range desc.Schema.Fields {
		if f.PrimaryKey {
			pk = f.Name
		}
	}
	filter := ""
	if pk != "" {
		filter = pk + " >= 0"
	}
	rs, err := cli.Query(ctx, milvusclient.NewQueryOption(name).
		WithFilter(filter).WithOutputFields("*").WithLimit(1000000))
	if err != nil {
		fmt.Printf("   [导出] 跳过：Query 失败 %v\n", err)
		return
	}

	dataPath := filepath.Join(dir, name+".data.json")
	rows := resultToRows(rs)
	if b, e := json.MarshalIndent(rows, "", " "); e == nil {
		_ = os.WriteFile(dataPath, b, 0o644)
	}
	fmt.Printf("   [导出] ✅ %d 行 → %s (+schema)\n", len(rows), dataPath)
}

// resultToRows 把 ResultSet 转成按行的 map 切片，便于迁移导入。
func resultToRows(rs milvusclient.ResultSet) []map[string]any {
	n := rs.ResultCount
	rows := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		rows[i] = map[string]any{}
	}
	for _, col := range rs.Fields {
		for i := 0; i < n; i++ {
			if v, err := col.Get(i); err == nil {
				rows[i][col.Name()] = v
			}
		}
	}
	return rows
}
