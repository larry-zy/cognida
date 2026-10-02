/** Read each cursor once; reject broken pagination instead of silently truncating options. */
export async function collectPages<T>(
  fetch: (
    cursor: string,
  ) => Promise<{
    data?: { items?: T[] | null; next_cursor?: string; has_more?: boolean };
  }>,
): Promise<T[]> {
  const result: T[] = [],
    seen = new Set<string>();
  let cursor = "";
  for (;;) {
    const page = (await fetch(cursor)).data;
    result.push(...(page?.items || []));
    const next = page?.next_cursor || "";
    if (!next) {
      if (page?.has_more) throw new Error("分页游标缺失，请刷新重试");
      return result;
    }
    if (seen.has(next)) throw new Error("分页游标重复，请刷新重试");
    seen.add(next);
    cursor = next;
  }
}
