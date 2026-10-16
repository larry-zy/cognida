import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({
    authEpoch: 0,
    user: { id: 1 },
    currentTenant: { id: 1 },
    token: "test-token",
  }),
}));
vi.mock("vue-router", () => ({ useRoute: () => ({ query: {} }) }));
const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  previewFeature: vi.fn(),
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
    const wrapper = mount(FeaturesView, {
      global: { stubs: { RouterLink: true } },
    });
    await flushPromises();
    const form = wrapper.get("form");
    await form.find("input").setValue("Spend");
    await form
      .findAll("select")
      .find((s) => s.find('option[value="source:2"]').exists())!
      .setValue("source:2");
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
    api.action.mockResolvedValue({
      data: {
        count: 3,
        receipt_id: "receipt",
        feature_id: "created",
        version: 1,
        output_ref_id: "output:1",
        available_at: "2026-10-16T00:00:00Z",
      },
    });
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "计算并物化所选版本")!
      .trigger("click");
    await flushPromises();
    expect(api.action).toHaveBeenCalledWith("/features/created/materialize", {
      version: 1,
    });
    expect(wrapper.text()).toContain("已物化 3 个特征值");
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
    const wrapper = mount(FeaturesView, {
      global: { stubs: { RouterLink: true } },
    });
    await flushPromises();
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("Versioned"))!
      .trigger("click");
    await flushPromises();
    const form = wrapper
      .findAll("form")
      .find((f) => f.find('input[placeholder="customer-1001"]').exists())!;
    await form.get('input[type="number"]').setValue("1");
    await form.get('input[placeholder="customer-1001"]').setValue("entity");
    await form.get('input[placeholder="输入特征值"]').setValue("12");
    await form.trigger("submit");
    await flushPromises();
    expect(api.create).toHaveBeenCalledWith(
      "/feature-values",
      expect.objectContaining({
        values: [
          expect.objectContaining({
            version: 1,
            value: null,
            value_exact: "12",
          }),
        ],
      }),
    );
    wrapper.unmount();
  });
});
