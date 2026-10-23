<script setup lang="ts">
import { onMounted } from 'vue'
import { NAlert, NButton, NSkeleton } from 'naive-ui'
import ComposeNode from './ComposeNode.vue'
import { type ComposePageDefinition, type Definition } from './schema'
import { requestServiceJSON, validateRecordResult } from './transport'
import { useResource } from './useResource'
const props = defineProps<{ page: ComposePageDefinition; serviceId: string; definition: Definition }>()
const resource = useResource(async signal => props.page.source ? validateRecordResult(await requestServiceJSON(props.serviceId, props.page.id, { signal })) : {})
const { data, error, loading } = resource
onMounted(() => { if (props.page.source) void resource.load() })
</script>
<template>
  <div class="compose-page">
    <div v-if="page.source" class="compose-toolbar"><NButton size="small" :loading="loading" @click="resource.load">刷新数据</NButton></div>
    <NAlert v-if="error" type="error" title="页面数据加载失败" :bordered="false">{{ error }}</NAlert>
    <NSkeleton v-if="loading && !data" :repeat="3" height="64px" />
    <div v-else class="compose-blocks" :aria-busy="loading">
      <ComposeNode v-for="block in page.blocks" :key="block.id" :node="block" :data="data ?? {}" :definition="definition" :service-id="serviceId" />
    </div>
  </div>
</template>
<style>
.compose-page, .compose-blocks { display: grid; gap: 20px; min-width: 0; }
.compose-toolbar { display: flex; justify-content: flex-end; }
</style>
