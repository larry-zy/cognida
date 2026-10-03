import { effectScope, ref } from "vue";
import { describe, it, expect, vi } from "vitest";
import { useCursorOptions } from "../useCursorOptions";
describe("bounded option pagination", () => {
  it("fetches only one page per explicit request and coalesces duplicate clicks", async () => {
    const scope = effectScope();
    const items = ref<number[]>([]);
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ data: { items: [1], next_cursor: "two" } })
      .mockResolvedValueOnce({ data: { items: [2], next_cursor: "" } });
    const pager = scope.run(() => useCursorOptions(items, fetch))!;
    await pager.reset();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(items.value).toEqual([1]);
    const a = pager.loadMore(),
      b = pager.loadMore();
    expect(a).toBe(b);
    await a;
    expect(items.value).toEqual([1, 2]);
    expect(pager.state.hasMore).toBe(false);
    scope.stop();
  });
  it("ignores stale reset results and late results after disposal", async () => {
    const scope = effectScope();
    const items = ref<number[]>([]);
    let finish!: (v: unknown) => void;
    const fetch = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise((r) => {
            finish = r;
          }),
      )
      .mockResolvedValueOnce({ data: { items: [2] } });
    const pager = scope.run(() => useCursorOptions(items, fetch))!;
    const old = pager.reset();
    await pager.reset();
    finish({ data: { items: [1], next_cursor: "old" } });
    await old;
    expect(items.value).toEqual([2]);
    fetch.mockImplementationOnce(
      () =>
        new Promise((r) => {
          finish = r;
        }),
    );
    const last = pager.reset();
    scope.stop();
    finish({ data: { items: [3] } });
    await last;
    expect(items.value).toEqual([]);
  });
  it("preserves a failed cursor for explicit retry", async () => {
    const scope = effectScope();
    const items = ref<number[]>([]);
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ data: { items: [1], next_cursor: "two" } })
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ data: { items: [2] } });
    const pager = scope.run(() => useCursorOptions(items, fetch))!;
    await pager.reset();
    await expect(pager.loadMore()).rejects.toThrow("offline");
    expect(items.value).toEqual([1]);
    await pager.loadMore();
    expect(fetch).toHaveBeenLastCalledWith("two");
    expect(items.value).toEqual([1, 2]);
    scope.stop();
  });
  it("exposes a retryable first-page failure", async () => {
    const scope = effectScope(),
      items = ref<number[]>([]);
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({ data: { items: [7] } });
    const pager = scope.run(() => useCursorOptions(items, fetch))!;
    await expect(pager.reset()).rejects.toThrow("offline");
    expect(pager.state.error).toBe("offline");
    expect(pager.state.loading).toBe(false);
    await pager.loadMore();
    expect(items.value).toEqual([7]);
    expect(pager.state.error).toBe("");
    scope.stop();
  });
});
