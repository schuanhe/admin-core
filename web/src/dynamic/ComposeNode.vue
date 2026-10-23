<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { NAlert, NCard, NDivider, NEmpty, NProgress, NTabs, NTabPane } from 'naive-ui'
import { displayValue, ownValue, type Block, type DataRecord, type Definition } from './schema'
import { safeInlineStyle } from './safeCSS'
import { dynamicPath } from '../utils'
import AppIcon from '../components/AppIcon.vue'
import EmbeddedPage from './EmbeddedPage.vue'

const props = defineProps<{ node: Block; data: DataRecord; definition: Definition; serviceId: string }>()
const value = computed(() => props.node.value_key ? ownValue(props.data, props.node.value_key) : props.node.value)
const percentage = computed(() => typeof value.value === 'number' && Number.isFinite(value.value) && value.value >= 0 && value.value <= 100 ? value.value : null)
const style = computed<CSSProperties>(() => ({
  ...(['grid', 'stack', 'tab'].includes(props.node.type) ? { display: 'grid', gap: `${props.node.gap ?? 16}px` } : {}),
  ...(props.node.type === 'grid' ? { gridTemplateColumns: `repeat(${props.node.columns ?? 2}, minmax(0, 1fr))` } : {}),
  ...safeInlineStyle(props.node.style),
}))
</script>
<template>
  <div class="compose-node" :class="node.class_name" :data-component="node.type" :data-block-id="node.id" :style="style">
    <template v-if="node.type === 'stack' || node.type === 'grid' || node.type === 'tab'">
      <ComposeNode v-for="child in node.children" :key="child.id" :node="child" :data="data" :definition="definition" :service-id="serviceId" />
    </template>
    <NCard v-else-if="node.type === 'card'" :title="node.title" :bordered="false" class="compose-card">
      <div class="compose-blocks"><ComposeNode v-for="child in node.children" :key="child.id" :node="child" :data="data" :definition="definition" :service-id="serviceId" /></div>
    </NCard>
    <NTabs v-else-if="node.type === 'tabs'" type="line" :animated="false">
      <NTabPane v-for="child in node.children" :key="child.id" :name="child.id" :tab="child.title ?? child.id" display-directive="if">
        <ComposeNode :node="child" :data="data" :definition="definition" :service-id="serviceId" />
      </NTabPane>
    </NTabs>
    <p v-else-if="node.type === 'text'" class="compose-text">{{ node.value_key ? displayValue(value) : node.text }}</p>
    <NAlert v-else-if="node.type === 'alert'" :title="node.title" :type="node.tone ?? 'info'" :bordered="false">{{ node.text }}</NAlert>
    <div v-else-if="node.type === 'statistic'" class="compose-statistic">
      <span class="compose-label">{{ node.title }}</span>
      <strong>{{ displayValue(value) }}<small v-if="node.unit">{{ node.unit }}</small></strong>
    </div>
    <div v-else-if="node.type === 'progress'" class="compose-progress">
      <span v-if="node.title" class="compose-label">{{ node.title }}</span>
      <NProgress v-if="percentage !== null" type="line" :percentage="percentage" />
      <span v-else class="compose-muted">暂无有效进度</span>
    </div>
    <dl v-else-if="node.type === 'descriptions'" class="compose-descriptions" :style="{ gridTemplateColumns: `repeat(${node.columns ?? 2}, minmax(0, 1fr))` }">
      <div v-for="field in node.fields" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ displayValue(ownValue(data, field.key)) }}</dd></div>
    </dl>
    <NDivider v-else-if="node.type === 'divider'">{{ node.title }}</NDivider>
    <template v-else-if="node.type === 'list'">
      <ul v-if="node.items?.length" class="compose-list"><li v-for="(item, index) in node.items" :key="index">{{ item }}</li></ul>
      <NEmpty v-else description="暂无内容" />
    </template>
    <RouterLink v-else-if="node.type === 'page-link' && node.page_id" class="compose-page-link" :to="dynamicPath(serviceId, node.page_id)">{{ node.title }}<AppIcon name="arrow" :size="16" /></RouterLink>
    <EmbeddedPage v-else-if="node.type === 'page' && node.page_id" :page-id="node.page_id" :definition="definition" :service-id="serviceId" />
  </div>
</template>
<style>
.compose-node { min-width: 0; overflow-wrap: anywhere; }
.compose-card { border: 1px solid var(--border-default); border-radius: var(--radius-card); }
.compose-text { margin: 0; line-height: 1.8; white-space: pre-wrap; }
.compose-label { font-size: 13px; color: var(--text-secondary); }
.compose-statistic, .compose-progress { display: grid; gap: 10px; }
.compose-statistic strong { font-size: 30px; font-weight: 650; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; line-height: 1.2; }
.compose-statistic small { margin-left: 7px; font-size: 13px; font-weight: 400; color: var(--text-secondary); }
.compose-muted { font-size: 13px; color: var(--text-tertiary); }
.compose-descriptions { display: grid; gap: 20px; margin: 0; }
.compose-descriptions dt { color: var(--text-secondary); font-size: 12px; margin-bottom: 7px; }
.compose-descriptions dd { margin: 0; white-space: pre-wrap; }
.compose-list { margin: 0; padding-left: 22px; line-height: 1.9; white-space: pre-wrap; }
.compose-page-link { display: inline-flex; align-items: center; gap: 7px; color: var(--accent-text); text-decoration: underline; text-underline-offset: 3px; padding: 8px 0; }
.compose-page-link:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; border-radius: 3px; }
@media (max-width: 680px) { .compose-node[data-component='grid'], .compose-descriptions { grid-template-columns: minmax(0, 1fr) !important; } }
</style>
