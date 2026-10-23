<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { NAlert, NButton, NDataTable, NEmpty, NPagination, NPopconfirm, NSpace, type DataTableColumns } from 'naive-ui'
import { displayValue, ownValue, type Action, type DataRecord, type TablePageDefinition } from './schema'
import { requestServiceJSON, validateMutationResult, validateTableResult } from './transport'
import { useResource } from './useResource'

const props = defineProps<{ page: TablePageDefinition; serviceId: string }>()
const currentPage = ref(1)
const pageSize = computed(() => props.page.page_size ?? 20)
const table = useResource(async signal => validateTableResult(await requestServiceJSON(props.serviceId, props.page.id, {
  signal, query: { page: currentPage.value, page_size: pageSize.value },
}), pageSize.value))
const { data, error, loading } = table
let selectedAction: { action: Action; row: DataRecord } | null = null
const operation = useResource(async signal => {
  if (!selectedAction) return false
  const { action, row } = selectedAction
  const id = ownValue(row, 'id')
  if (typeof id !== 'string' && typeof id !== 'number') throw new Error('此行缺少文本或数字类型的 id，不能执行操作。')
  const result = await requestServiceJSON(props.serviceId, props.page.id, { signal, operation: 'action', actionId: action.id, body: { id } })
  validateMutationResult(result)
  return true
})
const { error: operationError, loading: operating } = operation
const success = ref('')
async function perform(action: Action, row: DataRecord) {
  if (operating.value) return
  selectedAction = { action, row }
  success.value = ''
  if (await operation.load()) {
    success.value = '操作已完成。'
    await table.load()
    if (currentPage.value > 1 && data.value && data.value.items.length === 0) currentPage.value--
  }
}
const columns = computed<DataTableColumns<DataRecord>>(() => {
  const result: DataTableColumns<DataRecord> = props.page.columns.map(column => ({
    key: column.key, title: column.label, minWidth: 120, ellipsis: { tooltip: true },
    render: row => displayValue(ownValue(row, column.key)),
  }))
  if (props.page.actions?.length) result.push({
    key: '__actions', title: '操作', minWidth: 140,
    render: row => h(NSpace, { size: 'small' }, () => props.page.actions!.map(action => h(NPopconfirm, {
      positiveText: '确认执行', negativeText: '取消', onPositiveClick: () => perform(action, row),
    }, {
      trigger: () => h(NButton, { size: 'small', secondary: true, disabled: loading.value || operating.value || !['string', 'number'].includes(typeof ownValue(row, 'id')) }, () => action.label),
      default: () => action.confirm ?? `确认对当前记录执行“${action.label}”？`,
    }))),
  })
  return result
})
watch(currentPage, () => { void table.load() }, { immediate: true })
</script>

<template>
  <div class="dynamic-content">
    <div class="dynamic-actions">
      <span class="dynamic-muted">共 {{ data?.total ?? 0 }} 条记录 · 每页 {{ pageSize }} 条</span>
      <NButton size="small" :loading="loading" :disabled="operating" @click="table.load">刷新数据</NButton>
    </div>
    <NAlert v-if="error" type="error" title="表格加载失败" :bordered="false">{{ error }}</NAlert>
    <NAlert v-if="operationError" type="error" title="操作失败" :bordered="false">{{ operationError }}</NAlert>
    <NAlert v-if="success" type="success" :bordered="false" closable @close="success = ''">{{ success }}</NAlert>
    <NDataTable :columns="columns" :data="data?.items ?? []" :loading="loading" :bordered="false" :scroll-x="Math.max(columns.length * 140, 500)">
      <template #empty><NEmpty :description="error ? '暂时无法获取服务数据' : '暂无数据'" /></template>
    </NDataTable>
    <div class="dynamic-pagination">
      <NPagination v-model:page="currentPage" :item-count="data?.total ?? 0" :page-size="pageSize" :disabled="loading || operating" :page-slot="5" />
    </div>
  </div>
</template>
