# 本地 Milvus 路径故障：无损诊断、持久恢复与回滚

2026-10-15 在原 `milvus-standalone` 上重新核实：本次 `key not found` 的 bloom 文件没有丢失，而是写入与读取使用了不同的本地路径。旧文档根据目录大小判断数据丢失、要求清卷重建的结论已撤回。**目录小、加载超时或健康检查正常，都不足以证明数据丢失或数据可用。**

此流程针对 `milvusdb/milvus:v3.0-beta`、`COMMON_STORAGETYPE=local`、嵌入 etcd 和 RocksMQ 的单机实例。不适用于 MinIO/S3；不自动修改 etcd、集合、索引或原始文件。不要把现有 Storage V3 数据卷直接交给 2.x 镜像。

## 本次实证

运行版本为 `3.0-beta`，构建提交 `63701018b1`，构建时间 2026-05-08。容器工作目录 `/milvus`，本地数据根 `/var/lib/milvus/data/`；宿主机完整状态卷挂载到 `/var/lib/milvus`。默认健康检查一直成功，但三个集合分别卡在 0%、0%、33%。

| 集合 | ID | 统计行数 | 失败读取位置 | 文件实际位置 |
|---|---|---:|---|---|
| `link` | 468300566087713215 | 117 | `/milvus/files/insert_log/...` | `/var/lib/milvus/data/files/insert_log/...` |
| `coworker_knowledge_bm25_v1` | 469301484585695175 | 21 | `/milvus/files/insert_log/...` | `/var/lib/milvus/data/files/insert_log/...` |
| `semantic_cache_vectors` | 469481819875950923 | 2 | `/var/lib/milvus/data/insert_log/...` | `/var/lib/milvus/data/var/lib/milvus/data/insert_log/...` |

