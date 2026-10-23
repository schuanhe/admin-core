<script setup lang="ts">
import { computed, onBeforeUnmount, watch } from 'vue'
import { NAlert } from 'naive-ui'
import { compileScopedCSS, safeInlineStyle } from './safeCSS'
const props = defineProps<{ stylesheet?: string; style?: Record<string, string>; className?: string }>()
const scope = `s${crypto.randomUUID()}`
const result = computed(() => {
  try { return { css: compileScopedCSS(props.stylesheet ?? '', scope), style: safeInlineStyle(props.style), error: '' } }
  catch (error) { return { css: '', style: {}, error: error instanceof Error ? error.message : '页面样式无效。' } }
})
let sheet: HTMLStyleElement | null = null
watch(result, value => {
  sheet?.remove()
  sheet = null
  if (value.error || !value.css) return
  sheet = document.createElement('style')
  sheet.dataset.serviceStylesheet = scope
  sheet.textContent = value.css
  document.head.appendChild(sheet)
}, { immediate: true })
onBeforeUnmount(() => sheet?.remove())
</script>
<template>
  <NAlert v-if="result.error" type="error" title="页面样式无法应用" :bordered="false">{{ result.error }}</NAlert>
  <div v-else :data-service-scope="scope" class="service-style-boundary">
    <div class="service-page" :class="className" :style="result.style"><slot /></div>
  </div>
</template>
<style scoped>
/* 边界不接收服务style/class；布局与绘制隔离防止页面内容覆盖控制台。 */
.service-style-boundary { contain: layout paint style; isolation: isolate; min-width: 0; max-width: 100%; }
.service-page { min-width: 0; overflow-wrap: anywhere; }
</style>
