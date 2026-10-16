import { test, expect, type Page } from "@playwright/test";
import {
  loginThroughUI,
  createInput,
  getData,
  attachJSON,
  selectPagedAsset,
} from "./lifecycle.helpers";
async function action(page: Page, path: string, click: () => Promise<unknown>) {
  const wait = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === `/api/v1${path}` &&
      r.request().method() === "POST",
  );
  await click();
  const response = await wait;
  expect(response.ok(), await response.text()).toBe(true);
  return (await response.json()).data;
}
test("warehouse real join/aggregate → candidate → approval → fixed historical output", async ({
  page,
  playwright,
}, info) => {
  test.skip(
    process.env.COGNIDA_E2E_WAREHOUSE !== "1",
    "Requires isolated warehouse environment and real lineage backend",
  );
  const api = await loginThroughUI(page, playwright);
  try {
    const source = await createInput(
      page,
      {
        id: "warehouse-facts",
        source: "table",
        columns: ["id", "customer", "amount", "event"],
        rows: [
          [
            "01",
            "A",
            "9007199254740993.123456789012345678",
            "2026-01-01T00:00:00Z",
          ],
          ["02", "A", "0.1", "2026-01-02T00:00:00Z"],
          ["03", "B", "2", "2026-01-02T00:00:00Z"],
        ],
      },
      `数仓事实-${Date.now()}`,
    );
    const dimension = await createInput(
      page,
      {
        id: "warehouse-dimensions",
        source: "table",
        columns: ["customer", "region"],
        rows: [
          ["A", "north"],
          ["B", "south"],
        ],
      },
      `数仓维度-${Date.now()}`,
    );
    await page.goto("/warehouse");
    const form = page.getByRole("form", { name: "建模方案" });
    await form
      .getByLabel("模型名称", { exact: true })
      .fill(`区域营收-${Date.now()}`);
    await form.getByLabel("主题域", { exact: true }).fill("销售");
    await form.getByLabel("最终事实粒度", { exact: true }).fill("每区域一行");
    await form.getByLabel("输出唯一键", { exact: true }).fill("dim_region");
    await form.getByLabel("事实来源步骤", { exact: true }).fill("orders");
    await form.getByLabel("稳定事实键", { exact: true }).fill("id");
    await form
      .getByLabel("事件时间列（增量必填）", { exact: true })
      .fill("event");
    await form.getByLabel("最终输出步骤", { exact: true }).fill("regional");
    const nodes = form.locator("article");
    await nodes.nth(0).getByLabel("步骤标识", { exact: true }).fill("orders");
    await selectPagedAsset(page, nodes.nth(0).getByLabel(/^来源快照/), `${source.asset_id}:${source.version}`, "加载更多来源资产");
    await form
      .getByRole("button", { name: "添加转换步骤（最多 8 步）", exact: true })
      .click();
    await nodes.nth(1).getByLabel(/^操作/).selectOption("source");
    await nodes
      .nth(1)
      .getByLabel("步骤标识", { exact: true })
      .fill("customers");
    await selectPagedAsset(page, nodes.nth(1).getByLabel(/^来源快照/), `${dimension.asset_id}:${dimension.version}`, "加载更多来源资产");
    await form
      .getByRole("button", { name: "添加转换步骤（最多 8 步）", exact: true })
      .click();
    const join = nodes.nth(2);
    await join.getByLabel(/^操作/).selectOption("left_join_many_to_one");
    await join.getByLabel("步骤标识", { exact: true }).fill("detail");
    await join
      .getByLabel("输入步骤", { exact: true })
      .fill("orders, customers");
    await join.getByLabel("事实关联键", { exact: true }).fill("customer");
    await join.getByLabel("维度唯一键", { exact: true }).fill("customer");
    await join.getByLabel("带入的维度列", { exact: true }).fill("region");
    await form
      .getByRole("button", { name: "添加转换步骤（最多 8 步）", exact: true })
      .click();
    const aggregate = nodes.nth(3);
    await aggregate.getByLabel(/^操作/).selectOption("aggregate");
    await aggregate.getByLabel("步骤标识", { exact: true }).fill("regional");
    await aggregate.getByLabel(/^数据层/).selectOption("summary");
    await aggregate.getByLabel("输入步骤", { exact: true }).fill("detail");
    await aggregate.getByLabel("分组键", { exact: true }).fill("dim_region");
    await aggregate
      .getByRole("button", { name: "添加度量", exact: true })
      .click();
    await aggregate.getByLabel("度量名称", { exact: true }).fill("revenue");
    await aggregate.getByLabel("度量来源列", { exact: true }).fill("amount");
    await aggregate
      .getByLabel("业务口径", { exact: true })
      .fill("按订单原始金额精确求和");
    const preview = await action(page, "/warehouse/preview", () =>
      form
        .getByRole("button", { name: "预览真实转换与影响", exact: true })
        .click(),
    );
    expect(preview.total_rows).toBe(2);
    expect(preview.exact_rows[0][1]).toBe(
      "9007199254740993.223456789012345678",
    );
    expect(preview.stages.map((s: any) => s.node)).toEqual([
      "orders",
      "customers",
      "detail",
      "regional",
    ]);
    await expect(page.getByRole("region", { name: "建模预览" })).toContainText(
      "9007199254740993.223456789012345678",
    );
    const model = await action(page, "/warehouse/models", () =>
      form
        .getByRole("button", { name: "保存固定模型版本", exact: true })
        .click(),
    );
    await expect(page).toHaveURL(new RegExp(`model_id=${model.id}`));
    const task = await action(page, "/warehouse/runs", () =>
      page.getByRole("button", { name: "生成受管候选", exact: true }).click(),
    );
    let run: any;
    await expect
      .poll(
        async () => {
          run = await getData(api, `/warehouse/runs/${task.id}`);
          return run.status;
        },
        { timeout: 60000, intervals: [500, 1000, 2000] },
      )
      .toBe("awaiting_approval");
    expect(run.lineage_registered).toBe(true);
    expect(run.partitions).toHaveLength(1);
    expect(
      (await getData<any>(api, `/warehouse/models/${model.id}`)).active_ref,
    ).toBeFalsy();
    await page.goto(`/warehouse?run_id=${task.id}`);
    await expect(
      page.getByRole("heading", { name: "候选待审批", exact: true }),
    ).toBeVisible();
    const approved = await action(
      page,
      `/warehouse/runs/${task.id}/approve`,
      () =>
        page
          .getByRole("button", { name: "管理员批准并切换", exact: true })
          .click(),
    );
    expect(approved.status).toBe("approved");
    expect(approved.published_ref).toBeTruthy();
    expect(
      (await getData<any>(api, `/warehouse/models/${model.id}`)).active_ref,
    ).toBe(approved.published_ref);
    const [id, version] = approved.published_ref.split(":");
    const output = await api.get(
      `/api/v1/lifecycle/assets/${id}?version=${version}`,
    );
    expect(output.status()).toBe(200);
    expect(await output.text()).toContain(
      "9007199254740993.223456789012345678",
    );
    await page.reload();
    await expect(
      page.getByRole("heading", { name: "已批准并切换", exact: true }),
    ).toBeVisible();
    await page
      .getByRole("region", { name: "数仓任务详情" })
      .screenshot({ path: info.outputPath("warehouse-approved-detail.png") });
    await attachJSON(info, "warehouse-real-hand-calculated-evidence", {
      preview,
      model,
      run,
      approved,
    });
  } finally {
    await api.dispose();
  }
});
