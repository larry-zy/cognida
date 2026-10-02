import { useAuthStore } from "@/stores/auth";

// Keep the same key after an uncertain failure, and coalesce double clicks.
// A completed action gets a fresh key the next time the user deliberately runs it.
const pending = new Map<string, { key: string; promise?: Promise<unknown> }>();
export async function withIdempotency<T>(
  path: string,
  payload: unknown,
  send: (key: string) => Promise<T>,
): Promise<T> {
  const auth = useAuthStore();
  const identity = JSON.stringify([
    auth.user?.id,
    auth.currentTenant?.id,
    path,
    payload,
  ]);
  let entry = pending.get(identity);
  if (entry?.promise) return entry.promise as Promise<T>;
  if (!entry) {
    entry = { key: crypto.randomUUID() };
    pending.set(identity, entry);
  }
  const promise = send(entry.key);
  entry.promise = promise;
  try {
    const result = await promise;
    pending.delete(identity);
    return result;
  } catch (error) {
    entry.promise = undefined;
    throw error;
  }
}
