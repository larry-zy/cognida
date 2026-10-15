import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { createPinia } from "pinia";
import { reactive } from "vue";
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
const navigation = vi.hoisted(() => ({ leave: vi.fn() }));
vi.mock("vue-router", () => ({ useRoute: () => reactive(route), onBeforeRouteLeave: navigation.leave }));
vi.mock("@/utils/request", () => ({
  http: { get: vi.fn().mockResolvedValue({ data: { items: [] } }) },
  resetAuthFailure: vi.fn(),
}));
import LifecycleWorkbench from "./LifecycleWorkbench.vue";
describe("published asset analysis", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
  beforeEach(() => {
    vi.clearAllMocks();
    route.path = "/analysis";
    route.query = {};
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
  it("prefills a collection item draft without starting collection or a pipeline", async () => {
    route.path = "/pipelines";
    route.query = { source: "collection", run_id: "actual-run", item_id: "actual-item", name: "Source page" };
    api.createAsset.mockResolvedValue({ data: { asset_id: "collected-input", version: 1 } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    const form = wrapper.get("details form");
    expect(form.element.closest("details")?.open).toBe(true);
    expect((form.get("select").element as HTMLSelectElement).value).toBe("collection");
    expect((form.get("input").element as HTMLInputElement).value).toBe("Source page");
    expect(form.findAll("input").map(input => (input.element as HTMLInputElement).value)).toEqual(["Source page", "actual-run", "actual-item"]);
    expect(api.createAsset).not.toHaveBeenCalled();
    expect(api.start).not.toHaveBeenCalled();
    await form.trigger("submit");
    await flushPromises();
    expect(api.createAsset).toHaveBeenCalledWith(expect.anything(), "Source page", expect.objectContaining({ kind: "collection", run_id: "actual-run", item_id: "actual-item" }));
    expect(api.start).not.toHaveBeenCalled();
    wrapper.unmount();
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
  it.each([
    ["knowledge", { knowledge_id: "document-1" }],
    ["evaluation", { run_id: "evaluation-1" }],
    ["result", { session_id: "session-1", result_id: "result-1" }],
  ])("imports only %s source fields from a mixed query", async (source, fields) => {
    route.path = "/pipelines";
    route.query = { source, name: "Imported source", run_id: "unrelated-run", item_id: "unrelated-item", session_id: "unrelated-session", result_id: "unrelated-result", knowledge_id: "unrelated-document", ...fields };
    api.createAsset.mockResolvedValue({ data: { asset_id: "created", version: 1 } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(api.createAsset).not.toHaveBeenCalled();
    await wrapper.get("details form").trigger("submit");
    await flushPromises();
    expect(api.createAsset.mock.calls[0][2]).toEqual({ kind: source, ...fields });
    expect(api.start).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it.each([
    { source: "result", session_id: "session-only" },
    { source: "collection", run_id: "run-only" },
    { source: "knowledge", knowledge_id: ["one", "two"] },
    { source: "toString", run_id: "run" },
  ])("ignores incomplete or ambiguous source queries", async (query) => {
    route.query = query;
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect((wrapper.get("details form select").element as HTMLSelectElement).value).toBe("text");
    expect(api.createAsset).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("clears old handles when an existing page receives an incomplete source link", async () => {
    route.query = { source: "collection", run_id: "run", item_id: "item" };
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    reactive(route).query = { source: "knowledge", knowledge_id: "document" };
    await flushPromises();
    expect((wrapper.get("details form select").element as HTMLSelectElement).value).toBe("knowledge");
    expect(wrapper.get("details form").findAll("input").map(input => (input.element as HTMLInputElement).value)).toEqual(["导入来源", "document"]);
    reactive(route).query = { source: "collection", run_id: "run-only" };
    await flushPromises();
    expect((wrapper.get("details form select").element as HTMLSelectElement).value).toBe("text");
    expect(api.createAsset).not.toHaveBeenCalled();
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
  it("uses explicit numeric columns while preserving text identifiers", async () => {
    api.createAsset.mockResolvedValue({ data: { asset_id: "table", version: 1 } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    const form = wrapper.get("details form");
    await form.get("select").setValue("table");
    await form.get('input[placeholder="date,revenue,region"]').setValue("id,revenue");
    await form.get('[aria-label="revenue 类型"]').setValue("number");
    const editor = wrapper.findComponent({ name: "RecordEditor" });
    editor.vm.$emit("update:modelValue", [{ id: "000123", revenue: "100" }, { id: "9007199254740993", revenue: "200.5" }, { id: "003", revenue: "" }]);
    await flushPromises();
    expect(form.get('[aria-label="数据行1 revenue"]').attributes("type")).toBe("number");
    expect(form.get('[aria-label="数据行1 id"]').attributes("type")).toBe("text");
    await form.trigger("submit");
    await flushPromises();
    expect(api.createAsset.mock.calls[0][0].rows).toEqual([["000123", 100], ["9007199254740993", 200.5], ["003", null]]);
    wrapper.unmount();
  });
  it.each(["1e309", "NaN", "9007199254740993", "-9007199254740992"])("rejects non-finite or unsafe numeric value %s before submitting", async (value) => {
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    const form = wrapper.get("details form");
    await form.get("select").setValue("table");
    await form.get('input[placeholder="date,revenue,region"]').setValue("amount");
    await form.get('[aria-label="amount 类型"]').setValue("number");
    wrapper.findComponent({ name: "RecordEditor" }).vm.$emit("update:modelValue", [{ amount: value }]);
    await form.trigger("submit");
    await flushPromises();
    expect(api.createAsset).not.toHaveBeenCalled();
    expect(wrapper.get('[role="alert"]').text()).toMatch(/有限数值|安全整数范围/);
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
  it("opens a linked invocation read-only and isolates it from source draft parameters", async () => {
    route.path = '/experts';
    route.query = { invocation_id: 'linked-invocation' };
    api.get.mockResolvedValue({ data: { invocation: { id: 'linked-invocation', status: 'succeeded', output: { summary: 'Linked result', refs: [] } }, approvals: [] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(api.get).toHaveBeenCalledWith('invocations', 'linked-invocation');
    expect(wrapper.text()).toContain('Linked result');
    reactive(route).query = { source: 'collection', run_id: 'collection-run', item_id: 'source-item' };
    await flushPromises();
    expect(wrapper.text()).not.toContain('Linked result');
    expect(api.get).toHaveBeenCalledTimes(1);
    expect(api.createAsset).not.toHaveBeenCalled(); expect(api.start).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("does not reveal an older run after a linked query switches to a denied run", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'old-run' };
    let finish!: (value: unknown) => void;
    api.get.mockImplementation((_kind, id) => id === 'old-run' ? new Promise(resolve => { finish = resolve; }) : Promise.reject(new Error('Run not found or not authorized')));
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.text()).toContain('正在读取执行详情');
    reactive(route).query = { run_id: 'denied-run' }; await flushPromises();
    expect(wrapper.text()).toContain('Run not found or not authorized');
    finish({ data: { run: { id: 'old-run', status: 'succeeded', output: { summary: 'Old private report' } }, stages: [], approvals: [] } });
    await flushPromises();
    expect(wrapper.text()).not.toContain('Old private report');
    expect(api.start).not.toHaveBeenCalled();
    reactive(route).query = {}; await flushPromises();
    expect(wrapper.text()).not.toContain('Run not found or not authorized');
    wrapper.unmount();
  });
  it("keeps advanced defaults collapsed and reveals the stage with invalid options", async () => {
    const wrapper = pipeline(); await selectPipeline(wrapper);
    const quality = wrapper.get('details[data-options-stage="inspect-input"]');
    expect((quality.element as HTMLDetailsElement).open).toBe(false);
    expect((wrapper.get('details[data-options-stage="publish-approved"]').element as HTMLDetailsElement).open).toBe(true);
    expect((wrapper.get('details[data-options-stage="explain-data"]').element as HTMLDetailsElement).open).toBe(true);
    await quality.get('[data-option="min_score"]').setValue('101');
    await wrapper.get('form').trigger('submit'); await flushPromises();
    expect((quality.element as HTMLDetailsElement).open).toBe(true);
    expect(api.start).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("prioritizes pending approval and reports actual completed stage counts", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'review-run' };
    api.get.mockResolvedValue({ data: { run: { id: 'review-run', status: 'awaiting_approval' }, stages: [
      { id: 'stage-one', stage_key: 'access', agent_id: 'data-access', status: 'succeeded', attempt: 1 },
      { id: 'stage-two', stage_key: 'publication', agent_id: 'data-asset', status: 'awaiting_approval', attempt: 1 },
    ], approvals: [{ id: 'approval', status: 'pending', reason: 'Confirm the publication destination' }] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.get('progress').attributes('value')).toBe('1');
    expect(wrapper.get('progress').attributes('max')).toBe('2');
    expect(wrapper.text()).toContain('下一步：审核后继续执行');
    expect(wrapper.html().indexOf('批准并继续')).toBeLessThan(wrapper.html().indexOf('class="pipeline-stages"'));
    expect(api.start).not.toHaveBeenCalled(); wrapper.unmount();
  });
  it.each(['cancelled', 'quarantined'])("guides %s runs to new configuration instead of unsupported retry", async (status) => {
    route.path = '/pipelines'; route.query = { run_id: 'terminal-run' };
    api.get.mockResolvedValue({ data: { run: { id: 'terminal-run', status }, stages: [], approvals: [] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.findAll('button').some(button => button.text() === '重试失败任务')).toBe(false);
    expect(wrapper.text()).toContain('重新发起任务'); wrapper.unmount();
  });
  it("keeps a cancelled run's pending approval read-only", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'cancelled-run' };
    api.get.mockResolvedValue({ data: { run: { id: 'cancelled-run', status: 'cancelled' }, stages: [], approvals: [{ id: 'obsolete-approval', status: 'pending', reason: 'Original publication request' }] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.text()).not.toContain('下一步：审核后继续执行');
    expect(wrapper.findAll('button').some(button => button.text() === '批准并继续')).toBe(false);
    expect(wrapper.text()).toContain('此审批记录仅供查看');
    expect(wrapper.text()).toContain('Original publication request'); wrapper.unmount();
  });
  it("opens the report shortcut and focuses the actual report panel", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'completed-run' };
    api.get.mockResolvedValue({ data: { run: { id: 'completed-run', status: 'succeeded' }, stages: [{ id: 'analysis', stage_key: 'analysis', agent_id: 'data-analysis', status: 'succeeded', attempt: 1, output: { refs: [{ asset_id: 'actual-insight', version: 3, kind: 'insight' }] } }], approvals: [] } });
    api.asset.mockResolvedValue({ data: { ref: { asset_id: 'actual-insight', version: 3 }, content: { source_ref: 'published:1', record_count: 3, columns: [{ name: 'revenue', numeric_count: 3, mean: 200 }] } } });
    const wrapper = mount(LifecycleWorkbench, { attachTo: document.body, global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    await wrapper.findAll('button').find(button => button.text() === '查看分析报告')!.trigger('click'); await flushPromises();
    expect(api.asset).toHaveBeenCalledWith('actual-insight', 3);
    expect(wrapper.get('[aria-label="分析报告"]').text()).toContain('200');
    expect(document.activeElement).toBe(wrapper.get('.asset-result').element);
    wrapper.unmount(); wrapper.element.remove();
  });
  it("warns before discarding an edited input but allows leaving after successful creation", async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    api.createAsset.mockResolvedValue({ data: { asset_id: 'created-input', version: 1 } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    await wrapper.get('details form input').setValue('Unsaved input');
    const guard = navigation.leave.mock.calls.at(-1)![0];
    expect(guard()).toBe(false); expect(confirm).toHaveBeenCalledOnce();
    const pending = new Event('beforeunload', { cancelable: true }); window.dispatchEvent(pending); expect(pending.defaultPrevented).toBe(true);
    await wrapper.get('details form').trigger('submit'); await flushPromises();
    expect(guard()).toBe(true);
    const submitted = new Event('beforeunload', { cancelable: true }); window.dispatchEvent(submitted); expect(submitted.defaultPrevented).toBe(false);
    wrapper.unmount();
  });
  it("orders execution cards by the running template even when another launch template is selected", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'existing-run' };
    const keys = ['input-v2', 'quality-v2', 'clean-v2', 'enrich-v2', 'publish-v2', 'analyze-v2', 'evaluate-v2'];
    const agentIDs = ['data-access', 'data-quality', 'data-cleaning', 'data-enrichment', 'data-asset', 'data-analysis', 'data-evaluation'];
    const definition = keys.map((id, index) => ({ id, agent_id: agentIDs[index], depends_on: [] }));
    api.templates.mockResolvedValue({ data: { templates: [
      { id: 'running-template', name: '当前流程', version: 1, stages: definition },
      { id: 'other-template', name: '待发起的另一流程', version: 1, stages: [...definition].reverse() },
    ] } });
    const shuffled = [
      { id: 'unknown-z', stage_key: 'extension-z', agent_id: 'extension', status: 'waiting', attempt: 1 },
      ...[6, 1, 3, 0, 5, 2, 4].map(index => ({ id: `random-id-${index}`, stage_key: keys[index], agent_id: agentIDs[index], status: index === 6 ? 'queued' : 'succeeded', attempt: 1 })),
      { id: 'unknown-a', stage_key: 'extension-a', agent_id: 'extension', status: 'waiting', attempt: 1 },
    ];
    const originalOrder = shuffled.map(stage => stage.stage_key);
    api.get.mockResolvedValue({ data: { run: { id: 'existing-run', template_id: 'running-template', template_version: 1, status: 'running' }, stages: shuffled, approvals: [] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    await wrapper.get('.launch-form select').setValue('other-template'); await flushPromises();
    expect(wrapper.findAll('.pipeline-stage strong small').map(node => node.text())).toEqual([...keys, 'extension-z', 'extension-a']);
    expect(wrapper.get('.journey-panel h2').text()).toBe('当前执行流程 · 当前流程');
    expect(wrapper.findAll('.journey-panel li strong').map(node => node.text())).toEqual(['数据接入', '质量检查', '数据清洗', '数据增强', '审批与发布', '分析报告', '结果评估']);
    expect(wrapper.get('progress').attributes('value')).toBe('6');
    expect(wrapper.get('progress').attributes('max')).toBe('9');
    expect(shuffled.map(stage => stage.stage_key)).toEqual(originalOrder);
    wrapper.unmount();
  });
  it("uses the stable built-in execution order without borrowing an unrelated template", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'historical-run' };
    api.templates.mockResolvedValue({ data: { templates: [{ id: 'new-template', name: 'New template', version: 1, stages: [{ id: 'evaluation', agent_id: 'data-evaluation' }, { id: 'access', agent_id: 'data-access' }] }] } });
    api.get.mockResolvedValue({ data: { run: { id: 'historical-run', template_id: 'unavailable-template', status: 'succeeded' }, stages: [
      { id: 'eval', stage_key: 'evaluation', agent_id: 'data-evaluation', status: 'succeeded', attempt: 1 },
      { id: 'input', stage_key: 'access', agent_id: 'data-access', status: 'succeeded', attempt: 1 },
    ], approvals: [] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises(); await wrapper.get('.launch-form select').setValue('new-template');
    expect(wrapper.findAll('.pipeline-stage strong small').map(node => node.text())).toEqual(['access', 'evaluation']);
    expect(wrapper.find('.journey-panel').exists()).toBe(false);
    wrapper.unmount();
  });
  it("does not apply a newer definition to a historical run with the same template ID", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'historical-version' };
    api.templates.mockResolvedValue({ data: { templates: [{ id: 'versioned-template', name: '新版流程', version: 2, stages: [
      { id: 'evaluation', agent_id: 'data-evaluation', depends_on: [] },
      { id: 'new-stage', agent_id: 'data-quality', depends_on: [] },
      { id: 'access', agent_id: 'data-access', depends_on: [] },
    ] }] } });
    api.get.mockResolvedValue({ data: { run: { id: 'historical-version', template_id: 'versioned-template', template_version: 1, status: 'succeeded' }, stages: [
      { id: 'eval', stage_key: 'evaluation', agent_id: 'data-evaluation', status: 'succeeded', attempt: 1 },
      { id: 'old-z', stage_key: 'legacy-z', agent_id: 'extension', status: 'succeeded', attempt: 1 },
      { id: 'input', stage_key: 'access', agent_id: 'data-access', status: 'succeeded', attempt: 1 },
      { id: 'old-a', stage_key: 'legacy-a', agent_id: 'extension', status: 'succeeded', attempt: 1 },
    ], approvals: [] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.findAll('.pipeline-stage strong small').map(node => node.text())).toEqual(['access', 'evaluation', 'legacy-z', 'legacy-a']);
    expect(wrapper.find('.journey-panel').exists()).toBe(false);
    expect(wrapper.get('progress').attributes('value')).toBe('4');
    expect(wrapper.get('progress').attributes('max')).toBe('4');
    expect(wrapper.find('.pipeline-stage').text()).not.toContain('new-stage');
    wrapper.unmount();
  });
  it("translates fixed approval and artifact labels while preserving unknown messages verbatim", async () => {
    route.path = '/pipelines'; route.query = { run_id: 'review-run' };
    const unknownError = 'upstream gateway: TLS certificate rejected <code>731</code>';
    const unknownReason = 'Custom reviewer explanation: inspect source transaction 42';
    api.get.mockResolvedValue({ data: { run: { id: 'review-run', status: 'awaiting_approval' }, stages: [{ id: 'inspect', stage_key: 'quality', agent_id: 'data-quality', status: 'failed', attempt: 1, error: unknownError, output: { summary: 'quality score below threshold', gate: { status: 'failed', reason: unknownReason }, refs: [{ asset_id: 'raw', version: 1, kind: 'raw_asset' }, { asset_id: 'quality', version: 1, kind: 'quality_report' }, { asset_id: 'analysis', version: 1, kind: 'insight' }] } }], approvals: [
      { id: 'pending', status: 'pending', reason: 'administrator approval required for publication or model cost' },
      { id: 'rejected', status: 'rejected', reason: unknownReason },
    ] } });
    const wrapper = mount(LifecycleWorkbench, { global: { plugins: [createPinia()], stubs: { RouterLink: true } } });
    await flushPromises();
    expect(wrapper.findAll('.wb-review h3').map(node => node.text())).toEqual(['审批 · 待审批', '审批 · 已拒绝']);
    expect(wrapper.text()).toContain('发布资产或调用模型前，需要管理员审核批准。');
    expect(wrapper.text()).toContain('质量评分低于阈值，请检查输入数据或调整处理参数。');
    expect(wrapper.get('.pipeline-stage .wb-error').text()).toBe(unknownError);
    expect(wrapper.get('.failure-reason').text()).toBe(unknownError);
    expect(wrapper.text()).toContain(unknownReason);
    expect(wrapper.find('.pipeline-stage code').exists()).toBe(false);
    expect(wrapper.findAll('.pipeline-stage button').map(button => button.text())).toEqual(['查看原始输入产物', '查看质量报告产物', '查看分析报告产物']);
    wrapper.unmount();
  });
});
