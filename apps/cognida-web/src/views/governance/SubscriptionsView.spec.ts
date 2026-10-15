import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
const api = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), update: vi.fn(), action: vi.fn() }));
const lifecycle = vi.hoisted(() => ({ assets: vi.fn(), asset: vi.fn() }));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({ lifecycleApi: lifecycle }));
import SubscriptionsView from "./SubscriptionsView.vue";
const subscription = (id: string, name = id) => ({
  id, name, version: 2, kind: "report", enabled: true,
  schedule: { cron: "0 9 * * *", timezone: "Asia/Shanghai" },
  input: { asset_ref_id: "source:2", prompt: "Explain revenue" }, watch: null,
});
const setup = () => mount(SubscriptionsView, { global: { stubs: { RouterLink: true } } });
describe("subscription goal and report workflow", () => {
  beforeEach(() => {
    vi.resetAllMocks();
    api.list.mockResolvedValue({ data: { items: [] } });
    api.create.mockResolvedValue({ data: subscription("created") });
    lifecycle.assets.mockResolvedValue({ data: { items: [{ asset_id: "source", version: 2, name: "Revenue" }] } });
  });
  it("persists the report goal with the authorized immutable source and no readonly fields", async () => {
    const wrapper = setup();
    await flushPromises();
    const form = wrapper.get("form");
    await form.get("input").setValue("Weekly revenue");
    await form.findAll("select")[1].setValue("source:2");
    await form.get("textarea").setValue("  Explain the revenue change  ");
    await form.trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith("/subscriptions", expect.objectContaining({
      name: "Weekly revenue", input: expect.objectContaining({ asset_ref_id: "source:2", prompt: "Explain the revenue change" }), channel: "in_app",
    }));
    expect(api.create.mock.calls[0][1]).not.toHaveProperty("created_at");
    expect(wrapper.text()).toContain("订阅已保存");
    wrapper.unmount();
  });
  it("does not overwrite a newly selected subscription with an older save response", async () => {
    let finish!: (value: unknown) => void;
    api.create.mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    api.list.mockImplementation((path: string) => Promise.resolve({ data: { items: path === "/subscriptions" ? [subscription("existing", "Existing report")] : [] } }));
    const wrapper = setup();
    await flushPromises();
    await wrapper.get("form input").setValue("New report");
    await wrapper.get("form").trigger("submit");
    await wrapper.findAll("button").find((button) => button.text().includes("Existing report"))!.trigger("click");
    await flushPromises();
    finish({ data: subscription("new", "Old save result") });
    await flushPromises();
    expect((wrapper.get("form input").element as HTMLInputElement).value).toBe("Existing report");
    expect(wrapper.text()).not.toContain("Old save result");
    wrapper.unmount();
  });
  it("renders model findings and real evidence from a completed report", async () => {
    api.list.mockImplementation((path: string) => Promise.resolve({ data: { items: path === "/subscriptions" ? [subscription("report", "Sales report")] : [{ id: "run", scheduled_at: "2026-10-15T00:00:00Z", status: "completed", result_ref_id: "insight:3" }] } }));
    lifecycle.asset.mockResolvedValue({ data: { ref: { asset_id: "insight", version: 3 }, content: {
      prompt: "Explain revenue", narrative: "Revenue increased in the supplied snapshot.",
      findings: [{ text: "Two transactions", evidence_ids: ["stats-1"] }],
      evidence: [{ id: "stats-1", content: "record_count: 2" }],
      limitations: ["No prior period was supplied"], record_count: 2,
      columns: [{ name: "amount", numeric_count: 2, min: 10, max: 20, mean: 15, missing_count: 0 }],
      decision: { verification_status: "semantic_unverified" },
    } } });
    const wrapper = setup();
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text().includes("Sales report"))!.trigger("click");
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text().includes("查看分析结果"))!.trigger("click");
    await flushPromises();
    expect(lifecycle.asset).toHaveBeenCalledWith("insight", 3);
    const report = wrapper.get('[aria-label="分析报告"]');
    expect(report.text()).toContain("Two transactions");
    expect(report.text()).toContain("record_count: 2");
    expect(report.text()).toContain("No prior period was supplied");
    expect(report.text()).toContain("semantic_unverified");
    expect(report.findAll("tbody tr")).toHaveLength(1);
    wrapper.unmount();
  });
  it("discards a pending report after selecting a different subscription", async () => {
    let finish!: (value: unknown) => void;
    lifecycle.asset.mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    api.list.mockImplementation((path: string) => Promise.resolve({ data: { items: path === "/subscriptions" ? [subscription("one", "First report"), subscription("two", "Second report")] : path.includes("/one/") ? [{ id: "run", scheduled_at: "2026-10-15T00:00:00Z", status: "completed", result_ref_id: "insight:1" }] : [] } }));
    const wrapper = setup();
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text().includes("First report"))!.trigger("click");
    await flushPromises();
    await wrapper.findAll("button").find((button) => button.text().includes("查看分析结果"))!.trigger("click");
    await wrapper.findAll("button").find((button) => button.text().includes("Second report"))!.trigger("click");
    await flushPromises();
    finish({ data: { ref: { asset_id: "insight", version: 1 }, content: { narrative: "Stale report" } } });
    await flushPromises();
    expect(wrapper.text()).not.toContain("Stale report");
    expect(wrapper.find('[aria-label="分析报告"]').exists()).toBe(false);
    wrapper.unmount();
  });
});
