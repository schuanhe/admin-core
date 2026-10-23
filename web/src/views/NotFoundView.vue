<script setup lang="ts">
import { NButton, NResult } from 'naive-ui'
</script>
<template>
  <div class="not-found">
    <NResult
      status="404"
      title="没有找到这个页面"
      description="页面地址可能已变更，或对应的服务登记已删除。"
      ><template #footer
        ><RouterLink to="/"><NButton type="primary">返回工作站概览</NButton></RouterLink></template
      ></NResult
    >
  </div>
</template>
