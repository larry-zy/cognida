import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { reactive } from "vue";
import StrategiesPanel from "./StrategiesPanel.vue";
const api = vi.hoisted(() => ({
  consumers: vi.fn(),
  head: vi.fn(),
  versions: vi.fn(),
  version: vi.fn(),
  events: vi.fn(),
  write: vi.fn(),
}));
vi.mock("@/api/governance/strategies", () => ({ strategyApi: api }));
const state = reactive({
  authEpoch: 1,
  user: { id: 1 },
  currentTenant: { id: 1 },
});
vi.mock("@/stores/auth", () => ({ useAuthStore: () => state }));
const base = {
  id: "base",
  version: 1,
  kind: "cleaning",
  digest: "exact-baseline",
  body: { inherit_defaults: true },
  gates: {},
  evidence_refs: [],
};
const candidate = {
  ...base,
  id: "candidate",
  version: 2,
  baseline_id: "base",
  digest: "exact-candidate",
  body: { operations: ["trim"], keep: "first", min_score: 70 },
};
beforeEach(() => {
  vi.resetAllMocks();
  state.authEpoch = 1;
  state.user.id = 1;
  state.currentTenant.id = 1;
  api.consumers.mockResolvedValue({
    data: { cleaning: "lifecycle.cleaning.v1" },
  });
  api.head.mockResolvedValue({
    data: { active_id: "base", baseline_id: "base", revision: 8 },
  });
  api.versions.mockResolvedValue({
    data: { items: [base, candidate], next_cursor: "" },
  });
  api.version.mockResolvedValue({ data: candidate });
  api.events.mockResolvedValue({
    data: {
      items: [
        {
          id: "approval",
          version_id: "candidate",
          kind: "approval",
          payload: { approved: true },
        },
      ],
      next_cursor: "",
    },
  });
  api.write.mockResolvedValue({
    data: { id: "receipt", version_id: "candidate" },
  });
});
describe("governed strategies", () => {
  it("only reads on mount and sends exact digest and revision on explicit activation", async () => {
    const w = mount(StrategiesPanel);
    await flushPromises();
    expect(api.write).not.toHaveBeenCalled();
    await w
      .findAll("button")
      .find((b) => b.text().includes("v2"))!
      .trigger("click");
    await flushPromises();
    await w.findAll("textarea").at(-1)!.setValue("verified paired evidence");
    await w.findAll("select").at(-1)!.setValue("approval");
    await w
      .findAll("button")
      .find((b) => b.text() === "确认激活此版本")!
      .trigger("click");
    await flushPromises();
    expect(api.write).toHaveBeenCalledWith(
      "cleaning.default",
      "/versions/candidate/activate",
      {
        digest: "exact-candidate",
        approval_id: "approval",
        expected_revision: 8,
        reason: "verified paired evidence",
        rollback: false,
      },
    );
    w.unmount();
  });
  it("discards an old identity list and unlocks refresh after a scope switch", async () => {
    let finish!: (x: any) => void;
    api.versions.mockReturnValueOnce(new Promise((r) => (finish = r)));
    const w = mount(StrategiesPanel);
    await flushPromises();
    state.authEpoch++;
    state.currentTenant.id = 2;
    await flushPromises();
    finish({ data: { items: [{ ...base, id: "old-secret-version" }] } });
    await flushPromises();
    expect(w.text()).not.toContain("old-secret-version");
    expect(
      w
        .findAll("button")
        .find((b) => b.text() === "刷新策略")!
        .attributes("disabled"),
    ).toBeUndefined();
    expect(api.write).not.toHaveBeenCalled();
    w.unmount();
  });

  it("reads the selected version approval from the second real cursor page after approval and refresh", async () => {
    const older = Array.from({ length: 50 }, (_, i) => ({
      id: `old-${i}`,
      version_id: "old-version",
      kind: "approval",
      payload: { approved: true },
    }));
    let approved = false;
    api.events.mockImplementation(
      async (_slot: string, cursor = "", kind = "") => {
        if (kind === "regression")
          return {
            data: {
              items: [
                {
                  id: "regression",
                  version_id: "candidate",
                  kind: "regression",
                  payload: { passed: true },
                },
              ],
              next_cursor: "",
            },
          };
        if (kind === "approval" && cursor === "approval-page-2")
          return {
            data: {
              items: approved
                ? [
                    {
                      id: "actual-approved",
                      version_id: "candidate",
                      kind: "approval",
                      payload: { approved: true, digest: "exact-candidate" },
                    },
                  ]
                : [],
              next_cursor: "",
            },
          };
        return { data: { items: older, next_cursor: "approval-page-2" } };
      },
    );
    api.write.mockImplementation(async () => {
      approved = true;
      return {
        data: {
          id: "actual-approved",
          version_id: "candidate",
          kind: "approval",
          payload: { approved: true, digest: "exact-candidate" },
        },
      };
    });
    const w = mount(StrategiesPanel);
    await flushPromises();
    await w
      .findAll("button")
      .find((b) => b.text().includes("v2"))!
      .trigger("click");
    await flushPromises();
    const field = (name: string) =>
      w
        .findAll("label")
        .find((l) => l.text().startsWith(name))!
        .find("select");
    await field("回归记录").setValue("regression");
    await w.findAll("textarea").at(-1)!.setValue("reviewed fixed evidence");
    await w
      .findAll("button")
      .find((b) => b.text() === "批准当前确切摘要")!
      .trigger("click");
    await flushPromises();
    expect(api.events).toHaveBeenCalledWith(
      "cleaning.default",
      "approval-page-2",
      "approval",
    );
    expect(
      field("批准回执")
        .findAll("option")
        .map((o) => o.attributes("value")),
    ).toEqual(["", "actual-approved"]);
    await w
      .findAll("button")
      .find((b) => b.text() === "刷新策略")!
      .trigger("click");
    await flushPromises();
    expect(field("批准回执").text()).toContain("actual-approved");
    expect(api.write).toHaveBeenCalledTimes(1);
    w.unmount();
  });
  it.each(["version", "identity"])(
    "discards late receipt pages after a %s change and stops their cursor chain",
    async (change) => {
      const w = mount(StrategiesPanel);
      await flushPromises();
      let finish!: (x: any) => void,
        waiting = true;
      api.events.mockImplementation((_slot: string, cursor = "", kind = "") => {
        if (kind === "approval" && cursor === "pending-page" && waiting) {
          waiting = false;
          return new Promise((r) => (finish = r));
        }
        if (kind === "approval" && cursor === "")
          return Promise.resolve({
            data: { items: [], next_cursor: "pending-page" },
          });
        return Promise.resolve({ data: { items: [], next_cursor: "" } });
      });
      await w
        .findAll("button")
        .find((b) => b.text().includes("v2"))!
        .trigger("click");
      await flushPromises();
      if (change === "version")
        await w
          .findAll("button")
          .find((b) => b.text().includes("v1"))!
          .trigger("click");
      else {
        state.authEpoch++;
        state.currentTenant.id = 2;
      }
      await flushPromises();
      finish({
        data: {
          items: [
            {
              id: "late-private-approval",
              version_id: "base",
              kind: "approval",
              payload: { approved: true },
            },
          ],
          next_cursor: "must-not-follow",
        },
      });
      await flushPromises();
      const approval = w
        .findAll("label")
        .find((l) => l.text().startsWith("批准回执"))!
        .find("select");
      expect(approval.text()).not.toContain("late-private-approval");
      expect(
        api.events.mock.calls.some((call) => call[1] === "must-not-follow"),
      ).toBe(false);
      expect(w.text()).not.toContain("正在读取当前版本");
      w.unmount();
    },
  );
  it("reads an explicitly saved candidate by ID when it is beyond the first version page", async () => {
    api.versions.mockResolvedValue({
      data: { items: [base], next_cursor: "later-versions" },
    });
    api.write.mockResolvedValue({ data: candidate });
    const w = mount(StrategiesPanel);
    await flushPromises();
    await w.findAll("textarea")[0]!.setValue("asset:1");
    await w.findAll("textarea")[1]!.setValue('{"operations":["trim"]}');
    await w.find("form").trigger("submit");
    await flushPromises();
    expect(api.version).toHaveBeenCalledWith("cleaning.default", "candidate");
    expect(w.text()).toContain("v2 版本核对");
    expect(
      w.findAll("button").filter((b) => b.text().includes("v2")),
    ).toHaveLength(1);
    api.versions.mockResolvedValueOnce({
      data: { items: [candidate], next_cursor: "" },
    });
    await w
      .findAll("button")
      .find((b) => b.text() === "更多版本")!
      .trigger("click");
    await flushPromises();
    expect(api.versions).toHaveBeenCalledWith(
      "cleaning.default",
      "later-versions",
    );
    expect(
      w.findAll("button").filter((b) => b.text().includes("v2")),
    ).toHaveLength(1);
    w.unmount();
  });
});
