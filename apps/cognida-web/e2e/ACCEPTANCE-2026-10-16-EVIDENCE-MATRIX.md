# 固定样本、质量门槛与文件格式验收

样本为开发者人工定义的合成案例。分别记录流程执行、标签质量、检索质量与研究证据质量，不以流程成功代替质量评测，也不将固定案例通过率视为生产数据分布下的准确率。

## 固定样本和门槛（OpenSpec 1.4）

| 维度 | 固定输入与期望 | 验收门槛 | 自动证据 |
| --- | --- | --- | --- |
| 流程执行 | North/100/January、South/200/February、West/300/March | 七阶段真实成功；审批前无发布；发布后真实回执、报告和原始精确字符串 | lifecycle.spec.ts、document-formats.spec.ts |
| 数值报告 | 收入100、200、300 | record_count=3、numeric_count=3、均值200；不猜测文本列为数值 | lifecycle.spec.ts、SQL类型真实DB回归 |
| 分类/实体/知识标签 | `😀北京订单营收增长。`；销售关键词`营收`，城市词条`北京` | 依据逐字命中；实体偏移可重放；候选待审核；规则无伪造模型置信度 | domain-specialists.spec.ts、治理单元/DB回归 |
| 人工QA | `北京是什么？`；人工改答案为`北京是中国的首都，也是直辖市。` | 审核导出保留人工答案；再次预标注不覆盖人工/已审结果 | domain-specialists.spec.ts |
| 离线答案评分 | 两行答案一同一异，包含`00123` | exact_match分别100/0；model_calls=0；模式明确offline_existing_answers | durable-domains.spec.ts |
| 检索 | 唯一合成标记、指定授权KB和真实文档ID | Top20命中文档/标记；sources含milvus_vector，不以database_lexical冒充；撤权不可读 | verifyVectorRetrieval、知识检索真实DB/API回归 |
| 研究持久执行 | 授权KB三地区事实 | 有来源和主张；semantic_review=true、fact_checked=false；模型调用≤4；刷新报告不变不重生成 | durable-domains.spec.ts |
| 研究证据质量 | 下表人工案例 | 引文存在不等于支持；不将冲突、转载、无来源包装成事实已核实 | research_evidence_test.go、research_test.go |

标签和检索门槛是固定案例的精确断言，不代表一般模型精确率、召回率或泛化能力。这些指标需要另行扩充人工标注数据集。

## 研究证据案例（OpenSpec 10.3）

| 案例 | 人工期望 | 检查 |
| --- | --- | --- |
| 主张和原文均为“Paris is the capital of France.” | supported / supports | 引文逐字命中，仍fact_checked=false |
| 主张罗马是法国首都，引文是巴黎博物馆 | insufficient / context | 真实但无关的引文不等于支持 |
| 主张试验改善生存，原文“did not improve survival” | conflict / contradicts | 不允许同时supported和contradicts |
| 主张全部患者康复，原文2025年10人中的2人 | insufficient / context | 范围、时间和数量约束保留 |
| 不存在的引文或主张ID | 拒绝审查输出 | 不设置语义审查成功 |
| 两站转载同一段、同URL不同追踪参数 | 同一独立性分组 | 转载不计独立支持，独立反证单独分组 |
| 无来源、供应商fallback | 不成功或明确部分失败 | 不生成空证据成功报告，不采信fallback |
| 第三次模型调用、首轮语义审查失败 | failed；保留已有证据，不发布可用报告产物 | 记录调用数3（人工给定模型响应），无自动付费重放，错误脱敏 |
| 可解析初稿缺必填字段或必需引用，预算内修复成功 | 一次结构修复后仍须独立语义审查；共4次模型调用，停止无预算的补查 | 先持久预留2个名额；取消不调用；全部原有引用先核对，缺引用与伪引文共存也不能混入结构修复；修复/首次审查无效均failed |
| 首轮审查有效、可选补充审查失败 | partial、success=false；保留首轮历史和全部来源，当前支持结论降为待核实，冲突保留 | 重新核对首轮来源摘要、逐字引文及当前权限；新增未审来源单列，取消/超时/撤权不降为可用部分报告 |

语义单测使用人工给定模型响应验证校验器和失败处理，**不是实际模型质量测试**。真实模型页面案例另行验证授权检索、规划、综合、审查及持久恢复，模型判断仍需人工复核。

## 文件格式矩阵

`document-fixtures.py`生成真实字节，PDF/OOXML并非改后缀的文本。每例有独立标记及三地区事实；DOCX表格、XLSX第二工作表和PPTX表格有必须保留的文字；`00123`及`9007199254740993.123456789012345678`作为原始文本保留。

