import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { reactive } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
const api = vi.hoisted(() => ({
  models: vi.fn(),
  model: vi.fn(),
  preview: vi.fn(),
  save: vi.fn(),
  runs: vi.fn(),
  run: vi.fn(),
  createRun: vi.fn(),
  control: vi.fn(),
}));
vi.mock("@/api/warehouse", () => ({ warehouseApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi.fn().mockResolvedValue({
      data: { items: [{ asset_id: "source", version: 1, name: "订单快照" }] },
    }),
  },
}));
const auth = reactive({
  authEpoch: 0,
  user: { id: 1 },
  currentTenant: { id: 1 },
});
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
import WarehouseView from "./WarehouseView.vue";
import { lifecycleApi } from "@/api/lifecycle";
const definition = {
  name: "区域营收",
  subject: "销售",
  grain: "每区域一行",
  grain_keys: ["region"],
  fact_node: "orders",
  fact_keys: ["id"],
  event_field: "event",
  update_policy: "upsert",
  delete_policy: "unsupported",
  late_policy: "reject_incremental_use_backfill",
  nodes: [
    { id: "orders", kind: "source", layer: "raw", source_ref: "source:1" },
  ],
  output_node: "orders",
};
const model = { id: "model", version: 1, digest: "definition", definition };
const run = {
  id: "run",
  version: 3,
  status: "awaiting_approval",
  request: { model_id: "model", model_version: 1, mode: "full" },
  output_ref: "output:1",
  lineage_registered: true,
  partitions: [
    {
      index: 0,
      count: 2,
      digest: "digest",
      output_ref: "part:1",
      committed_at: "2026-10-16T00:00:00Z",
    },
  ],
  stages: [
    {
      node: "orders",
      layer: "raw",
      kind: "source",
      input_rows: 0,
      output_rows: 2,
      digest: "digest",
    },
  ],
};
const preview = {
  definition_digest: "definition",
  total_rows: 1,
  output: {
    columns: ["region", "revenue"],
    rows: [["north", 9007199254740992]],
  },
  exact_rows: [['"north"', "9007199254740993.123456789012345678"]],
  stages: run.stages,
  lineage: [],
  impact: [{ unknown: true, name: "未登记外部消费依赖未知" }],
  limitations: ["不检测删除"],
};
let mounted: VueWrapper[] = [];
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(lifecycleApi.assets).mockResolvedValue({
    data: { items: [{ asset_id: "source", version: 1, name: "订单快照" }] },
  } as any);
  auth.authEpoch = 0;
  api.models.mockResolvedValue({ data: { items: [model] } });
  api.model.mockResolvedValue({ data: model });
  api.runs.mockResolvedValue({ data: { items: [run] } });
  api.run.mockResolvedValue({ data: run });
  api.preview.mockResolvedValue({ data: preview });
  api.save.mockResolvedValue({ data: { ...model, version: 2 } });
  api.createRun.mockResolvedValue({ data: { ...run, status: "queued" } });
});
afterEach(() => {
  mounted.forEach((w) => w.unmount());
  mounted = [];
  vi.useRealTimers();
  vi.restoreAllMocks();
});
async function open(path = "/warehouse?model_id=model") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/warehouse", component: WarehouseView },
      { path: "/analysis", component: { template: "<p>资产</p>" } },
      { path: "/other", component: { template: "<p>其他</p>" } },
    ],
  });
  await router.push(path);
  await router.isReady();
  const w = mount(
    { template: "<router-view />" },
    { global: { plugins: [router] } },
  );
  mounted.push(w);
  await flushPromises();
  return { w, router };
}
const button = (w: VueWrapper, name: string) =>
  w.findAll("button").find((b) => b.text() === name)!;
const field = (w: VueWrapper, name: string) =>
  w.findAll("label").find((l) => l.text().startsWith(name))!;
