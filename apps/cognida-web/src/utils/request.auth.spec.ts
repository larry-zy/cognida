import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createPinia, setActivePinia } from "pinia";
import { AxiosError, type InternalAxiosRequestConfig } from "axios";
import { flushPromises } from "@vue/test-utils";
const { toast } = vi.hoisted(() => ({ toast: vi.fn() }));
vi.mock("@/utils/element", () => ({ ElMessage: { error: toast } }));
vi.mock("@/utils/security", () => ({
  storage: { get: vi.fn(), set: vi.fn(), remove: vi.fn() },
}));
import service, { http, resetAuthFailure } from "./request";
import { useAuthStore } from "@/stores/auth";
const response = (
  config: InternalAxiosRequestConfig,
  data: unknown,
  status = 200,
) => ({ data, status, statusText: String(status), headers: {}, config });
const fail = (config: InternalAxiosRequestConfig, status: number) =>
  new AxiosError(
    "HTTP " + status,
    "ERR_BAD_RESPONSE",
    config,
    undefined,
    response(config, { message: "rejected" }, status),
  );
describe("refresh and transport integration", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    resetAuthFailure();
    vi.clearAllMocks();
    vi.stubGlobal("window", {
      location: { pathname: "/protected", href: "/protected" },
    });
    vi.spyOn(console, "error").mockImplementation(() => {});
    const auth = useAuthStore();
    auth.token = "expired";
    auth.refreshToken = "original";
    auth.user = { id: 1 } as typeof auth.user;
    auth.currentTenant = { id: 1, name: "one", created_at: "", updated_at: "" };
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });
  it.each(["network", "server"] as const)(
    "redirects exactly once when refresh fails with %s",
    async (mode) => {
      service.defaults.adapter = async (c) => {
        if (c.url === "/auth/refresh") {
          if (mode === "network")
            throw new AxiosError("offline", "ERR_NETWORK", c);
          throw fail(c, 500);
        }
        throw fail(c, 401);
      };
      await expect(http.get("/protected")).rejects.toThrow();
      expect(useAuthStore().token).toBe("");
      expect(window.location.href).toBe("/login");
      expect(
        toast.mock.calls.filter(([text]) => text === "登录已过期，请重新登录"),
      ).toHaveLength(1);
    },
  );
  it("does not clear or redirect a new login when an obsolete refresh fails", async () => {
    let rejectRefresh!: (error: Error) => void,
      refreshConfig!: InternalAxiosRequestConfig;
    service.defaults.adapter = async (c) => {
      if (c.url === "/auth/refresh") {
        refreshConfig = c;
        return new Promise((_r, reject) => {
          rejectRefresh = reject;
        });
      }
      if (c.url === "/auth/login")
        return response(c, {
          code: 0,
          data: {
            access_token: "new-token",
            refresh_token: "new-refresh",
            user: { id: 2 },
            tenant_id: 2,
          },
        });
      throw fail(c, 401);
    };
    const old = http.get("/protected");
    const assertion = expect(old).rejects.toThrow();
    await flushPromises();
    await useAuthStore().login({ email: "new@example.test", password: "test" });
    rejectRefresh(fail(refreshConfig, 500));
    await assertion;
    expect(useAuthStore().token).toBe("new-token");
    expect(window.location.href).toBe("/protected");
    expect(toast).not.toHaveBeenCalled();
  });
});
