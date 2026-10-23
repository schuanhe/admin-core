<script setup lang="ts">
import { appearance, type ThemeMode } from '../theme'
import AppIcon from './AppIcon.vue'
const modes: { value: ThemeMode; label: string; icon: string }[] = [
  { value: 'light', label: '白天', icon: 'sun' },
  { value: 'dark', label: '黑夜', icon: 'moon' },
]
</script>
<template>
  <div class="theme-toggle" role="group" aria-label="外观模式">
    <button v-for="mode in modes" :key="mode.value" type="button"
      :class="{ selected: appearance.mode === mode.value }"
      :aria-pressed="appearance.mode === mode.value" :aria-label="`切换到${mode.label}模式`"
      @click="appearance.mode = mode.value">
      <AppIcon :name="mode.icon" :size="15" /><span>{{ mode.label }}</span>
    </button>
  </div>
</template>
