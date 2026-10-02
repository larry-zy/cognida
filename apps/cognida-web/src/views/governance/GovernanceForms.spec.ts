import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  action: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: {
    assets: vi.fn().mockResolvedValue({
      data: { items: [{ asset_id: "source", version: 2, name: "Orders" }] },
    }),
  },
}));
import SubscriptionsView from "./SubscriptionsView.vue";
import FeaturesView from "./FeaturesView.vue";
describe("governance create contracts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ data: { items: [] } });
    api.create.mockResolvedValue({ data: { id: "created", version: 1 } });
  });
  it("creates an in-app subscription without readonly empty timestamp fields", async () => {
    const wrapper = mount(SubscriptionsView, {
      global: { stubs: { RouterLink: true } },
    });
    await flushPromises();
    const form = wrapper.get("form");
    await form.find("input").setValue("Weekly summary");
    const asset = form
      .findAll("select")
      .find((s) => s.find('option[value="source:2"]').exists())!;
    await asset.setValue("source:2");
    await form.trigger("submit");
    await flushPromises();
    const payload = api.create.mock.calls[0][1];
    expect(payload.name).toBe("Weekly summary");
    expect(payload.input.asset_ref_id).toBe("source:2");
    expect(payload.channel).toBe("in_app");
    for (const field of [
      "id",
      "version",
      "created_at",
      "updated_at",
      "next_run_at",
    ])
      expect(payload).not.toHaveProperty(field);
  });
  it("registers a versioned feature and calculates from its source", async () => {
    const wrapper = mount(FeaturesView);
    await flushPromises();
    const form = wrapper.get("form");
    await form.find("input").setValue("Spend");
    await form.findAll("select")[1].setValue("source:2");
    await form.find('input[placeholder="如：total_spend"]').setValue("amount");
    await form.trigger("submit");
    await flushPromises();
    const payload = api.create.mock.calls[0][1];
    expect(payload).toMatchObject({
      name: "Spend",
      type: "numerical",
      source_ref_id: "source:2",
      definition: { kind: "field", field: "amount" },
    });
    expect(payload).not.toHaveProperty("created_at");
    api.action.mockResolvedValue({ data: { count: 3 } });
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "从来源资产计算所选版本")!
      .trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenCalledWith("/features/created/compute", {
      version: 1,
    });
    expect(wrapper.text()).toContain("已计算并写入 3 个实体特征值");
  });
  it("parses a selected historical version using its own type", async () => {
    const base = {
      id: "feature",
      name: "Versioned",
      version: 2,
      type: "categorical",
      definition: { field: "amount" },
      source_ref_id: "source:2",
    };
    api.list.mockImplementation((path: string) =>
      Promise.resolve({
        data: {
          items: path.endsWith("/versions")
            ? [base, { ...base, version: 1, type: "numerical" }]
            : [base],
        },
      }),
    );
    const wrapper = mount(FeaturesView);
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("Versioned"))!
      .trigger("click");
    await flushPromises();
    const form = wrapper.findAll("form")[1];
    await form.get('input[type="number"]').setValue("1");
    await form.get('input[placeholder="customer-1001"]').setValue("entity");
    await form.get('input[placeholder="输入特征值"]').setValue("12");
    await form.trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/feature-values",
      expect.objectContaining({
        values: [expect.objectContaining({ version: 1, value: 12 })],
      }),
    );
    wrapper.unmount();
  });
});
