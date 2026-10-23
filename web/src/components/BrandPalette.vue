<script setup lang="ts">
const colors = [
  { label: '品牌主色', code: '#3478F6', variable: '--brand-primary', usage: '按钮 · 链接 · 选中导航' },
  { label: '辅助色', code: '#6AA8FF', variable: '--brand-secondary', usage: '次级强调 · 交互反馈' },
  { label: '浅色强调', code: '#D9EBFF', variable: '--brand-soft', usage: '选中底色 · 轻量提示' },
]
</script>
<template>
  <div class="brand-palette" aria-label="当前蓝白配色">
    <div v-for="color in colors" :key="color.code" class="brand-color">
      <span class="brand-color-swatch" :style="{ backgroundColor: `var(${color.variable})` }" aria-hidden="true" />
      <span><strong>{{ color.label }} <code>{{ color.code }}</code></strong><small>{{ color.usage }}</small></span>
    </div>
  </div>
</template>
