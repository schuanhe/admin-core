<script setup lang="ts">
import { NAlert, NButton } from 'naive-ui'
defineProps<{ message: string; retry?: boolean }>()
defineEmits<{ retry: [] }>()
</script>
<template>
  <NAlert v-if="message" type="error" title="操作未完成" class="error-notice"
    ><div class="error-content">
      <span>{{ message }}</span
      ><NButton v-if="retry" size="small" secondary @click="$emit('retry')">重试</NButton>
    </div></NAlert
  >
</template>
