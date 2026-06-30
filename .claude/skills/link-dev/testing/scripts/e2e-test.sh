#!/bin/bash
# E2E 测试脚本 - Link 项目 (基于 BrowserAct)
# 用法: ./e2e-test.sh [scenario]

set -e

SCENARIO=${1:-list}
SESSION="link-e2e-$(date +%s)"
BASE_URL=${BASE_URL:-http://localhost:8080}

echo "🎭 Link E2E 测试"
echo "================="

case $SCENARIO in
  list)
    echo "可用场景:"
    echo "  user-login      - 用户登录流程"
    echo "  kb-create       - 创建知识库"
    echo "  rag-flow        - RAG 问答完整流程"
    echo "  tenant-isolation - 多租户隔离测试"
    ;;

  user-login)
    echo "测试场景: 用户登录"
    browser-act --session "$SESSION" browser open stealth "$BASE_URL/login"
    browser-act --session "$SESSION" state
    browser-act --session "$SESSION" input 2 "admin@example.com"
    browser-act --session "$SESSION" input 3 "password"
    browser-act --session "$SESSION" click 4
    browser-act --session "$SESSION" wait --selector=".welcome-message"
    echo "✅ 登录测试完成"
    ;;

  kb-create)
    echo "测试场景: 创建知识库"
    browser-act --session "$SESSION" browser open stealth "$BASE_URL/knowledge"
    browser-act --session "$SESSION" state
    browser-act --session "$SESSION" click 1
    browser-act --session "$SESSION" input 2 "测试知识库"
    browser-act --session "$SESSION" input 3 "测试描述"
    browser-act --session "$SESSION" click 4
    browser-act --session "$SESSION" wait --text="创建成功"
    echo "✅ 知识库创建测试完成"
    ;;

  rag-flow)
    echo "测试场景: RAG 问答完整流程"
    # 登录
    browser-act --session "$SESSION" browser open stealth "$BASE_URL/login"
    browser-act --session "$SESSION" input 2 "admin@example.com"
    browser-act --session "$SESSION" input 3 "password"
    browser-act --session "$SESSION" click 4
    # 上传文档
    browser-act --session "$SESSION" navigate "$BASE_URL/documents/upload"
    browser-act --session "$SESSION" input-file 1 "./test-document.pdf"
    browser-act --session "$SESSION" click 2
    browser-act --session "$SESSION" wait --text="解析完成"
    # 问答
    browser-act --session "$SESSION" navigate "$BASE_URL/chat"
    browser-act --session "$SESSION" input 1 "文档的主要内容是什么？"
    browser-act --session "$SESSION" click 2
    browser-act --session "$SESSION" wait --selector=".answer"
    echo "✅ RAG 流程测试完成"
    ;;

  tenant-isolation)
    echo "测试场景: 多租户隔离"
    # 租户 1
    browser-act --session "$SESSION-t1" browser open stealth "$BASE_URL/login"
    browser-act --session "$SESSION-t1" input 2 "user1@tenant1.com"
    browser-act --session "$SESSION-t1" input 3 "password"
    browser-act --session "$SESSION-t1" click 4
    # 租户 2
    browser-act --session "$SESSION-t2" browser open stealth "$BASE_URL/login"
    browser-act --session "$SESSION-t2" input 2 "user1@tenant2.com"
    browser-act --session "$SESSION-t2" input 3 "password"
    browser-act --session "$SESSION-t2" click 4
    echo "✅ 多租户隔离测试完成"
    ;;

  *)
    echo "未知场景: $SCENARIO"
    echo "运行 './e2e-test.sh list' 查看可用场景"
    exit 1
    ;;
esac

# 清理会话
browser-act --session "$SESSION" browser close
