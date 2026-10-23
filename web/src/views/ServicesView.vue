<script setup lang="ts">
import { h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton,
  NDataTable,
  NInput,
  NSelect,
  NSpace,
  useDialog,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import PageHeading from '../components/PageHeading.vue'
import ErrorNotice from '../components/ErrorNotice.vue'
import StatusBadge from '../components/StatusBadge.vue'
import AppIcon from '../components/AppIcon.vue'
import { api } from '../api'
import type { Activation, Service } from '../models'
import { refreshMenus } from '../state'
import { formatDate, errorMessage, servicePath } from '../utils'

const router = useRouter(),
  dialog = useDialog(),
  message = useMessage()
const items = ref<Service[]>([]),
  loading = ref(true),
  error = ref(''),
  q = ref(''),
  activation = ref<string | null>(null),
  online = ref<string | null>(null),
  busyId = ref('')
const activationOptions = [
  { label: '待启用', value: 'pending' },
  { label: '已启用', value: 'enabled' },
  { label: '已禁用', value: 'disabled' },
]
const onlineOptions = [
  { label: '在线', value: 'online' },
  { label: '离线', value: 'offline' },
  { label: '未检测', value: 'unknown' },
]
let generation = 0,
  debounce: ReturnType<typeof setTimeout> | undefined
async function load() {
  const current = ++generation
  loading.value = true
  error.value = ''
  const query = new URLSearchParams({ q: q.value.trim() })
  if (activation.value) query.set('activation', activation.value)
  if (online.value) query.set('online', online.value)
  try {
    const result = await api<{ items: Service[] }>(`/services?${query}`)
    if (current === generation) items.value = result.items ?? []
  } catch (cause) {
    if (current === generation) error.value = errorMessage(cause)
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(q, () => {
  clearTimeout(debounce)
  debounce = setTimeout(load, 300)
})
watch([activation, online], load)
onMounted(load)
onUnmounted(() => {
  clearTimeout(debounce)
  generation++
})
async function changeState(service: Service, state: Activation) {
  busyId.value = service.id
  try {
    await api(servicePath(service.id), { method: 'PATCH', body: { activation: state } })
    message.success(state === 'enabled' ? '服务已启用' : '服务已禁用')
    await Promise.all([load(), refreshMenus()])
  } catch (cause) {
    message.error(errorMessage(cause))
  } finally {
    busyId.value = ''
  }
}
async function check(service: Service) {
  busyId.value = service.id
  try {
    const result = await api<{ service: Service }>(`${servicePath(service.id)}/check`, {
      method: 'POST',
    })
    message[result.service.online === 'online' ? 'success' : 'warning'](
      result.service.online === 'online' ? '服务在线' : '服务当前不可达，登记记录已保留',
    )
    await Promise.all([load(), refreshMenus()])
  } catch (cause) {
    message.error(errorMessage(cause))
  } finally {
    busyId.value = ''
  }
}
function remove(service: Service) {
  dialog.warning({
    title: `删除「${service.name}」的登记？`,
    content:
      '仅删除控制中心保存的登记信息、页面定义和注册事件，不会删除服务自身的文件、数据库或容器。若服务再次注册，将作为待启用服务重新登记。',
    positiveText: '确认删除登记',
    negativeText: '取消',
    onPositiveClick: async () => {
      busyId.value = service.id
      try {
        await api(servicePath(service.id), { method: 'DELETE' })
        message.success('服务登记已删除')
        await Promise.all([load(), refreshMenus()])
        return true
      } catch (cause) {
        message.error(errorMessage(cause))
        return false
      } finally {
        busyId.value = ''
      }
    },
  })
}
const columns: DataTableColumns<Service> = [
  {
    title: '服务名称 / 唯一标识',
    key: 'name',
    fixed: 'left',
    width: 210,
    ellipsis: { tooltip: true },
    render: (row) =>
      h('div', { class: 'table-service' }, [
        h(
          NButton,
          { text: true, onClick: () => router.push(servicePath(row.id)) },
          { default: () => row.name },
        ),
        h('small', row.id),
      ]),
  },
  { title: '版本', key: 'version', width: 100, ellipsis: { tooltip: true } },
  { title: '登记地址', key: 'base_url', width: 230, ellipsis: { tooltip: true } },
  {
    title: '启用状态',
    key: 'activation',
    width: 115,
    render: (row) => h(StatusBadge, { value: row.activation, kind: 'activation' }),
  },
  {
    title: '在线状态',
    key: 'online',
    width: 105,
    render: (row) => h(StatusBadge, { value: row.online, kind: 'online' }),
  },
  {
    title: '最近注册',
    key: 'registered_at',
    width: 175,
    render: (row) => formatDate(row.registered_at),
  },
  { title: '最近检查', key: 'checked_at', width: 175, render: (row) => formatDate(row.checked_at) },
  {
    title: '操作',
    key: 'action',
    fixed: 'right',
    width: 210,
    render: (row) =>
      h(
        NSpace,
        { size: 12, wrap: false },
        {
          default: () => [
            h(
              NButton,
              {
                text: true,
                type: 'primary',
                disabled: Boolean(busyId.value),
                onClick: () =>
                  changeState(row, row.activation === 'enabled' ? 'disabled' : 'enabled'),
              },
              { default: () => (row.activation === 'enabled' ? '禁用' : '启用') },
            ),
            h(
              NButton,
              {
                text: true,
                disabled: Boolean(busyId.value),
                loading: busyId.value === row.id,
                onClick: () => check(row),
              },
              { default: () => '检测' },
            ),
            h(
              NButton,
              {
                text: true,
                type: 'error',
                disabled: Boolean(busyId.value),
                onClick: () => remove(row),
              },
              { default: () => '删除' },
            ),
          ],
        },
      ),
  },
]
</script>
<template>
  <PageHeading
    title="服务管理"
    description="管理独立服务的登记、启用状态与连接情况。"
    eyebrow="服务登记"
    ><NButton :loading="loading" @click="load"
      ><template #icon><AppIcon name="refresh" :size="16" /></template>刷新列表</NButton
    ></PageHeading
  ><ErrorNotice :message="error" retry @retry="load" />
  <section class="panel">
    <div class="filter-bar">
      <NInput
        v-model:value="q"
        clearable
        placeholder="搜索服务名称、标识或地址"
        class="service-search"
        :maxlength="200"
        ><template #prefix><AppIcon name="search" :size="16" /></template></NInput
      ><NSelect
        v-model:value="activation"
        :options="activationOptions"
        placeholder="全部启用状态"
        clearable
        class="filter-select"
      /><NSelect
        v-model:value="online"
        :options="onlineOptions"
        placeholder="全部在线状态"
        clearable
        class="filter-select"
      /><span class="result-count">{{ items.length }} 个服务</span>
    </div>
    <NDataTable
      :columns="columns"
      :data="items"
      :loading="loading"
      :bordered="false"
      :scroll-x="1320"
      :row-key="(row: Service) => row.id"
      :pagination="{ pageSize: 20, showSizePicker: true, pageSizes: [10, 20, 50] }"
      ><template #empty
        ><div class="table-empty">
          <AppIcon name="services" :size="36" /><strong>{{
            q || activation || online ? '没有符合条件的服务' : '等待第一个服务登记'
          }}</strong>
          <p>
            {{
              q || activation || online
                ? '试着修改搜索词或筛选条件。'
                : '服务通过受认证保护的注册接口主动登记，随后会显示为待启用。'
            }}
          </p>
        </div></template
      ></NDataTable
    >
  </section>
  <div class="page-tip">
    <AppIcon name="shield" :size="17" /><span
      >启用状态与在线状态独立管理。服务离线时，仍可禁用或删除登记；恢复连接无需重新启用。</span
    >
  </div>
</template>
