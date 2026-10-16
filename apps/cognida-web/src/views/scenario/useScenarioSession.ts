import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useAuthStore } from '@/stores/auth'

/** Every page action belongs to one identity and selection. A late GET must never
 * dispatch the next write or render data in a different tenant's workbench. */
export function useScenarioSession(onIdentityChange: () => void) {
  const auth = useAuthStore()
  const identity = computed(() => `${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}`)
  const busy = ref(false), error = ref(''), notice = ref('')
  let generation = 0
  function invalidate() { generation++; busy.value = false; error.value = ''; notice.value = '' }
  async function run(action: (current: () => boolean) => Promise<void>) {
    if (busy.value) return
    const epoch = generation, owner = identity.value
    const current = () => epoch === generation && owner === identity.value
    busy.value = true; error.value = ''; notice.value = ''
    try { await action(current) }
    catch (e) { if (current()) error.value = e instanceof Error ? e.message : '操作失败，请核对当前状态后重试' }
    finally { if (current()) busy.value = false }
  }
  watch(identity, () => { invalidate(); onIdentityChange() })
  onBeforeUnmount(invalidate)
  const capture = () => { const epoch = generation, owner = identity.value; return () => epoch === generation && owner === identity.value }
  return { busy, error, notice, run, invalidate, capture }
}