| 格式 | 支持范围与特别断言 | 真实页面状态 |
| --- | --- | --- |
| TXT | UTF-8正文，事实及标记 | PASS |
| Markdown | UTF-8 Markdown正文 | PASS |
| PDF | 可提取文字层；无正文PDF明确failed且0分块 | PASS |
| DOCX | 文档顺序段落+表格；不受extract_tables=false影响 | PASS |
| XLSX | 默认全部工作表，显式sheet参数仍限定；第二sheet和精确字符串 | PASS |
| CSV | UTF-8多行与前导零 | PASS |
| HTML | 离线正文，不执行脚本/下载资源；脚本不入正文 | PASS |
| PPTX | OOXML幻灯片及表格文字 | PASS |

.markdown/.htm为文本别名。旧二进制.doc/.xls/.ppt上传明确400并提示转换。本轮不声称实现旧Office转换、扫描PDF自动OCR、图片理解或PDF表格结构重建。独立OCR能力不等于上传页面已接自动OCR。

## 真实 HTTPS JSON 正向采集（OpenSpec 7.3）

公开 JSONPlaceholder 测试服务的 `/posts?_limit=3` 返回合成记录。通过真实页面登记一次性合成 Bearer、固定来源版本，实际 worker 发起 1 次 HTTPS 请求，保留 1 页、3 条记录和摘要。原始数字 ID 保留为字符串 `1/2/3` 并带 `NUMERIC` 元数据。

实际 `run.output_refs` 中的 raw candidate 被页面选入七阶段；没有二次导入或人工补写状态。审批前无发布，人工批准后得到真实 `versioned_object_store` 发布回执和 `record_count=3` 分析报告。`api-sources-positive.spec.ts` 首轮 **6.7 秒通过（总 7.6 秒）**；最终报告区域截图复验 **8.8 秒通过（总 9.8 秒）**，截图和附件在 `/private/tmp/cognida-complete-20261016/e2e-api-source-positive-report`。截图已目检，报告显示 3 行，`id` 数值数量为 3、均值为 2、范围为 1–3。

该公网服务不要求或校验 Bearer；此结果证明真实 HTTPS JSON 采集和后续审批报告链路，**不证明生产第三方鉴权**。认证头、同 origin 限制、游标／快照、重试和凭据反射拒绝由独立真实 TLS 集成 fixture 验证。公网来源没有快照令牌，页面如实显示只保证实际读取范围，未宣称跨页一致性。

最终综合场景显式配置3次总请求预算。只对零进度且回执允许继续的partial，在原截止时间内通过UI继续，最多两次；逐次保存失败和恢复回执，全部采集共用原140秒观察上限。成功后仍必须只有1页、3条精确记录，随后走原七阶段、审批和报告断言。若首次成功，则不能据此声称本次触发了恢复分支；确定性恢复边界由真实TLS/数据库集成验证。

## 证据状态和清理

- Go上传格式/API与空正文定向race通过；Python真实OOXML及文档合同17项通过；前端上传/目录过滤18项、类型检查和E2E类型检查通过。
- 研究证据4个定向race独立通过；实际模型页面证据由主控记录，与fake-model单测分开。
- 8格式实际页面最终 **1 testcase passed，8种格式和无正文PDF负例全部通过，52.7秒（总53.2秒）**。每种格式经UI上传→Python真实解析→knowledge深链→七阶段→明确审批→发布→报告→真实Milvus命中；没有API拦截或人工改终态。
- 首轮TXT七阶段和报告成功，检索仅以随机标记查询且遇MySQL降级结果时提前退出，因此未通过。最终使用标记加正文事实查询，轮询只在真实目标文档具有`milvus_vector`来源时通过；保留60秒上限，SQL降级仍会失败。没有放松检索后端断言。
- 截图保存在隔离运行目录`/private/tmp/cognida-complete-20261016/e2e-document-formats-vector`，包含8种格式报告。专用MySQL/Redis/Go/Python；共享Milvus使用专用租户和知识库，凭据不进仓库或输出。
- 最终综合E2E 37/37通过后，专用文档、向量、图节点、隔离数据库及本轮服务均已清理；共享集合、原有服务和其他租户数据保留。12次反馈撤回在环境删除前已单独验证文档/分块/向量为0。见[最终综合证据](evidence/2026-10-16-complete-loop.json)与[清理回执](evidence/2026-10-16-cleanup.json)。
