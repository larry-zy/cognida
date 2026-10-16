import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { reactive } from "vue";
import { createRouter, createMemoryHistory } from "vue-router";
const api = vi.hoisted(() => ({
  get: vi.fn(),
  list: vi.fn(),
  create: vi.fn(),
  action: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi
      .fn()
      .mockResolvedValue({
        data: { items: [{ asset_id: "source", version: 1, name: "Orders" }] },
      }),
  },
}));
const auth = reactive({
  authEpoch: 0,
  user: { id: 1 },
  currentTenant: { id: 1 },
});
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
import FeatureJobsPanel from "./FeatureJobsPanel.vue";
import DriftView from "./DriftView.vue";
const feature = {
  id: "feature",
  version: 3,
  name: "amount",
  type: "numerical",
  definition: { kind: "field", field: "amount", event_time_field: "event" },
  source_ref_id: "source:1",
  created_at: "",
  updated_at: "",
} as any;
const job = {
  id: "job",
  version: 1,
  status: "queued",
  request: {
    feature_id: "feature",
    feature_version: 3,
    source_ref_id: "source:1",
    mode: "full",
    partition_size: 100,
  },
  partitions: [
    { index: 0, status: "pending", count: 0, digest: "", output_ref_id: "" },
  ],
  total_rows: 2,
  committed_rows: 0,
  embedding: false,
};
const base = {
  id: "base",
  version: 2,
  kind: "baseline",
  status: "approved",
  body: {
    name: "真实基线",
    source_ref_id: "source:1",
    source_digest: "digest",
    sampling: { limit: 100, minimum: 20, seed: "fixed" },
  },
};
const report = {
  id: "report",
  version: 1,
  kind: "report",
  status: "insufficient",
  body: {
    baseline_id: "base",
    baseline_ref_id: "source:1",
    current_ref_id: "source:1",
    baseline_rows: 2,
    current_rows: 2,
    baseline_sample: 2,
    current_sample: 2,
    changed: false,
    limitations: ["分布变化不等于质量下降"],
    metrics: [
      {
        field: "amount",
        method: "empirical_ks",
        baseline_count: 2,
        current_count: 2,
        threshold: 0.2,
        status: "insufficient",
      },
    ],
  },
};
const button = (w: VueWrapper, text: string) =>
  w.findAll("button").find((b) => b.text() === text)!;
const label = (w: VueWrapper, text: string) =>
  w.findAll("label").find((l) => l.text().startsWith(text))!;
const mounted: VueWrapper[] = [];
beforeEach(() => {
  vi.clearAllMocks();
  auth.authEpoch = 0;
  auth.currentTenant.id = 1;
  api.list.mockResolvedValue({ data: { items: [] } });
  api.get.mockResolvedValue({ data: job });
  api.create.mockResolvedValue({ data: job });
});
afterEach(() => {
  for (const w of mounted.splice(0)) w.unmount();
  vi.useRealTimers();
});
function jobs() {
  const w = mount(FeatureJobsPanel, {
    props: { feature, assets: [] },
    global: { stubs: { RouterLink: true } },
  });
  mounted.push(w);
  return w;
}
async function drift(query = "") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/drift", component: DriftView },
      { path: "/analysis", component: { template: "asset" } },
      { path: "/tasks", component: { template: "task" } },
    ],
  });
  await router.push("/drift" + query);
  await router.isReady();
  const w = mount(DriftView, { global: { plugins: [router] } });
  mounted.push(w);
  await flushPromises();
  return w;
}
describe("durable feature operations", () => {
  it("submits pinned version and explicit backfill bounds, then shows real cancel_requested", async () => {
    const w = jobs();
    await flushPromises();
    await label(w, "物化模式").get("select").setValue("backfill");
    await label(w, "回填起始").get("input").setValue("2026-01-01T00:00");
    await label(w, "回填截止").get("input").setValue("2026-01-02T00:00");
    await w.get("form").trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/feature-jobs",
      expect.objectContaining({
        feature_id: "feature",
        feature_version: 3,
        mode: "backfill",
        source_ref_id: "source:1",
        event_from: new Date("2026-01-01T00:00").toISOString(),
      }),
    );
    api.action.mockResolvedValue({
      data: { ...job, version: 2, status: "cancel_requested" },
    });
    await button(w, "请求取消").trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenCalledWith("/feature-jobs/job/cancel", {
      expected_version: 1,
    });
    expect(w.text()).toContain("等待后端确认");
    expect(w.text()).not.toContain("已取消");
  });
  it("drops a late job response after identity changes and never auto-submits a deep link", async () => {
    let finish!: (v: any) => void;
    api.get.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const w = mount(FeatureJobsPanel, {
      props: { feature, assets: [], jobId: "job" },
      global: { stubs: { RouterLink: true } },
    });
    mounted.push(w);
    await flushPromises();
    auth.authEpoch++;
    api.get.mockResolvedValue({ data: undefined });
    await flushPromises();
    finish({ data: { ...job, error: "old account private error" } });
    await flushPromises();
    expect(w.text()).not.toContain("old account private error");
    expect(api.create).not.toHaveBeenCalled();
    expect(api.action).not.toHaveBeenCalled();
  });
  it("preserves unconfirmed model state without presenting a retry that replays cost", async () => {
    api.get.mockResolvedValue({
      data: { ...job, status: "reconcile_required", error: "模型调用无法核实" },
    });
    const w = mount(FeatureJobsPanel, {
      props: { feature, assets: [], jobId: "job" },
      global: { stubs: { RouterLink: true } },
    });
    mounted.push(w);
    await flushPromises();
    expect(w.text()).toContain("需核对模型调用");
    expect(w.findAll("button").some((b) => b.text().includes("重试"))).toBe(
      false,
    );
    expect(api.create).not.toHaveBeenCalled();
  });
});
describe("real-reference drift workflow", () => {
  it("renders insufficient evidence without making a probability claim", async () => {
    api.get.mockResolvedValue({ data: report });
    const w = await drift("?record_id=report");
    expect(w.text()).toContain("样本不足");
    expect(w.text()).toContain("经验分布距离");
    expect(w.text()).toContain("— / 0.2");
    expect(w.text()).not.toContain("管理员创建治理草稿");
    expect(api.action).not.toHaveBeenCalled();
  });
  it("creates a baseline candidate and only approves on explicit separate action", async () => {
    api.action
      .mockResolvedValueOnce({
        data: { ...base, version: 1, status: "pending_approval" },
      })
      .mockResolvedValueOnce({ data: base });
    const w = await drift();
    await label(w, "基线名称").get("input").setValue("真实基线");
    await label(w, "参考快照").get("select").setValue("source:1");
    await w.get("form").trigger("submit");
    await flushPromises();
    expect(api.action).toHaveBeenCalledTimes(1);
    expect(api.action.mock.calls[0][1]).not.toHaveProperty("approved_by");
    expect(w.text()).toContain("待管理员批准");
    await button(w, "管理员批准此基线").trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenLastCalledWith(
      "/drift/baselines/base/approve",
      { expected_version: 1 },
    );
  });
  it("clears a prior account report on permission failure after tenant change", async () => {
    api.get
      .mockResolvedValueOnce({ data: report })
      .mockRejectedValue(new Error("无权查看"));
    const w = await drift("?record_id=report");
    expect(w.text()).toContain("经验分布距离");
    auth.currentTenant.id++;
    await flushPromises();
    expect(w.text()).not.toContain("经验分布距离");
    expect(w.text()).toContain("无权查看");
  });
});
