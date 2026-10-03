import { onScopeDispose, reactive, type Ref } from "vue";
type Page<T> = {
  data?: { items?: T[] | null; next_cursor?: string; has_more?: boolean };
};
/** Fetch a single option page on demand. Reset/unmount invalidates previous responses. */
export function useCursorOptions<T>(
  items: Ref<T[]>,
  fetch: (cursor: string) => Promise<Page<T>>,
) {
  const state = reactive({ loading: false, hasMore: false, error: "" });
  let generation = 0,
    cursor = "",
    loaded = false;
  let pending: Promise<void> | undefined;
  const seen = new Set<string>();
  function loadMore(): Promise<void> {
    if (pending) return pending;
    if (loaded && !state.hasMore) return Promise.resolve();
    const epoch = generation,
      requestedCursor = cursor;
    state.loading = true;
    state.error = "";
    const promise = (async () => {
      const page = (await fetch(requestedCursor)).data;
      if (epoch !== generation) return;
      const next = page?.next_cursor || "";
      if ((page?.has_more && !next) || (next && seen.has(next)))
        throw new Error("分页游标异常，请刷新重试");
      if (next) seen.add(next);
      items.value = requestedCursor
        ? [...items.value, ...(page?.items || [])]
        : page?.items || [];
      cursor = next;
      loaded = true;
      state.hasMore = !!next;
    })()
      .catch((error) => {
        if (epoch === generation)
          state.error = error instanceof Error ? error.message : "选项加载失败";
        throw error;
      })
      .finally(() => {
        if (epoch === generation) {
          pending = undefined;
          state.loading = false;
        }
      });
    pending = promise;
    return promise;
  }
  function reset() {
    generation++;
    pending = undefined;
    cursor = "";
    loaded = false;
    seen.clear();
    state.loading = false;
    state.hasMore = false;
    items.value = [];
    return loadMore();
  }
  onScopeDispose(() => {
    generation++;
    pending = undefined;
  });
  return { state, reset, loadMore };
}