describe("warehouse managed workflow", () => {
  it("previews exact values and never saves or runs until explicitly requested", async () => {
    const { w } = await open();
    await w.get('form[aria-label="建模方案"]').trigger("submit");
    await flushPromises();
    expect(w.text()).toContain("9007199254740993.123456789012345678");
    expect(w.text()).toContain("未登记外部消费依赖未知");
    expect(api.save).not.toHaveBeenCalled();
    expect(api.createRun).not.toHaveBeenCalled();
    await field(w, "主题域").get("input").setValue("retail");
    expect(w.find('[aria-label="建模预览"]').exists()).toBe(false);
    expect(button(w, "保存固定模型版本").attributes("disabled")).toBeDefined();
  });
  it("restores a run by query without dispatching or approving it", async () => {
    const { w } = await open("/warehouse?run_id=run");
    expect(w.text()).toContain("候选待审批");
    expect(w.text()).toContain("1 个真实分区回执");
    expect(api.createRun).not.toHaveBeenCalled();
    expect(api.control).not.toHaveBeenCalled();
    api.control.mockResolvedValue({
      data: {
        ...run,
        status: "approved",
        version: 4,
        published_ref: "published:1",
      },
    });
    await button(w, "管理员批准并切换").trigger("click");
    await flushPromises();
    expect(api.control).toHaveBeenCalledWith(
      expect.objectContaining({ id: "run", version: 3 }),
      "approve",
    );
    expect(w.text()).toContain("已批准并切换");
  });
  it("shows cancel_requested until the backend reports a terminal state", async () => {
    api.run.mockResolvedValue({ data: { ...run, status: "running" } });
    const { w } = await open("/warehouse?run_id=run");
    api.control.mockResolvedValue({
      data: { ...run, status: "cancel_requested" },
    });
    await button(w, "请求取消").trigger("click");
    await flushPromises();
    expect(w.text()).toContain("等待执行器确认终态");
    expect(w.findAll("h2").some((h) => h.text() === "已取消")).toBe(false);
  });
  it("discards private late run responses after identity changes", async () => {
    let resolve!: (v: unknown) => void;
    api.run.mockImplementationOnce(() => new Promise((r) => (resolve = r)));
    const { w } = await open("/warehouse?run_id=run");
    auth.authEpoch++;
    api.run.mockRejectedValue({ response: { status: 403 } });
    await flushPromises();
    resolve({ data: { ...run, error: "old account private content" } });
    await flushPromises();
    expect(w.text()).not.toContain("old account private content");
    expect(w.find('[aria-label="数仓任务详情"]').exists()).toBe(false);
    expect(api.createRun).not.toHaveBeenCalled();
  });
  it("clears a run if its second model authorization read fails", async () => {
    api.model.mockRejectedValue({ response: { status: 403 } });
    const { w } = await open("/warehouse?run_id=run");
    expect(w.find('[aria-label="数仓任务详情"]').exists()).toBe(false);
    expect(w.find('[role="alert"]').exists()).toBe(true);
  });
  it("keeps unsaved changes when a manual refresh is declined", async () => {
    const { w } = await open();
    await field(w, "主题域").get("input").setValue("unsaved");
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    const before = api.model.mock.calls.length;
    await button(w, "刷新列表与状态").trigger("click");
    await flushPromises();
    expect(confirm).toHaveBeenCalled();
    expect(api.model).toHaveBeenCalledTimes(before);
    expect(field(w, "主题域").get("input").element.value).toBe("unsaved");
  });
  it("guards same-route navigation and keeps the visible task URL coherent", async () => {
    const { w, router } = await open();
    await field(w, "主题域").get("input").setValue("unsaved");
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    await router.push("/warehouse?run_id=run");
    expect(confirm).toHaveBeenCalled();
    expect(router.currentRoute.value.query.model_id).toBe("model");
    confirm.mockReturnValue(true);
    await router.push("/warehouse?run_id=run");
    await flushPromises();
    expect(router.currentRoute.value.query.run_id).toBe("run");
    expect(w.text()).toContain("候选待审批");
  });
});
