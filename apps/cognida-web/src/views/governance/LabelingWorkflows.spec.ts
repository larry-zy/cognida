import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { reactive } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  action: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi
      .fn()
      .mockResolvedValue({
        data: { items: [{ asset_id: "source", version: 1, name: "Source" }] },
      }),
  },
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
const auth = reactive({
  authEpoch: 0,
  user: { id: 1 },
  currentTenant: { id: 1 },
  token: "scope-token",
});
import LabelingWorkbench from "./LabelingWorkbench.vue";
const task = {
  id: "task",
  name: "Review fixture",
  type: "classification",
  labels: [{ id: "sales", name: "销售", keywords: ["订单"] }],
};
const item = {
  id: "item",
  task_id: "task",
  version: 1,
  content: "订单已送达",
  annotation: { labels: [], entities: [], relations: [] },
  review_status: "pending",
  provenance: {
    source: "rule",
    confidence: null,
    decision_status: "uncertain",
    evidence_spans: [
      { label_id: "sales", rule: "keyword", match: "订单", start: 0, end: 2 },
    ],
  },
};
const button = (w: VueWrapper, text: string) =>
  w.findAll("button").find((b) => b.text() === text)!;
const field = (w: VueWrapper, text: string) =>
  w.findAll("label").find((l) => l.text().startsWith(text))!;
