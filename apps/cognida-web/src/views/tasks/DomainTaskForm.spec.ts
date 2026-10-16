import { beforeEach, describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import type { Capability } from "@/api/taskcontrol";
const api = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  asset: vi.fn(),
  profiles: vi.fn(),
  metadata: vi.fn(),
}));
vi.mock("@/api/sandbox", () => ({
  sandboxApi: { profiles: api.profiles, metadata: api.metadata },
}));
vi.mock("@/api/governance", () => ({ governanceApi: api }));
vi.mock("@/api/lifecycle", () => ({ lifecycleApi: { asset: api.asset } }));
const StandardSelectStub={props:['modelValue','taskTypes'],emits:['update:modelValue','selected'],template:`<button type="button" aria-label="选择已发布标准" @click="$emit('update:modelValue','frozen-v1');$emit('selected',{id:'frozen-v1',task_type:taskTypes[0]})">选择已发布标准</button>`};
import DomainTaskForm from "./DomainTaskForm.vue";
const capabilities: Capability[] = [
  "tagging",
  "named_entity",
  "qa",
  "feature-engineering",
].map((name) => ({
  id: `governance.${name}`,
  version: 1,
  domain: "governance",
  parameter_kind: name === "feature-engineering" ? "feature" : "governance",
  input_kinds: ["raw_asset", "cleaned_asset"],
  output_kinds: ["result"],
  tools: [`governance.${name}`],
  permissions: ["read"],
  requires_approval: false,
  execution: {
    profile: name,
    base_calls: name === "feature-engineering" ? 2 : 33,
    calls_per_input: 1,
    max_timeout_seconds: 120,
  },
}));
const asset = {
  asset_id: "source",
  version: 3,
  kind: "cleaned_asset" as const,
  provenance: {preparation:{version:1}},
  status: "available",
  name: "样本",
};
const setup = () =>
  mount(DomainTaskForm, {
    props: { capabilities, assets: [asset] as never, busy: false },
    global: { stubs: { StandardVersionSelect:StandardSelectStub, RouterLink: { template: "<a><slot /></a>" } } },
  });
