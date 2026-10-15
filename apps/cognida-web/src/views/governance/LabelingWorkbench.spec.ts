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
  update: vi.fn(),
  create: vi.fn(),
  action: vi.fn(),
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({
  lifecycleApi: { assets: vi.fn().mockResolvedValue({ data: { items: [] } }) },
}));
import LabelingWorkbench from "./LabelingWorkbench.vue";
const task = {
  id: "task",
  name: "Review fixture",
  type: "classification",
  labels: [{ id: "yes", name: "Yes" }],
};
const item = {
  id: "item",
  content: "Actual content",
  version: 4,
  review_status: "pending",
  annotation: { labels: ["yes"], entities: [], relations: [] },
  provenance: { source: "rule", confidence: 0.8 },
};
beforeEach(() => {
  vi.clearAllMocks();
  api.list.mockImplementation((path: string) =>
    Promise.resolve({
      data: {
        items: path.endsWith("/items")
          ? [item]
          : path.endsWith("/results")
            ? [
                {
                  id: item.id,
                  content: item.content,
                  version: 4,
                  review_status: "pending",
                  provenance: item.provenance,
                  tags: ["yes"],
                  entities: [],
                  keywords: [],
                },
              ]
            : [task],
      },
    }),
  );
  api.action.mockResolvedValue({ data: {} });
});
async function open(mode = "annotation", type = "classification") {
  task.type = type;
  const wrapper = mount(LabelingWorkbench, {
    global: { stubs: { RouterLink: true } },
  });
  await flushPromises();
  if (mode === "tagging") {
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "智能打标")!
      .trigger("click");
    await flushPromises();
  }
  await wrapper
    .findAll("button")
    .find((b) => b.text().includes("Review fixture"))!
    .trigger("click");
  await flushPromises();
  await wrapper
    .findAll("button")
    .find((b) => b.text().includes("Actual content"))!
    .trigger("click");
  return wrapper;
}
describe("annotation provenance and review", () => {
  it("reviews untouched preannotation without overwriting provenance", async () => {
    const wrapper = await open();
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "审核通过")!
      .trigger("click");
    await flushPromises();
    expect(api.update).not.toHaveBeenCalled();
    expect(api.action).toHaveBeenCalledWith(
      "/annotation-tasks/task/items/item/review",
      { decision: "approved", comment: "", expected_version: 4 },
    );
    wrapper.unmount();
  });
  it("reviews tagging through the real review endpoint without overwriting provenance", async () => {
    const wrapper = await open("tagging");
    expect(
      wrapper
        .findComponent({ name: "RecordEditor" })
        .props("fields")
        .some((field: { key: string }) => field.key === "type"),
    ).toBe(true);
    api.action.mockResolvedValue({ data: undefined });
    await wrapper
      .findAll("button")
      .find((b) => b.text() === "审核通过")!
      .trigger("click");
    await flushPromises();
    expect(api.update).not.toHaveBeenCalled();
    expect(api.action).toHaveBeenCalledWith("/tagging-results/item/review", {
      decision: "approved",
      comment: "",
      expected_version: 4,
    });
    wrapper.unmount();
  });
  it("keeps entity annotation separate from classification labels", async () => {
    const wrapper = await open("annotation", "named_entity");
    expect(wrapper.find("select[multiple]").exists()).toBe(false);
    const editor = wrapper.findComponent({ name: "RecordEditor" });
    expect(
      editor
        .props("fields")
        .some((field: { key: string }) => field.key === "label"),
    ).toBe(true);
    wrapper.unmount();
  });
});
