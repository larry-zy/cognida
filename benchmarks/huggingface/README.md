# 公开数据验收

该基准测量实际清洗器、规则/模型标注器、特征物化与漂移统计。它区分确定性正确性、模型质量和生产运行条件；不以流程成功替代准确率。

- [固定协议、采样与边界](PROTOCOL.md)
- [2026-10-16 实测报告](REPORT-2026-10-16.md)
- [数据版本、许可记录和 SHA256](datasets.lock.json)
- [清洗基准详细说明](../../services/cognida-python/tests/benchmarks/cleaning_README.md)

## 准备数据

从仓库根目录运行。Python 使用项目 `.venv`，需要 `requests`、`pyarrow`；不会执行 Hugging Face dataset scripts。数据下载到仓库外，原始语料不提交 Git。

```bash
export COGNIDA_HF_DATA_DIR=/private/tmp/cognida-hf-data
services/cognida-python/.venv/bin/python benchmarks/huggingface/fetch.py --output "$COGNIDA_HF_DATA_DIR"
services/cognida-python/.venv/bin/python benchmarks/huggingface/prepare.py --data "$COGNIDA_HF_DATA_DIR"
services/cognida-python/.venv/bin/python -m unittest discover -s benchmarks/huggingface -p 'test_*.py' -v
```

默认必须有已提交的 lock，下载摘要不符即失败。`--create-lock` 仅用于明确建立新实验，不能替换旧锁或覆盖旧实验。准备器重新检查锁定清单、实际源路径、字节数和哈希；生成的 `manifest.json`、`label-config.json` 与 split JSONL 都指向本次本地目录。

## 专用服务与数据库

Go 集成测试要求提前迁移的独立 MySQL 库（名称以 `cognida_review_` 开头），不能使用业务库。设置 `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME` 后，在 `services/cognida-go` 执行 `go run ./cmd/migrate-db up`，确认 migration 版本至少 36、dirty=false；然后设置指向同一库的 `COGNIDA_DOMAIN_TEST_DSN`、`COGNIDA_HF_TEST_DSN`。仅启动任务所属服务，记录 PID/容器 ID；完成后停止这些服务并回收测试库和凭据。

标注/特征 fixture 自行清理各自随机租户记录，使用真实 MySQL 和本地不可变对象文件。特征 HTTP fixture 在进程内调生产 handler；跨 Go/Python 采集测试另启实际 TCP server，并真实执行 Redis claim/ack。身份授权是显式测试 fixture，不声称验收生产登录、角色目录或身份提供商。

## 运行清洗与接口链路

在 `services/cognida-python` 中执行清洗 README 的 TestClient 和 live 模式。live fixture 仅绑定回环地址，以新随机 `INTERNAL_HTTP_AUTH_TOKEN` 保护，不打印或提交该 token。

在 `services/cognida-go` 中设置：

- `COGNIDA_HF_REDIS_ADDR`：本轮专属 Redis 地址。
- `COGNIDA_HF_COMPUTE_URL`：上述 Python fixture 地址。
- `INTERNAL_HTTP_AUTH_TOKEN`：同一临时 token。
- `COGNIDA_HF_INGESTION_OUTPUT`：本轮独立输出文件路径。

```bash
go test -tags=integration ./internal/handler -run '^TestHFIngestionComputeHTTPBenchmark$' -count=1 -v
```

这条测试涵盖固定 512 条官方 test 记录的采集、质量、清洗、不可变资产、幂等和跨租户隔离，以及 8 条类型控制；审批发布/搜索不在该新增测试范围。不能将它计作新一轮浏览器 E2E。

## 特征和漂移

在 `services/cognida-go` 中：

```bash
export COGNIDA_FEATURE_BENCHMARK_MANIFEST="$COGNIDA_HF_DATA_DIR/wine_quality/manifest.json"
export COGNIDA_FEATURE_BENCHMARK_OUTPUT_DIR=/private/tmp/cognida-hf-feature-results
# 具体测试名称可在 feature_benchmark_* / drift_benchmark_* 中查阅。
go test -tags=integration ./internal/service/governance -run '^Test(Feature|Drift)Benchmark' -count=1 -v
```

普通和 `-race` 输出使用不同目录，保留所有运行记录。10k 行为重复扩展的投影压力输入；未验证生产并发或 SLA。

## 标注与数据集

runner 为 `TestHFLabelBenchmark`。三个数据集使用独立且全新的输出目录：

```bash
export COGNIDA_LABEL_BENCHMARK_CONFIG="$COGNIDA_HF_DATA_DIR/banking77/label-config.json"
export COGNIDA_LABEL_BENCHMARK_OUTPUT=/private/tmp/cognida-hf-label-results/banking77
COGNIDA_LABEL_BENCHMARK_MODE=prepare go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -v
COGNIDA_LABEL_BENCHMARK_MODE=rules go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -v
COGNIDA_LABEL_BENCHMARK_MODE=oracle go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -v
```

对 `massive_zh`、`few_nerd` 同样先完成 prepare，固定全部样本、模型、代码和 prompt 摘要。rules 使用实际生产规则引擎全量评分；oracle 用少量人工正确标签走审核/不可变数据集发布，不能计入自动标注成绩。

真实模型读取现有受管 Chat 配置。在明确同意使用当前供应商和费用后，逐数据集执行，**不得并行运行三个数据集**：

```bash
COGNIDA_LABEL_BENCHMARK_LIVE=1 COGNIDA_LABEL_BENCHMARK_MODE=model \
  go test -tags=integration ./internal/service/governance -run '^TestHFLabelBenchmark$' -count=1 -timeout=45m -v
```

每个数据集最多两个并行调用；本协议共 335 条。`model.started` 和逐行 `model-outcomes.jsonl` 防止误重跑覆盖结果。出现失败先检查 journal、保留原分母，不能删除文件后重新抽题冒充首轮结果。模型使用生产 20 秒 deadline、4096 output tokens；费用不可得时报告 null。公开数据经已配置的模型供应商处理，不向供应商传测试 gold。

## 回归与审查

确定性核心逻辑单测、真实数据库/API 测试、关键路径 `-race` 与独立 Code Review 都须完成。`benchmarks/huggingface/results/2026-10-16/` 只保留统计和运行证据摘要，不含 token、DSN、整份语料或模型输入正文。报告应同时列出失败、弃权、当前容量限制、未测试的真实生产条件和清理回执。
