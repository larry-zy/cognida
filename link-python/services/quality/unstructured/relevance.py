"""主题相关性评估器。

评估文本与预期主题的相关性。
"""

import re
from typing import Any

import jieba

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator


@register_evaluator("relevance")
class RelevanceEvaluator(UnstructuredEvaluator):
    """主题相关性评估器。

    评估文本是否与预期主题相关。
    """

    dimension_name = "relevance"
    description = "关键词匹配和主题分类"

    # 简单的主题分类词典
    TOPIC_KEYWORDS = {
        "科技": ["技术", "科技", "互联网", "软件", "硬件", "AI", "算法", "数据", "编程", "开发"],
        "金融": ["金融", "银行", "证券", "基金", "股票", "投资", "理财", "保险", "贷款", "利率"],
        "医疗": ["医疗", "健康", "医院", "医生", "药物", "治疗", "疾病", "症状", "诊断", "护理"],
        "教育": ["教育", "学校", "老师", "学生", "学习", "课程", "培训", "考试", "知识", "研究"],
        "体育": ["体育", "运动", "比赛", "足球", "篮球", "运动员", "教练", "冠军", "联赛", "奥运"],
        "娱乐": ["娱乐", "电影", "音乐", "明星", "演员", "歌手", "综艺", "节目", "影视", "演出"],
    }

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本主题相关性。

        Args:
            text: 要评估的文本
            config: 配置参数，可包含 expected_keywords, expected_topics

        Returns:
            主题相关性评分
        """
        config = config or {}
        issues: list[TextQualityIssue] = []
        details: dict[str, Any] = {}

        if not text:
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=0.0,
                passed=False,
                issues=[
                    TextQualityIssue(
                        type="empty_text",
                        severity=SeverityLevel.CRITICAL,
                        description="文本为空",
                    )
                ],
                details={},
            )

        # 分词
        words = list(jieba.cut(text))
        words_set = set(words)

        # 关键词匹配
        expected_keywords = config.get("expected_keywords") or config.get("keywords", [])
        if expected_keywords:
            # 使用子串匹配而不是分词集合匹配，更准确
            matched_keywords = [kw for kw in expected_keywords if kw in text]
            match_rate = len(matched_keywords) / len(expected_keywords) if expected_keywords else 0

            details["keyword_matches"] = list(matched_keywords)  # 使用测试期望的键名
            details["keyword_match_rate"] = match_rate

            if match_rate < 0.3:
                issues.append(
                    TextQualityIssue(
                        type="low_keyword_match",
                        severity=SeverityLevel.WARNING,
                        description=(
                            f"关键词匹配率过低: {match_rate:.1%} "
                            f"({len(matched_keywords)}/{len(expected_keywords)})"
                        ),
                    )
                )
        else:
            match_rate = 1.0  # 没有关键词要求时默认满分

        # 主题分类
        topic_scores = self._classify_topic(words_set)
        detected_topic = max(topic_scores, key=topic_scores.get) if topic_scores else "未知"
        details["detected_topic"] = detected_topic
        details["topic_scores"] = topic_scores

        # 检查是否符合预期主题
        expected_topics = config.get("expected_topics", [])
        if expected_topics:
            if detected_topic not in expected_topics:
                issues.append(
                    TextQualityIssue(
                        type="topic_mismatch",
                        severity=SeverityLevel.WARNING,
                        description=(
                            f"检测到的主题 '{detected_topic}' 与预期主题 "
                            f"{expected_topics} 不匹配"
                        ),
                        suggestion=f"预期主题: {', '.join(expected_topics)}",
                    )
                )

        # 计算分数
        base_score = 100.0

        # 关键词匹配影响分数
        if expected_keywords:
            base_score = base_score * match_rate

        # 主题匹配影响分数
        if expected_topics and detected_topic not in expected_topics:
            base_score *= 0.6

        for issue in issues:
            if issue.severity == SeverityLevel.WARNING:
                base_score -= 15

        score = max(0.0, min(100.0, base_score))

        threshold = config.get("threshold", 60)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def _classify_topic(self, words: set[str]) -> dict[str, float]:
        """对文本进行主题分类。

        Args:
            words: 词汇集合

        Returns:
            各主题的得分字典
        """
        topic_scores = {}

        for topic, keywords in self.TOPIC_KEYWORDS.items():
            # 计算匹配的关键词数
            matched = len(words & set(keywords))
            # 计算得分
            score = matched / len(keywords) if keywords else 0
            topic_scores[topic] = score

        return topic_scores

    def extract_keywords(self, text: str, top_n: int = 10) -> list[str]:
        """提取文本关键词。

        Args:
            text: 文本内容
            top_n: 返回前N个关键词

        Returns:
            关键词列表
        """
        import jieba.analyse

        keywords = jieba.analyse.extract_tags(text, topK=top_n, withWeight=False)
        return keywords
