import { beforeEach, describe, expect, it, vi } from "vitest";
const transport = vi.hoisted(() => ({
  post: vi.fn(),
  get: vi.fn(),
  put: vi.fn(),
}));
vi.mock("@/utils/request", () => ({ http: transport }));
vi.mock("@/stores/auth", () => ({
  useAuthStore: () => ({ user: { id: 1 }, currentTenant: { id: 1 } }),
}));
import { governanceApi } from "./index";
beforeEach(() => {
  vi.clearAllMocks();
  transport.post.mockResolvedValue({ data: {} });
});
describe("feature HTTP contracts", () => {
  it("previews a typed feature without a persistence endpoint or idempotency mutation", async () => {
    const feature = {
      name: "Exact",
      type: "numerical",
      source_ref_id: "source:1",
      definition: {
        kind: "expression" as const,
        expression: {
          op: "to_number" as const,
          args: [
            { op: "literal" as const, value: "9007199254740993.123456789" },
          ],
        },
      },
    };
    await governanceApi.previewFeature(feature, 20);
    expect(transport.post).toHaveBeenCalledWith("/features/preview", {
      feature,
      limit: 20,
    });
  });
  it("reuses the same materialization key after an unknown response", async () => {
    transport.post.mockRejectedValueOnce(new Error("connection lost"));
    await expect(
      governanceApi.action("/features/feature/materialize", { version: 2 }),
    ).rejects.toThrow("connection lost");
    await governanceApi.action("/features/feature/materialize", { version: 2 });
    const first = transport.post.mock.calls[0],
      second = transport.post.mock.calls[1];
    expect(first[0]).toBe("/features/feature/materialize");
    expect(first[1]).toEqual({ version: 2 });
    expect(first[2].headers["Idempotency-Key"]).toBeTruthy();
    expect(first[2].headers["Idempotency-Key"]).toBe(
      second[2].headers["Idempotency-Key"],
    );
  });
});
