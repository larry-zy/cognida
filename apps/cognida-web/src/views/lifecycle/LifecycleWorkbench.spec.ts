import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
const api = vi.hoisted(() => ({
  agents: vi.fn(),
  templates: vi.fn(),
  assets: vi.fn(),
  asset: vi.fn(),
  list: vi.fn(),
  get: vi.fn(),
  start: vi.fn(),
  createAsset: vi.fn(),
  knowledgeBases: vi.fn(),
}));
vi.mock("@/api/lifecycle", () => ({ lifecycleApi: api }));
const route = vi.hoisted(() => ({ path: "/analysis", query: {} }));
vi.mock("vue-router", () => ({ useRoute: () => route }));
vi.mock("@/utils/request", () => ({
  http: { get: vi.fn().mockResolvedValue({ data: { items: [] } }) },
  resetAuthFailure: vi.fn(),
}));
import LifecycleWorkbench from "./LifecycleWorkbench.vue";
describe("published asset analysis", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });
  beforeEach(() => {
    vi.clearAllMocks();
    route.path = "/analysis";
    api.agents.mockResolvedValue({
      data: {
        agents: [
          {
            id: "data-analysis",
            name: "Analysis",
            tool_allowlist: ["analyze"],
            risk_class: "read",
          },
        ],
      },
    });
    api.templates.mockResolvedValue({ data: { templates: [] } });
    api.assets.mockResolvedValue({
      data: {
        items: [
          {
            asset_id: "published-1",
            version: 2,
            kind: "published_asset",
            status: "published",
          },
        ],
      },
    });
    api.list.mockResolvedValue({ data: { items: [], next_cursor: "" } });
    api.knowledgeBases.mockResolvedValue({ data: { items: [{ id: "kb-1", name: "Policies", status: 1 }], total: 1, page: 1, size: 100 } });
    api.asset.mockResolvedValue({ data: { ref: { asset_id: "published-1", version: 2 }, content: { kind: "text", text: "Policy" } } });
  });
  it("defaults to published inputs and renders independent invocation output", async () => {
    api.start.mockResolvedValue({ data: { id: "run-1" } });
    api.get.mockResolvedValue({
      data: {
        invocation: {
          id: "run-1",
          status: "succeeded",
          output: {
            summary: "Actual analysis",
            refs: [],
            gate: { status: "passed", reason: "validated", evidence_refs: [] },
            decision: {
              verification_status: "unverified",
              risks: ["Limited rows"],
            },
          },
        },
        approvals: [],
      },
    });
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    expect(api.assets).toHaveBeenCalledWith("published_asset", "");
    const selects = wrapper.findAll("select");
    await selects[1].setValue("published-1:2");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith(
      "invocations",
      expect.objectContaining({
        agent_id: "data-analysis",
        input_refs: [{ asset_id: "published-1", version: 2 }],
      }),
    );
    expect(wrapper.text()).toContain("Actual analysis");
    expect(wrapper.text()).toContain("Limited rows");
    wrapper.unmount();
  });
  it("serializes editable table rows in the server column order", async () => {
    api.createAsset.mockResolvedValue({
      data: { asset_id: "raw-1", version: 1 },
    });
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    const form = wrapper.get("details form");
    await form.get("input").setValue("Orders");
    await form.get("select").setValue("table");
    await form
      .get('input[placeholder="date,revenue,region"]')
      .setValue("id,amount");
    const editor = wrapper.findComponent({ name: "RecordEditor" });
    editor.vm.$emit("update:modelValue", [
      { amount: "12", id: "a" },
      { id: "b", amount: "20" },
    ]);
    await form.trigger("submit");
    await flushPromises();
    expect(api.createAsset).toHaveBeenCalledWith(
      {
        kind: "table",
        columns: ["id", "amount"],
        rows: [
          ["a", "12"],
          ["b", "20"],
        ],
      },
      "Orders",
      undefined,
    );
    wrapper.unmount();
  });
  it("keeps the last selected execution when responses arrive out of order", async () => {
    api.list.mockResolvedValue({
      data: {
        items: [
          { id: "first", status: "running" },
          { id: "second", status: "running" },
        ],
      },
    });
    let first!: (value: unknown) => void, second!: (value: unknown) => void;
    api.get.mockImplementation(
      (_kind, id) =>
        new Promise((resolve) => {
          if (id === "first") first = resolve;
          else second = resolve;
        }),
    );
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("first"))!
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("second"))!
      .trigger("click");
    second({
      data: {
        invocation: {
          id: "second",
          status: "succeeded",
          output: { summary: "Second result" },
        },
        approvals: [],
      },
    });
    await flushPromises();
    first({
      data: {
        invocation: {
          id: "first",
          status: "succeeded",
          output: { summary: "Obsolete result" },
        },
        approvals: [],
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("Second result");
    expect(wrapper.text()).not.toContain("Obsolete result");
    wrapper.unmount();
  });
  it("does not duplicate form submission while the first write is pending", async () => {
    let finish!: (value: unknown) => void;
    api.start.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await wrapper.findAll("select")[1].setValue("published-1:2");
    await wrapper.get("form").trigger("submit");
    await wrapper.get("form").trigger("submit");
    expect(api.start).toHaveBeenCalledTimes(1);
    finish({ data: undefined });
    await flushPromises();
    wrapper.unmount();
  });
  it("keeps SSE reconnect alive, coalesces refreshes, and closes when unmounted", async () => {
    route.path = "/pipelines";
    vi.useFakeTimers();
    const streams: Array<{
      onmessage: () => void;
      onerror: () => void;
      close: ReturnType<typeof vi.fn>;
    }> = [];
    vi.stubGlobal(
      "EventSource",
      class {
        onmessage = () => {};
        onerror = () => {};
        close = vi.fn();
        addEventListener = vi.fn();
        constructor() {
          streams.push(this);
        }
      },
    );
    api.list.mockResolvedValue({
      data: { items: [{ id: "run", status: "running" }] },
    });
    let finish!: (value: unknown) => void;
    api.get
      .mockImplementationOnce(() =>
        Promise.resolve({
          data: {
            run: { id: "run", status: "running" },
            stages: [],
            approvals: [],
          },
        }),
      )
      .mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      );
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("run"))!
      .trigger("click");
    await flushPromises();
    const stream = streams[streams.length - 1];
    stream.onerror();
    expect(stream.close).not.toHaveBeenCalled();
    stream.onmessage();
    stream.onmessage();
    await flushPromises();
    expect(api.get.mock.calls.length).toBe(2);
    await vi.advanceTimersByTimeAsync(8000);
    expect(api.get.mock.calls.length).toBe(2);
    wrapper.unmount();
    expect(stream.close).toHaveBeenCalled();
    if (finish)
      finish({
        data: {
          run: { id: "run", status: "succeeded" },
          stages: [],
          approvals: [],
        },
      });
    await flushPromises();
  });
  it("ignores an older history append after a fresh reload", async () => {
    let finish!: (v: unknown) => void;
    api.list
      .mockResolvedValueOnce({
        data: { items: [{ id: "initial" }], next_cursor: "page2" },
      })
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            finish = r;
          }),
      )
      .mockResolvedValueOnce({
        data: { items: [{ id: "fresh" }], next_cursor: "" },
      });
    const wrapper = mount(LifecycleWorkbench, {
      global: { plugins: [createPinia()], stubs: { RouterLink: true } },
    });
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "加载更多")!
      .trigger("click");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "刷新历史")!
      .trigger("click");
    await flushPromises();
    finish({ data: { items: [{ id: "stale-page" }], next_cursor: "page3" } });
    await flushPromises();
    expect(wrapper.text()).toContain("fresh");
    expect(wrapper.text()).not.toContain("stale-page");
    expect(wrapper.text()).not.toContain("initial");
    wrapper.unmount();
  });
  function pipeline(stages = [
    { id: "inspect-input", agent_id: "data-quality" },
    { id: "sanitize-input", agent_id: "data-cleaning" },
    { id: "enrich-input", agent_id: "data-enrichment" },
    { id: "publish-approved", agent_id: "data-asset" },
    { id: "explain-data", agent_id: "data-analysis" },
  ]) {
    route.path = "/pipelines";
    api.templates.mockResolvedValue({ data: { templates: [{ id: "custom", name: "Custom", version: 1, stages }] } });
    api.start.mockResolvedValue({ data: undefined });
    return mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
  }
  async function selectPipeline(wrapper: ReturnType<typeof pipeline>) {
    await flushPromises();
    await wrapper.findAll("select")[0].setValue("custom");
    await wrapper.findAll("select")[1].setValue("published-1:2");
    await flushPromises();
  }
  it("requires a text publication target before submitting a pipeline", async () => {
    const wrapper = pipeline();
    await selectPipeline(wrapper);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("文本发布前请选择目标知识库");
    wrapper.unmount();
  });
  it("maps typed parameters to real template stage IDs, including publication and goal", async () => {
    const wrapper = pipeline();
    await selectPipeline(wrapper);
    await wrapper.get('[data-stage="inspect-input"] [data-option="min_score"]').setValue("85");
    await wrapper.get('[data-stage="sanitize-input"] [data-option="min_score"]').setValue("90");
    await wrapper.get('[data-stage="sanitize-input"] [data-option="columns"]').setValue("text");
    await wrapper.get('[data-stage="sanitize-input"] [data-option="keep"]').setValue("last");
    await wrapper.get('[data-stage="sanitize-input"] input[value="pii_masker"]').setValue(true);
    const enrich = wrapper.get('[data-stage="enrich-input"]');
    await enrich.get('[data-option="method"]').setValue("llm");
    await enrich.findAll("button").find((button) => button.text().includes("添加标签规则"))!.trigger("click");
    await enrich.get('[aria-label="标签 1 名称"]').setValue("Support");
    await enrich.get('[aria-label="标签 1 关键词"]').setValue("refund,delivery");
    await wrapper.get('[data-option="knowledge_base_id"]').setValue("kb-1");
    await wrapper.get('[data-option="prompt"]').setValue("Explain policy changes");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith("runs", {
      template_id: "custom", input_refs: [{ asset_id: "published-1", version: 2 }],
      options: { stage_options: {
        "inspect-input": { min_score: 85 },
        "sanitize-input": { min_score: 90, operations: ["trim", "dedup", "pii_masker"], columns: ["text"], keep: "last" },
        "enrich-input": expect.objectContaining({ method: "llm", labels: [{ name: "Support", keywords: ["refund", "delivery"] }] }),
        "publish-approved": { knowledge_base_id: "kb-1" },
        "explain-data": { prompt: "Explain policy changes" },
      } },
    });
    wrapper.unmount();
  });
  it("allows table publication without a knowledge base", async () => {
    api.asset.mockResolvedValue({ data: { content: { kind: "table", columns: ["revenue"], rows: [[12]] } } });
    const wrapper = pipeline([{ id: "release-table", agent_id: "data-asset" }]);
    await selectPipeline(wrapper);
    expect(wrapper.text()).toContain("表格发布为不可变资产快照");
    expect(wrapper.find('[data-option="knowledge_base_id"]').exists()).toBe(false);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith("runs", expect.objectContaining({ options: { stage_options: { "release-table": {} } } }));
    wrapper.unmount();
  });
  it("rejects unsupported quality values and empty cleaning selections before execution", async () => {
    const wrapper = pipeline([{ id: "clean", agent_id: "data-cleaning" }]);
    await selectPipeline(wrapper);
    await wrapper.get('[data-option="min_score"]').setValue("101");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("清洗后质量阈值须为");
    await wrapper.get('[data-option="min_score"]').setValue("75");
    await wrapper.get('input[value="trim"]').setValue(false);
    await wrapper.get('input[value="dedup"]').setValue(false);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).not.toHaveBeenCalled();
    expect(wrapper.text()).toContain("请选择有效的清洗操作");
    wrapper.unmount();
  });
  it("shows asset authorization failures and does not submit publication", async () => {
    api.asset.mockRejectedValue(new Error("Asset access denied"));
    const wrapper = pipeline([{ id: "release", agent_id: "data-asset" }]);
    await selectPipeline(wrapper);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.text()).toContain("Asset access denied");
    expect(api.start).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("places standalone expert parameters directly in options", async () => {
    route.path = "/experts";
    api.agents.mockResolvedValue({ data: { agents: [{ id: "data-asset", name: "Publisher", tool_allowlist: [], risk_class: "write" }] } });
    api.start.mockResolvedValue({ data: undefined });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    await wrapper.findAll("select")[0].setValue("data-asset");
    await wrapper.findAll("select")[1].setValue("published-1:2");
    await flushPromises();
    await wrapper.get('[data-option="knowledge_base_id"]').setValue("kb-1");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith("invocations", expect.objectContaining({ options: { knowledge_base_id: "kb-1" } }));
    wrapper.unmount();
  });
  it("loads later authorized knowledge-base pages and retries failed pages", async () => {
    api.knowledgeBases
      .mockResolvedValueOnce({ data: { items: [{ id: "kb-1", name: "Policies" }], total: 101, page: 1, size: 100 } })
      .mockRejectedValueOnce(new Error("Connection interrupted"))
      .mockResolvedValueOnce({ data: { items: [{ id: "kb-later", name: "Later target" }], total: 101, page: 2, size: 100 } });
    const wrapper = pipeline([{ id: "release", agent_id: "data-asset" }]);
    await selectPipeline(wrapper);
    await wrapper.findAll("button").find((b) => b.text() === "加载更多知识库")!.trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("Connection interrupted");
    await wrapper.findAll("button").find((b) => b.text() === "重试加载知识库")!.trigger("click");
    await flushPromises();
    expect(api.knowledgeBases.mock.calls).toEqual([[1], [2], [2]]);
    await wrapper.get('[data-option="knowledge_base_id"]').setValue("kb-later");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith("runs", expect.objectContaining({ options: { stage_options: { release: { knowledge_base_id: "kb-later" } } } }));
    wrapper.unmount();
  });
  it("ignores an older input-kind lookup after switching assets", async () => {
    let finish!: (value: unknown) => void;
    api.assets.mockResolvedValue({ data: { items: [{ asset_id: "published-1", version: 2 }, { asset_id: "table", version: 1 }] } });
    api.asset.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }))
      .mockResolvedValueOnce({ data: { content: { kind: "table" } } });
    const wrapper = pipeline([{ id: "release", agent_id: "data-asset" }]);
    await selectPipeline(wrapper);
    await wrapper.findAll("select")[1].setValue("table:1");
    await flushPromises();
    finish({ data: { content: { kind: "text" } } });
    await flushPromises();
    expect(wrapper.find('[data-option="knowledge_base_id"]').exists()).toBe(false);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.start).toHaveBeenCalledWith("runs", expect.objectContaining({ input_refs: [{ asset_id: "table", version: 1 }], options: { stage_options: { release: {} } } }));
    wrapper.unmount();
  });
  it("opens a standalone output as an escaped report with evidence and limitations", async () => {
    api.start.mockResolvedValue({ data: { id: "analysis-run" } });
    api.get.mockResolvedValue({ data: { invocation: { id: "analysis-run", status: "succeeded", output: { summary: "Analysis ready", refs: [{ asset_id: "insight", version: 1, kind: "insight" }], decision: { evidence_refs: [] } } }, approvals: [] } });
    api.asset.mockResolvedValue({ data: { ref: { asset_id: "insight", version: 1, kind: "insight" }, content: { narrative: '<img src=x onerror="alert(1)">Revenue grew', prompt: "Explain revenue", findings: [{ text: "Revenue rose", evidence_ids: ["row-1"] }], evidence: [{ id: "row-1", content: "Revenue: 12" }], limitations: ["Only one period"], decision: { verification_status: "unverified" } } } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    await wrapper.findAll("select")[1].setValue("published-1:2");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    await wrapper.findAll("button").find((b) => b.text().includes("查看产出"))!.trigger("click");
    await flushPromises();
    const report = wrapper.get('[aria-label="分析报告"]');
    expect(report.text()).toContain("Revenue: 12");
    expect(report.text()).toContain("Only one period");
    expect(report.text()).toContain("unverified");
    expect(report.find("img").exists()).toBe(false);
    expect(report.get("a").attributes("href")).toMatch(/^#report-/);
    wrapper.unmount();
  });
});