describe("fixed specialist plans", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({
      data: { items: [{ id: "feature", version: 2, name: "收入" }] },
    });
    api.get.mockImplementation((path: string) =>
      Promise.resolve({
        data: path.includes("/requirements?")
          ? {
              feature_id: "feature",
              version: 1,
              max_calls_per_attempt: 3,
              max_attempts: 1,
              embedding: false,
            }
          : { id: "feature", version: 1, source_ref_id: "source:3" },
      }),
    );
    api.asset.mockResolvedValue({ data: { ref: asset } });
  });
  it("freezes the selected published standard and budgets without dispatching", async () => {
    const wrapper = setup();
    await wrapper.get('[aria-label="领域能力"]').setValue("governance.tagging");
    await wrapper.get('[aria-label="领域处理资产"]').setValue("source:3");
    await wrapper.get('[aria-label="选择已发布标准"]').trigger('click');
    await wrapper.get("form").trigger("submit");
    const [draft, plan] = wrapper.emitted("preview")![0] as any[];
    expect(draft.input_refs).toEqual([{ asset_id: "source", version: 3 }]);
    expect(plan.nodes[0].parameters.governance).toEqual({standard_version_id:'frozen-v1'});
    expect(plan.budget).toMatchObject({ max_calls: 34, max_attempts: 1 });
    expect(api.get).not.toHaveBeenCalled();
    wrapper.unmount();
  });
  it("rejects missing standard and QA duplicate columns", async () => {
    const wrapper = setup();
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("governance.named_entity");
    await wrapper.get('[aria-label="领域处理资产"]').setValue("source:3");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.text()).toContain("已发布标准版本");
    expect(wrapper.emitted("preview")).toBeUndefined();
    await wrapper.get('[aria-label="领域能力"]').setValue("governance.qa");
    await wrapper.get('[aria-label="领域处理资产"]').setValue("source:3");
    await wrapper.get('[aria-label="选择已发布标准"]').trigger("click");
    await wrapper.get('[aria-label="参考答案列"]').setValue("question");
    await wrapper.get("form").trigger("submit");
    expect(wrapper.text()).toContain("非空且不同");
    expect(wrapper.emitted("preview")).toBeUndefined();
    wrapper.unmount();
  });
  it("resolves the selected historical definition and uses its real source", async () => {
    const wrapper = setup();
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("governance.feature-engineering");
    await flushPromises();
    await wrapper.get('[aria-label="专家特征定义"]').setValue("feature");
    await wrapper.get('[aria-label="专家特征版本"]').setValue(1);
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.get).toHaveBeenCalledWith("/features/feature/versions/1");
    expect(api.asset).toHaveBeenCalledWith("source", 3);
    expect(api.get).toHaveBeenCalledWith("/features/feature/versions/1/requirements?source_ref_id=source%3A3");
    const [, plan] = wrapper.emitted("preview")![0] as any[];
    expect(plan.nodes[0].parameters).toEqual({
      feature: { feature_id: "feature", version: 1 },
    });
    expect(plan.budget.max_calls).toBe(3);
    wrapper.unmount();
  });
  it("budgets the selected source snapshot instead of the definition anchor", async () => {
    const actual = { ...asset, asset_id: "new-snapshot", version: 4 };
    const wrapper = setup();
    await wrapper.setProps({ assets: [asset, actual] as never });
    await wrapper.get('[aria-label="领域能力"]').setValue("governance.feature-engineering");
    await flushPromises();
    await wrapper.get('[aria-label="专家特征定义"]').setValue("feature");
    await wrapper.get('[aria-label="专家特征版本"]').setValue(1);
    await wrapper.get('[aria-label="专家特征来源"]').setValue("new-snapshot:4");
    api.asset.mockResolvedValue({ data: { ref: actual } });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(api.asset).toHaveBeenCalledWith("new-snapshot", 4);
    expect(api.get).toHaveBeenCalledWith("/features/feature/versions/1/requirements?source_ref_id=new-snapshot%3A4");
    const [draft, plan] = wrapper.emitted("preview")![0] as any[];
    expect(draft.input_refs).toEqual([{ asset_id: "new-snapshot", version: 4 }]);
    expect(plan.nodes[0].inputs[0].asset).toEqual(draft.input_refs[0]);
    expect(plan.nodes[0].parameters.feature).toEqual({ feature_id: "feature", version: 1 });
    wrapper.unmount();
  });
  it("drops feature reads completed after the form is unmounted", async () => {
    let resolve!: (value: unknown) => void;
    api.get.mockImplementation(
      () =>
        new Promise((r) => {
          resolve = r;
        }),
    );
    const wrapper = setup();
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("governance.feature-engineering");
    await flushPromises();
    await wrapper.get('[aria-label="专家特征定义"]').setValue("feature");
    await wrapper.get("form").trigger("submit");
    wrapper.unmount();
    resolve({ data: { source_ref_id: "source:3" } });
    await flushPromises();
    expect(wrapper.emitted("preview")).toBeUndefined();
    expect(api.asset).not.toHaveBeenCalled();
  });
  it("accepts nullable empty input kinds returned by the real capability registry", async () => {
    const wrapper = setup();
    await wrapper.setProps({
      capabilities: [
        {
          ...capabilities[0]!,
          id: "collection.snapshot",
          parameter_kind: "collection",
          input_kinds: null as never,
        },
      ],
    });
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("collection.snapshot");
    await wrapper
      .get('[aria-label="采集网页"]')
      .setValue("https://example.com/page");
    await wrapper.get("form").trigger("submit");
    expect(
      (wrapper.emitted("preview")![0] as any[])[1].nodes[0].inputs,
    ).toEqual([]);
    wrapper.unmount();
  });
  it("previews collection and research with explicit scope and no fabricated asset input", async () => {
    const wrapper = setup();
    await wrapper.setProps({
      capabilities: [
        ...capabilities,
        ...["collection", "research"].map((name) => ({
          id:
            name === "collection"
              ? "collection.snapshot"
              : "research.deep-research",
          version: 1,
          domain: name,
          parameter_kind: name,
          input_kinds: [],
          output_kinds: ["result"],
          tools: [name],
          permissions: ["read"],
          requires_approval: false,
          execution: {
            profile: name,
            base_calls: name === "collection" ? 3 : 20,
            calls_per_input: 0,
            max_timeout_seconds: 120,
          },
        })),
      ],
    });
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("collection.snapshot");
    await wrapper
      .get('[aria-label="采集网页"]')
      .setValue("https://example.com/a");
    await wrapper.get("form").trigger("submit");
    const [draft, plan] = wrapper.emitted("preview")![0] as any[];
    expect(draft.input_refs).toEqual([]);
    expect(plan.nodes[0].inputs).toEqual([]);
    expect(plan.nodes[0].parameters.collection.url).toBe(
      "https://example.com/a",
    );
    expect(plan.budget.max_calls).toBe(3);
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("research.deep-research");
    await wrapper.get('[aria-label="研究问题"]').setValue("Compare evidence");
    await wrapper.get('[aria-label="研究知识库"]').setValue("kb-one,kb-two");
    await wrapper.get("form").trigger("submit");
    const [, research] = wrapper.emitted("preview")![1] as any[];
    expect(research.nodes[0].parameters.research).toMatchObject({
      kb_ids: ["kb-one", "kb-two"],
      query: "Compare evidence",
      enable_web_search: false,
    });
    expect(research.budget.max_calls).toBe(20);
    wrapper.unmount();
  });
  it("binds three distinct existing-answer columns and never calls online evaluation", async () => {
    const wrapper = setup();
    await wrapper.setProps({
      capabilities: [
        {
          id: "evaluation.offline-text-v1",
          version: 1,
          domain: "evaluation",
          parameter_kind: "evaluation",
          input_kinds: ["raw_asset", "cleaned_asset"],
          output_kinds: ["result"],
          tools: ["evaluation.offline-score"],
          permissions: ["read"],
          requires_approval: false,
          execution: {
            profile: "offline",
            base_calls: 5,
            calls_per_input: 1,
            max_timeout_seconds: 300,
          },
        },
      ],
    });
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("evaluation.offline-text-v1");
    await wrapper.get('[aria-label="领域处理资产"]').setValue("source:3");
    await wrapper.get("form").trigger("submit");
    const [, plan] = wrapper.emitted("preview")![0] as any[];
    expect(plan.nodes[0].parameters.evaluation).toEqual({
      question_column: "question",
      reference_answer_column: "reference_answer",
      generated_answer_column: "generated_answer",
    });
    expect(plan.budget.max_calls).toBe(6);
    expect(wrapper.text()).toContain("不会调用模型生成答案");
    wrapper.unmount();
  });
  it("uses fixed-version embedding requirements and blocks missing budget provenance", async () => {
    const wrapper = setup();
    await wrapper
      .get('[aria-label="领域能力"]')
      .setValue("governance.feature-engineering");
    await flushPromises();
    await wrapper.get('[aria-label="专家特征定义"]').setValue("feature");
    await wrapper.get('[aria-label="专家特征版本"]').setValue(1);
    api.get.mockImplementation((path: string) =>
      Promise.resolve({
        data: path.includes("/requirements?")
          ? {
              feature_id: "feature",
              version: 1,
              max_calls_per_attempt: 12,
              max_attempts: 1,
              embedding: true,
              requires_zero_cost_reservation: true,
            }
          : { id: "feature", source_ref_id: "source:3" },
      }),
    );
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    const [, plan] = wrapper.emitted("preview")![0] as any[];
    expect(plan.budget.max_calls).toBe(12);
    expect(plan.budget.max_cost_micros).toBe(0);
    expect(wrapper.text()).toContain("不保证供应商费用上限");
    api.get.mockResolvedValue({
      data: { id: "feature", source_ref_id: "source:3" },
    });
    await wrapper.get("form").trigger("submit");
    await flushPromises();
    expect(wrapper.emitted("preview")).toHaveLength(1);
    expect(wrapper.text()).toContain("固定特征版本或来源读取失败");
    wrapper.unmount();
  });
});

