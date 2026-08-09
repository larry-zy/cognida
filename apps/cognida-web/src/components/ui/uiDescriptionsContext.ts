import type { ComputedRef, InjectionKey } from 'vue'

export type UiDescriptionsSize = 'sm' | 'md' | 'lg'
export type UiDescriptionsDirection = 'horizontal' | 'vertical'

export interface UiDescriptionsContext {
  column: ComputedRef<number>
  direction: ComputedRef<UiDescriptionsDirection>
  border: ComputedRef<boolean>
  size: ComputedRef<UiDescriptionsSize>
}

export const UiDescriptionsContextKey: InjectionKey<UiDescriptionsContext> =
  Symbol('UiDescriptionsContext')
