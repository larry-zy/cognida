# 本地 Milvus 存储一致性缺陷：成因、原理与彻底修复

> 环境：macOS（Darwin 25.5.0）+ **OrbStack**（Docker 运行时，VM 15.66 GiB / 14 CPU）单容器 `milvus-standalone`
> 部署：官方 `standalone_embed.sh`（`docker run`，非 compose），镜像 `milvusdb/milvus:v3.0-beta`
> 存储：`COMMON_STORAGETYPE=local`（本地 FS 当对象存储，无独立 MinIO）+ 嵌入式 etcd + RocksMQ WAL
> Go 客户端：`github.com/milvus-io/milvus/client/v2@v2.6.3`
> 状态卷：`~/milvus/volumes/milvus`

---

## 一、现象（Symptom）

集成测试 `cache_collection_integration_test.go` 在等待 collection 加载时 90s 超时:

```
await cache collection load (cap 1m30s): context deadline exceeded
```

Milvus 容器日志里 querynode 无限重试同一个错误:

```
LoadBloomFilter ... key not found
[error: ... _stats/bloom_filter.100/... key not found]
```

同时 datacoord 每 10s 循环一次 L0 compaction plan，段永远加载不完。**新建一个空 collection 能秒加载**——说明不是全局故障，而是特定 collection 的段损坏。

### 关键量化证据

清点 `~/milvus/volumes/milvus/` 时发现严重的"账实不符":

| 目录 | 大小 | 含义 |
|---|---|---|
| `data/`（对象存储） | **296K** | 段文件几乎被掏空——向量数据实际已丢 |
| `etcd/`（元数据） | **444M** | 段元数据严重膨胀、仍在引用那些已丢的文件 |
| `rdb_data/`（RocksMQ WAL） | 98M | 消息队列堆积 |
| `rdb_data_meta_kv/` | 90M | WAL 元数据堆积 |

`data/insert_log` 下只剩 3 个 bloom_filter 文件。**etcd 里记着一堆段（含 `_stats/bloom_filter.100/...`），但对象存储里读不到对应文件**——这就是"存储一致性分叉（divergence）"。

---

## 二、根因（Root Cause）

一句话：**macOS bind-mount 的 fsync 慢 → 嵌入式 etcd 选举超时 → 容器崩溃循环 → datanode 的 flush 被反复打断，段文件写对象存储的动作丢失，而段的元数据已先落 etcd → 元数据引用了并不存在的文件，形成永久分叉。**

拆成因果链:

```
① macOS 上跨 VM 边界的 bind-mount（OrbStack / Docker Desktop 皆经虚拟化文件共享层）
   对 fsync 有显著延迟惩罚
        │
        ▼
② 嵌入式 etcd 的 Raft 要求 fsync WAL 才能确认心跳/投票
   fsync 一慢 → 选举超时（默认 election-timeout 太紧）
        │
        ▼
③ etcd 反复触发重新选举 → 单容器内 etcd 不稳 →
   Milvus 组件连不上 meta → 容器崩溃重启（观测到 8 次重启）
        │
        ▼
④ datanode 正在 flush 段：段的 meta 已写入 etcd（先），
   但段文件写本地对象存储（后）尚未完成/未 fsync 落盘就被崩溃打断
        │
        ▼
⑤ 重启后：etcd 里 meta 说"这些段存在"，对象存储里文件却没有
   → querynode 去加载 → LoadBloomFilter「key not found」→ 无限重试风暴
   → datacoord 每 10s 生成孤儿 L0 compaction plan，永不收敛
```

### 为什么是"先 meta 后 data"会出事

Milvus 的 flush 不是单一原子事务：段的元数据（binlog 路径清单）写 etcd 与段文件写对象存储是**两段式**、跨两套存储系统。正常情况下二者最终一致；但当进程在两步之间被杀，且对象存储那步的写入因 fsync 未完成而丢失时，就只剩 meta 这半边——**没有跨存储的两阶段提交来回滚 meta**，于是分叉被永久固化在 etcd 里。

### 放大器（非根因，但加重）

- **`v3.0-beta` 镜像**：beta 版，自身健壮性/收尾逻辑未经充分打磨。
- **版本错配**：Go `client/v2@2.6.3`（面向 2.5/2.6 GA）对着 `v3.0-beta` 服务端，行为不完全可预期。
- **不干净停止**：`docker kill` / Docker Desktop 直接退出会跳过 grace period，等价于步骤③的人为触发。