日志中的 6 个缺失 bloom 文件都在对应的另一位置找到。精确版本的 [`LocalChunkManager.Read`](https://github.com/milvus-io/milvus/blob/63701018b1/internal/storage/local_chunk_manager.go#L115-L118) 直接读取传入路径，不补本地根目录；相对 key 因而以进程工作目录解析。该版本 [`packed.WriteFile`](https://github.com/milvus-io/milvus/blob/63701018b1/internal/storagev2/packed/ffi_filesystem.go) 将完整路径传给同样配置了根目录的文件系统；官方 [issue #49419](https://github.com/milvus-io/milvus/issues/49419) 描述了重复根目录问题。该 issue 被 stale bot 关闭，不能据此认定当前镜像已修复。

两个路径别名让既有读写访问同一批原文件。它们是已验证的兼容措施；后续引擎升级须在完整备份的副本上单独验证，不能在原卷上试降级。

## 只读诊断

在仓库根目录运行，Python 3.9+ 标准库即可，不需要安装 SDK：

```bash
MILVUS_VOLUME_ROOT="$HOME/milvus/volumes/milvus"
python3 services/cognida-go/scripts/milvus_local_paths.py diagnose \
  --container milvus-standalone \
  --data-root "$MILVUS_VOLUME_ROOT/data"
```

工具读取 Docker inspect、最近两分钟的有界日志及 loopback Milvus REST API，只输出集合加载状态、统计行数、缺失 key 的存在位置和持久挂载状态。不会打印环境变量、样本正文、向量、主键或完整日志。鉴权时从已有 `MILVUS_TOKEN` 环境变量读取；不要将密钥写进 URL 或命令行。

若 `existing_host_paths` 为空，停止使用此路径修复流程：应继续查备份与实际对象位置，不造空 bloom 文件，不删除元数据掩盖缺失。日志没有缺失 key 也不代表所有数据健康，需执行下文实际查询验证。

## 停机备份及受限修复

先停止使用此状态卷的所有写入服务，保留原容器。以下命令创建独立备份目录；`container-inspect.json` 可能含环境密钥，权限必须限制且不要提交仓库。

```bash
MILVUS_VOLUME_ROOT="$HOME/milvus/volumes/milvus"
MILVUS_BACKUP_ROOT="$HOME/milvus/backups/$(date +%Y%m%d-%H%M%S)-path-repair"
umask 077
mkdir -p "$MILVUS_BACKUP_ROOT"
docker inspect milvus-standalone > "$MILVUS_BACKUP_ROOT/container-inspect.json"
docker stop -t 60 milvus-standalone
cp -a "$MILVUS_VOLUME_ROOT" "$MILVUS_BACKUP_ROOT/volume"
cp -p "$HOME/milvus/user.yaml" "$HOME/milvus/embedEtcd.yaml" "$MILVUS_BACKUP_ROOT/"
```

停止后复制 `data`、`etcd`、`rdb_data` 和 `rdb_data_meta_kv` 整体，不能只备份 `data` 或只备份 etcd。也可另外保存 tar 和 SHA256；脚本的 `--backup-root` 需要上述目录式副本，不直接接受 tar。备份后还应保留原镜像 ID、端口、healthcheck、restart policy、网络和挂载配置，不打印或公开 inspect 文件。

先 dry-run，再显式 apply：

```bash
python3 services/cognida-go/scripts/milvus_local_paths.py repair-host \
  --data-root "$MILVUS_VOLUME_ROOT/data"
python3 services/cognida-go/scripts/milvus_local_paths.py repair-host \
  --data-root "$MILVUS_VOLUME_ROOT/data" \
  --backup-root "$MILVUS_BACKUP_ROOT/volume" --apply
```

唯一的宿主机改动是相对符号链接：

```text
data/insert_log -> var/lib/milvus/data/insert_log
```

工具验证 data 根与 Docker inspect 的 bind source 完全一致，目标必须是该根内真实目录。apply 要求目标容器停止，且不存在可写挂载同一卷的其他运行容器；备份与停机原卷的每个普通文件必须 SHA256 一致。只替换不存在或空的 `insert_log`，拒绝有内容的目录和其他符号链接。重复执行已有正确别名不会改数据。修复收据写在备份目录，用于回滚及备份完整性核验。

## 让容器重建后仍有效

另一个别名必须持久化到容器启动配置：

```text
宿主机 $MILVUS_VOLUME_ROOT/data/files  → 容器 /milvus/files
```

仅在运行容器内建立 `/milvus/files` 符号链接能经受 restart，却不能保证 recreate 后保留。重建前确认原 `/milvus/files` 不存在、为空目录，或已经是指向 `/var/lib/milvus/data/files` 的正确链接；如有其他内容先取证，不能直接用挂载遮住它。

在原有 `docker run` 参数中追加下面这一项，其他参数必须按已保存的 inspect 保持原样：

```bash
--mount "type=bind,src=$MILVUS_VOLUME_ROOT/data/files,dst=/milvus/files"
```

若原来使用 Compose，在对应 Milvus 服务原有 volumes 下追加：

```yaml
volumes:
  - ${MILVUS_VOLUME_ROOT}/data/files:/milvus/files
```

这只是追加挂载片段，不是一份完整部署文件。不要直接用仓库根目录另一套版本的 Compose 覆盖原实例。先停止并重命名旧容器保留回滚，再用相同镜像 ID、原配置及新增挂载创建同名容器；不能让两个容器同时写同一卷。宿主机 `insert_log` 相对链接本身位于持久卷中，无需每次重建重做。

## 启动后验收

原集合的加载意图保留在元数据中，本次启动后自动恢复加载。诊断工具不调用 load/release，也不创建数据。运行：

```bash
python3 services/cognida-go/scripts/milvus_local_paths.py verify \
  --data-root "$MILVUS_VOLUME_ROOT/data"
```

验收需同时检查：

- `persistent_files_mount=true`；容器 recreate 后仍为 true。
- 原集合 ID 保持，加载状态为 `LoadStateLoaded` 且进度 100。
- `rowCount` 与修复前基线一致；实际 `count(*)` 单独记录。统计行数和可见行数可能因删除或 TTL 不一致，不能用统计行数冒充实际读到的数据。
- 有可见行的 FloatVector 集合取一个原向量做限定原主键的搜索；只有 BM25 的集合取一个原文本做同样限定的搜索。样本只用于本机同一 Milvus 的请求，不输出。
- 空集合或没有支持的向量字段会明确报告 `not_tested`，不能算作搜索通过。还应另用专用测试集合或明确测试 scope 做 insert → flush → release/load → search → delete 的持久读写验收，不能把该探针写进原业务记录。

2026-10-15 修复及 recreate 后的只读结果：`link` 可见 117 行、dense 搜索通过；`coworker_knowledge_bm25_v1` 可见 21 行、BM25 搜索通过；`semantic_cache_vectors` 统计 2 行、可见 0 行，因此没有宣称旧缓存内容搜索通过。三个原 ID 保持，近期日志无缺失 key。后续测试产生的新行应另记，不应要求统计数字永远固定。

## 回滚

先停新容器，确认没有其他容器写同一卷。工具创建的宿主机别名可用相同备份目录回滚：

```bash
python3 services/cognida-go/scripts/milvus_local_paths.py rollback \
  --data-root "$MILVUS_VOLUME_ROOT/data" \
  --backup-root "$MILVUS_BACKUP_ROOT/volume"
python3 services/cognida-go/scripts/milvus_local_paths.py rollback \
  --data-root "$MILVUS_VOLUME_ROOT/data" \
  --backup-root "$MILVUS_BACKUP_ROOT/volume" --apply
```

回滚只删除工具记录的精确符号链接，必要时恢复原空目录，保留别名目标内后续写入的文件。它不回滚数据或 etcd，也不删除新文件。如果别名由人工创建、没有本工具收据，工具拒绝代为回滚；按已保存的人工操作记录检查并撤回。

容器配置回滚：保持新容器停止，恢复保留的原容器名称与原启动配置；不得并发启动两者。撤回路径桥接可能恢复之前的加载失败，这与数据回滚是不同操作。只有确认需要整体灾难恢复时才另行制定状态卷恢复计划，不能把停机快照直接覆盖存在新写入的原卷。

## 工具回归

```bash
python3 -m unittest discover \
  -s services/cognida-go/scripts/tests -p 'test_milvus_local_paths.py' -v
```

覆盖 dry-run 无写入、固定范围、其他运行写者拦截、备份逐文件校验、非空目录保护、异名链接保护、幂等 apply、保留新数据的 rollback、loopback/只读 API 限制及样本不输出。真实实例的 API 验收用上面的 `diagnose` / `verify` 命令完成。
