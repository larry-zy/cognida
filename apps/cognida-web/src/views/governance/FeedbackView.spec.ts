import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { mount, flushPromises, type VueWrapper } from "@vue/test-utils";
import { reactive } from "vue";
import { createRouter, createMemoryHistory } from "vue-router";
const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  publications: vi.fn(),
  create: vi.fn(),
  correct: vi.fn(),
  rework: vi.fn(),
  candidate: vi.fn(),
  control: vi.fn(),
  withdraw: vi.fn(),
  rollback: vi.fn(),
  assets: vi.fn(),
  asset: vi.fn(),
  capabilities: vi.fn(),
  evidenceResults: vi.fn(),
  versions: vi.fn(),
  summary: vi.fn(),
}));
vi.mock("@/api/feedback", () => ({ feedbackApi: mocks }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: { assets: mocks.assets, asset: mocks.asset },
}));
vi.mock("@/api/taskcontrol", () => ({
  taskControlApi: { capabilities: mocks.capabilities, summary: mocks.summary },
}));
vi.mock("@/api/evaluation/versions", () => ({
  datasetVersionsApi: { list: mocks.versions },
}));
vi.mock("@/api/knowledge", () => ({
  knowledgeApi: {
    getPage: vi.fn(async () => ({
      data: { items: [{ id: "kb", name: "Knowledge" }], total: 1 },
    })),
  },
}));
vi.mock("@/api/governance", () => ({
  governanceApi: { list: vi.fn(async () => ({ data: { items: [] } })) },
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
const auth = reactive({
  authEpoch: 0,
  user: { id: 1 },
  currentTenant: { id: 1 },
});
import FeedbackView from "./FeedbackView.vue";
const input = {
  asset_id: "base",
  version: 1,
  kind: "raw_asset",
  status: "available",
  name: "Knowledge asset",
  provenance: { source_type: "knowledge", source_id: "doc" },
};
const caseRow = () => ({
  id: "case",
  version: 1,
  status: "open",
  title: "Real feedback",
  baseline_ref: input,
  evidence: { question: "q", problem: "wrong answer" },
  gate: { metric: "rouge_l", min_pairs: 2, min_improvement: 0.1 },
  budget: { max_calls: 100, max_rounds: 2 },
  reserved_calls: 0,
  rounds: [],
  corrections: [],
});
const wrappers: VueWrapper[] = [];
const button = (w: VueWrapper, text: string) =>
  w.findAll("button").find((v) => v.text() === text)!;
const field = (w: VueWrapper, text: string) =>
  w.findAll("label").find((v) => v.text().startsWith(text))!;
async function open(query = "") {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/feedback", component: FeedbackView },
      { path: "/tasks", component: { template: "<p>Task</p>" } },
      { path: "/analysis", component: { template: "<p>Asset</p>" } },
    ],
  });
  await router.push("/feedback" + query);
  await router.isReady();
  const w = mount(
    { template: "<router-view />" },
    { global: { plugins: [router] } },
  );
  wrappers.push(w);
  await flushPromises();
  return { w, router };
}
beforeEach(() => {
  vi.clearAllMocks();
  auth.authEpoch = 0;
  auth.user.id = 1;
  auth.currentTenant.id = 1;
  mocks.versions.mockResolvedValue({
    data: {
      items: [
        {
          id: "dataset-v",
          dataset_id: "reviewed",
          profile: "rag",
          name: "Verified",
          version: 1,
        },
      ],
      next_cursor: "",
    },
  });
  mocks.list.mockResolvedValue({
    data: { items: [caseRow()], next_cursor: "" },
  });
  mocks.get.mockResolvedValue({ data: caseRow() });
  mocks.publications.mockResolvedValue({ data: { items: [] } });
  mocks.assets.mockResolvedValue({ data: { items: [input], next_cursor: "" } });
  mocks.asset.mockResolvedValue({
    data: { ref: input, content: { kind: "text", text: "source" } },
  });
  mocks.capabilities.mockResolvedValue({
    data: {
      capabilities: [
        {
          id: "lifecycle.data-cleaning",
          version: 1,
          input_kinds: ["raw_asset"],
          output_kinds: ["cleaned_asset"],
          tools: ["clean"],
          permissions: ["read"],
          execution: {
            base_calls: 2,
            calls_per_input: 0,
            max_timeout_seconds: 60,
          },
        },
      ],
    },
  });
  vi.spyOn(window, "confirm").mockReturnValue(false);
});
afterEach(() => {
  for (const w of wrappers.splice(0)) w.unmount();
  vi.restoreAllMocks();
});
describe("feedback evidence workflow", () => {
  it("keeps a failed dataset selector explicit and allows a safe read retry without submission", async () => {
    mocks.versions.mockRejectedValueOnce({ response: { status: 403 } });
    const { w } = await open();
    expect(w.text()).toContain("评测版本读取失败");
    await button(w, "重新读取评测版本").trigger("click");
    await flushPromises();
    expect(w.text()).not.toContain("评测版本读取失败");
    expect(field(w, "固定评测版本").text()).toContain("Verified");
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it("loads an authorized case deep link even when its independent version selector fails", async () => {
    mocks.versions.mockRejectedValueOnce({ response: { status: 403 } });
    const { w } = await open("?case_id=case");
    expect(mocks.get).toHaveBeenCalledWith("case");
    expect(w.text()).toContain("Real feedback");
    expect(w.text()).toContain("评测版本读取失败");
    await button(w, "重新读取评测版本").trigger("click");
    await flushPromises();
    expect(w.text()).not.toContain("评测版本读取失败");
    expect(w.text()).toContain("Real feedback");
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it("loads referenced evaluation text through an authorized read and ignores text supplied in the URL", async () => {
    mocks.evidenceResults.mockResolvedValue({
      data: {
        results: [{ id: 7, question: "authorized original" }],
        next_cursor: "",
      },
    });
    const { w } = await open(
      "?task_id=evaluation&result_id=7&question=untrusted",
    );
    expect(mocks.evidenceResults).toHaveBeenCalledWith("evaluation", "");
    expect(field(w, "原始问题").get("textarea").element.value).toBe(
      "authorized original",
    );
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it("rejects a stale evaluation reference response after identity changes", async () => {
    let resolve!: (value: unknown) => void;
    mocks.evidenceResults
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            resolve = r;
          }),
      )
      .mockRejectedValue({ response: { status: 403 } });
    const { w } = await open("?task_id=evaluation&result_id=7");
    auth.authEpoch++;
    await flushPromises();
    resolve({
      data: {
        results: [{ id: 7, question: "old private question" }],
        next_cursor: "next",
      },
    });
    await flushPromises();
    expect(field(w, "原始问题").get("textarea").element.value).toBe("");
    expect(mocks.evidenceResults).toHaveBeenCalledTimes(2);
    expect(mocks.create).not.toHaveBeenCalled();
  });
  it("resolves an output reference before selecting it and never executes a deep link", async () => {
    const { w } = await open("?output_ref=base:1");
    expect(mocks.asset).toHaveBeenCalledWith("base", 1);
    expect(field(w, "问题输出资产").get("select").element.value).toBe("base:1");
    expect(mocks.create).not.toHaveBeenCalled();
    expect(mocks.rework).not.toHaveBeenCalled();
  });
  it("opens a deep link read-only and hides publication actions before actual validation", async () => {
    const { w } = await open("?case_id=case");
    expect(mocks.get).toHaveBeenCalledWith("case");
    expect(w.text()).toContain("Real feedback");
    expect(mocks.rework).not.toHaveBeenCalled();
    expect(mocks.control).not.toHaveBeenCalled();
    expect(button(w, "审核通过")).toBeUndefined();
    expect(button(w, "激活已批准版本")).toBeUndefined();
  });
  it("creates a fixed dataset/configuration case with explicit input evidence only on submit", async () => {
    mocks.create.mockResolvedValue({ data: caseRow() });
    const { w, router } = await open();
    await field(w, "反馈标题").get("input").setValue("Repair");
    await field(w, "基线知识资产").get("select").setValue("base:1");
    await field(w, "问题输出资产").get("select").setValue("base:1");
    await field(w, "目标知识库").get("select").setValue("kb");
    await field(w, "固定评测版本").get("select").setValue("dataset-v");
    await field(w, "原始问题").get("textarea").setValue("question");
    await field(w, "发现的错误").get("textarea").setValue("problem");
    expect(mocks.create).not.toHaveBeenCalled();
    await w.get('form[aria-label="记录质量反馈"]').trigger("submit");
    await flushPromises();
    expect(mocks.create).toHaveBeenCalledWith(
      expect.objectContaining({
        dataset_version_id: "dataset-v",
        baseline_ref: { asset_id: "base", version: 1 },
        evaluation_config: {
          dataset_id: "reviewed",
          type: "rag",
          kb_id: "kb",
          config: { top_k: 5, graders: ["rouge", "bleu"] },
        },
      }),
    );
    expect(router.currentRoute.value.query.case_id).toBe("case");
    expect(mocks.rework).not.toHaveBeenCalled();
  });
  it("does not continue GET-to-rework POST after identity changes", async () => {
    const { w } = await open("?case_id=case");
    let resolve!: (v: unknown) => void;
    mocks.asset.mockReturnValue(new Promise((r) => (resolve = r)));
    await button(w, "创建并启动返工").trigger("click");
    await flushPromises();
    auth.authEpoch++;
    auth.user.id = 2;
    await flushPromises();
    resolve({ data: { ref: input } });
    await flushPromises();
    expect(mocks.rework).not.toHaveBeenCalled();
  });
  it("discarded detail requests cannot replace the new task or reveal the old report", async () => {
    let resolve!: (v: unknown) => void;
    mocks.get.mockImplementation((id: string) =>
      id === "old"
        ? new Promise((r) => (resolve = r))
        : Promise.resolve({
            data: { ...caseRow(), id: "new", title: "New scope" },
          }),
    );
    const { w } = await open("?case_id=old");
    auth.authEpoch++;
    mocks.get.mockResolvedValue({
      data: { ...caseRow(), id: "new", title: "New scope" },
    });
    await flushPromises();
    resolve({ data: { ...caseRow(), title: "Private old report" } });
    await flushPromises();
    expect(w.text()).not.toContain("Private old report");
    expect(w.text()).toContain("New scope");
  });
  it("keeps unsubmitted correction on same-route navigation when confirmation is dismissed", async () => {
    const { w, router } = await open("?case_id=case");
    await field(w, "建议的完整修正文本")
      .get("textarea")
      .setValue("unfinished correction");
    await router.push("/feedback?case_id=other");
    expect(router.currentRoute.value.query.case_id).toBe("case");
    expect(field(w, "建议的完整修正文本").get("textarea").element.value).toBe(
      "unfinished correction",
    );
    expect(mocks.get).not.toHaveBeenCalledWith("other");
  });
  it("requires explicit review then activation and coalesces repeated clicks", async () => {
    mocks.publications.mockResolvedValue({
      data: {
        items: [
          { id: "pub", version: 3, status: "validated", source_ref: input },
        ],
      },
    });
    const { w } = await open("?case_id=case");
    expect(button(w, "审核通过").attributes("disabled")).toBeDefined();
    await field(w, "审核理由").get("input").setValue("checked real pairs");
    let resolve!: (v: unknown) => void;
    mocks.control.mockReturnValue(new Promise((r) => (resolve = r)));
    await button(w, "审核通过").trigger("click");
    await button(w, "审核通过").trigger("click");
    expect(mocks.control).toHaveBeenCalledTimes(1);
    expect(mocks.control.mock.calls[0]![1]).toBe("approve");
    expect(button(w, "激活已批准版本")).toBeUndefined();
    mocks.publications.mockResolvedValue({
      data: {
        items: [
          { id: "pub", version: 4, status: "approved", source_ref: input },
        ],
      },
    });
    resolve({ data: {} });
    await flushPromises();
    expect(button(w, "激活已批准版本")).toBeDefined();
    expect(mocks.control).toHaveBeenCalledTimes(1);
  });
});
