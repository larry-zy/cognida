# 评测数据集格式说明

## 数据集目录结构

```
evaluation/datasets/{dataset_id}/
├── meta.json       # 元信息
└── samples.jsonl   # 数据样本
```

对于 RAG 类型的数据集，还可以包含：
```
├── corpus.jsonl    # 文档语料 (可选)
```

## meta.json 格式

```json
{
  "dataset_id": "数据集ID，与目录名相同",
  "name": "数据集名称",
  "description": "数据集描述",
  "fields": ["字段1", "字段2", ...],
  "metadata": {
    "version": "1.0",
    "type": "llm|agent|rag",
    "created_at": "2026-05-03"
  }
}
```

## samples.jsonl 格式

每行一个 JSON 对象，表示一个样本。

### LLM 问答数据集

```json
{"question": "问题内容", "answer": "参考答案"}
```

### Agent 评测数据集

```json
{
  "question": "问题内容",
  "answer": "期望的最终答案",
  "expected_tools": ["tool1", "tool2"],
  "expected_steps": ["步骤1", "步骤2"]
}
```

### RAG 评测数据集

```json
{
  "question": "问题内容",
  "answer": "参考答案",
  "relevant_docs": ["doc_id_1", "doc_id_2"]
}
```

## corpus.jsonl 格式 (RAG)

每行一个文档对象：

```json
{"doc_id": "文档ID", "content": "文档内容"}
```

## 示例数据集

| 数据集ID | 类型 | 描述 |
|---------|------|------|
| sample_qa | llm | 简单问答数据集 |
| sample_agent | agent | 包含工具调用的Agent数据集 |
| sample_rag | rag | 包含文档语料的RAG数据集 |
