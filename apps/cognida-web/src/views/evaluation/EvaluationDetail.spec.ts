import { afterEach, describe, expect, it, vi } from "vitest";
import {
  flushPromises,
  mount,
  RouterLinkStub,
  type VueWrapper,
} from "@vue/test-utils";
import { reactive } from "vue";
const auth = reactive({
  authEpoch: 0,
  currentTenant: { id: 1 },
  user: { id: 1 },
});
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
const api = vi.hoisted(() => ({ getResult: vi.fn(), getQAResults: vi.fn() }));
vi.mock("@/api/evaluation", () => ({ evaluationApi: api }));
vi.mock("vue-router", () => ({
  useRoute: () => ({ params: { id: "evaluation-1" }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("@/utils/sse", () => ({
  connectTaskProgress: vi.fn(() => ({ disconnect: vi.fn() })),
}));
import EvaluationDetail from "./EvaluationDetail.vue";
let wrapper: VueWrapper | undefined;
afterEach(() => wrapper?.unmount());
describe("evaluation result import", () => {
  it("renders the real flat offline response and deterministic sample scores after refresh", async () => {
    api.getResult.mockResolvedValue({
      data: {
        task_id: "evaluation-1",
        type: "offline",
        status: "completed",
        total_count: 2,
        success_count: 2,
        failure_count: 0,
        qa_results: [
          {
            question: "code",
            reference_answer: "00123",
            generated_answer: "00123",
            scores: { exact_match: 100 },
          },
          {
            question: "city",
            reference_answer: "北京",
            generated_answer: "上海",
            scores: { exact_match: 0 },
          },
        ],
      },
    });
    wrapper = mount(EvaluationDetail, {
      global: {
        stubs: { RouterLink: RouterLinkStub, "el-icon": true },
        directives: { loading: {} },
      },
    });
    await flushPromises();
    expect(wrapper.text()).toContain("已完成");
    expect(wrapper.text()).toContain("本次未调用模型生成答案");
    expect(wrapper.text()).toContain("00123");
    expect(wrapper.text()).toContain("exact_match: 100");
    expect(wrapper.text()).toContain("exact_match: 0");
  });
  it("links actual result IDs to feedback without placing question or answers in the URL", async () => {
    api.getResult.mockResolvedValue({
      data: {
        task_id: "evaluation-1",
        type: "rag",
        status: "completed",
        qa_results: [
          {
            id: 42,
            question: "private question",
            reference_answer: "private answer",
          },
        ],
      },
    });
    wrapper = mount(EvaluationDetail, {
      global: {
        stubs: { RouterLink: RouterLinkStub, "el-icon": true },
        directives: { loading: {} },
      },
    });
    await flushPromises();
    const link = wrapper
      .findAllComponents(RouterLinkStub)
      .find((x) => x.text() === "记录此结果反馈并返工")!;
    expect(link.props("to")).toEqual({
      path: "/feedback",
      query: { task_id: "evaluation-1", result_id: "42" },
    });
  });
  it("shows a durable access error instead of an empty detail", async () => {
    api.getResult.mockRejectedValue({ response: { status: 403 } });
    wrapper = mount(EvaluationDetail, {
      global: {
        stubs: { RouterLink: RouterLinkStub, "el-icon": true },
        directives: { loading: {} },
      },
    });
    await flushPromises();
    expect(wrapper.get('[role="alert"]').text()).toContain("当前不可访问");
    expect(wrapper.text()).not.toContain("任务信息");
  });
  it("drops a previous identity response and clears its answer text", async () => {
    let resolve!: (value: unknown) => void;
    api.getResult
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            resolve = r;
          }),
      )
      .mockRejectedValue({ response: { status: 403 } });
    wrapper = mount(EvaluationDetail, {
      global: {
        stubs: { RouterLink: RouterLinkStub, "el-icon": true },
        directives: { loading: {} },
      },
    });
    auth.authEpoch++;
    await flushPromises();
    resolve({
      data: {
        task_id: "evaluation-1",
        type: "offline",
        status: "completed",
        qa_results: [
          {
            question: "old private question",
            reference_answer: "old",
            generated_answer: "old",
          },
        ],
      },
    });
    await flushPromises();
    expect(wrapper.text()).not.toContain("old private question");
    expect(wrapper.text()).toContain("当前不可访问");
  });
  it.each(["completed", "failed"])(
    "offers an explicit import only for a completed task: %s",
    async (status) => {
      api.getResult.mockResolvedValue({
        data: {
          task: {
            task_id: "evaluation-1",
            status,
            total_count: 3,
            success_count: 3,
          },
          dataset_name: "QA set",
        },
      });
      api.getQAResults.mockResolvedValue({ data: { results: [], total: 0 } });
      wrapper = mount(EvaluationDetail, {
        global: {
          stubs: { RouterLink: RouterLinkStub, "el-icon": true },
          directives: { loading: {} },
        },
      });
      await flushPromises();
      const links = wrapper
        .findAllComponents(RouterLinkStub)
        .filter((link) => link.text() === "转入流水线");
      expect(links).toHaveLength(status === "completed" ? 1 : 0);
      if (status === "completed")
        expect(links[0].props("to")).toEqual({
          path: "/pipelines",
          query: {
            source: "evaluation",
            run_id: "evaluation-1",
            name: "QA set评测结果",
          },
        });
    },
  );
});