describe("warehouse fixed dependency plans", () => {
  const cap: Capability = {
    id: "warehouse.transform",
    version: 1,
    domain: "warehouse",
    parameter_kind: "warehouse",
    input_kinds: ["raw_asset", "cleaned_asset"],
    output_kinds: ["result"],
    tools: ["warehouse.transform"],
    permissions: ["read"],
    requires_approval: false,
    execution: {
      profile: "warehouse.managed-v1",
      base_calls: 102,
      calls_per_input: 1,
      max_timeout_seconds: 120,
    },
  };
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({
      data: {
        items: [
          { id: "warehouse", version: 2, definition: { name: "区域营收" } },
        ],
      },
    });
    api.get.mockResolvedValue({
      data: {
        profile: "warehouse.managed-v1",
        model_id: "warehouse",
        version: 2,
        max_attempts: 1,
        max_calls_per_attempt: 104,
        input_refs: [
          { asset_id: "fact", version: 1 },
          { asset_id: "dimension", version: 3 },
        ],
      },
    });
    api.asset.mockImplementation(async (id: string, version: number) => ({
      data: { ref: { ...asset, asset_id: id, version } },
    }));
  });
  it("binds every fixed source and budgets all source reads before emitting a candidate", async () => {
    const w = setup();
    await w.setProps({ capabilities: [cap] });
    await w.get('[aria-label="领域能力"]').setValue("warehouse.transform");
    await flushPromises();
    await w.get('[aria-label="专家数仓模型"]').setValue("warehouse");
    await w.get("form").trigger("submit");
    await flushPromises();
    expect(api.get).toHaveBeenCalledWith(
      "/warehouse/models/warehouse/requirements?version=2",
    );
    const [draft, plan] = w.emitted("preview")![0] as any[];
    expect(draft.input_refs).toEqual([
      { asset_id: "fact", version: 1 },
      { asset_id: "dimension", version: 3 },
    ]);
    expect(plan.nodes[0].inputs).toHaveLength(2);
    expect(plan.nodes[0].parameters).toEqual({
      warehouse: { model_id: "warehouse", version: 2, mode: "full" },
    });
    expect(plan.budget.max_calls).toBe(104);
    w.unmount();
  });
  it("does not continue to the next source read after unmount", async () => {
    let resolve!: (v: unknown) => void;
    api.asset.mockImplementationOnce(() => new Promise((r) => (resolve = r)));
    const w = setup();
    await w.setProps({ capabilities: [cap] });
    await w.get('[aria-label="领域能力"]').setValue("warehouse.transform");
    await flushPromises();
    await w.get('[aria-label="专家数仓模型"]').setValue("warehouse");
    await w.get("form").trigger("submit");
    await flushPromises();
    w.unmount();
    resolve({ data: { ref: { ...asset, asset_id: "fact", version: 1 } } });
    await flushPromises();
    expect(api.asset).toHaveBeenCalledTimes(1);
    expect(w.emitted("preview")).toBeUndefined();
  });
});

