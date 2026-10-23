<script setup lang="ts">
import {
  NConfigProvider,
  NDialogProvider,
  NMessageProvider,
  zhCN,
  dateZhCN,
} from 'naive-ui'
import { naiveTheme, themeOverrides } from './theme'
</script>
<template>
  <NConfigProvider :locale="zhCN" :date-locale="dateZhCN" :theme="naiveTheme" :theme-overrides="themeOverrides">
    <NDialogProvider
      ><NMessageProvider><RouterView /></NMessageProvider
    ></NDialogProvider>
  </NConfigProvider>
</template>
