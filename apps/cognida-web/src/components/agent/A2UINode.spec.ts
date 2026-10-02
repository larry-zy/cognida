import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import A2UINode from './A2UINode.vue'
import { A2UI_CTX } from './a2ui-context'

describe('bar chart signed values', () => {
  it.each([[-10, 0, 10], [-20, -10, 0], [0, 10, 20]])('plots %j relative to a shared zero axis', (...values) => {
    const wrapper = mount(A2UINode, {
      props: { id: 'root' },
      global: { provide: {
        [A2UI_CTX as symbol]: {
          nodes: ref({ root: { id: 'root', type: 'BarChart', props: { series: { path: '/series' } } } }),
          dataModel: ref({ series: { actual: values } }), onAction: vi.fn()
        }
      } }
    })
    const axisY = Number(wrapper.get('line').attributes('y1'))
    const rects = wrapper.findAll('rect')
    values.forEach((value, i) => {
      const y = Number(rects[i].attributes('y'))
      const height = Number(rects[i].attributes('height'))
      if (value < 0) { expect(y).toBe(axisY); expect(height).toBeGreaterThan(0) }
      if (value === 0) expect(height).toBe(0)
      if (value > 0) { expect(y).toBeLessThan(axisY); expect(y + height).toBeCloseTo(axisY) }
    })
    wrapper.unmount()
  })
})

it('emits bounded chart evidence for mouse and keyboard drill-down', async () => {
  const onAction=vi.fn()
  const wrapper=mount(A2UINode,{props:{id:'root'},global:{provide:{[A2UI_CTX as symbol]:{nodes:ref({root:{id:'root',type:'BarChart',props:{series:{path:'/series'},title:'Revenue'}}}),dataModel:ref({series:{labels:['East','West'],actual:[12,20]},meta:{result_id:'result-1'}}),onAction}}}})
  await wrapper.findAll('rect')[0].trigger('click')
  expect(onAction).toHaveBeenLastCalledWith({name:'drill_down',params:{component_id:'root',title:'Revenue',label:'East',value:12,result_id:'result-1'}})
  await wrapper.findAll('rect')[1].trigger('keydown',{key:'Enter'})
  expect(onAction.mock.lastCall?.[0].params.label).toBe('West')
})
