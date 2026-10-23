<script setup lang="ts">
import { computed, onErrorCaptured, ref, watch, type Component } from 'vue'
import { NAlert, NCard } from 'naive-ui'
import DetailPage from './DetailPage.vue'
import FormPage from './FormPage.vue'
import TablePage from './TablePage.vue'
import ComposePage from './ComposePage.vue'
import ScopedSurface from './ScopedSurface.vue'
import { validateDefinition, type DynamicService, type BasicPageDefinition } from './schema'

const props = defineProps<{ service: DynamicService; pageId: string }>()
// 映射仅引用中心编译时组件，服务字符串绝不会成为 Vue 模板或可执行代码。
const components: Record<BasicPageDefinition['type'], Component> = { table: TablePage, form: FormPage, detail: DetailPage }
const renderingError = ref('')
const result = computed(() => {
  try {
    const definition = validateDefinition(props.service.definition)
    const page = definition.pages.find(item => item.id === props.pageId)
    if (!page) return { error: '服务未提供此页面，请刷新服务登记信息或选择其他菜单。', page: null }
    return { error: '', page, definition }
  } catch (cause) { return { error: cause instanceof Error ? cause.message : '页面定义无效。', page: null } }
})
const pageKey = computed(() => `${props.service.id}:${props.service.base_url}:${props.pageId}:${JSON.stringify(result.value.page)}`)
watch(pageKey, () => { renderingError.value = '' })
onErrorCaptured(() => { renderingError.value = '此服务页面暂时无法显示。请切换页面后重试，其他管理功能仍然可用。'; return false })
</script>

<template>
  <section class="dynamic-page" aria-label="服务动态页面">
    <NAlert v-if="service.activation !== 'enabled'" type="warning" title="服务尚未启用" :bordered="false">请在服务管理中启用此服务后使用其页面。</NAlert>
    <NAlert v-else-if="result.error || renderingError" type="error" title="无法显示服务页面" :bordered="false">{{ result.error || renderingError }}</NAlert>
    <template v-else-if="result.page">
      <NAlert v-if="service.online === 'offline'" type="warning" title="最近检测到服务离线" :bordered="false">登记记录和启用状态已保留。可在服务恢复后刷新数据，或到服务详情重新检测。</NAlert>
      <ScopedSurface :stylesheet="result.page.stylesheet" :style="result.page.style" :class-name="result.page.class_name">
      <template v-if="result.page.type === 'compose' && result.definition">
        <header class="compose-heading"><h1>{{ result.page.title }}</h1><p v-if="result.page.description" class="dynamic-description">{{ result.page.description }}</p></header>
        <ComposePage :key="pageKey" :page="result.page" :definition="result.definition" :service-id="service.id" />
      </template>
      <NCard v-else-if="result.page.type !== 'compose'" :bordered="false" class="dynamic-card">
        <template #header><div class="dynamic-heading">{{ result.page.title }}</div></template>
        <p v-if="result.page.description" class="dynamic-description">{{ result.page.description }}</p>
        <component :is="components[result.page.type]" :key="pageKey" :page="result.page" :service-id="service.id" />
      </NCard>
      </ScopedSurface>
    </template>
  </section>
</template>

<style>
.dynamic-page { display: grid; gap: 18px; min-width: 0; }
.dynamic-card { border-radius: var(--radius-card); border: 1px solid var(--border-default); box-shadow: var(--shadow-subtle); }
.dynamic-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.dynamic-description { color: var(--text-secondary); line-height: 1.7; margin: -4px 0 24px; white-space: pre-wrap; overflow-wrap: anywhere; }
.dynamic-content { display: grid; gap: 18px; min-width: 0; }
.dynamic-actions { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.dynamic-muted { color: var(--text-secondary); font-size: 13px; }
.dynamic-pagination { display: flex; justify-content: flex-end; }
.dynamic-form { max-width: 680px; }
.dynamic-form .n-form-item { margin-bottom: 4px; }
.dynamic-alert-action { margin-top: 12px; }
.dynamic-value { white-space: pre-wrap; overflow-wrap: anywhere; }
.dynamic-empty { padding: 36px 16px; }
.compose-heading h1 { margin: 0 0 20px; font-size: 26px; font-weight: 600; letter-spacing: -0.02em; }
@media (max-width: 680px) { .dynamic-heading { align-items: flex-start; flex-direction: column; } .dynamic-actions { flex-wrap: wrap; } }
</style>
