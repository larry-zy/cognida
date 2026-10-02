import {afterEach,describe,expect,it} from 'vitest'
import {mount} from '@vue/test-utils'
import {defineComponent,nextTick} from 'vue'
import {i18n} from './index'
import {wt} from './workbench'
afterEach(()=>{i18n.global.locale.value='zh-CN'})
describe('workbench localization',()=>{it('reacts to the existing language preference without changing user content',async()=>{
 const Test=defineComponent({setup:()=>({wt}),template:"<button>{{wt('保存模型')}}</button>"})
 const w=mount(Test);expect(w.text()).toBe('保存模型');i18n.global.locale.value='en-US';await nextTick();expect(w.text()).toBe('Save model');expect(wt('User authored title')).toBe('User authored title')
})})
