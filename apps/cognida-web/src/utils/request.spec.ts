import { beforeEach, describe, it, expect, vi } from "vitest";
import type { AxiosResponse } from "axios";
const { auth, errorToast } = vi.hoisted(() => ({
  auth: {
    authEpoch: 1,
    user: { id: 1 },
    currentTenant: { id: 1 },
    token: "one",
    clearAuth: vi.fn(),
  },
  errorToast: vi.fn(),
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));
vi.mock("@/utils/element", () => ({ ElMessage: { error: errorToast } }));
import service, { http } from "./request";
describe("response identity fencing", () => {
  beforeEach(() => {
    auth.authEpoch = 1;
    auth.user.id = 1;
    auth.currentTenant.id = 1;
    auth.token = "one";
    vi.clearAllMocks();
  });
  it("rejects a successful response from the previous tenant before caller state can update", async () => {
    let finish!: (response: AxiosResponse) => void;
    let config: AxiosResponse["config"];
    service.defaults.adapter = (c) => {
      config = c;
      return new Promise((resolve) => {
        finish = resolve;
      });
    };
    const old = http.get("/user/profile");
    await Promise.resolve();
    await Promise.resolve();
    auth.authEpoch++;
    auth.currentTenant.id = 2;
    finish({
      data: { code: 0, data: { id: 1, tenant_id: 1 } },
      status: 200,
      statusText: "OK",
      headers: {},
      config: config!,
    });
    await expect(old).rejects.toThrow("登录身份已改变");
    expect(errorToast).not.toHaveBeenCalled();
  });
  it("allows in-flight responses across a same-identity token refresh", async () => {
    service.defaults.adapter = async (c) => {
      auth.token = "refreshed";
      return {
        data: { code: 0, data: { value: 42 } },
        status: 200,
        statusText: "OK",
        headers: {},
        config: c,
      };
    };
    await expect(http.get("/result")).resolves.toMatchObject({
      data: { value: 42 },
    });
  });
});
