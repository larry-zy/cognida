import { type Directive } from 'vue'

/**
 * 淡入上移动画指令
 * 用法: v-fade-in 或 v-fade-in="{ delay: 100, duration: 300 }"
 */
export const vFadeIn: Directive = {
  mounted(el, binding) {
    const options = binding.value || {}
    const delay = options.delay || 0
    const duration = options.duration || 300
    const distance = options.distance || 20

    el.style.opacity = '0'
    el.style.transform = `translateY(${distance}px)`
    el.style.transition = `opacity ${duration}ms ease-out, transform ${duration}ms ease-out`

    setTimeout(() => {
      el.style.opacity = '1'
      el.style.transform = 'translateY(0)'
    }, delay)
  }
}

/**
 * Stagger 进场动画指令
 * 用法: v-stagger="{ delay: 50, selector: '.child' }"
 * 对子元素进行交错动画
 */
export const vStagger: Directive = {
  mounted(el, binding) {
    const options = binding.value || {}
    const delay = options.delay || 50
    const selector = options.selector || ':scope > *'
    const duration = options.duration || 300

    const children = el.querySelectorAll(selector)
    children.forEach((child: Node, index: number) => {
      const node = child as HTMLElement
      node.style.opacity = '0'
      node.style.transform = 'translateY(20px)'
      node.style.transition = `opacity ${duration}ms ease-out, transform ${duration}ms ease-out`

      setTimeout(() => {
        node.style.opacity = '1'
        node.style.transform = 'translateY(0)'
      }, index * delay)
    })
  }
}

/**
 * 缩放进场动画指令
 */
export const vScaleIn: Directive = {
  mounted(el, binding) {
    const options = binding.value || {}
    const delay = options.delay || 0
    const duration = options.duration || 300

    el.style.opacity = '0'
    el.style.transform = 'scale(0.9)'
    el.style.transition = `opacity ${duration}ms ease-out, transform ${duration}ms ease-out`

    setTimeout(() => {
      el.style.opacity = '1'
      el.style.transform = 'scale(1)'
    }, delay)
  }
}

/**
 * 滑入动画指令
 * 用法: v-slide-in="{ direction: 'left', delay: 0 }"
 */
export const vSlideIn: Directive = {
  mounted(el, binding) {
    const options = binding.value || {}
    const delay = options.delay || 0
    const duration = options.duration || 300
    const direction = options.direction || 'bottom'

    const transforms: Record<string, string> = {
      left: 'translateX(-30px)',
      right: 'translateX(30px)',
      top: 'translateY(-30px)',
      bottom: 'translateY(30px)'
    }

    el.style.opacity = '0'
    el.style.transform = transforms[direction] || transforms.bottom
    el.style.transition = `opacity ${duration}ms ease-out, transform ${duration}ms ease-out`

    setTimeout(() => {
      el.style.opacity = '1'
      el.style.transform = 'translate(0)'
    }, delay)
  }
}
