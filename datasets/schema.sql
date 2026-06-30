-- ============================================================
-- 评测系统数据库 Schema
-- ============================================================

-- ============================================================
-- 评测任务表
-- ============================================================
CREATE TABLE IF NOT EXISTS judge_tasks (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id         BIGINT           NOT NULL COMMENT '用户ID',
    mode            VARCHAR(20)      NOT NULL COMMENT '评测模式: llm/agent/rag',
    dataset_id      VARCHAR(100)     NOT NULL COMMENT '数据集ID',
    model_id        VARCHAR(100)     NOT NULL COMMENT '模型ID',

    -- 评测配置
    metrics_config  JSON             NOT NULL COMMENT '评测指标配置',
    llm_judge_config JSON                 COMMENT 'LLM-as-Judge配置',

    -- 任务状态
    status          VARCHAR(20)      NOT NULL DEFAULT 'PENDING' COMMENT '状态: PENDING/QUEUED/INFERRING/EVALUATING/COMPLETED/FAILED',
    progress        INT              NOT NULL DEFAULT 0 COMMENT '进度 0-100',
    current_stage   VARCHAR(50)          COMMENT '当前阶段描述',

    -- 统计信息
    total_samples   INT              NOT NULL DEFAULT 0 COMMENT '总样本数',
    evaluated_samples INT             NOT NULL DEFAULT 0 COMMENT '已评测样本数',
    failed_samples  INT              NOT NULL DEFAULT 0 COMMENT '失败样本数',

    -- LLM 推理统计
    total_tokens    INT              NOT NULL DEFAULT 0 COMMENT '总token数',
    input_tokens    INT              NOT NULL DEFAULT 0 COMMENT '输入token数',
    output_tokens   INT              NOT NULL DEFAULT 0 COMMENT '输出token数',
    estimated_cost  DECIMAL(10, 4)   NOT NULL DEFAULT 0 COMMENT '预估成本(USD)',

    -- 错误信息
    error_message   TEXT                 COMMENT '失败原因',
    error_details   JSON                 COMMENT '错误详情',

    -- 时间戳
    created_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at      DATETIME             COMMENT '开始执行时间',
    completed_at    DATETIME             COMMENT '完成时间',
    updated_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_mode (mode),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评测任务表';

-- ============================================================
-- 评测结果汇总表
-- ============================================================
CREATE TABLE IF NOT EXISTS judge_results (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id         BIGINT           NOT NULL COMMENT '任务ID',
    mode            VARCHAR(20)      NOT NULL COMMENT '评测模式',

    -- LLM 指标
    rouge_1         DECIMAL(5, 4)        COMMENT 'ROUGE-1 分数',
    rouge_2         DECIMAL(5, 4)        COMMENT 'ROUGE-2 分数',
    rouge_l         DECIMAL(5, 4)        COMMENT 'ROUGE-L 分数',
    bleu_1          DECIMAL(5, 4)        COMMENT 'BLEU-1 分数',
    bleu_2          DECIMAL(5, 4)        COMMENT 'BLEU-2 分数',
    bleu_4          DECIMAL(5, 4)        COMMENT 'BLEU-4 分数',
    semantic_sim    DECIMAL(5, 4)        COMMENT '语义相似度',

    -- Agent 指标
    answer_accuracy DECIMAL(5, 4)        COMMENT '答案准确率',
    tool_precision  DECIMAL(5, 4)        COMMENT '工具选择精确率',
    tool_recall     DECIMAL(5, 4)        COMMENT '工具选择召回率',
    tool_f1         DECIMAL(5, 4)        COMMENT '工具选择F1',
    traj_exact_match DECIMAL(5, 4)       COMMENT '轨迹完全匹配率',
    traj_similarity DECIMAL(5, 4)        COMMENT '轨迹相似度',
    step_ratio      DECIMAL(5, 4)        COMMENT '步骤效率比率',
    avg_steps       DECIMAL(5, 2)        COMMENT '平均步骤数',

    -- RAG 指标
    retrieval_p1    DECIMAL(5, 4)        COMMENT 'Precision@1',
    retrieval_p5    DECIMAL(5, 4)        COMMENT 'Precision@5',
    retrieval_r10   DECIMAL(5, 4)        COMMENT 'Recall@10',
    retrieval_ndcg  DECIMAL(5, 4)        COMMENT 'NDCG@10',
    retrieval_mrr   DECIMAL(5, 4)        COMMENT 'MRR',
    faithfulness    DECIMAL(5, 4)        COMMENT '忠实度',
    context_relevance DECIMAL(5, 4)      COMMENT '上下文相关性',
    noise_ratio     DECIMAL(5, 4)        COMMENT '噪声比例',

    -- LLM-as-Judge 指标
    llm_judge_accuracy    DECIMAL(3, 2)  COMMENT 'Judge: 准确性评分',
    llm_judge_completeness DECIMAL(3, 2) COMMENT 'Judge: 完整性评分',
    llm_judge_clarity     DECIMAL(3, 2)  COMMENT 'Judge: 清晰度评分',
    llm_judge_relevance   DECIMAL(3, 2)  COMMENT 'Judge: 相关性评分',
    llm_judge_reasoning   DECIMAL(3, 2)  COMMENT 'Judge: 推理能力评分',
    llm_judge_tool_use    DECIMAL(3, 2)  COMMENT 'Judge: 工具使用评分',
    llm_judge_efficiency  DECIMAL(3, 2)  COMMENT 'Judge: 效率评分',
    llm_judge_correctness DECIMAL(3, 2)  COMMENT 'Judge: 正确性评分',
    llm_judge_faithfulness DECIMAL(3, 2) COMMENT 'Judge: 忠实度评分',

    -- 扩展字段 (JSON 存储其他自定义指标)
    metrics_json    JSON                 COMMENT '其他指标JSON',

    created_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uk_task_id (task_id),
    INDEX idx_mode (mode)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='评测结果汇总表';

-- ============================================================
-- 样本结果详情表
-- ============================================================
CREATE TABLE IF NOT EXISTS judge_sample_results (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id         BIGINT           NOT NULL COMMENT '任务ID',
    sample_index    INT              NOT NULL COMMENT '样本索引',

    -- 输入输出
    question        TEXT             NOT NULL COMMENT '问题',
    reference       TEXT                 COMMENT '参考答案',
    output          TEXT             NOT NULL COMMENT '模型输出',

    -- LLM 指标 (样本级别)
    rouge_1         DECIMAL(5, 4)        COMMENT 'ROUGE-1',
    rouge_l         DECIMAL(5, 4)        COMMENT 'ROUGE-L',
    bleu_4          DECIMAL(5, 4)        COMMENT 'BLEU-4',
    semantic_sim    DECIMAL(5, 4)        COMMENT '语义相似度',

    -- Agent 特有字段
    trajectory      JSON                 COMMENT 'Agent轨迹',
    tools_expected  JSON                 COMMENT '期望使用的工具',
    tools_used      JSON                 COMMENT '实际使用的工具',
    tool_match_score DECIMAL(5, 4)       COMMENT '工具匹配分数',
    traj_match_score DECIMAL(5, 4)       COMMENT '轨迹匹配分数',
    steps_count     INT                  COMMENT '实际步骤数',
    optimal_steps   INT                  COMMENT '最优步骤数',

    -- RAG 特有字段
    retrieved_docs  JSON                 COMMENT '检索到的文档ID列表',
    relevant_docs   JSON                 COMMENT '相关文档ID列表',
    retrieval_score DECIMAL(5, 4)        COMMENT '检索分数',

    -- LLM-as-Judge 评分 (样本级别)
    llm_judge_scores JSON                COMMENT 'LLM Judge各维度评分',

    -- 扩展字段
    metrics_json    JSON                 COMMENT '其他指标JSON',

    created_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_task_id (task_id),
    INDEX idx_sample_index (sample_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='样本结果详情表';

-- ============================================================
-- Agent 轨迹详情表 (可选，用于详细分析)
-- ============================================================
CREATE TABLE IF NOT EXISTS judge_agent_trajectories (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id         BIGINT           NOT NULL COMMENT '任务ID',
    sample_index    INT              NOT NULL COMMENT '样本索引',
    step_number     INT              NOT NULL COMMENT '步骤序号',

    action_type     VARCHAR(20)      NOT NULL COMMENT '动作类型: thought/tool_call/observation/finish',
    tool_name       VARCHAR(50)          COMMENT '工具名称',
    tool_input      TEXT                 COMMENT '工具输入',
    content         TEXT                 COMMENT '内容/结果',

    created_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_task_sample (task_id, sample_index),
    INDEX idx_step (step_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Agent轨迹详情表';

-- ============================================================
-- RAG 检索文档详情表 (可选，用于检索分析)
-- ============================================================
CREATE TABLE IF NOT EXISTS judge_rag_retrieval (
    id              BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_id         BIGINT           NOT NULL COMMENT '任务ID',
    sample_index    INT              NOT NULL COMMENT '样本索引',
    doc_id          VARCHAR(100)     NOT NULL COMMENT '文档ID',
    rank            INT              NOT NULL COMMENT '检索排名',

    is_relevant     BOOLEAN          NOT NULL DEFAULT FALSE COMMENT '是否相关',
    relevance_score DECIMAL(5, 4)        COMMENT '相关性分数',
    doc_content     TEXT                 COMMENT '文档内容快照',

    created_at      DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_task_sample (task_id, sample_index),
    INDEX idx_doc_id (doc_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='RAG检索文档详情表';
