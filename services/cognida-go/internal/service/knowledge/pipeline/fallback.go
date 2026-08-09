// Package rag 提供 RAG 基础设施层实现。
//
// 本文件收敛检索读路径上的「降级/兜底」逻辑：当某个可选增强环节（重排等）失败时，
// 绝不因该环节出错而清空已命中的候选，而是记录并回退到未增强前的结果。历史上这类
// 兜底散落在多个检索方法里各写一遍（filtered, _ = Rerank(...)），一旦 Rerank 报错
// 就把 filtered 静默置空 → 空答案。集中到此处后语义单一、行为一致、便于复用。
package rag

import (
	"context"
	"log"

	domainrag "cognida/internal/model/rag"
)

// rerankOrKeep 应用重排；失败时记录并回退到重排前的原顺序，绝不因重排失败清空命中。
//
// 调用方只需 `filtered = rerankOrKeep(ctx, r.reranker, filtered, query)`，无需再各自
// 处理 error——这正是原先散落三处的 `filtered, _ = r.reranker.Rerank(...)` 的隐患：
// 忽略 error 的同时把返回值（出错时为 nil）覆盖回 filtered，等于把好好的候选清空。
func rerankOrKeep(ctx context.Context, reranker domainrag.Reranker, docs []*domainrag.Document, query string) []*domainrag.Document {
	if reranker == nil || len(docs) == 0 {
		return docs
	}
	reranked, err := reranker.Rerank(ctx, docs, query)
	if err != nil {
		log.Printf("[retriever] rerank failed, keep pre-rerank order (%d docs): %v", len(docs), err)
		return docs
	}
	if len(reranked) == 0 {
		// 重排成功但产出空集通常是策略实现问题；保留原候选而非返回空答案。
		log.Printf("[retriever] rerank returned empty, keep pre-rerank order (%d docs)", len(docs))
		return docs
	}
	return reranked
}
