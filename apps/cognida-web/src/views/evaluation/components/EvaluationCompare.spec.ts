import {describe,it,expect,vi} from 'vitest'
import {mount,flushPromises} from '@vue/test-utils'
const post=vi.hoisted(()=>vi.fn())
vi.mock('@/utils/request',()=>({http:{post}}))
import EvaluationCompare from './EvaluationCompare.vue'
describe('paired evaluation comparison',()=>{it('renders insufficient evidence without presenting release approval',async()=>{
 post.mockResolvedValue({data:{pairs:2,missing_pairs:1,baseline_mean:0.5,candidate_mean:0.6,improvement:0.1,lower_95:null,upper_95:null,gate:'insufficient_evidence',recommendation:'Collect more paired cases',method:'paired_scores',limitations:['Only 2 pairs']}})
 const w=mount(EvaluationCompare);const inputs=w.findAll('input');await inputs[0].setValue('baseline');await inputs[1].setValue('candidate');await w.get('form').trigger('submit');await flushPromises()
 expect(post).toHaveBeenCalledWith('/evaluation/compare',expect.objectContaining({baseline_id:'baseline',candidate_id:'candidate',min_pairs:20}));expect(w.text()).toContain('证据不足');expect(w.text()).toContain('Only 2 pairs');expect(w.text()).not.toContain('可进入人工发布审查')
})})
