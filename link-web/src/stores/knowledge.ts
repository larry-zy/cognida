import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { KnowledgeBase, FAQEntry } from '@/types'

export const useKnowledgeStore = defineStore('knowledge', () => {
  // 状态
  const knowledgeBases = ref<KnowledgeBase[]>([])
  const currentKnowledgeBase = ref<KnowledgeBase | null>(null)
  const faqEntries = ref<FAQEntry[]>([])

  // 设置知识库列表
  function setKnowledgeBases(list: KnowledgeBase[]) {
    knowledgeBases.value = list
  }

  // 设置当前知识库
  function setCurrentKnowledgeBase(kb: KnowledgeBase | null) {
    currentKnowledgeBase.value = kb
  }

  // 添加知识库
  function addKnowledgeBase(kb: KnowledgeBase) {
    knowledgeBases.value.push(kb)
  }

  // 更新知识库
  function updateKnowledgeBase(id: string, data: Partial<KnowledgeBase>) {
    const index = knowledgeBases.value.findIndex(kb => kb.id === id)
    if (index !== -1) {
      knowledgeBases.value[index] = { ...knowledgeBases.value[index], ...data }
    }
    if (currentKnowledgeBase.value?.id === id) {
      currentKnowledgeBase.value = { ...currentKnowledgeBase.value, ...data }
    }
  }

  // 删除知识库
  function removeKnowledgeBase(id: string) {
    knowledgeBases.value = knowledgeBases.value.filter(kb => kb.id !== id)
    if (currentKnowledgeBase.value?.id === id) {
      currentKnowledgeBase.value = null
    }
  }

  // 设置FAQ列表
  function setFAQEntries(list: FAQEntry[]) {
    faqEntries.value = list
  }

  // 添加FAQ
  function addFAQEntry(faq: FAQEntry) {
    faqEntries.value.push(faq)
  }

  // 更新FAQ
  function updateFAQEntry(id: string, data: Partial<FAQEntry>) {
    const index = faqEntries.value.findIndex(faq => faq.id === id)
    if (index !== -1) {
      faqEntries.value[index] = { ...faqEntries.value[index], ...data }
    }
  }

  // 删除FAQ
  function removeFAQEntry(id: string) {
    faqEntries.value = faqEntries.value.filter(faq => faq.id !== id)
  }

  return {
    knowledgeBases,
    currentKnowledgeBase,
    faqEntries,
    setKnowledgeBases,
    setCurrentKnowledgeBase,
    addKnowledgeBase,
    updateKnowledgeBase,
    removeKnowledgeBase,
    setFAQEntries,
    addFAQEntry,
    updateFAQEntry,
    removeFAQEntry
  }
})
