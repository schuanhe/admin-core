<script setup lang="ts">
import { onMounted } from 'vue'
import { NAlert, NButton, NDescriptions, NDescriptionsItem, NEmpty, NSpin } from 'naive-ui'
import { displayValue, ownValue, type DetailPageDefinition } from './schema'
import { requestServiceJSON, validateRecordResult } from './transport'
import { useResource } from './useResource'

const props = defineProps<{ page: DetailPageDefinition; serviceId: string }>()
const resource = useResource(async signal => validateRecordResult(await requestServiceJSON(props.serviceId, props.page.id, { signal })))
const { data, error, loading } = resource
onMounted(() => { void resource.load() })
</script>

<template>
  <div class="dynamic-content">
    <div class="dynamic-actions"><span class="dynamic-muted">只读详情</span><NButton size="small" :loading="loading" @click="resource.load">刷新数据</NButton></div>
    <NAlert v-if="error" type="error" title="详情加载失败" :bordered="false">{{ error }}</NAlert>
    <NSpin :show="loading">
      <NDescriptions v-if="data" bordered :column="2" label-placement="top">
        <NDescriptionsItem v-for="field in page.fields" :key="field.key" :label="field.label"><span class="dynamic-value">{{ displayValue(ownValue(data, field.key)) }}</span></NDescriptionsItem>
      </NDescriptions>
      <NEmpty v-else :description="loading ? '正在加载详情' : '暂无详情数据'" class="dynamic-empty" />
    </NSpin>
  </div>
</template>
