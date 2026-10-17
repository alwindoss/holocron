<script setup>
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useNotes } from '../stores/notes'
import TreeNode from '../components/TreeNode.vue'
import Editor from '../components/Editor.vue'
import TagBar from '../components/TagBar.vue'

const store = useNotes()
const route = useRoute()
const router = useRouter()

onMounted(store.init)
// The route drives which note is open; on narrow screens it also picks the visible pane.
watch(() => route.params.id, (id) => store.open(id || null), { immediate: true })

async function newNote(folder = '') {
  const n = await store.create(folder)
  if (n) router.push('/n/' + n.id)
}
async function del() {
  if (await store.remove()) router.replace('/')
}
const syncLabel = computed(() => {
  const s = store.status
  if (!s.configured) return 'Sync off'
  if (store.syncing) return 'Syncing…'
  if (s.lastError) return 'Sync error'
  return s.pending ? `${s.pending} pending` : 'Synced'
})
</script>

<template>
  <div class="shell" :class="{ 'show-editor': !!route.params.id }">
    <aside class="sidebar">
      <header class="bar">
        <h1>Holocron</h1>
        <button title="New note" @click="newNote('')">＋ Note</button>
        <button title="New folder" @click="store.newFolder('')">📂</button>
        <RouterLink to="/settings" title="Settings" class="btn">⚙</RouterLink>
      </header>
      <input v-model="store.query" class="search" type="search" placeholder="Search notes…" />
      <div v-if="store.tags.length" class="tags">
        <button
          v-for="t in store.tags"
          :key="t.tag"
          class="chip"
          :class="{ on: store.tagFilter === t.tag }"
          @click="store.tagFilter = store.tagFilter === t.tag ? '' : t.tag"
        >#{{ t.tag }} <small>{{ t.count }}</small></button>
      </div>
      <nav class="tree">
        <template v-if="store.filtered">
          <RouterLink v-for="n in store.filtered" :key="n.id" :to="'/n/' + n.id" class="row note" :class="{ active: store.cur?.id === n.id }">
            <span class="grow">📝 {{ n.path }}</span>
          </RouterLink>
          <p v-if="!store.filtered.length" class="muted pad">No matches</p>
        </template>
        <TreeNode v-else :node="store.tree" />
        <p v-if="!store.notes.length && !store.folders.length" class="muted pad">No notes yet — tap “＋ Note”.</p>
      </nav>
      <footer class="foot" @click="store.sync()">
        <span class="dot" :class="{ err: store.status.lastError, off: !store.status.configured }" /> {{ syncLabel }}
      </footer>
    </aside>

    <main class="pane">
      <template v-if="store.cur">
        <header class="bar">
          <RouterLink to="/" class="btn back" title="Back">‹</RouterLink>
          <h2 class="grow" :title="store.cur.path">{{ store.cur.path }}</h2>
          <button title="Rename / move" @click="store.move()">✎</button>
          <button title="Delete" @click="del">🗑</button>
        </header>
        <TagBar :tags="store.cur.tags" @change="store.setTags" />
        <Editor :key="store.cur.id + ':' + store.tick" :content="store.cur.content" @change="store.edit" />
      </template>
      <div v-else class="empty">
        <p>Select a note or create a new one.</p>
        <button @click="newNote('')">＋ New note</button>
      </div>
    </main>
  </div>
</template>
