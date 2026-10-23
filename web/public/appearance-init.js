// 在首屏绘制前恢复外观；不使用内联脚本，兼容控制中心的 CSP。
(() => {
  let appearance
  try {
    appearance = JSON.parse(localStorage.getItem('workstation.appearance') || 'null')
  } catch {
    appearance = null
  }
  const theme = appearance?.theme === 'dark' ? 'dark' : 'light'
  document.documentElement.dataset.theme = theme
  document.documentElement.dataset.accent = 'brand'
  document.documentElement.style.colorScheme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#13171e' : '#f6f8fa')
})()
