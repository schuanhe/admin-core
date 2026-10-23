<script setup lang="ts">
import { onErrorCaptured, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NSkeleton } from 'naive-ui'
import ErrorNotice from '../components/ErrorNotice.vue'
import DynamicPage from '../dynamic/DynamicPage.vue'
import { api } from '../api'
import type { Service } from '../models'
import { errorMessage, servicePath } from '../utils'

const route = useRoute(),
  router = useRouter(),
  service = ref<Service | null>(null),
  loading = ref(true),
  error = ref(''),
  renderError = ref(''),
  rendererKey = ref(0)
onErrorCaptured(() => {
  renderError.value = '该服务页面暂时无法渲染。其他页面和控制中心管理功能不受影响。'
  return false
})
async function load() {
  loading.value = true
  error.value = ''
  renderError.value = ''
  rendererKey.value++
  try {
    service.value = (await api<{ service: Service }>(servicePath(String(route.params.id)))).service
  } catch (cause) {
    error.value = errorMessage(cause)
    service.value = null
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
<template>
  <ErrorNotice :message="error || renderError" retry @retry="load" />
  <NSkeleton v-if="loading" height="300px" />
  <template v-else-if="service && !renderError">
    <NAlert v-if="service.activation !== 'enabled'" type="info" title="该服务尚未启用">
      <div class="error-content">
        <span>管理员启用服务后才能访问服务工作台。</span>
        <NButton size="small" @click="router.push(servicePath(service.id))">查看服务</NButton>
      </div>
    </NAlert>
    <DynamicPage
      v-else
      :key="rendererKey"
      :service="service"
      :page-id="String(route.params.pageId)"
    />
  </template>
</template>
