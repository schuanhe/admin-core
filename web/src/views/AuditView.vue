<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NButton, NDataTable, NPagination, type DataTableColumns } from 'naive-ui'
import PageHeading from '../components/PageHeading.vue'
import ErrorNotice from '../components/ErrorNotice.vue'
import AppIcon from '../components/AppIcon.vue'
import { api } from '../api'
import type { Audit } from '../models'
import { errorMessage, formatDate } from '../utils'

const items = ref<Audit[]>([]),
  total = ref(0),
  page = ref(1),
  pageSize = ref(20),
  loading = ref(true),
  error = ref('')
let generation = 0
const columns: DataTableColumns<Audit> = [
  { title: '时间', key: 'created_at', width: 190, render: (row) => formatDate(row.created_at) },
  { title: '操作人', key: 'actor', width: 150, ellipsis: { tooltip: true } },
  { title: '操作', key: 'action', width: 180, ellipsis: { tooltip: true } },
  { title: '对象', key: 'target', width: 180, ellipsis: { tooltip: true } },
  { title: '说明', key: 'message', minWidth: 260, ellipsis: { tooltip: true } },
]
async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const result = await api<{ items: Audit[]; total: number }>(
      `/audit?page=${page.value}&page_size=${pageSize.value}`,
    )
    if (current === generation) {
      items.value = result.items ?? []
      total.value = result.total
    }
  } catch (cause) {
    if (current === generation) error.value = errorMessage(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}
function changePage(value: number) {
  page.value = value
  void load()
}
function changeSize(value: number) {
  pageSize.value = value
  page.value = 1
  void load()
}
onMounted(load)
</script>
<template>
  <PageHeading
    title="操作审计"
    description="追踪登录、注册、服务管理与系统设置的重要操作。"
    eyebrow="操作历史"
    ><NButton :loading="loading" @click="load"
      ><template #icon><AppIcon name="refresh" :size="16" /></template>刷新记录</NButton
    ></PageHeading
  ><ErrorNotice :message="error" retry @retry="load" />
  <section class="panel">
    <div class="panel-header">
      <div>
        <h2>审计记录</h2>
        <p>按时间倒序展示，不包含明文密码、注册密钥或会话凭据。</p>
      </div>
      <span class="subtle-count">共 {{ total }} 条</span>
    </div>
    <NDataTable
      :columns="columns"
      :data="items"
      :loading="loading"
      :bordered="false"
      :scroll-x="960"
      :row-key="(row: Audit) => row.id"
      ><template #empty
        ><div class="table-empty">
          <AppIcon name="audit" :size="36" /><strong>暂无审计记录</strong>
          <p>重要操作会自动记录在此处。</p>
        </div></template
      ></NDataTable
    >
    <div class="table-pagination">
      <NPagination
        :page="page"
        :page-size="pageSize"
        :item-count="total"
        :page-sizes="[20, 50, 100]"
        show-size-picker
        :disabled="loading"
        @update:page="changePage"
        @update:page-size="changeSize"
      />
    </div>
  </section>
</template>
