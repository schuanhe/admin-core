<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSkeleton,
  NSwitch,
  useMessage,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import PageHeading from '../components/PageHeading.vue'
import ErrorNotice from '../components/ErrorNotice.vue'
import AppIcon from '../components/AppIcon.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import BrandPalette from '../components/BrandPalette.vue'
import { api } from '../api'
import { workspace } from '../state'
import type { Settings } from '../models'
import { errorMessage } from '../utils'

const settings = ref<Settings | null>(null),
  loading = ref(true),
  saving = ref(false),
  error = ref(''),
  formRef = ref<FormInst | null>(null),
  message = useMessage()
const form = reactive({ site_name: '', check_interval_seconds: 30 as number | null, handbook_enabled: false })
const handbookURL = new URL('/api/v1/integration/handbook', window.location.origin).href
async function copyHandbook() {
  try { await navigator.clipboard.writeText(handbookURL); message.success('接入手册地址已复制') }
  catch { message.error('复制失败，请选中下方地址手动复制') }
}
const rules: FormRules = {
  site_name: [
    { required: true, whitespace: true, message: '请输入站点名称', trigger: ['input', 'blur'] },
  ],
  check_interval_seconds: [
    {
      type: 'number',
      required: true,
      min: 10,
      max: 3600,
      message: '检查间隔应为 10–3600 秒',
      trigger: ['change', 'blur'],
    },
  ],
}
async function load() {
  loading.value = true
  error.value = ''
  try {
    settings.value = await api<Settings>('/settings')
    form.site_name = settings.value.site_name
    form.check_interval_seconds = settings.value.check_interval_seconds
    form.handbook_enabled = settings.value.handbook_enabled
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}
async function save() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  saving.value = true
  try {
    settings.value = await api<Settings>('/settings', {
      method: 'PATCH',
      body: {
        site_name: form.site_name.trim(),
        check_interval_seconds: form.check_interval_seconds,
        handbook_enabled: form.handbook_enabled,
      },
    })
    workspace.siteName = settings.value.site_name
    message.success('系统设置已保存')
  } catch (cause) {
    message.error(errorMessage(cause))
  } finally {
    saving.value = false
  }
}
onMounted(load)
</script>
<template>
  <PageHeading
    title="系统设置"
    description="调整工作站的基本信息与服务检查策略。"
    eyebrow="工作站偏好"
  />
  <section class="panel appearance-panel" aria-label="外观偏好">
    <div class="appearance-header">
      <div class="settings-section-title">
        <AppIcon name="palette" />
        <div><h2>外观与配色</h2><p>统一蓝白配色。明暗模式选择仅保存在当前浏览器。</p></div>
      </div>
      <ThemeToggle />
    </div>
    <BrandPalette />
  </section>
  <ErrorNotice :message="error" retry @retry="load" /><NSkeleton
    v-if="loading && !settings"
    height="360px"
  />
  <div v-if="settings" class="settings-grid">
    <section class="panel settings-form">
      <div class="settings-section-title">
        <AppIcon name="settings" />
        <div>
          <h2>基本设置</h2>
          <p>修改后即时应用，并记录操作审计。</p>
        </div>
      </div>
      <NForm ref="formRef" :model="form" :rules="rules" :disabled="saving" @submit.prevent="save"
        ><NFormItem label="站点名称" path="site_name"
          ><NInput
            v-model:value="form.site_name"
            placeholder="个人工作站"
            :maxlength="80"
            show-count /></NFormItem
        ><NFormItem label="健康检查间隔（秒）" path="check_interval_seconds"
          ><NInputNumber
            v-model:value="form.check_interval_seconds"
            :min="10"
            :max="3600"
            :precision="0"
            :step="10"
            style="width: 100%"
        /></NFormItem>
        <p class="field-hint">定期检查只更新服务在线状态，不修改启用状态，不删除离线服务。</p>
        <div class="handbook-setting">
          <div class="handbook-setting-header">
            <div><h3 id="handbook-switch-label">服务接入手册</h3><p id="handbook-switch-help" class="field-hint">公开组件、CSS及注册规则，方便其他AI接入。默认关闭，保存设置后生效。</p></div>
            <NSwitch v-model:value="form.handbook_enabled" aria-labelledby="handbook-switch-label" aria-describedby="handbook-switch-help" />
          </div>
          <label class="handbook-url-label" for="handbook-url">手册接口地址</label>
          <NInput id="handbook-url" :value="handbookURL" readonly />
          <p class="field-hint">开启后无需登录即可读取通用手册；不返回密钥、已登记服务或实际部署策略。关闭后，两种格式均返回404。</p>
          <div v-if="settings.handbook_enabled" class="handbook-links">
            <NButton size="small" @click="copyHandbook">复制AI接入地址</NButton>
            <a :href="handbookURL" target="_blank" rel="noopener noreferrer">查看手册</a>
            <a :href="`${handbookURL}?format=json`" target="_blank" rel="noopener noreferrer">JSON契约</a>
          </div>
          <p v-else class="field-hint">接口当前未开放。</p>
        </div>
        <div class="form-footer">
          <NButton attr-type="submit" type="primary" :loading="saving">保存设置</NButton>
        </div></NForm
      >
    </section>
    <div>
      <section class="panel security-settings">
        <div class="settings-section-title">
          <AppIcon name="shield" />
          <div>
            <h2>安全边界</h2>
            <p>以下配置仅可在服务端部署时修改。</p>
          </div>
        </div>
        <div class="readonly-setting">
          <span>管理员会话有效期</span><strong>{{ settings.session_hours }} 小时</strong>
        </div>
        <div class="readonly-setting">
          <span>业务接入方式</span><strong>中心鉴权网关</strong>
        </div>
        <div class="readonly-setting">
          <span>已授权业务服务</span><strong>{{ settings.gateway_service_count }} 个</strong>
        </div>
        <div class="readonly-setting vertical">
          <span>允许登记的服务地址</span>
          <div class="origin-list">
            <code v-for="origin in settings.allowed_service_origins" :key="origin">{{
              origin
            }}</code
            ><span v-if="!settings.allowed_service_origins?.length" class="muted"
              >未配置任何地址</span
            >
          </div>
        </div>
      </section>
      <NAlert type="info" :show-icon="false" class="section-gap"
        ><strong>密钥不会显示在控制台中</strong>
        <p class="alert-description">
          注册密钥和业务令牌独立管理，均不返回浏览器。中心只调用部署白名单与页面定义共同允许的接口；服务还需验证业务令牌并限制中心的来源IP。
        </p></NAlert
      >
    </div>
  </div>
</template>

<style scoped>
.handbook-setting { border-top: 1px solid var(--border-default); margin-top: 24px; padding-top: 24px; }
.handbook-setting-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 16px; }
.handbook-setting h3 { font-size: 15px; margin: 0 0 8px; font-weight: 600; }
.handbook-setting .field-hint { line-height: 1.7; margin: 8px 0; }
.handbook-url-label { display: block; margin-bottom: 8px; font-size: 13px; color: var(--text-secondary); }
.handbook-links { display: flex; gap: 16px; align-items: center; flex-wrap: wrap; margin-top: 16px; }
.handbook-links a { color: var(--accent-text); text-decoration: underline; text-underline-offset: 3px; }
.handbook-links a:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; }
</style>
