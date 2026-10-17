<script setup>
import { ref } from 'vue'
const props = defineProps({ tags: { type: Array, default: () => [] } })
const emit = defineEmits(['change'])
const text = ref('')
function add() {
  const t = text.value.trim().replace(/^#/, '').toLowerCase()
  text.value = ''
  if (t && !props.tags.includes(t)) emit('change', [...props.tags, t])
}
</script>

<template>
  <div class="tagbar">
    <span v-for="t in tags" :key="t" class="chip">
      #{{ t }}
      <button aria-label="remove tag" @click="emit('change', tags.filter((x) => x !== t))">×</button>
    </span>
    <input v-model="text" placeholder="add tag…" enterkeyhint="done" @keydown.enter.prevent="add" @blur="add" />
  </div>
</template>
