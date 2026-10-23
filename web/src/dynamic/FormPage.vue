<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { NAlert, NButton, NDatePicker, NForm, NFormItem, NInput, NInputNumber, NRadio, NRadioGroup, NSelect, NSpace, NSpin, NSwitch, type SelectOption } from 'naive-ui'
import { ownValue, validateFormValues, type DataRecord, type Field, type FormPageDefinition, type Scalar } from './schema'
import { requestServiceJSON, validateMutationResult, validateRecordResult } from './transport'
import { useResource } from './useResource'

const props = defineProps<{ page: FormPageDefinition; serviceId: string }>()
const model = reactive<DataRecord>({})
const errors = ref<Record<string, string>>({})
const success = ref('')
for (const field of props.page.fields) model[field.key] = field.type === 'boolean' ? false : ['text', 'textarea'].includes(field.type) ? '' : null
const initial = useResource(async signal => props.page.source ? validateRecordResult(await requestServiceJSON(props.serviceId, props.page.id, { signal })) : {})
const { data, error: initialError, loading: initialLoading } = initial
watch(data, value => {
  if (!value) return
  for (const field of props.page.fields) {
    const next = ownValue(value, field.key)
    if (next !== undefined) model[field.key] = next
  }
  errors.value = validateFormValues(props.page.fields, model)
})
const submit = useResource(async signal => {
  const payload: DataRecord = Object.create(null) as DataRecord
  for (const field of props.page.fields) payload[field.key] = model[field.key] ?? null
  const result = await requestServiceJSON(props.serviceId, props.page.id, { signal, operation: 'submit', body: payload })
  validateMutationResult(result)
  return true
})
const { error: submitError, loading: submitting } = submit
const locked = computed(() => initialLoading.value || submitting.value)
function optionsFor(field: Field): SelectOption[] {
  return field.options?.map(option => ({ label: option.label, value: option.value })) ?? []
}
function setValue(key: string, value: Scalar) {
  model[key] = value
  delete errors.value[key]
  success.value = ''
}
async function save() {
  if (locked.value || initialError.value) return
  success.value = ''
  errors.value = validateFormValues(props.page.fields, model)
  if (Object.keys(errors.value).length) return
  if (await submit.load()) success.value = '提交成功，服务已接收数据。'
}
onMounted(() => { if (props.page.source) void initial.load() })
</script>

<template>
  <div class="dynamic-content dynamic-form">
    <NAlert v-if="initialError" type="error" title="初始数据加载失败" :bordered="false">
      {{ initialError }}
      <div class="dynamic-alert-action"><NButton size="small" @click="initial.load">重新加载</NButton></div>
    </NAlert>
    <NAlert v-if="submitError" type="error" title="提交失败" :bordered="false">{{ submitError }}</NAlert>
    <NAlert v-if="success" type="success" :bordered="false">{{ success }}</NAlert>
    <NSpin :show="initialLoading">
      <NForm label-placement="top" :disabled="locked || !!initialError" @submit.prevent="save">
        <NFormItem v-for="field in page.fields" :key="field.key" :label="field.label" :required="field.required" :feedback="errors[field.key] || field.help" :validation-status="errors[field.key] ? 'error' : undefined">
          <NInput v-if="field.type === 'text' || field.type === 'textarea'" :type="field.type === 'textarea' ? 'textarea' : 'text'" :rows="field.rows ?? 3" :value="typeof model[field.key] === 'string' ? String(model[field.key]) : ''" :maxlength="field.max_length ?? 2000" :placeholder="field.placeholder ?? `请输入${field.label}`" @update:value="value => setValue(field.key, value)" />
          <NInputNumber v-else-if="field.type === 'number'" :value="typeof model[field.key] === 'number' ? model[field.key] as number : null" :min="field.min" :max="field.max" :placeholder="field.placeholder ?? `请输入${field.label}`" style="width: 100%" @update:value="value => setValue(field.key, value)" />
          <NSelect v-else-if="field.type === 'select'" :value="typeof model[field.key] === 'string' || typeof model[field.key] === 'number' ? model[field.key] as string | number : null" :options="optionsFor(field)" clearable :placeholder="field.placeholder ?? `请选择${field.label}`" @update:value="value => setValue(field.key, value)" />
          <NRadioGroup v-else-if="field.type === 'radio'" :value="typeof model[field.key] === 'string' || typeof model[field.key] === 'number' ? model[field.key] as string | number : null" @update:value="value => setValue(field.key, value)"><NSpace><NRadio v-for="option in field.options" :key="option.value" :value="option.value">{{ option.label }}</NRadio></NSpace></NRadioGroup>
          <NDatePicker v-else-if="field.type === 'date'" type="date" value-format="yyyy-MM-dd" :formatted-value="typeof model[field.key] === 'string' && model[field.key] ? String(model[field.key]) : null" :placeholder="field.placeholder ?? `请选择${field.label}`" clearable style="width: 100%" @update:formatted-value="value => setValue(field.key, typeof value === 'string' ? value : null)" />
          <NSwitch v-else-if="field.type === 'boolean'" :value="model[field.key] === true" @update:value="value => setValue(field.key, value)" />
        </NFormItem>
        <NButton type="primary" attr-type="submit" :loading="submitting" :disabled="locked || !!initialError">{{ page.submit_label ?? '保存设置' }}</NButton>
      </NForm>
    </NSpin>
  </div>
</template>
