<template>
 <section class="wb-panel" aria-label="模型标注建议"><h3>模型建议 · 待人工复核</h3><p class="wb-footnote">单次处理当前样本，使用真实配置模型。标签与关系必须附原文证据；问答只可引用已有问题和参考答案。人工内容与已审核内容不会覆盖。</p>
 <label>标注指南<textarea v-model="guidelines" maxlength="4000" :disabled="busy || protectedItem" placeholder="说明标签含义、边界和应保留不确定的情况" /></label>
 <button type="button" :disabled="busy || protectedItem" @click="suggest">{{busy ? '模型正在生成待审建议…' : '调用模型生成当前样本建议'}}</button><p v-if="protectedItem" class="wb-muted">当前内容由人工编辑或已审核，模型建议已锁定。</p><p v-if="error" role="alert" class="wb-error">{{error}}</p>
 </section>
</template>
<script setup lang="ts">
import {computed,ref,watch,onBeforeUnmount} from 'vue'
import {useAuthStore} from '@/stores/auth'
import {governanceApi,type AnnotationItem,type TagResult} from '@/api/governance'
const props=defineProps<{kind:'annotation'|'tagging';taskId:string;item:AnnotationItem|TagResult}>()
const emit=defineEmits<{updated:[AnnotationItem|TagResult]}>()
const auth=useAuthStore(),guidelines=ref(''),busy=ref(false),error=ref('')
const protectedItem=computed(()=>props.item.provenance.source==='human'||['approved','rejected'].includes(props.item.review_status))
let generation=0
watch(()=>`${auth.authEpoch}:${auth.user?.id}:${auth.currentTenant?.id}:${props.kind}:${props.taskId}:${props.item.id}:${props.item.version}`,()=>{generation++;busy.value=false;error.value=''})
onBeforeUnmount(()=>{generation++})
async function suggest(){if(busy.value||protectedItem.value)return;const epoch=generation;busy.value=true;error.value='';try{const path=`/${props.kind}-tasks/${props.taskId}/${props.kind==='annotation'?'items':'results'}/${props.item.id}/suggest`;const response=await governanceApi.action<AnnotationItem|TagResult>(path,{expected_version:props.item.version,guidelines:guidelines.value});if(epoch!==generation)return;if(!response.data)throw new Error('未取得待审建议，请刷新核对');emit('updated',response.data)}catch(e){if(epoch===generation)error.value=e instanceof Error?e.message:'模型建议失败，未审批或发布任何内容'}finally{if(epoch===generation)busy.value=false}}
</script>