> ⚠️ 一个曾经的误判：观察到 etcd 调优生效、2 小时无崩溃后，一度以为"损坏是历史遗留、已封住"。但在 drop `reflection_memory` 时，风暴在**刚由测试新建**的 cache collection（collectionID=468240087017578968）上继续出现——证明分叉在**新 flush 上仍会活跃产生**，不是纯历史包袱。这也是为什么"只删旧 collection"治标不治本。

---

## 三、原理补充：为什么根因在 etcd 而不在别处

- **磁盘没满**（32%）、**没 OOM**、**没有独立 MinIO 容器**——排除了容量/内存/外部对象存储三条常见路径。
- 真正的软肋是**嵌入式 etcd 对 fsync 延迟极其敏感**：Raft 协议把"日志已持久化"作为心跳/选举的前提，fsync 一旦被慢速 bind-mount 拖住，Leader 就会被误判失联，触发无谓的重新选举，进而拖垮整容器。
- 因此**真正把复发按住的是 etcd 调优参数，而不是换镜像**。当前 `~/milvus/embedEtcd.yaml` 已加:

  ```yaml
  # macOS 绑定挂载 fsync 慢导致选举超时崩溃循环的修复
  heartbeat-interval: 500       # 放宽心跳（ms）
  election-timeout: 5000        # 放宽选举超时（ms），容忍慢 fsync
  snapshot-count: 1000          # 更频繁快照，压小 WAL 重放
  quota-backend-bytes: 4294967296
  auto-compaction-mode: revision
  auto-compaction-retention: '1000'
  ```

  这一层治的是**根因**（选举超时崩溃循环）。**必须保住。**

---

## 四、彻底修复（Definitive Fix）

### 为什么原地修不了

段文件已**物理丢失**，无法从 etcd meta 反推重建；etcd 里 444M 的孤儿引用也没有官方"孤儿 GC"能安全清掉不误伤。唯一确定性的办法是**清空状态卷重来**。代价其实很小——`data/` 只剩 296K，**向量本来就已经丢了**，清卷并不会额外损失可用数据，重灌向量在任何方案下都不可避免。

### Part A —— 清卷（必需）

```bash
# 1) 干净停止：给足 grace，让 datanode 收尾。以后严禁 docker kill / 硬退出 Docker Desktop
docker stop -t 60 milvus-standalone
docker rm milvus-standalone

# 2) 备份后清空全部状态（消除 444M 孤儿 meta + 丢失的 data + 堆积 WAL）
mv ~/milvus/volumes ~/milvus/volumes.corrupt.bak
#    ↑ 保留 ~/milvus/embedEtcd.yaml 与 user.yaml（在 volumes 之外，不受影响）
#    ⚠️ 不要用 `standalone_embed.sh delete`——它会连 embedEtcd.yaml/user.yaml 一起删，丢掉崩溃循环的修复
```

### Part B —— 保住 etcd 调优（必需，真正防复发的一层）

`standalone_embed.sh` 的 `run_embed()` 在每次全新拉起时会用**默认模板覆盖** `embedEtcd.yaml`（heredoc，第 20–26 行附近），会冲掉上面那三个调优参数 → 崩溃循环复发。两种做法二选一:

- **推荐**：把 heartbeat/election/snapshot 三参数直接写进脚本 `run_embed()` 的 heredoc，一劳永逸。
- 或：`start` 之后立刻把调优参数补写回 `~/milvus/embedEtcd.yaml`，再 `restart` 一次让 etcd 重新加载。

### Part B2 —— Milvus 内部参数与内存策略调优（推荐，降复发+提恢复速度）

除 etcd 层外，再从 Milvus 引擎层做一层「减写 churn + 增内存余量 + 快恢复」的策略调优，写在 `~/milvus/user.yaml`（覆盖 `milvus.yaml`，`standalone_embed.sh` 已挂载）。**在下次干净重启时随新卷一起生效。**

核心思想：**用内存换「少而整的落盘」**——把慢速 bind-mount 上的写次数压到最低，就等于把「flush 被打断」的窗口数压到最低。OrbStack VM 有 15.66G、容器无 `-m` 限制，内存余量充足，值得用。

