<script setup lang="ts">
import { computed, onErrorCaptured, ref, type Component } from 'vue'
import { NAlert, NCard } from 'naive-ui'
import TablePage from './TablePage.vue'
import FormPage from './FormPage.vue'
import DetailPage from './DetailPage.vue'
import ScopedSurface from './ScopedSurface.vue'
import type { BasicPageDefinition, Definition } from './schema'
const props = defineProps<{ pageId: string; serviceId: string; definition: Definition }>()
const components: Record<BasicPageDefinition['type'], Component> = { table: TablePage, form: FormPage, detail: DetailPage }
const page = computed(() => props.definition.pages.find((page): page is BasicPageDefinition => page.id === props.pageId && page.type !== 'compose'))
const error = ref('')
onErrorCaptured(() => { error.value = '该嵌入页面暂时无法显示。'; return false })
</script>
<template>
  <NAlert v-if="error || !page" type="error" title="嵌入页面无法显示">{{ error || '引用页面不存在或不能嵌入。' }}</NAlert>
  <ScopedSurface v-else :stylesheet="page.stylesheet" :style="page.style" :class-name="page.class_name">
    <NCard :title="page.title" :bordered="false" class="compose-card">
      <p v-if="page.description" class="dynamic-description">{{ page.description }}</p>
      <component :is="components[page.type]" :key="page.id" :page="page" :service-id="serviceId" />
    </NCard>
  </ScopedSurface>
</template>
