<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NSpin,
  useMessage,
  type FormInst,
  type FormRules,
} from 'naive-ui'
import AppIcon from '../components/AppIcon.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
import { api } from '../api'
import { acceptSession } from '../state'
import { errorMessage } from '../utils'
import type { Session } from '../models'

const route = useRoute(),
  router = useRouter(),
  message = useMessage()
const checking = ref(true),
  initialized = ref(true),
  busy = ref(false),
  error = ref(''),
  formRef = ref<FormInst | null>(null)
const form = reactive({ username: '', password: '', confirmation: '', bootstrap_token: '' })
const rules = computed<FormRules>(() => ({
  username: [{ required: true, message: '请输入管理员用户名', trigger: ['input', 'blur'] }],
  password: [
    { required: true, message: '请输入密码', trigger: ['input', 'blur'] },
    {
      validator: (_rule, value: string) =>
        initialized.value || (value.length >= 12 && new TextEncoder().encode(value).length <= 72),
      message: '初始化密码至少 12 位，且不超过 72 字节',
      trigger: 'blur',
    },
  ],
  confirmation: [
    {
      validator: (_rule, value: string) => initialized.value || value === form.password,
      message: '两次输入的密码不一致',
      trigger: 'blur',
    },
  ],
  bootstrap_token: [
    {
      validator: (_rule, value: string) => initialized.value || Boolean(value),
      message: '请输入部署时配置的初始化密钥',
      trigger: 'blur',
    },
  ],
}))
async function checkStatus() {
  checking.value = true
  error.value = ''
  try {
    initialized.value = (
      await api<{ initialized: boolean }>('/auth/status', { silentUnauthorized: true })
    ).initialized
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    checking.value = false
  }
}
onMounted(checkStatus)
async function submit() {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (!initialized.value) {
      await api('/auth/setup', {
        method: 'POST',
        body: {
          username: form.username,
          password: form.password,
          bootstrap_token: form.bootstrap_token,
        },
        silentUnauthorized: true,
      })
      initialized.value = true
      form.bootstrap_token = ''
      form.confirmation = ''
      message.success('管理员已初始化')
    }
    acceptSession(
      await api<Session>('/auth/login', {
        method: 'POST',
        body: { username: form.username, password: form.password },
        silentUnauthorized: true,
      }),
    )
    form.password = ''
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(
      redirect.startsWith('/') && !redirect.startsWith('//') && !redirect.startsWith('/login')
        ? redirect
        : '/',
    )
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="login-page">
    <div class="login-appearance"><ThemeToggle /></div>
    <section class="login-intro">
      <div class="brand">
        <span class="brand-icon"><AppIcon name="workstation" :size="26" /></span
        ><span><strong>工作站</strong><small>个人控制中心</small></span>
      </div>
      <div class="login-intro-body">
        <div class="eyebrow">你的服务，井然有序</div>
        <h1>一处掌控，<br />各自独立。</h1>
        <p>
          为个人工作站构建的轻量控制台。<br />发现服务、管理状态，让日常工作专注于真正重要的事。
        </p>
        <div class="login-features">
          <div><AppIcon name="services" /><span>独立运行的服务</span></div>
          <div><AppIcon name="shield" /><span>清晰可控的访问边界</span></div>
          <div><AppIcon name="page" /><span>统一的管理体验</span></div>
        </div>
      </div>
      <small class="login-footnote">中心控制器 v1 · 服务登记与管理</small>
    </section>
    <section class="login-main">
      <div class="login-card">
        <div class="login-icon">
          <AppIcon :name="initialized ? 'shield' : 'workstation'" :size="26" />
        </div>
        <h2>{{ initialized ? '欢迎回来' : '初始化你的工作站' }}</h2>
        <p class="muted">
          {{
            initialized ? '使用管理员账号登录控制中心。' : '创建首个管理员。此操作仅可执行一次。'
          }}
        </p>
        <NSpin :show="checking">
          <NAlert v-if="error" type="error" class="login-alert">
            <div class="error-content">
              <span>{{ error }}</span
              ><NButton v-if="!form.username" size="small" @click="checkStatus">重新连接</NButton>
            </div>
          </NAlert>
          <NAlert v-if="route.query.expired" type="warning" class="login-alert"
            >会话已失效，请重新登录。</NAlert
          >
          <NForm
            ref="formRef"
            :model="form"
            :rules="rules"
            :disabled="busy || checking"
            size="large"
            @submit.prevent="submit"
          >
            <NFormItem label="管理员用户名" path="username"
              ><NInput
                v-model:value="form.username"
                placeholder="输入用户名"
                :maxlength="64"
                :input-props="{ autocomplete: 'username', name: 'username' }"
            /></NFormItem>
            <NFormItem label="密码" path="password"
              ><NInput
                v-model:value="form.password"
                type="password"
                show-password-on="click"
                :placeholder="initialized ? '输入密码' : '至少 12 位的安全密码'"
                :maxlength="72"
                :input-props="{
                  autocomplete: initialized ? 'current-password' : 'new-password',
                  name: 'password',
                }"
            /></NFormItem>
            <template v-if="!initialized">
              <NFormItem label="确认密码" path="confirmation"
                ><NInput
                  v-model:value="form.confirmation"
                  type="password"
                  show-password-on="click"
                  placeholder="再次输入密码"
                  :maxlength="72"
                  :input-props="{ autocomplete: 'new-password' }"
              /></NFormItem>
              <NFormItem label="初始化密钥" path="bootstrap_token"
                ><NInput
                  v-model:value="form.bootstrap_token"
                  type="password"
                  show-password-on="click"
                  placeholder="部署配置中的 bootstrap_token"
                  :input-props="{ autocomplete: 'off' }"
              /></NFormItem>
              <p class="login-security-note">
                初始化密钥由部署者配置，不会保存在浏览器中。若系统已初始化，请刷新此页面。
              </p>
            </template>
            <NButton
              attr-type="submit"
              type="primary"
              block
              size="large"
              :loading="busy"
              :disabled="checking"
              >{{ initialized ? '登录控制中心' : '创建管理员并登录'
              }}<template #icon><AppIcon name="arrow" :size="18" /></template
            ></NButton>
          </NForm>
        </NSpin>
        <div class="login-bottom">
          <AppIcon name="shield" :size="15" /><span>专属于你的工作站，安全访问。</span>
        </div>
      </div>
    </section>
  </div>
</template>