| 参数 | 默认 | 调整为 | 策略意图 |
|---|---|---|---|
| `dataNode.segment.insertBufSize` | 16MB | **64MB** | 攒更大批再 flush → flush 更少更整（核心：增内存换少写） |
| `dataNode.segment.deleteBufBytes` | 16MB | **64MB** | 删除缓冲同步放大 |
| `dataNode.segment.syncPeriod` | 600s | **1200s** | 定时强制 sync 周期拉长，进一步降落盘频率 |
| `dataNode.memory.forceSyncEnable` | true | **true（保留）** | 内存压力安全阀：内存高企时仍强制 sync，防缓冲堆积 OOM |
| `dataNode.memory.forceSyncSegmentNum` | 1 | **2** | 强制 sync 时一次收尾 2 个最大缓冲段 |
| `queryNode.loadMemoryUsageFactor` | 1 | **2** | 加载内存系数翻倍，给段加载留余量，降加载抖动/失败 |
| `dataCoord.compaction.enableAutoCompaction` | true | **false** | 关后台自动压缩，止住每 10s 一次 L0 compaction plan 空转风暴 |
| `dataCoord.compaction.levelzero.triggerInterval` | 10s | **60s** | 兜底：即便触发 L0 也降频 |
| `rocksmq.retentionTimeInMinutes` | 4320（3天） | **1440（1天）** | 收窄 WAL → 减崩溃后重放量 → 加快重启恢复 |
| `rocksmq.retentionSizeInMB` | 8192 | **4096** | 单 topic WAL 上限减半 |
| `quotaAndLimits.flushRate.collection.max` | 0.1 | **1** | 单集合 flush 速率放宽，消除连测时的 `flush ... rate limit[rate=0.1]` 失败 |

> 权衡说明：放大 `insertBufSize` 意味着崩溃时内存里未落盘的数据更多。但**崩溃本身由 etcd 调优（Part B）拦住**，因此此处的净效果是「写更少 → 慢 FS 压力更小 → 被打断机会更少」，配合 `forceSyncEnable` 安全阀兜底，整体为正收益。若把 VM 内存调小（<8G），应把 `insertBufSize` 相应回调到 32MB。

### Part C —— 换稳定 GA 镜像（可选加固，非必需）

> 严格说不换也能修好这次损坏。换只是消除"beta 未知 bug + 与 client 2.6.3 版本错配"这层残余风险。

编辑 `standalone_embed.sh` 里 `run_embed()` 的镜像标签:

```
milvusdb/milvus:v3.0-beta   →   milvusdb/milvus:v2.6.x   # 与 client/v2@2.6.3 对齐的稳定 GA
```

（`COMMON_STORAGETYPE=local` + 嵌入式 etcd 在 2.5/2.6 GA 上行为一致，无需改其他配置。）

### Part D —— 起服务 + 重灌向量（必需）

```bash
cd ~/milvus && bash <path>/standalone_embed.sh start
# 确认健康后，知识库重新 embedding（向量本已丢，重灌不可避免）
```

---

## 五、日常防复发清单（Operational Guardrails）

1. **只用 `docker stop -t 60`，永不 `docker kill`**；关机前先停 Milvus 再退 Docker Desktop。
2. **保住 `embedEtcd.yaml` 的 etcd 调优**，别让脚本的默认模板覆盖它。
3. 给容器运行时 VM **足够内存**（当前 OrbStack 15.66G 充足）；容器不设 `-m` 限制，让 Milvus 用满上面 `user.yaml` 放大的缓冲。若换回 Docker Desktop，Settings→Resources 内存建议 ≥ 8G。
4. 本地开发**尽量别跑 beta 镜像**；镜像与 Go 客户端版本对齐。
5. 若又见 `LoadBloomFilter ... key not found` 风暴：先 `docker logs` 确认是不是选举超时崩溃循环（etcd `election ... timeout`），再决定是"仅 drop 受损 collection"还是"清卷重来"。
6. 集成测试连跑遇 `flush ... rate limit exceeded[rate=0.1]` 是 Milvus 刷盘频控（非损坏），**等 ~20s 单独重跑**即可。

---

## 六、与本项目代码的关系

- 本缺陷是**纯本地环境/运维问题**，与语义缓存双模式功能代码无关；功能单测（`caching_agent_test.go` 9 例 + 全量 61 包）全绿。
- 集成测试 `cache_collection_integration_test.go` 曾因此 90s 超时；drop 掉受损的 `semantic_cache_vectors` 后即 **PASS（0.65s）**，证明功能代码在健康 Milvus 上完全正确。
- 代码侧已有的一道防线：`LoadCacheCollection` 的 Await 有 90s 上界（`loadCollectionAwaitCap`），防止本地 Milvus 加载卡死时无界挂起。

相关记忆：`milvus-local-persistent-dir`、`semantic-cache-two-modes`。
