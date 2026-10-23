<script setup lang="ts">
import { iconPaths, isIconName } from './icons'
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 20 })
</script>
<template>
  <svg :width="size" :height="size" viewBox="0 0 24 24" fill="none" stroke="currentColor"
    stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
    <path :d="isIconName(name) ? iconPaths[name] : iconPaths.page" />
  </svg>
</template>
