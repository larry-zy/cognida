import { beforeEach, describe, it, expect, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
vi.mock("@/utils/request", () => ({ http: {}, resetAuthFailure: vi.fn() }));
import { useAuthStore } from "@/stores/auth";
import { withIdempotency } from "./idempotent";
describe("retry identity", () => {
  beforeEach(() => setActivePinia(createPinia()));
  it("reuses keys after failure, changes after success and scopes by tenant", async () => {
    const send = vi
      .fn()
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValue("done");
    await expect(
      withIdempotency("/test", { name: "first" }, send),
    ).rejects.toThrow("network");
    await withIdempotency("/test", { name: "first" }, send);
    expect(send.mock.calls[0][0]).toBe(send.mock.calls[1][0]);
    await withIdempotency("/test", { name: "first" }, send);
    expect(send.mock.calls[2][0]).not.toBe(send.mock.calls[1][0]);
    send.mockRejectedValueOnce(new Error("network"));
    await expect(withIdempotency("/tenant-test", {}, send)).rejects.toThrow();
    useAuthStore().currentTenant = {
      id: 22,
      name: "different",
      created_at: "",
      updated_at: "",
    };
    await withIdempotency("/tenant-test", {}, send);
    expect(send.mock.calls[3][0]).not.toBe(send.mock.calls[4][0]);
  });
});
