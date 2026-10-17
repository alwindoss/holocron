<script setup>
import { onMounted, ref } from 'vue'
import { useNotes } from '../stores/notes'
import { useUi } from '../stores/ui'

const store = useNotes()
const ui = useUi()
const url = ref('')
const token = ref('')
const hasToken = ref(false)

onMounted(async () => {
  const s = await NotesService.GetServer()
  url.value = s.url
  hasToken.value = s.hasToken
  store.status = await NotesService.Status()
})

async function save() {
  try {
    await NotesService.SetServer(url.value, token.value)
    token.value = ''
    hasToken.value = (await NotesService.GetServer()).hasToken
    store.status = await NotesService.Status()
    ui.say('Saved')
    if (store.status.configured) store.sync()
  } catch (e) {
    ui.say(e)
  }
}
</script>

<template>
  <div class="settings">
    <header class="bar">
      <RouterLink to="/" class="btn back">‹</RouterLink>
      <h2 class="grow">Sync settings</h2>
    </header>
    <div class="form">
      <label>Server URL<input v-model="url" type="url" placeholder="https://notes.example.com" autocapitalize="off" /></label>
      <label>
        Access token
        <input v-model="token" type="password" :placeholder="hasToken ? '•••••••• (saved — leave blank to keep)' : 'token'" autocapitalize="off" />
      </label>
      <div class="btns">
        <button class="primary" @click="save">Save</button>
        <button :disabled="!store.status.configured || store.syncing" @click="store.sync()">Sync now</button>
      </div>
      <p class="muted">
        {{ store.status.configured ? 'Pending changes: ' + store.status.pending : 'Sync is off.' }}
        <span v-if="store.status.lastSync"> · Last sync {{ new Date(store.status.lastSync * 1000).toLocaleString() }}</span>
      </p>
      <p v-if="store.status.lastError" class="error">{{ store.status.lastError }}</p>
      <p class="muted">Leave the URL empty to turn sync off. Use https in production.</p>
    </div>
  </div>
</template>
