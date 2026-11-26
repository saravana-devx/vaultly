// Runs before the app renders so the page never flashes the wrong theme.
// Order: a choice saved by the toggle, otherwise the operating system setting.
(function () {
  var theme = 'dark'
  try {
    var saved = localStorage.getItem('vaultly-theme')
    if (saved === 'light' || saved === 'dark') theme = saved
    else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) theme = 'light'
  } catch (e) {}
  document.documentElement.setAttribute('data-theme', theme)
  var meta = document.querySelector('meta[name="theme-color"]')
  if (meta) meta.setAttribute('content', theme === 'light' ? '#FBFCFC' : '#07090d')
})()
