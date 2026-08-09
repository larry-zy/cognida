"""重复度评估器。

评估文本与已有内容的相似度。
"""

from typing import Any

from datasketch import MinHash, MinHashLSH

from .base import UnstructuredEvaluator
from ..models import SeverityLevel, TextQualityIssue, UnstructuredDimensionScore
from ..registry import register_evaluator


@register_evaluator("duplication")
class DuplicationEvaluator(UnstructuredEvaluator):
    """重复度评估器。

    评估文本与参考内容的相似度，支持精确重复和近似重复检测。
    """

    dimension_name = "duplication"
    description = "精确重复检测和近似重复检测"

    def __init__(self) -> None:
        """初始化重复度评估器。"""
        super().__init__()
        self._reference_texts: list[str] = []
        self._lsh: MinHashLSH | None = None
        self._minhashes: list[tuple[int, MinHash]] = []

    def set_references(self, texts: list[str]) -> None:
        """设置参考文本集合。

        Args:
            texts: 参考文本列表
        """
        self._reference_texts = texts
        self._build_lsh_index(texts)

    def _build_lsh_index(self, texts: list[str]) -> None:
        """构建 LSH 索引。

        Args:
            texts: 文本列表
        """
        self._lsh = MinHashLSH(
            threshold=0.5,  # Jaccard 相似度阈值
            num_perm=128,  # 排列数
        )
        self._minhashes = []

        for idx, text in enumerate(texts):
            minhash = self._create_minhash(text)
            self._minhashes.append((idx, minhash))
            self._lsh.insert(idx, minhash)

    def _create_minhash(self, text: str, num_perm: int = 128) -> MinHash:
        """创建 MinHash 签名。

        Args:
            text: 文本内容
            num_perm: 排列数

        Returns:
            MinHash 对象
        """
        # 使用 5-gram 分词
        words = text.lower().split()
        shingles = []

        for i in range(len(words) - 4):
            shingle = " ".join(words[i:i+5])
            shingles.append(hash(shingle))

        minhash = MinHash(num_perm=num_perm)
        for shingle in shingles:
            minhash.update(shingle)

        return minhash

    def _calculate_jaccard_similarity(self, text1: str, text2: str) -> float:
        """计算 Jaccard 相似度。

        Args:
            text1: 文本1
            text2: 文本2

        Returns:
            Jaccard 相似度 (0-1)
        """
        words1 = set(text1.lower().split())
        words2 = set(text2.lower().split())

        if not words1 or not words2:
            return 0.0

        intersection = words1 & words2
        union = words1 | words2

        return len(intersection) / len(union) if union else 0.0

    def _detect_internal_duplicates(self, text: str) -> int:
        """检测文本内部的重复句子。

        Args:
            text: 文本内容

        Returns:
            重复句子的数量
        """
        import re

        # 按句号、问号、感叹号、换行符分句
        sentences = re.split(r'[。！？\n.!?]+', text)
        sentences = [s.strip() for s in sentences if s.strip()]

        if len(sentences) < 2:
            return 0

        # 检测重复句子
        seen = set()
        duplicates = 0
        for sentence in sentences:
            # 标准化（去除空格、转小写）
            normalized = sentence.lower().strip()
            if normalized in seen:
                duplicates += 1
            else:
                seen.add(normalized)

        return duplicates

    def evaluate(
        self,
        text: str,
        config: dict[str, Any] | None = None,
    ) -> UnstructuredDimensionScore:
        """评估文本重复度。

        Args:
            text: 要评估的文本
            config: 配置参数，可包含 reference_texts, similarity_threshold

        Returns:
            重复度维度评分
        """
        config = config or {}
        issues: list[TextQualityIssue] = []
        details: dict[str, Any] = {}

        if not text:
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=100.0,  # 空文本不参与重复度评估
                passed=True,
                issues=[],
                details={},
            )

        # 获取参考文本
        reference_texts = config.get("reference_texts", self._reference_texts)
        if not reference_texts:
            # 没有参考文本时，检测文本内部的重复（句子级重复）
            internal_duplicates = self._detect_internal_duplicates(text)
            if internal_duplicates > 0:
                issues.append(
                    TextQualityIssue(
                        type="internal_duplicate",
                        severity=SeverityLevel.WARNING,
                        description=f"文本包含 {internal_duplicates} 处重复内容",
                    )
                )
                # 根据重复程度计算分数
                score = max(50.0, 100 - internal_duplicates * 20)
                details = {
                    "reference_count": 0,
                    "internal_duplicates": internal_duplicates,
                    "duplication_ratio": internal_duplicates / max(1, len(text.split("。"))),
                }
            else:
                # 无重复
                details = {"reference_count": 0, "internal_duplicates": 0}
                score = 100.0

            threshold = config.get("threshold", 70)
            passed = self.is_passed(score, threshold)

            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=score,
                passed=passed,
                issues=issues,
                details=details,
            )

        similarity_threshold = config.get("similarity_threshold", 0.85)
        details["reference_count"] = len(reference_texts)

        # 精确重复检测
        exact_matches = [i for i, ref in enumerate(reference_texts) if text == ref]
        if exact_matches:
            issues.append(
                TextQualityIssue(
                    type="exact_duplicate",
                    severity=SeverityLevel.CRITICAL,
                    description=f"文本与 {len(exact_matches)} 个参考文本完全重复",
                )
            )
            return UnstructuredDimensionScore(
                name=self.dimension_name,
                score=0.0,
                passed=False,
                issues=issues,
                details=details,
            )

        # 近似重复检测
        max_similarity = 0.0
        max_sim_idx = -1

        for idx, ref in enumerate(reference_texts):
            similarity = self._calculate_jaccard_similarity(text, ref)
            if similarity > max_similarity:
                max_similarity = similarity
                max_sim_idx = idx

        details["max_similarity"] = max_similarity
        details["most_similar_index"] = max_sim_idx

        if max_similarity >= similarity_threshold:
            issues.append(
                TextQualityIssue(
                    type="near_duplicate",
                    severity=SeverityLevel.WARNING,
                    description=(
                        f"文本与参考文本 #{max_sim_idx} 高度相似 "
                        f"(相似度: {max_similarity:.2%} >= {similarity_threshold:.2%})"
                    ),
                )
            )

        # 计算分数（相似度越高，分数越低）
        score = max(0.0, 100 * (1 - max_similarity))

        threshold = config.get("threshold", 70)
        passed = self.is_passed(score, threshold)

        return UnstructuredDimensionScore(
            name=self.dimension_name,
            score=score,
            passed=passed,
            issues=issues,
            details=details,
        )

    def find_duplicates(
        self, texts: list[str], threshold: float = 0.85
    ) -> list[tuple[int, int, float]]:
        """批量查找重复文本。

        Args:
            texts: 要检查的文本列表
            threshold: 相似度阈值

        Returns:
            重复对列表 [(idx1, idx2, similarity), ...]
        """
        duplicates = []

        # 构建 LSH 索引
        self._build_lsh_index(texts)

        for i, text1 in enumerate(texts):
            minhash1 = self._create_minhash(text1)

            # 使用 LSH 查找候选
            candidates = set()
            if self._lsh:
                try:
                    candidates = self._lsh.query(minhash1)
                except Exception:
                    pass

            for j in candidates:
                if i < j:  # 避免重复比较
                    similarity = self._calculate_jaccard_similarity(texts[i], texts[j])
                    if similarity >= threshold:
                        duplicates.append((i, j, similarity))

        # 按相似度降序排序
        duplicates.sort(key=lambda x: x[2], reverse=True)
        return duplicates
