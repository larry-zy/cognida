import { beforeEach, describe, expect, it, vi } from "vitest";
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
vi.mock("vue-router", () => ({
  useRoute: () => ({ path: "/analysis", query: {} }),
}));
vi.mock("@/utils/request", () => ({
  http: { get: vi.fn().mockResolvedValue({ data: { items: [] } }) },
  resetAuthFailure: vi.fn(),
}));
import LifecycleWorkbench from "./LifecycleWorkbench.vue";
describe("published asset analysis", () => {
  beforeEach(() => {
    vi.clearAllMocks();
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
});
