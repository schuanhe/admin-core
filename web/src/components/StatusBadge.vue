<script setup lang="ts">
import { activationLabels, onlineLabels } from '../utils'
import type { Activation, Online } from '../models'
defineProps<{ value: Activation | Online; kind: 'activation' | 'online' }>()
</script>
<template>
  <span class="status-badge" :class="value"
    ><span class="status-dot" />{{
      kind === 'activation' ? activationLabels[value as Activation] : onlineLabels[value as Online]
    }}</span
  >
</template>
