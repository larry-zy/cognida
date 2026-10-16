# HF 清洗保真 benchmark

该 benchmark 只读主控固定 revision 下载的本地语料，不联网下载。主测量经过生产
`/api/v1/lifecycle/clean` 路由、Pydantic 校验、请求限制和真实 CleanerRegistry。
TestClient 模式不验证网络和认证；可另用本机 live HTTP 模式验证这两层。
它不代替 Go 采集、持久化、审批或发布的端到端验收。

每个数据集提供一个 manifest；可重复传入 `--manifest`：

```json
{
  "dataset_id": "mteb/banking77",
  "revision": "精确的40位commit SHA",
  "license": "数据集声明的license",
  "files": [{
    "split": "test",
    "path": "test.jsonl",
    "sha256": "实际文件的64位SHA256",
    "format": "jsonl",
    "text_field": "text",
    "label_field": "label"
  }]
}
```

`path` 相对 manifest 所在目录，也可为绝对路径。支持 JSONL 和 Parquet；记录全部
字段保存为 canonical JSON，另保留完全相同的文本列。主控同时提供 `raw_files`：
每项包含 `source_path`、`path`、`sha256`。这两份已批准语料的原文件分别为
`<split>.jsonl` 和 `<split>/zh-CN.json.gz`。完整运行同时校验下载原文件 hash、
逐行原文完全相同，并将所有原始字段保存在 `hf_record_json._hf_original` 中参与保真核对。
缺原文件、固定 revision、行数不符或 hash 不符时直接失败，不能回退到在线数据。

## Oracle 与边界

- 原始基线：每条 HF 原记录分配唯一 `source_id`，显式 exact dedup 不得误删自然重复文本、
  合法空文本或首尾空白。按原始顺序逐行比对全部字段、JSON 类型和输出 hash。
- 人工扰动：每 5 条复制一条完整行；每 13 条额外插入一行全 null；每 3 条只在
  `working_text` 派生列添加 Unicode/ASCII 首尾空白。原文列从不改写。
- 有合法原始首尾空白的行仍全部进入基线，但不进入 padding 恢复测试。该排除数量单独报告。
- 先仅 trim `working_text`，再整行 drop_empty / exact dedup；分别测 keep first 和 last。
  只清空工作列、仍带来源或原文的行不符合整行空行条件，必须保留。
- 整数 9007199254740993、DECIMAL 文本 9007199254740993.123456789012345678、
  字符串 00123、bool、null 是明确标记的人工保真 guard，不冒充真实财务数据。
- Oracle 不调用 Pandas 或生产清洗器。输出必须同时匹配内容、顺序、保留的源行下标及幂等性。
  报告含误删、漏删、字段/类型变化、lineage/schema 错误、每阶段耗时和输入/预期/实际 hash。
- 每批最多 2000 原始行，增广后仍低于生产 10000 行 / 100000 cell / 4 MiB 限制；
  特长单元格或请求超限直接失败，不静默截断。批次间不声称做全局去重。
- 人工扰动是确定性规则回归，不代表自然脏数据上的 precision/recall，也不能把全部 HF 原文当作可删除项。

## 执行

在 `services/cognida-python` 下：

```bash
HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 .venv/bin/python -m pytest tests/benchmarks/test_cleaning_benchmark.py -q
HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 .venv/bin/python tests/benchmarks/cleaning_benchmark.py \
  --manifest /private/tmp/task-data/banking77/manifest.json \
  --manifest /private/tmp/task-data/massive-zh/manifest.json \
  --output /private/tmp/task-evidence/cleaning-testclient.json
```

本机 live fixture 只绑定回环地址，先核端口空闲，使用新随机 token（环境文件 0600，
不提交也不打印）。启动 `APP_ENV=test INTERNAL_HTTP_AUTH_TOKEN=...` 的
`.venv/bin/python -m uvicorn cleaning_http_fixture:app --app-dir tests/benchmarks --host 127.0.0.1 --port 62510`。
客户端从 `COGNIDA_CLEANING_BENCH_TOKEN` 读取同一 token，追加
`--base-url http://127.0.0.1:62510`。客户端拒绝向外部地址发送 token。
运行后必须停止该任务创建的进程并清理随机 token；保留不含原文和凭据的统计 JSON。
