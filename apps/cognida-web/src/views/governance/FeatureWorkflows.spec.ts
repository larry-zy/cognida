import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { reactive } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  action: vi.fn(),
  previewFeature: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi.fn().mockResolvedValue({
      data: { items: [{ asset_id: "source", version: 1, name: "Orders" }] },
    }),
  },
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
const auth = reactive({
  authEpoch: 0,
  user: { id: 1 },
  currentTenant: { id: 1 },
});
import FeaturesView from "./FeaturesView.vue";
import FeatureExpressionEditor from "./FeatureExpressionEditor.vue";
const feature = {
  id: "feature",
  name: "Spend",
  version: 2,
  type: "numerical",
  definition: { kind: "field", field: "amount" },
  source_ref_id: "source:1",
};
const preview = {
  values: [
    {
      entity_id: "customer",
      value: 9007199254740992,
      value_exact: "9007199254740993.1234567890123456789",
      timestamp: "2026-10-16T00:00:00Z",
    },
  ],
  total_count: 1,
  warnings: [],
  definition_digest: "definition-digest",
  source_digest: "source-digest",
};
const button = (w: VueWrapper, text: string) =>
  w.findAll("button").find((b) => b.text() === text)!;
const field = (w: VueWrapper, text: string) =>
  w.findAll("label").find((l) => l.text().startsWith(text))!;
