import { defineStore } from 'pinia'
import { ref } from 'vue'

// Tiny promise-based dialogs (window.prompt/confirm are unreliable in mobile webviews).
export const useUi = defineStore('ui', () => {
  const dialog = ref(null)
  const toast = ref('')
  let toastTimer
  function ask(title, value = '') {
    return new Promise((resolve) => (dialog.value = { kind: 'ask', title, value, resolve }))
  }
  function confirm(title) {
    return new Promise((resolve) => (dialog.value = { kind: 'confirm', title, resolve }))
  }
  function close(result) {
    const d = dialog.value
    dialog.value = null
    d?.resolve(d.kind === 'confirm' ? !!result : result ?? null)
  }
  function say(msg) {
    toast.value = String(msg?.message || msg)
    clearTimeout(toastTimer)
    toastTimer = setTimeout(() => (toast.value = ''), 4000)
  }
  return { dialog, toast, ask, confirm, close, say }
})
