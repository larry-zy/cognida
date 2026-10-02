import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  save: vi.fn(),
  transition: vi.fn(),
  coverage: vi.fn(),
  diagnose: vi.fn(),
}));
vi.mock("@/api/semantic", () => ({ semanticApi: api }));
vi.mock("@/utils/request", () => ({
  http: {
    get: vi.fn().mockResolvedValue({
      data: { items: [{ id: "ds", name: "Orders DB" }] },
    }),
  },
}));
import SemanticModels from "./SemanticModels.vue";
import type { SemanticInput } from "@/api/semantic";
describe("semantic modeling console", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ data: { items: [], next_cursor: "" } });
    api.coverage.mockResolvedValue({ data: { items: [], enabled: true } });
  });
  it("creates an editable draft through structured logical-table fields and publishes it", async () => {
    api.save.mockImplementation((_id: string, input: SemanticInput) =>
      Promise.resolve({
        data: {
          ...input,
          model: {
            id: "draft-1",
            name: input.name,
            description: input.description,
            status: "draft",
            version: 1,
          },
        },
      }),
    );
    api.transition.mockResolvedValue({
      data: {
        model: { id: "draft-1", name: "Sales", status: "active", version: 2 },
        logical_tables: [],
        dimensions: [],
        measures: [],
        metrics: [],
        relations: [],
      },
    });
    const wrapper = mount(SemanticModels);
    await flushPromises();
    await wrapper.get('input[placeholder="例如：订单销售"]').setValue("Sales");
    await wrapper
      .findAll("button")
      .find((b) => b.text().includes("添加逻辑表"))!
      .trigger("click");
    await wrapper.get('input[aria-label="逻辑表1 逻辑名"]').setValue("orders");
    await wrapper.get('input[aria-label="逻辑表1 物理表"]').setValue("orders");
    await wrapper.get('select[aria-label="逻辑表1 数据源"]').setValue("ds");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.save).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({
        name: "Sales",
        status: "draft",
        logical_tables: [
          expect.objectContaining({
            name: "orders",
            physical_table: "orders",
            database_id: "ds",
          }),
        ],
      }),
    );
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "保存并发布")!
      .trigger("click");
    await flushPromises();
    expect(api.transition).toHaveBeenCalledWith("draft-1", "publish");
    expect(wrapper.text()).toContain("模型已发布");
    wrapper.unmount();
  });
  it("shows a server validation failure while retaining user input", async () => {
    api.save.mockRejectedValue(new Error("指标引用的逻辑表不存在"));
    const wrapper = mount(SemanticModels);
    await flushPromises();
    await wrapper
      .get('input[placeholder="例如：订单销售"]')
      .setValue("Unsaved");
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("逻辑表不存在");
    expect(
      (
        wrapper.get('input[placeholder="例如：订单销售"]')
          .element as HTMLInputElement
      ).value,
    ).toBe("Unsaved");
    wrapper.unmount();
  });
});
