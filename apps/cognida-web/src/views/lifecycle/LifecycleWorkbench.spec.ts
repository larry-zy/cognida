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
});