describe("sandbox fixed task parameters", () => {
  const resources = {
    cpu_millis: 1000,
    memory_bytes: 256 * 1048576,
    disk_bytes: 32 * 1048576,
    processes: 32,
    timeout_seconds: 60,
    input_bytes: 8 * 1048576,
    output_bytes: 8 * 1048576,
    output_files: 16,
    log_bytes: 65536,
  };
  const cap: Capability = {
    id: "sandbox.execute",
    requires_approval: false,
    version: 1,
    domain: "sandbox",
    parameter_kind: "sandbox",
    input_kinds: ["raw_asset", "cleaned_asset"],
    output_kinds: ["cleaned_asset"],
    tools: ["sandbox.execute"],
    permissions: ["sandbox"],
    execution: {
      profile: "sandbox.controlled-development-v1",
      base_calls: 18,
      calls_per_input: 1,
      max_timeout_seconds: 60,
    },
  };
  const mountSandbox = async () => {
    vi.clearAllMocks();
    api.profiles.mockResolvedValue({
      data: {
        enabled: true,
        profile: {
          ID: "python",
          Version: 1,
          Image: "registry.test/python@sha256:" + "a".repeat(64),
          MaxResources: resources,
        },
      },
    });
    api.asset.mockResolvedValue({ data: { ref: asset } });
    api.metadata.mockResolvedValue({
      data: {
        TenantID: 1,
        Ref: { asset_id: "source", version: 3 },
        SHA256: "sha256:" + "b".repeat(64),
        SizeBytes: 100,
      },
    });
    const w = mount(DomainTaskForm, {
      props: { capabilities: [cap], assets: [asset] as any, busy: false },
      global: { stubs: { RouterLink: { template: "<a><slot /></a>" } } },
    });
    await w.get('[aria-label="领域能力"]').setValue(cap.id);
    await flushPromises();
    await w.get('[aria-label="Sandbox代码资产"]').setValue("source:3");
    return w;
  };
  it("freezes real metadata and complete hard limits without executing", async () => {
    const w = await mountSandbox();
    await w.get('[aria-label="Sandbox运行秒数"]').setValue(20);
    await w.get("form").trigger("submit");
    await flushPromises();
    const [draft, plan] = w.emitted("preview")![0] as any[];
    expect(draft.input_refs).toEqual([{ asset_id: "source", version: 3 }]);
    expect(plan.nodes[0].parameters.sandbox.code).toMatchObject({
      name: "code",
      sha256: "sha256:" + "b".repeat(64),
      size_bytes: 100,
    });
    expect(plan.budget).toMatchObject({
      max_calls: 19,
      max_attempts: 1,
      timeout_seconds: 20,
    });
    expect(plan.nodes[0].output_kinds).toEqual(["cleaned_asset"]);
    expect(plan.nodes[0].parameters.sandbox).not.toHaveProperty("execution_id");
    w.unmount();
  });
  it("does not continue to metadata after unmount during asset authorization", async () => {
    const w = await mountSandbox();
    let resolve!: (v: unknown) => void;
    api.asset.mockImplementationOnce(() => new Promise((r) => (resolve = r)));
    await w.get("form").trigger("submit");
    await flushPromises();
    w.unmount();
    resolve({ data: { ref: asset } });
    await flushPromises();
    expect(api.metadata).not.toHaveBeenCalled();
    expect(w.emitted("preview")).toBeUndefined();
  });
});
