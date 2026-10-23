<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NDataTable, NSkeleton, type DataTableColumns } from 'naive-ui'
import PageHeading from '../components/PageHeading.vue'
import ErrorNotice from '../components/ErrorNotice.vue'
import StatusBadge from '../components/StatusBadge.vue'
import AppIcon from '../components/AppIcon.vue'
import { api } from '../api'
import type { Dashboard, Service } from '../models'
import { formatDate, errorMessage, servicePath } from '../utils'
import { session } from '../state'

const router = useRouter(),
  data = ref<Dashboard | null>(null),
  loading = ref(true),
  error = ref(''),
  refreshed = ref('')
const stats = computed(() => [
  {
    label: '登记服务',
    value: data.value?.total,
    note: '工作站中的全部服务',
    icon: 'services',
    color: 'primary',
  },
  {
    label: '已启用',
    value: data.value?.enabled,
    note: '可在服务工作台中访问',
    icon: 'check',
    color: 'green',
  },
  {
    label: '待启用',
    value: data.value?.pending,
    note: '等待你的确认与启用',
    icon: 'clock',
    color: 'amber',
  },
  {
    label: '当前离线',
    value: data.value?.offline,
    note: '保留登记及原启用状态',
    icon: 'dot',
    color: 'gray',
  },
])
const columns: DataTableColumns<Service> = [
  {
    title: '服务',
    key: 'name',
    minWidth: 220,
    render: (row) =>
      h('div', { class: 'table-service' }, [h('strong', row.name), h('small', row.id)]),
  },
  {
    title: '版本',
    key: 'version',
    width: 130,
    render: (row) => h('span', { class: 'version-label' }, row.version),
  },
  {
    title: '启用状态',
    key: 'activation',
    width: 130,
    render: (row) => h(StatusBadge, { value: row.activation, kind: 'activation' }),
  },
  {
    title: '在线状态',
    key: 'online',
    width: 110,
    render: (row) => h(StatusBadge, { value: row.online, kind: 'online' }),
  },
  {
    title: '最近注册',
    key: 'registered_at',
    minWidth: 170,
    render: (row) => formatDate(row.registered_at),
  },
  {
    title: '',
    key: 'action',
    width: 80,
    render: (row) =>
      h(
        NButton,
        { text: true, type: 'primary', onClick: () => router.push(servicePath(row.id)) },
        { default: () => '查看' },
      ),
  },
]
async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api<Dashboard>('/dashboard')
    refreshed.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
<template>
  <PageHeading
    title="工作站概览"
    :description="`你好，${session.username}。在这里了解你的服务运行情况。`"
    eyebrow="运行总览"
    ><NButton :loading="loading" @click="load"
      ><template #icon><AppIcon name="refresh" :size="16" /></template>刷新状态</NButton
    ></PageHeading
  >
  <ErrorNotice :message="error" retry @retry="load" />
  <div class="stats-grid">
    <article v-for="(stat, index) in stats" :key="stat.label" class="stat-card" :class="{ 'stat-primary': index === 0 }">
      <div class="stat-top">
        <span>{{ stat.label }}</span
        ><span class="stat-icon" :class="stat.color"><AppIcon :name="stat.icon" :size="19" /></span>
      </div>
      <NSkeleton v-if="loading && !data" text width="64px" height="44px" /><strong v-else>{{
        stat.value ?? '—'
      }}</strong
      ><small>{{ stat.note }}</small>
    </article>
  </div>
  <section class="panel">
    <div class="panel-header">
      <div>
        <h2>最近登记的服务</h2>
        <p>新服务登记后，需要管理员手动启用。</p>
      </div>
      <NButton text type="primary" @click="router.push('/services')"
        >全部服务<template #icon><AppIcon name="arrow" :size="16" /></template
      ></NButton>
    </div>
    <NDataTable
      :columns="columns"
      :data="data?.recent ?? []"
      :loading="loading"
      :bordered="false"
      :scroll-x="850"
      :row-key="(row: Service) => row.id"
      ><template #empty
        ><div class="table-empty">
          <AppIcon name="services" :size="35" /><strong>还没有服务登记</strong>
          <p>启动独立测试服务后，登记记录会自动出现在这里。</p>
          <NButton secondary type="primary" @click="router.push('/services')">进入服务管理</NButton>
        </div></template
      ></NDataTable
    >
  </section>
  <div class="overview-bottom">
    <section class="info-card">
      <span class="info-card-icon"><AppIcon name="shield" :size="24" /></span>
      <div>
        <h3>服务独立，控制中心始终可用</h3>
        <p>服务离线不会删除登记或改变启用状态。服务恢复后，重新检测即可更新在线状态。</p>
      </div>
    </section>
    <div class="refresh-label">
      <span class="status-dot online" />{{
        refreshed ? `最近刷新 ${refreshed}` : '等待连接控制中心'
      }}
    </div>
  </div>
</template>