async function open(query = "") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/features", component: FeaturesView },
      { path: "/analysis", component: { template: "<p>Asset</p>" } },
    ],
  });
  await router.push("/features" + query);
  await router.isReady();
  const wrapper = mount(FeaturesView, { global: { plugins: [router] } });
  await flushPromises();
  return { wrapper, router };
}
async function draft(w: VueWrapper) {
  await field(w, "名称").get("input").setValue("Spend");
  await field(w, "来源资产").get("select").setValue("source:1");
}
beforeEach(() => {
  vi.clearAllMocks();
  auth.authEpoch = 0;
  auth.currentTenant.id = 1;
  api.list.mockResolvedValue({ data: { items: [] } });
  api.get.mockResolvedValue({ data: feature });
  api.previewFeature.mockResolvedValue({ data: preview });
  api.create.mockResolvedValue({ data: feature });
});
describe("feature computation workflow", () => {
  it("submits the feature head ID when historical version records have independent IDs", async () => {
    api.list.mockImplementation(async (path: string) => ({
      data: {
        items: path.endsWith("/versions")
          ? [{ ...feature, id: "immutable-version-record" }]
          : [],
      },
    }));
    const job = {
      id: "job",
      version: 1,
      status: "completed",
      request: { feature_id: "feature", feature_version: 2, mode: "full" },
      committed_rows: 0,
      total_rows: 0,
      partitions: [],
    };
    api.create.mockResolvedValue({ data: job });
    api.get.mockImplementation(async (path: string) => ({
      data: path.startsWith("/feature-jobs/") ? job : feature,
    }));
    const { wrapper } = await open("?feature_id=feature");
    await wrapper.get('[aria-label="分区物化任务"] form').trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/feature-jobs",
      expect.objectContaining({ feature_id: "feature", feature_version: 2 }),
    );
    wrapper.unmount();
  });
  it("keeps optional event time when switching from field to expression", async () => {
    const { wrapper } = await open();
    await draft(wrapper);
    await field(wrapper, "事件时间字段").get("input").setValue("occurred_at");
    await field(wrapper, "计算方式").get("select").setValue("expression");
    await wrapper
      .getComponent(FeatureExpressionEditor)
      .get("input")
      .setValue("amount");
    await button(wrapper, "预览计算结果").trigger("click");
    await flushPromises();
    expect(
      api.previewFeature.mock.calls[0][0].definition.event_time_field,
    ).toBe("occurred_at");
    wrapper.unmount();
  });
  it("discards an old materialization receipt after the selected version changes", async () => {
    let finish!: (value: unknown) => void;
    api.action.mockImplementation(
      () => new Promise((resolve) => (finish = resolve)),
    );
    const { wrapper } = await open("?feature_id=feature");
    await button(wrapper, "计算并物化所选版本").trigger("click");
    await field(wrapper, "特征版本").get("input").setValue(1);
    finish({
      data: { receipt_id: "old", output_ref_id: "old:1", count: 7, version: 2 },
    });
    await flushPromises();
    expect(wrapper.text()).not.toContain("已物化 7");
    wrapper.unmount();
  });
  it("discards a query response after entity filters change", async () => {
    let finish!: (value: unknown) => void;
    api.action.mockImplementation(
      () => new Promise((resolve) => (finish = resolve)),
    );
    const { wrapper } = await open("?feature_id=feature");
    await field(wrapper, "读取实体").get("input").setValue("original");
    await wrapper
      .findAll("form")
      .find((f) => f.text().includes("读取实体"))!
      .trigger("submit");
    await field(wrapper, "读取实体").get("input").setValue("new");
    finish({
      data: [
        { entity_id: "original", feature_id: "feature", version: 2, value: 7 },
      ],
    });
    await flushPromises();
    expect(
      wrapper
        .findAll("table")
        .some((table) => table.text().includes("original")),
    ).toBe(false);
    wrapper.unmount();
  });
  it("writes manual decimal values using the exact literal channel", async () => {
    const { wrapper } = await open("?feature_id=feature");
    const form = wrapper
      .findAll("form")
      .find((f) => f.find('input[placeholder="customer-1001"]').exists())!;
    await form.get('input[placeholder="customer-1001"]').setValue("entity");
    await form
      .get('input[placeholder="输入特征值"]')
      .setValue("0.12345678901234567890123456789");
    await form.trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/feature-values",
      expect.objectContaining({
        values: [
          expect.objectContaining({
            value: null,
            value_exact: "0.12345678901234567890123456789",
          }),
        ],
      }),
    );
    wrapper.unmount();
  });
  it("previews a typed nested expression without saving, then invalidates the preview on edit", async () => {
    const { wrapper } = await open();
    await draft(wrapper);
    await field(wrapper, "计算方式").get("select").setValue("expression");
    const editor = wrapper.getComponent(FeatureExpressionEditor);
    await editor.get("select").setValue("mul");
    const args = editor.findAllComponents(FeatureExpressionEditor);
    await args[0].get("input").setValue("amount");
    await args[1].get("input").setValue("2");
    await button(wrapper, "预览计算结果").trigger("click");
    await flushPromises();
    expect(api.previewFeature).toHaveBeenCalledWith(
      expect.objectContaining({
        definition: {
          kind: "expression",
          expression: {
            op: "mul",
            args: [
              { op: "column", field: "amount" },
              { op: "to_number", args: [{ op: "literal", value: "2" }] },
            ],
          },
        },
      }),
      20,
    );
    expect(api.create).not.toHaveBeenCalled();
    expect(wrapper.get('[aria-label="特征预览"]').text()).toContain(
      "9007199254740993.1234567890123456789",
    );
    await field(wrapper, "名称").get("input").setValue("Edited");
    expect(wrapper.find('[aria-label="特征预览"]').exists()).toBe(false);
    wrapper.unmount();
  });
  it("sends entity/event/window fields as a window definition and not an expression", async () => {
    const { wrapper } = await open();
    await draft(wrapper);
    await field(wrapper, "计算方式").get("select").setValue("window_aggregate");
    await field(wrapper, "来源字段").get("input").setValue("amount");
    await field(wrapper, "实体标识字段").get("input").setValue("customer_id");
    await field(wrapper, "事件时间字段").get("input").setValue("occurred_at");
    await field(wrapper, "窗口秒数").get("input").setValue(600);
    await field(wrapper, "聚合方法").get("select").setValue("mean");
    await button(wrapper, "预览计算结果").trigger("click");
    await flushPromises();
    expect(api.previewFeature).toHaveBeenCalledWith(
      expect.objectContaining({
        definition: {
          kind: "window_aggregate",
          field: "amount",
          entity_field: "customer_id",
          event_time_field: "occurred_at",
          window_seconds: 600,
          aggregate: "mean",
        },
      }),
      20,
    );
    wrapper.unmount();
  });
  it("discards an outstanding preview when tenant changes", async () => {
    let finish!: (value: unknown) => void;
    api.previewFeature.mockImplementation(
      () => new Promise((resolve) => (finish = resolve)),
    );
    const { wrapper } = await open();
    await draft(wrapper);
    await field(wrapper, "来源字段").get("input").setValue("amount");
    await button(wrapper, "预览计算结果").trigger("click");
    auth.currentTenant.id = 2;
    await flushPromises();
    finish({ data: preview });
    await flushPromises();
    expect(wrapper.find('[aria-label="特征预览"]').exists()).toBe(false);
    expect(
      (field(wrapper, "名称").get("input").element as HTMLInputElement).value,
    ).toBe("");
    wrapper.unmount();
  });
  it("restores an execution version read-only and shows the real materialization receipt", async () => {
    api.get.mockImplementation((path: string) =>
      Promise.resolve({
        data: path.startsWith("/governance-executions/")
          ? { feature_id: "feature", feature_version: 1 }
          : { ...feature, version: 1 },
      }),
    );
    api.list.mockResolvedValue({
      data: { items: [{ ...feature, version: 1 }] },
    });
    const { wrapper } = await open("?execution_id=execution");
    expect(api.get).toHaveBeenCalledWith("/features/feature/versions/1");
    expect(api.action).not.toHaveBeenCalled();
    api.action.mockResolvedValue({
      data: {
        receipt_id: "actual-receipt",
        feature_id: "feature",
        version: 1,
        count: 2,
        source_ref_id: "source:1",
        output_ref_id: "output:1",
        available_at: "2026-10-16T00:05:00Z",
        definition_digest: "digest",
      },
    });
    await button(wrapper, "计算并物化所选版本").trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenCalledWith("/features/feature/materialize", {
      version: 1,
    });
    expect(wrapper.text()).toContain("已物化 2 个特征值");
    expect(wrapper.get("a").attributes("href")).toBe(
      "/analysis?asset=output:1",
    );
    wrapper.unmount();
  });
  it("clears prior feature details when a new deep link is denied", async () => {
    const { wrapper, router } = await open("?feature_id=feature");
    expect(wrapper.text()).toContain("实体特征值");
    api.get.mockRejectedValue(new Error("无权查看"));
    await router.push("/features?feature_id=private");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toBe("无权查看");
    expect(wrapper.text()).not.toContain("实体特征值");
    wrapper.unmount();
  });
  it("does not claim success when materialization does not return an output receipt", async () => {
    const { wrapper } = await open("?feature_id=feature");
    api.action.mockResolvedValue({ data: { count: 3 } });
    await button(wrapper, "计算并物化所选版本").trigger("click");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("未返回完整回执");
    expect(wrapper.text()).not.toContain("已物化 3");
    wrapper.unmount();
  });
  it("preserves exact decimal and large integer constants without browser Number coercion", async () => {
    const { wrapper } = await open();
    await draft(wrapper);
    await field(wrapper, "计算方式").get("select").setValue("expression");
    const editor = wrapper.getComponent(FeatureExpressionEditor);
    await editor.get("select").setValue("literal");
    await editor.get("input").setValue("9007199254740993.1234567890123456789");
    await button(wrapper, "预览计算结果").trigger("click");
    await flushPromises();
    expect(api.previewFeature).toHaveBeenCalledWith(
      expect.objectContaining({
        definition: expect.objectContaining({
          expression: {
            op: "to_number",
            args: [
              { op: "literal", value: "9007199254740993.1234567890123456789" },
            ],
          },
        }),
      }),
      20,
    );
    wrapper.unmount();
  });
});
