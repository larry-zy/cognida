import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  action: vi.fn(),
  create: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi.fn().mockResolvedValue({
      data: { items: [{ asset_id: "asset", version: 3, name: "Evidence" }] },
    }),
  },
}));
vi.mock("@/api/semantic", () => ({
  semanticApi: { list: vi.fn().mockResolvedValue({ data: { items: [] } }) },
}));
vi.mock("@/utils/request", () => ({
  http: { get: vi.fn().mockResolvedValue({ data: [] }) },
}));
import SuggestionsView from "./SuggestionsView.vue";
const suggestion = {
  id: "suggestion-1",
  version: 4,
  title: "Evidence based learning",
  source: { type: "conversation_insight", id: "message-1" },
  content: "Observed pattern",
  kind: "experience",
  proposal: { question: "q", answer: "a" },
  evidence_ref_ids: null,
  status: "pending",
};
describe("suggestion review", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ data: { items: [] } });
  });
  it("discovers actual candidates and submits versioned human review", async () => {
    api.action
      .mockResolvedValueOnce({ data: { items: [suggestion] } })
      .mockResolvedValueOnce({
        data: {
          ...suggestion,
          status: "applied",
          target_ref_id: "experience-1",
        },
      });
    const wrapper = mount(SuggestionsView, {
      global: { stubs: { RouterLink: true, StrategiesPanel: true } },
    });
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "发现改进机会")!
      .trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenCalledWith(
      "/improvement-suggestions/discover",
      { source_types: ["coverage", "conversation", "evaluation"], limit: 20 },
    );
    expect(wrapper.text()).toContain("Evidence based learning");
    expect(wrapper.text()).toContain("没有附加资产引用");
    await wrapper.get("textarea").setValue("Verified against source");
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "批准并采纳")!
      .trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenLastCalledWith(
      "/improvement-suggestions/suggestion-1/review",
      {
        decision: "approve",
        comment: "Verified against source",
        expected_version: 4,
      },
    );
    expect(wrapper.text()).toContain("已写入目标：experience-1");
  });
  it("uses immutable versioned evidence references in creation", async () => {
    const wrapper = mount(SuggestionsView, {
      global: { stubs: { RouterLink: true, StrategiesPanel: true } },
    });
    await flushPromises();
    expect(wrapper.get("select[multiple] option").attributes("value")).toBe(
      "asset:3",
    );
  });
});