async function open(query = "") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/labeling", component: LabelingWorkbench }],
  });
  await router.push("/labeling" + query);
  await router.isReady();
  const wrapper = mount(LabelingWorkbench, { global: { plugins: [router] } });
  await flushPromises();
  return { wrapper, router };
}
async function createDraft(w: VueWrapper) {
  await field(w, "任务名称").get("input").setValue("Sample task");
  await field(w, "来源资产").get("select").setValue("source:1");
}
beforeEach(() => {
  vi.clearAllMocks();
  auth.currentTenant.id = 1;
  auth.authEpoch = 0;
  api.list.mockImplementation((path: string) =>
    Promise.resolve({ data: { items: path.endsWith("/items") ? [item] : [] } }),
  );
  api.get.mockResolvedValue({ data: task });
  api.create.mockResolvedValue({ data: task });
  api.update.mockResolvedValue({ data: { ...item, version: 2 } });
  api.action.mockResolvedValue({ data: {} });
});
describe("rules, QA and governed review", () => {
  it("creates tagging with an explicit keyword label schema and never the legacy tag_schema", async () => {
    const { wrapper } = await open();
    await button(wrapper, "智能打标").trigger("click");
    await flushPromises();
    await createDraft(wrapper);
    await button(wrapper, "＋ 添加标签").trigger("click");
    await wrapper.get('[aria-label="标签1 标签名称"]').setValue("销售");
    await wrapper.get('[aria-label="标签1 匹配关键词"]').setValue("订单, 销售");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/tagging-tasks",
      expect.objectContaining({
        config: expect.objectContaining({
          label_schema: [
            expect.objectContaining({
              name: "销售",
              keywords: ["订单", "销售"],
            }),
          ],
        }),
      }),
    );
    expect(api.create.mock.calls[0][1].config).not.toHaveProperty("tag_schema");
    wrapper.unmount();
  });
  it("submits entity dictionary terms separately from classification keywords", async () => {
    const { wrapper } = await open();
    await createDraft(wrapper);
    await field(wrapper, "标注类型").get("select").setValue("named_entity");
    await button(wrapper, "＋ 添加标签").trigger("click");
    await wrapper.get('[aria-label="标签1 标签名称"]').setValue("组织");
    await wrapper
      .get('[aria-label="标签1 实体词条"]')
      .setValue("星河公司,银河公司");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.create.mock.calls[0][1]).toMatchObject({
      type: "named_entity",
      labels: [{ name: "组织", terms: ["星河公司", "银河公司"] }],
    });
    wrapper.unmount();
  });
  it("validates QA mapping and sends no artificial labels or answers", async () => {
    const { wrapper } = await open();
    await createDraft(wrapper);
    await field(wrapper, "标注类型").get("select").setValue("qa");
    await field(wrapper, "问题列").get("input").setValue("question");
    await field(wrapper, "参考答案列").get("input").setValue("question");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.create).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("两个不同的非空文本列名");
    await field(wrapper, "参考答案列").get("input").setValue("answer");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith("/annotation-tasks", {
      name: "Sample task",
      type: "qa",
      source_ref_id: "source:1",
      labels: [],
      qa_mapping: {
        question_column: "question",
        reference_answer_column: "answer",
      },
    });
    wrapper.unmount();
  });
  it("displays uncertain status and exact evidence spans without a fake confidence score", async () => {
    const { wrapper } = await open("?task_id=task");
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("订单已送达"))!
      .trigger("click");
    expect(wrapper.text()).toContain("未确定：规则未命中或存在歧义");
    expect(wrapper.get('[aria-label="规则命中依据"]').text()).toContain(
      "字符 [0, 2)",
    );
    expect(wrapper.text()).toContain("置信度：未评估");
    expect(api.action).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("keeps relation tasks manual and disables preannotation", async () => {
    api.get.mockResolvedValue({ data: { ...task, type: "relation" } });
    const { wrapper } = await open("?task_id=task");
    expect(button(wrapper, "生成预标注").attributes("disabled")).toBeDefined();
    await button(wrapper, "生成预标注").trigger("click");
    expect(api.action).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("edits QA and reviews the returned new version rather than the old suggestion", async () => {
    const qaItem = {
      ...item,
      annotation: {
        labels: [],
        entities: [],
        relations: [],
        qa: { question: "原始问题", reference_answer: "原始答案" },
      },
    };
    api.get.mockResolvedValue({ data: { ...task, type: "qa", labels: [] } });
    api.list.mockImplementation((path: string) =>
      Promise.resolve({
        data: { items: path.endsWith("/items") ? [qaItem] : [] },
      }),
    );
    api.update.mockResolvedValue({
      data: {
        ...qaItem,
        version: 2,
        annotation: {
          ...qaItem.annotation,
          qa: { question: "原始问题", reference_answer: "人工修正" },
        },
      },
    });
    const { wrapper } = await open("?task_id=task");
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("订单已送达"))!
      .trigger("click");
    await field(wrapper, "参考答案").get("textarea").setValue("人工修正");
    await button(wrapper, "审核通过").trigger("click");
    await flushPromises();
    expect(api.update).toHaveBeenCalledWith(
      "/annotation-tasks/task/items/item",
      expect.objectContaining({
        expected_version: 1,
        annotation: expect.objectContaining({
          qa: { question: "原始问题", reference_answer: "人工修正" },
        }),
      }),
    );
    expect(api.action).toHaveBeenCalledWith(
      "/annotation-tasks/task/items/item/review",
      { decision: "approved", comment: "", expected_version: 2 },
    );
    wrapper.unmount();
  });
  it("resolves governed execution links read-only and clears detail on denied replacement", async () => {
    api.get.mockImplementation((path: string) =>
      Promise.resolve({
        data: path.startsWith("/governance-executions/")
          ? { domain_id: "task", domain_kind: "annotation_task" }
          : task,
      }),
    );
    const { wrapper, router } = await open("?execution_id=execution");
    expect(api.get).toHaveBeenCalledWith("/annotation-tasks/task");
    expect(wrapper.text()).toContain("Review fixture");
    expect(api.action).not.toHaveBeenCalled();
    api.get.mockRejectedValue(new Error("无权查看"));
    await router.push("/labeling?execution_id=private");
    await flushPromises();
    expect(wrapper.text()).not.toContain("Review fixture");
    expect(wrapper.get('[role="alert"]').text()).toBe("无权查看");
    wrapper.unmount();
  });
  it("discards an old item response after a tenant switch", async () => {
    let finish!: (value: unknown) => void;
    api.list.mockImplementation((path: string) =>
      path.endsWith("/items")
        ? new Promise((resolve) => (finish = resolve))
        : Promise.resolve({ data: { items: [] } }),
    );
    const { wrapper } = await open("?task_id=task");
    api.get.mockRejectedValue(new Error("无权查看"));
    auth.currentTenant.id = 2;
    await flushPromises();
    finish({ data: { items: [item] } });
    await flushPromises();
    expect(wrapper.text()).not.toContain("订单已送达");
    expect(wrapper.text()).not.toContain("Review fixture");
    wrapper.unmount();
  });
  it("exports using both identity and tenant and never downloads an obsolete response", async () => {
    let finish!: (value: unknown) => void;
    const fetchMock = vi
      .fn()
      .mockImplementation(() => new Promise((resolve) => (finish = resolve)));
    vi.stubGlobal("fetch", fetchMock);
    const createURL = vi.fn();
    vi.stubGlobal(
      "URL",
      Object.assign(URL, {
        createObjectURL: createURL,
        revokeObjectURL: vi.fn(),
      }),
    );
    const { wrapper } = await open("?task_id=task");
    await button(wrapper, "导出 JSONL").trigger("click");
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/annotation-tasks/task/export?format=jsonl",
      { headers: { Authorization: "Bearer scope-token", "X-Tenant-ID": "1" } },
    );
    api.get.mockRejectedValue(new Error("无权查看"));
    auth.currentTenant.id = 2;
    await flushPromises();
    finish({ ok: true, blob: async () => new Blob(["private"]) });
    await flushPromises();
    expect(createURL).not.toHaveBeenCalled();
    wrapper.unmount();
    vi.unstubAllGlobals();
  });
});
