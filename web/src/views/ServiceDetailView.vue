<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NCollapse,
  NCollapseItem,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NEmpty,
  NSkeleton,
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
import type { RegistrationEvent, Service } from '../models'
import { refreshMenus } from '../state'
import { dynamicPath, formatDate, errorMessage, servicePath } from '../utils'

const route = useRoute(),
  router = useRouter(),
  message = useMessage(),
  dialog = useDialog()
const service = ref<Service | null>(null),
  events = ref<RegistrationEvent[]>([]),
  loading = ref(true),
  busy = ref(false),
  error = ref('')
const path = computed(() => servicePath(String(route.params.id)))
const definitionText = computed(() => JSON.stringify(service.value?.definition, null, 2))
const menus = computed(() =>
  Array.isArray(service.value?.definition?.menus) ? service.value.definition.menus : [],
)
const columns: DataTableColumns<RegistrationEvent> = [
  { title: '注册时间', key: 'created_at', width: 190, render: (row) => formatDate(row.created_at) },
  {
    title: '结果',
    key: 'success',
    width: 100,
    render: (row) =>
      h(
        'span',
        { class: row.success ? 'text-success' : 'text-error' },
        row.success ? '注册成功' : '注册失败',
      ),
  },
  { title: '结果说明', key: 'message', minWidth: 260, ellipsis: { tooltip: true } },
]
async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await api<{ service: Service; events: RegistrationEvent[] }>(path.value)
    service.value = result.service
    events.value = result.events ?? []
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}
onMounted(load)
async function changeState(activation: 'enabled' | 'disabled') {
  busy.value = true
  try {
    service.value = (
      await api<{ service: Service }>(path.value, { method: 'PATCH', body: { activation } })
    ).service
    message.success(activation === 'enabled' ? '服务已启用，菜单已更新' : '服务已禁用')
    await refreshMenus()
  } catch (cause) {
    message.error(errorMessage(cause))
  } finally {
    busy.value = false
  }
}
async function check() {
  busy.value = true
  try {
    service.value = (
      await api<{ service: Service }>(`${path.value}/check`, { method: 'POST' })
    ).service
    message[service.value.online === 'online' ? 'success' : 'warning'](
      service.value.online === 'online' ? '检测完成：服务在线' : '检测完成：服务离线',
    )
    await refreshMenus()
  } catch (cause) {
    message.error(errorMessage(cause))
  } finally {
    busy.value = false
  }
}
function remove() {
  dialog.warning({
    title: '确认删除服务登记？',
    content: `将删除「${service.value?.name}」在控制中心的记录和页面定义，不触碰服务自身的数据。服务再次注册后将恢复为待启用。`,
    positiveText: '删除登记',
    negativeText: '取消',
    onPositiveClick: async () => {
      busy.value = true
      try {
        await api(path.value, { method: 'DELETE' })
        await refreshMenus()
        message.success('登记记录已删除')
        await router.push('/services')
        return true
      } catch (cause) {
        message.error(errorMessage(cause))
        return false
      } finally {
        busy.value = false
      }
    },
  })
}
</script>
<template>
  <NButton text class="back-link" @click="router.push('/services')"
    ><template #icon><AppIcon name="back" :size="16" /></template>返回服务管理</NButton
  ><PageHeading
    :title="service?.name ?? '服务详情'"
    :description="service?.description || '查看登记信息、服务状态与最近注册结果。'"
    eyebrow="服务详情"
    ><NSpace v-if="service"
      ><NButton :loading="busy" @click="check"
        ><template #icon><AppIcon name="refresh" :size="16" /></template>重新检测</NButton
      ><NButton
        type="primary"
        :disabled="busy"
        @click="changeState(service.activation === 'enabled' ? 'disabled' : 'enabled')"
        >{{ service.activation === 'enabled' ? '禁用服务' : '启用服务' }}</NButton
      ><NButton secondary type="error" :disabled="busy" @click="remove">删除登记</NButton></NSpace
    ></PageHeading
  ><ErrorNotice :message="error" retry @retry="load" /><NSkeleton
    v-if="loading && !service"
    height="260px"
  />
  <template v-if="service"
    ><div class="detail-status">
      <StatusBadge :value="service.activation" kind="activation" /><StatusBadge
        :value="service.online"
        kind="online"
      /><span class="version-label">{{ service.version }}</span>
    </div>
    <NAlert v-if="service.last_error" type="warning" title="最近健康检查结果" class="section-gap">{{
      service.last_error
    }}</NAlert
    ><NAlert
      v-if="service.last_registration_error"
      type="error"
      title="最近注册失败"
      class="section-gap"
      >{{ service.last_registration_error }}。上一次有效定义仍然保留。</NAlert
    >
    <section class="panel detail-panel">
      <h2>登记信息</h2>
      <NDescriptions
        label-placement="top"
        :column="3"
        :label-style="{ color: 'var(--text-secondary)', fontSize: '12px' }"
        :content-style="{ overflowWrap: 'anywhere' }"
        ><NDescriptionsItem label="唯一标识"
          ><code>{{ service.id }}</code></NDescriptionsItem
        ><NDescriptionsItem label="服务版本">{{ service.version }}</NDescriptionsItem
        ><NDescriptionsItem label="协议版本">v{{ service.protocol_version }}</NDescriptionsItem
        ><NDescriptionsItem label="登记地址">{{ service.base_url }}</NDescriptionsItem
        ><NDescriptionsItem label="健康检查路径"
          ><code>{{ service.health_path }}</code></NDescriptionsItem
        ><NDescriptionsItem label="首次登记">{{ formatDate(service.created_at) }}</NDescriptionsItem
        ><NDescriptionsItem label="最近注册">{{
          formatDate(service.registered_at)
        }}</NDescriptionsItem
        ><NDescriptionsItem label="最近检查">{{ formatDate(service.checked_at) }}</NDescriptionsItem
        ><NDescriptionsItem label="最近更新">{{
          formatDate(service.updated_at)
        }}</NDescriptionsItem></NDescriptions
      >
    </section>
    <section class="panel">
      <div class="panel-header">
        <div>
          <h2>服务页面</h2>
          <p>由服务提供 JSON 定义，使用控制中心的内置组件安全渲染。</p>
        </div>
        <span class="subtle-count">{{ menus.length }} 个菜单</span>
      </div>
      <div class="service-page-list">
        <NAlert
          v-if="service.activation !== 'enabled'"
          type="info"
          :show-icon="false"
          class="section-gap"
          >启用服务后即可访问这些页面；启用操作不会启动或部署服务。</NAlert
        ><NEmpty v-if="!menus.length" description="服务尚未提供页面菜单" /><RouterLink
          v-for="menu in menus"
          v-else-if="service.activation === 'enabled'"
          :key="menu.id"
          :to="dynamicPath(service.id, menu.page_id)"
          class="service-page-link"
          ><span class="page-link-icon"><AppIcon name="page" /></span
          ><span
            ><strong>{{ menu.label }}</strong
            ><small>{{ menu.page_id }}</small></span
          ><AppIcon name="arrow" :size="18"
        /></RouterLink>
        <div
          v-else
          v-for="menu in menus"
          :key="`disabled-${menu.id}`"
          class="service-page-link disabled"
        >
          <span class="page-link-icon"><AppIcon name="page" /></span
          ><span
            ><strong>{{ menu.label }}</strong
            ><small>服务未启用</small></span
          >
        </div>
      </div>
    </section>
    <section class="panel section-gap">
      <div class="panel-header">
        <div>
          <h2>最近注册记录</h2>
          <p>服务重新注册不会改变管理员设置的启用状态。</p>
        </div>
      </div>
      <NDataTable :columns="columns" :data="events" :bordered="false" :scroll-x="600"
        ><template #empty>暂无注册记录</template></NDataTable
      >
    </section>
    <section class="panel detail-panel section-gap">
      <NCollapse
        ><NCollapseItem title="查看已保存的页面定义" name="definition">
          <pre class="definition-code">{{ definitionText }}</pre>
        </NCollapseItem></NCollapse
      >
    </section></template
  >
</template>
