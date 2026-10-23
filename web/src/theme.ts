import { computed, reactive, watch } from 'vue'
import { darkTheme, type GlobalThemeOverrides } from 'naive-ui'

export type ThemeMode = 'light' | 'dark'
const root = document.documentElement
export const appearance = reactive({
  mode: (root.dataset.theme === 'dark' ? 'dark' : 'light') as ThemeMode,
})
export const naiveTheme = computed(() => appearance.mode === 'dark' ? darkTheme : null)
const overrides = reactive<{ value: GlobalThemeOverrides }>({ value: {} })
export const themeOverrides = computed(() => overrides.value)

watch(() => appearance.mode, () => {
  root.dataset.theme = appearance.mode
  root.dataset.accent = 'brand'
  root.style.colorScheme = appearance.mode
  const styles = getComputedStyle(root)
  const token = (name: string) => styles.getPropertyValue(`--${name}`).trim()
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', token('surface-canvas'))
  try {
    localStorage.setItem('workstation.appearance', JSON.stringify({ theme: appearance.mode, palette: 'brand-blue-v1' }))
  } catch {
    // 隐私模式或存储配额受限时仍允许切换，仅无法跨刷新保存。
  }
  overrides.value = {
    common: {
      primaryColor: token('accent'), primaryColorHover: token('accent-hover'), primaryColorPressed: token('accent-pressed'), primaryColorSuppl: token('accent'),
      successColor: token('success'), warningColor: token('warning'), errorColor: token('error'), infoColor: token('info'),
      bodyColor: token('surface-canvas'), cardColor: token('surface-raised'), modalColor: token('surface-overlay'), popoverColor: token('surface-overlay'),
      tableColor: token('surface-raised'), inputColor: token('surface-raised'), actionColor: token('surface-sunken'),
      textColorBase: token('text-primary'), textColor1: token('text-primary'), textColor2: token('text-secondary'), textColor3: token('text-tertiary'),
      placeholderColor: token('text-tertiary'), borderColor: token('border-default'), dividerColor: token('border-subtle'),
      hoverColor: token('surface-hover'), borderRadius: '6px', fontSize: '14px', fontFamily: token('font-body'),
    },
    DataTable: { thColor: token('surface-sidebar'), tdColor: token('surface-raised'), tdColorHover: token('surface-hover'), thTextColor: token('text-secondary'), thFontWeight: '500', borderColor: token('border-subtle') },
    Button: { fontWeight: '500', textColorPrimary: token('on-accent'), textColorHoverPrimary: token('on-accent-hover'), textColorPressedPrimary: token('on-accent-hover'), textColorFocusPrimary: token('on-accent'), textColorTextPrimary: token('accent-text'), textColorTextHoverPrimary: token('accent-text') },
    Card: { borderRadius: '12px', borderColor: token('border-default') },
    Dialog: { borderRadius: '16px' },
  }
}, { immediate: true, flush: 'sync' })
