<script setup>
import { useRouter } from 'vue-router'
import { useNotes } from '../stores/notes'
const props = defineProps({ node: Object, depth: { type: Number, default: 0 } })
const store = useNotes()
const router = useRouter()
const isOpen = () => store.expanded.has(props.node.path)
async function newNote() {
  const n = await store.create(props.node.path)
  if (n) router.push('/n/' + n.id)
}
</script>

<template>
  <div>
    <div v-if="node.path" class="row folder" :style="{ paddingLeft: 8 + depth * 14 + 'px' }">
      <button class="grow" @click="store.toggle(node.path)">
        <span class="caret">{{ isOpen() ? '▾' : '▸' }}</span> 📁 {{ node.name }}
      </button>
      <span class="acts">
        <button title="New note" @click="newNote">＋</button>
        <button title="New subfolder" @click="store.newFolder(node.path)">📂</button>
        <button title="Rename folder" @click="store.renameFolder(node.path)">✎</button>
        <button v-if="!node.notes.length && !node.folders.length" title="Delete folder" @click="store.removeFolder(node.path)">✕</button>
      </span>
    </div>
    <template v-if="!node.path || isOpen()">
      <TreeNode v-for="f in node.folders" :key="f.path" :node="f" :depth="depth + (node.path ? 1 : 0)" />
      <RouterLink
        v-for="n in node.notes"
        :key="n.id"
        :to="'/n/' + n.id"
        class="row note"
        :class="{ active: store.cur?.id === n.id }"
        :style="{ paddingLeft: 8 + (depth + (node.path ? 1 : 0)) * 14 + 'px' }"
      >
        <span class="grow">📝 {{ store.title(n.path) }}</span>
        <span v-for="t in n.tags.slice(0, 2)" :key="t" class="mini">#{{ t }}</span>
      </RouterLink>
    </template>
  </div>
</template>
