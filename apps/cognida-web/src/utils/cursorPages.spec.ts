import { describe, expect, it, vi } from "vitest";
import { collectPages } from "./cursorPages";
describe("cursor option loading", () => {
  it("includes records beyond the first page", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({
        data: { items: [1], next_cursor: "second", has_more: true },
      })
      .mockResolvedValueOnce({
        data: { items: [2], next_cursor: "", has_more: false },
      });
    expect(await collectPages(fetch)).toEqual([1, 2]);
    expect(fetch).toHaveBeenNthCalledWith(2, "second");
  });
  it("rejects repeated cursors", async () => {
    await expect(
      collectPages(async () => ({ data: { items: [1], next_cursor: "same" } })),
    ).rejects.toThrow("分页游标重复");
  });
});
