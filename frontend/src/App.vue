<script setup>
import { ref, watch, nextTick } from 'vue'
import { useUi } from '@/stores/ui'

const ui = useUi()
const input = ref(null)
const text = ref('')
watch(() => ui.dialog, async (d) => {
  text.value = d?.value ?? ''
  if (d?.kind === 'ask') { await nextTick(); input.value?.focus(); input.value?.select() }
})
</script>

<template>
  <RouterView />
  <div v-if="ui.dialog" class="overlay" @click.self="ui.close(null)">
    <div class="modal" @keydown.esc="ui.close(null)">
      <h3>{{ ui.dialog.title }}</h3>
      <input v-if="ui.dialog.kind === 'ask'" ref="input" v-model="text" @keydown.enter="ui.close(text)" />
      <div class="btns">
        <button @click="ui.close(null)">Cancel</button>
        <button class="primary" @click="ui.close(ui.dialog.kind === 'ask' ? text : true)">OK</button>
      </div>
    </div>
  </div>
  <div v-if="ui.toast" class="toast">{{ ui.toast }}</div>
</template>