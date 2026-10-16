import { randomUUID } from "node:crypto";
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
  type Locator,
} from "@playwright/test";
import {
  loginThroughUI,
  createInput,
  getData,
  attachJSON,
  verifyVectorRetrieval,
} from "./lifecycle.helpers";
async function write(
  api: APIRequestContext,
  path: string,
  data: unknown,
  method = "POST",
) {
  const r = await api.fetch(`/api/v1${path}`, {
    method,
    data,
    headers: { "Idempotency-Key": randomUUID() },
  });
  expect(r.ok(), `${path}: ${r.status()} ${await r.text()}`).toBe(true);
  return (await r.json()).data;
}
async function click(page: Page, path: string, name: string) {
  const wait = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === `/api/v1${path}` &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name, exact: true }).click();
  const r = await wait;
  expect(r.ok(), `${path}: ${r.status()} ${await r.text()}`).toBe(true);
  await expect(
    page.getByRole("button", { name: "刷新真实状态", exact: true }),
  ).toBeEnabled();
  return (await r.json()).data;
}
async function selectPaged(
  page: Page,
  select: Locator,
  value: string,
  moreLabel: string,
) {
  for (let i = 0; i < 100; i++) {
    if (
      await select
        .locator("option")
        .evaluateAll(
          (options, expected) =>
            options.some(
              (option) => (option as HTMLOptionElement).value === expected,
            ),
          value,
        )
    ) {
      await select.selectOption(value);
      return;
    }
    const more = page.getByRole("button", { name: moreLabel, exact: true });
    await expect(
      more,
      `Required option ${value} must be reachable through real pagination`,
    ).toBeVisible();
    await more.click();
    await expect(
      page.getByRole("button", { name: "刷新真实状态", exact: true }),
    ).toBeEnabled();
  }
  throw new Error(`Pagination exceeded 100 pages for ${value}`);
}
async function search(api: APIRequestContext, kb: string, query: string) {
  return write(api, "/knowledge/search", {
    query,
    kb_ids: [kb],
    retrieval_mode: "vector",
    top_k: 20,
    min_score: 0,
    enable_rerank: false,
  });
}
test("feedback real source → bounded cleaning → isolated paired validation → approval activation and withdrawal", async ({
  page,
  playwright,
}, info) => {
  test.skip(
    process.env.COGNIDA_E2E_FEEDBACK !== "1",
    "Requires isolated workers, real evaluation model, document reader and Milvus; never mocks external results",
  );
  test.setTimeout(720000);
  const api = await loginThroughUI(page, playwright);
  let kb = "";
  let feedback: any;
  const marker = `FBA-${randomUUID().slice(0, 8)}`;
  const names = ["ALPHA", "BETA", "GAMMA", "DELTA"];
  const answers = ["KORU731", "VELA842", "NOMA953", "SERA164"];
  const wrong = ["OLD119", "OLD228", "OLD337", "OLD446"];
  const body = (values: string[]) =>
    `This is the official synthetic ${marker} repair acceptance register. Treat these entries as the authoritative lookup for this isolated exercise.\n` +
    names
      .map(
        (n, i) =>
          `Project ${marker}-${n} has launch token ${values[i]}. The launch token is a case-sensitive exact identifier; return the identifier only when asked about ${marker}-${n}.`,
      )
      .join("\n");
  try {
    const created = await write(api, "/knowledge-bases", {
      name: `feedback-${marker}`,
      description: "Isolated synthetic feedback acceptance",
      is_public: false,
      graph_enabled: false,
    });
    kb = created.id;
    const uploaded = await api.post(
      `/api/v1/knowledge-bases/${kb}/knowledge/file`,
      {
        multipart: {
          file: {
            name: `baseline-${marker}.txt`,
            mimeType: "text/plain",
            buffer: Buffer.from(body(wrong)),
          },
        },
      },
    );
    expect(uploaded.ok(), await uploaded.text()).toBe(true);
    const knowledgeID = (await uploaded.json()).data.knowledge_id;
    await expect
      .poll(
        async () => {
          const d = await getData<any>(
            api,
            `/knowledge-bases/${kb}/knowledge/${knowledgeID}/status`,
          );
          return d.parse_status;
        },
        { timeout: 120000, intervals: [1000, 2000] },
      )
      .toBe("completed");
    const baseline = await createInput(
      page,
      {
        id: "feedback-knowledge",
        source: "knowledge",
        knowledge_id: knowledgeID,
      },
      `baseline-${marker}`,
    );
    const sourceRef = `${baseline.asset_id}:${baseline.version}`;
    const chunks = await getData<any>(
      api,
      `/knowledge-bases/${kb}/chunks?knowledge_id=${knowledgeID}&page_size=100`,
    );
    const chunkIDs = (chunks.items || chunks.chunks).map((c: any) => c.id);
    expect(chunkIDs.length).toBeGreaterThan(0);
    const qa = await write(api, "/lifecycle/assets", {
      name: `fixed-${marker}`,
      input: {
        kind: "table",
        columns: ["question", "answer"],
        rows: names.map((n, i) => [
          `What is the launch token of project ${marker}-${n}? Return only the exact token.`,
          answers[i],
        ]),
      },
    });
    const task = await write(api, "/annotation-tasks", {
      name: `fixed-${marker}`,
      type: "qa",
      source_ref_id: `${qa.asset_id}:${qa.version}`,
      labels: [],
      qa_mapping: {
        question_column: "question",
        reference_answer_column: "answer",
      },
    });
    await write(api, `/annotation-tasks/${task.id}/preannotate`, {});
    const rows = (await getData<any>(api, `/annotation-tasks/${task.id}/items`))
      .items;
    expect(rows).toHaveLength(4);
    const selections = [];
    for (const row of rows) {
      const edited = await write(
        api,
        `/annotation-tasks/${task.id}/items/${row.id}`,
        {
          expected_version: row.version,
          annotation: {
            ...row.annotation,
            evaluation_profile: {
              kind: "rag",
              source_refs: [sourceRef],
              relevant_chunk_ids: chunkIDs,
            },
          },
        },
        "PUT",
      );
      const reviewed = await write(
        api,
        `/annotation-tasks/${task.id}/items/${row.id}/review`,
        {
          expected_version: edited.version,
          decision: "approved",
          comment: "Synthetic ground truth is fixed before any repair run.",
        },
      );
      selections.push({
        task_id: task.id,
        item_id: row.id,
        expected_version: reviewed.version,
      });
    }
    const version = await write(api, "/evaluation/dataset-versions", {
      dataset_id: `feedback-${marker}`,
      name: `fixed-${marker}`,
      profile: "rag",
      samples: selections,
      split: { algorithm: "none" },
    });
    await page.goto("/feedback");
    const form = page.getByRole("form", { name: "记录质量反馈" });
    await form
      .getByLabel("反馈标题", { exact: true })
      .fill(`修复 ${marker} 的错误令牌`);
    await selectPaged(
      page,
      form.getByLabel(/^基线知识资产/),
      sourceRef,
      "加载更多资产",
    );
    await form.getByLabel(/^问题输出资产/).selectOption(sourceRef);
    await selectPaged(
      page,
      form.getByLabel(/^目标知识库/),
      kb,
      "加载更多知识库",
    );
    await selectPaged(
      page,
      form.getByLabel(/^固定评测版本/),
      version.id,
      "加载更多评测版本",
    );
    await form
      .getByLabel("原始问题", { exact: true })
      .fill(rows[0].annotation.qa.question);
    await form
      .getByLabel("发现的错误与修复目标", { exact: true })
      .fill(
        "Baseline launch tokens are obsolete. Replace all four with the independently reviewed exact identifiers.",
      );
    feedback = await click(page, "/feedback-cases", "登记反馈与固定门槛");
    expect(feedback.active_version_id).toBe("");
    await page
      .getByLabel("建议的完整修正文本", { exact: true })
      .fill(body(answers));
    feedback = await click(
      page,
      `/feedback-cases/${feedback.id}/corrections`,
      "保存人工修正草稿",
    );
    expect(feedback.corrections).toHaveLength(1);
    feedback = await click(
      page,
      `/feedback-cases/${feedback.id}/rework`,
      "创建并启动返工",
    );
    const round = feedback.rounds.at(-1);
    expect(round.task_id).toBeTruthy();
    expect(
      round.status,
      "Rework must start the persisted plan, not stop at draft",
    ).not.toBe("draft");
    let summary: any;
    await expect
      .poll(
        async () => {
          summary = await getData<any>(
            api,
            `/governed-tasks/${round.task_id}/supervisor/summary`,
          );
          return summary.terminal;
        },
        { timeout: 120000, intervals: [1000, 2000] },
      )
      .toBe(true);
    expect(summary.status, JSON.stringify(summary)).toBe("completed");
    expect(summary.children).toHaveLength(1);
    await page
      .getByRole("button", { name: "读取真实返工产物", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "创建隔离候选", exact: true }),
    ).toBeVisible();
    let pub = await click(
      page,
      `/feedback-cases/${feedback.id}/candidates`,
      "创建隔离候选",
    );
    const projectedResponse = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname ===
          `/api/v1/feedback-publications/${pub.id}/project` &&
        r.request().method() === "POST",
    );
    await page
      .getByRole("button", { name: "建立隔离索引", exact: true })
      .click();
    const projected = await projectedResponse;
    if (projected.status() === 409) {
      // A completed insert can precede the vector store's query visibility.
      // Reconcile the same publication through its explicit UI action; never
      // recreate the candidate or replay an uncertain paid embedding call.
      await page
        .getByRole("button", { name: "刷新真实状态", exact: true })
        .click();
      const listed = await getData<{ items: any[] }>(
        api,
        `/feedback-cases/${feedback.id}/publications`,
      );
      pub = listed.items.find((v) => v.id === pub.id);
      expect(pub.status).toBe("reconcile_required");
      expect(pub.error).toBe("缺少真实投影回执");
      await attachJSON(info, "feedback-projection-initial-visibility", {
        status: projected.status(),
        publication: pub,
      });
    } else {
      expect(projected.ok(), await projected.text()).toBe(true);
      pub = (await projected.json()).data;
    }
    await expect
      .poll(
        async () => {
          if (pub.status !== "ready") {
            pub = await click(
              page,
              `/feedback-publications/${pub.id}/project`,
              "核对投影回执",
            );
          }
          return pub.status;
        },
        { timeout: 30000, intervals: [1000, 2000] },
      )
      .toBe("ready");
    expect(pub.status).toBe("ready");
    const projectionEvents = await getData<{
      items: { action: string; publication_id?: string }[];
    }>(api, `/feedback-cases/${feedback.id}/events`);
    expect(
      projectionEvents.items.filter(
        (event) =>
          event.publication_id === pub.id &&
          event.action === "projection_started",
      ),
    ).toHaveLength(1);
    await attachJSON(info, "feedback-projection-reconciliation", {
      publication: pub,
      events: projectionEvents.items,
    });
    const premature = await api.post(
      `/api/v1/feedback-publications/${pub.id}/approve`,
      {
        data: {
          expected_version: pub.version,
          comment: "Must reject before real fixed validation",
        },
      },
    );
    expect(premature.status()).toBe(409);
    expect(pub.projection.index).toBe("milvus");
    expect(pub.projection.chunk_ids.length).toBeGreaterThan(0);
    const isolated = await search(api, kb, marker);
    expect(
      (isolated.items || []).every(
        (i: any) => i.knowledge_id !== pub.knowledge_id,
      ),
    ).toBe(true);
    const direct = await api.get(
      `/api/v1/knowledge-bases/${kb}/knowledge/${pub.knowledge_id}`,
    );
    expect([403, 404]).toContain(direct.status());
    pub = await click(
      page,
      `/feedback-publications/${pub.id}/validate`,
      "启动固定基线复测",
    );
    expect(pub.baseline_evaluation_id).toBeTruthy();
    expect(pub.candidate_evaluation_id).toBeTruthy();
    const evaluations = [];
    for (const id of [
      pub.baseline_evaluation_id,
      pub.candidate_evaluation_id,
    ]) {
      let detail: any;
      await expect
        .poll(
          async () => {
            detail = await getData<any>(api, `/evaluation/tasks/${id}`);
            return ["completed", "failed", "cancelled", "timeout"].includes(
              detail.status,
            );
          },
          { timeout: 240000, intervals: [1000, 2000, 4000] },
        )
        .toBe(true);
      await attachJSON(
        info,
        `feedback-evaluation-${evaluations.length}`,
        detail,
      );
      expect(detail.status, JSON.stringify(detail)).toBe("completed");
      evaluations.push(detail);
    }
    pub = await click(
      page,
      `/feedback-publications/${pub.id}/compare`,
      "读取真实比较结果",
    );
    await attachJSON(info, "feedback-actual-fixed-comparison", {
      feedback,
      pub,
      evaluations,
      summary,
    });
    expect(pub.status, JSON.stringify(pub.comparison)).toBe("validated");
    expect(pub.comparison.pairs).toBe(4);
    expect(pub.comparison.missing_pairs).toBe(0);
    expect(pub.comparison.improvement).toBeGreaterThanOrEqual(
      feedback.gate.min_improvement,
    );
    expect(pub.comparison.gate).toBe("eligible_for_review");
    await page
      .getByLabel("审核理由", { exact: true })
      .fill("核对四组真实配对结果、冻结样本及候选隔离回执后批准");
    pub = await click(
      page,
      `/feedback-publications/${pub.id}/approve`,
      "审核通过",
    );
    feedback = await click(
      page,
      `/feedback-publications/${pub.id}/activate`,
      "激活已批准版本",
    );
    expect(feedback.active_version_id).toBe(pub.id);
    await page.reload();
    await expect(
      page.getByText("当前正式活动版本", { exact: true }),
    ).toBeVisible();
    await page.getByRole("region", { name: "候选验证与发布" }).screenshot({
      path: info.outputPath(
        "feedback-actual-fixed-validation-and-approval.png",
      ),
    });
    await verifyVectorRetrieval(
      page,
      info,
      kb,
      pub.knowledge_id,
      marker,
      answers[0],
    );
    const active = await search(api, kb, marker);
    expect(
      (active.items || []).every((i: any) => i.knowledge_id !== knowledgeID),
    ).toBe(true);
    await page.goto(`/feedback?case_id=${feedback.id}`);
    page.once("dialog", (d) => d.accept());
    feedback = await click(
      page,
      `/feedback-cases/${feedback.id}/rollback`,
      "回退到仍有效的基线",
    );
    expect(feedback.active_version_id).toBe("");
    const rolledBack = await search(api, kb, marker);
    expect(
      rolledBack.items.some((i: any) => i.knowledge_id === knowledgeID),
    ).toBe(true);
    expect(
      rolledBack.items.some((i: any) => i.knowledge_id === pub.knowledge_id),
    ).toBe(false);
    feedback = await click(
      page,
      `/feedback-publications/${pub.id}/activate`,
      "激活已批准版本",
    );
    expect(feedback.active_version_id).toBe(pub.id);
    page.once("dialog", (d) => d.accept());
    feedback = await click(
      page,
      `/feedback-cases/${feedback.id}/withdraw`,
      "撤回正式消费",
    );
    const withdrawn = await search(api, kb, marker);
    expect(
      (withdrawn.items || []).filter((i: any) =>
        [knowledgeID, pub.knowledge_id].includes(i.knowledge_id),
      ),
    ).toHaveLength(0);
    const deleted = await api.delete(
      `/api/v1/knowledge-bases/${kb}/knowledge/${knowledgeID}`,
    );
    expect(deleted.ok(), await deleted.text()).toBe(true);
    const deniedRollback = await api.post(
      `/api/v1/feedback-cases/${feedback.id}/rollback`,
      {
        data: {
          expected_version: feedback.version,
          publication_id: "baseline",
        },
      },
    );
    expect([403, 404]).toContain(deniedRollback.status());
    const deniedHistory = await api.get(
      `/api/v1/evaluation/results/${pub.candidate_evaluation_id}`,
    );
    expect([403, 404]).toContain(deniedHistory.status());
    const deniedAsset = await api.get(
      `/api/v1/lifecycle/assets/${baseline.asset_id}?version=${baseline.version}`,
    );
    expect([403, 404]).toContain(deniedAsset.status());
    await attachJSON(info, "feedback-consumption-gates", {
      before: isolated,
      active,
      rolledBack,
      withdrawn,
      publication_id: pub.id,
      case_id: feedback.id,
      premature_approval_status: premature.status(),
      revoked_rollback_status: deniedRollback.status(),
      revoked_history_status: deniedHistory.status(),
      revoked_asset_status: deniedAsset.status(),
    });
  } finally {
    if (kb) await api.delete(`/api/v1/knowledge-bases/${kb}`);
    await api.dispose();
  }
});
