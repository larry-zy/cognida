# Scripts

## MySQL

`ecommerce_demo_schema.sql` - 电商演示库 (ecommerce_demo) 30 张表建表脚本；为 `cmd/seed/ecommerce` 的 schema 镜像（手工建库/查阅用）。改表结构时与 `cmd/seed/ecommerce/main.go` 的 `createSchema()` 同步。

## Neo4j

`create_neo4j_indexes.cypher` - Neo4j 知识图谱索引创建脚本

## 测试脚本

- `test-agent-flow.sh` - Agent 流程集成测试
- `test-text2sql.sh` - Text2SQL 功能测试

## Milvus 本地路径诊断

`milvus_local_paths.py` 默认只读诊断及真实查询验收；修复宿主机路径别名必须显式 `--apply`，校验容器停止、固定挂载范围和完整备份。不会重建集合或修改元数据。用法、持久挂载及回滚见 [运维手册](MILVUS_LOCAL_STORAGE.md)。
