import { randomUUID } from "node:crypto";
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import {
  loginThroughUI,
  createInput,
  getData,
  attachJSON,
  selectPagedAsset,
} from "./lifecycle.helpers";
async function post(api: APIRequestContext, path: string, data: unknown) {
  const r = await api.post(`/api/v1${path}`, {
    data,
    headers: { "Idempotency-Key": randomUUID() },
  });
  expect(r.ok(), `${path}: ${await r.text()}`).toBe(true);
  return (await r.json()).data;
}
async function action(page: Page, path: string, click: () => Promise<unknown>) {
  const pending = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === `/api/v1${path}` &&
      r.request().method() === "POST",
  );
  await click();
  const r = await pending;
  expect(r.ok(), `${path}: ${await r.text()}`).toBe(true);
  return (await r.json()).data;
}
async function done(api: APIRequestContext, id: string) {
  let job: any;
  await expect
    .poll(
      async () => {
        job = await getData(api, `/feature-jobs/${id}`);
        return job.status;
      },
      { timeout: 60000, intervals: [500, 1000, 2000] },
    )
    .toBe("completed");
  expect(
    job.partitions.every(
      (p: any) =>
        p.status === "committed" &&
        p.digest &&
        p.output_ref_id &&
        p.available_at,
    ),
  ).toBe(true);
  return job;
}
test.describe("real partition materialization and reference drift", () => {
  test.beforeEach(() =>
    test.skip(
      process.env.COGNIDA_E2E_FEATURE_OPERATIONS !== "1",
      "Requires explicitly enabled isolated feature acceptance environment",
    ),
  );
  test("full partitions → incremental → backfill → withdrawal preserves historical exact values", async ({
    page,
    playwright,
  }, info) => {
    const api = await loginThroughUI(page, playwright);
    try {
      const source = await createInput(
        page,
        {
          id: "partition-source",
          source: "table",
          columns: ["entity", "amount", "event"],
          rows: [
            [
              "A",
              "9007199254740993.123456789012345678",
              "2026-01-01T00:00:00Z",
            ],
            ["B", "0.2", "2026-01-01T00:00:00Z"],
          ],
        },
        `分区来源-${Date.now()}`,
      );
      const feature = await post(api, "/features", {
        name: `精确物化-${Date.now()}`,
        type: "numerical",
        source_ref_id: `${source.asset_id}:${source.version}`,
        definition: {
          kind: "field",
          field: "amount",
          entity_field: "entity",
          event_time_field: "event",
        },
      });
      await page.goto(`/features?feature_id=${feature.id}`);
      const panel = page.getByRole("region", {
        name: "分区物化任务",
        exact: true,
      });
      await panel.getByLabel("每分区行数", { exact: true }).fill("1");
      const queued = await action(page, "/feature-jobs", () =>
        panel
          .getByRole("button", { name: "创建物化任务", exact: true })
          .click(),
      );
      const full = await done(api, queued.id);
      expect(full.committed_rows).toBe(2);
      await page.goto(`/features?job_id=${full.id}`);
      await expect(
        panel.getByRole("heading", { name: "已完成", exact: true }),
      ).toBeVisible();
      await expect(
        panel.getByRole("link", { name: "查看分区产物" }),
      ).toHaveCount(2);
      const current = await post(api, "/feature-values/query", {
        entity_ids: ["A"],
        features: [{ feature_id: feature.id, version: 1 }],
      });
      expect(current.items[0].value_exact).toBe(
        "9007199254740993.123456789012345678",
      );
      const incSource = await createInput(
        page,
        {
          id: "incremental-source",
          source: "table",
          columns: ["entity", "amount", "event"],
          rows: [["C", "0.3", "2026-01-02T00:00:00Z"]],
        },
        `增量来源-${Date.now()}`,
      );
      await page.goto(`/features?feature_id=${feature.id}`);
      await panel.getByLabel(/^物化模式/).selectOption("incremental");
      await selectPagedAsset(page, panel.getByLabel(/^物化来源快照/), `${incSource.asset_id}:${incSource.version}`);
      const inc = await done(
        api,
        (
          await action(page, "/feature-jobs", () =>
            panel
              .getByRole("button", { name: "创建物化任务", exact: true })
              .click(),
          )
        ).id,
      );
      expect(inc.committed_rows).toBe(1);
      expect(Date.parse(inc.watermark_after)).toBeGreaterThan(
        Date.parse(full.watermark_after),
      );
      const backSource = await createInput(
        page,
        {
          id: "backfill-source",
          source: "table",
          columns: ["entity", "amount", "event"],
          rows: [["A", "0.123456789012345678", "2026-01-01T00:00:00Z"]],
        },
        `回填来源-${Date.now()}`,
      );
      await page.goto(`/features?feature_id=${feature.id}`);
      await panel.getByLabel(/^物化模式/).selectOption("backfill");
      await selectPagedAsset(page, panel.getByLabel(/^物化来源快照/), `${backSource.asset_id}:${backSource.version}`);
      await panel
        .getByLabel("回填起始（含）", { exact: true })
        .fill("2025-12-31T00:00");
      await panel
        .getByLabel("回填截止（不含）", { exact: true })
        .fill("2026-01-02T00:00");
      const back = await done(
        api,
        (
          await action(page, "/feature-jobs", () =>
            panel
              .getByRole("button", { name: "创建物化任务", exact: true })
              .click(),
          )
        ).id,
      );
      expect(Date.parse(back.watermark_after)).toBe(Date.parse(inc.watermark_after));
      await page.goto(`/features?job_id=${back.id}`);
      await expect(
        panel.getByRole("heading", { name: "已完成", exact: true }),
      ).toBeVisible();
      const beforeWithdrawal = await post(api, "/feature-values/query", {
        entity_ids: ["A"],
        features: [{ feature_id: feature.id, version: 1 }],
      });
      expect(beforeWithdrawal.items[0].value_exact).toBe(
        "0.123456789012345678",
      );
      await action(page, `/feature-jobs/${back.id}/withdraw`, () =>
        panel
          .getByRole("button", { name: "管理员撤回特征值", exact: true })
          .click(),
      );
      const historical = await post(api, "/feature-values/query", {
        entity_ids: ["A"],
        features: [{ feature_id: feature.id, version: 1 }],
        as_of: back.partitions[0].available_at,
      });
      expect(historical.items[0].value_exact).toBe("0.123456789012345678");
      const restored = await post(api, "/feature-values/query", {
        entity_ids: ["A"],
        features: [{ feature_id: feature.id, version: 1 }],
      });
      expect(restored.items[0].value_exact).toBe(
        "9007199254740993.123456789012345678",
      );
      await panel.screenshot({
        path: info.outputPath("partition-history.png"),
      });
      await attachJSON(info, "real-feature-partition-history", {
        full,
        inc,
        back,
        historical,
        restored,
      });
    } finally {
      await api.dispose();
    }
  });
  test("real baseline candidate → separate approval → changed distribution → persistent monitoring", async ({
    page,
    playwright,
  }, info) => {
    const api = await loginThroughUI(page, playwright);
    try {
      const source = await createInput(
        page,
        {
          id: "reference",
          source: "table",
          columns: ["amount", "region"],
          column_types: { amount: "number" },
          rows: [
            [1, "north"],
            [2, "north"],
            [3, "south"],
          ],
        },
        `真实参考-${Date.now()}`,
      );
      const current = await createInput(
        page,
        {
          id: "current",
          source: "table",
          columns: ["amount", "region"],
          column_types: { amount: "number" },
          rows: [
            [11, "west"],
            [12, "west"],
            [13, "west"],
          ],
        },
        `当前批次-${Date.now()}`,
      );
      await page.goto("/drift");
      await page
        .getByLabel("基线名称", { exact: true })
        .fill(`验收基线-${Date.now()}`);
      await selectPagedAsset(page, page.getByLabel(/^参考快照/), `${source.asset_id}:${source.version}`, "加载更多可选资产");
      await page.getByLabel("最小有效样本", { exact: true }).fill("2");
      const baseline = await action(page, "/drift/baselines", () =>
        page.getByRole("button", { name: "提交基线候选", exact: true }).click(),
      );
      expect(baseline.status).toBe("pending_approval");
      await action(page, `/drift/baselines/${baseline.id}/approve`, () =>
        page
          .getByRole("button", { name: "管理员批准此基线", exact: true })
          .click(),
      );
      await page.getByRole("tab", { name: "比较报告", exact: true }).click();
      await page.getByLabel(/^已批准基线/).selectOption(baseline.id);
      await selectPagedAsset(page, page.getByLabel(/^当前快照/), `${current.asset_id}:${current.version}`, "加载更多可选资产");
      const report = await action(page, "/drift/compare", () =>
        page.getByRole("button", { name: "比较真实快照", exact: true }).click(),
      );
      expect(report.status).toBe("changed");
      expect(report.body.baseline_rows).toBe(3);
      expect(report.body.current_rows).toBe(3);
      expect(
        report.body.metrics.find(
          (m: any) => m.field === "amount" && m.method === "empirical_ks",
        ).effect,
      ).toBe(1);
      expect(
        report.body.metrics.find(
          (m: any) =>
            m.field === "region" && m.method === "categorical_total_variation",
        ).effect,
      ).toBe(1);
      const repeated = await action(page, "/drift/compare", () =>
        page.getByRole("button", { name: "比较真实快照", exact: true }).click(),
      );
      expect(repeated.id).toBe(report.id);
      await page.getByRole("tab", { name: "定时监控", exact: true }).click();
      await page
        .getByLabel("监控名称", { exact: true })
        .fill(`持续监控-${Date.now()}`);
      await page.getByLabel("检查间隔（秒）", { exact: true }).fill("60");
      const monitor = await action(page, "/drift/monitors", () =>
        page.getByRole("button", { name: "创建监控", exact: true }).click(),
      );
      let observed: any;
      await expect
        .poll(
          async () => {
            observed = await getData(api, `/drift/records/${monitor.id}`);
            return observed.body.last_report_id;
          },
          { timeout: 30000, intervals: [500, 1000, 2000] },
        )
        .toBe(report.id);
      await page.goto(`/drift?record_id=${report.id}`);
      const card = page.getByRole("region", {
        name: "漂移记录详情",
        exact: true,
      });
      await expect(card).toContainText("分布变化不等于质量下降");
      await card.screenshot({ path: info.outputPath("real-drift-report.png") });
      await attachJSON(info, "real-drift-reference-evidence", {
        baseline,
        report,
        monitor: observed,
      });
      const model = await getData<any>(api, "/feature-models");
      if (!model.available) {
        await page.goto("/features");
        await expect(page.getByText(/模型向量当前不可用/)).toBeVisible();
        await attachJSON(info, "embedding-honest-unavailability", model);
      }
    } finally {
      await api.dispose();
    }
  });
});
