import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useUi } from './ui'
import { NotesService } from '../../bindings/go.dalw.in/holocron/internal/notes'

const title = (p) => p.split('/').pop()

export const useNotes = defineStore('notes', () => {
  const ui = useUi()
  const notes = ref([])
  const folders = ref([])
  const tags = ref([])
  const cur = ref(null) // { id, path, tags, content, updated }
  const tick = ref(0) // bump to remount the editor
  const query = ref('')
  const tagFilter = ref('')
  const expanded = ref(new Set(JSON.parse(localStorage.getItem('expanded') || '[]')))
  const status = ref({ configured: false, pending: 0, lastSync: 0, lastError: '' })
  const syncing = ref(false)
  let saveTimer, syncTimer
  let unsaved = false

  const filtered = computed(() => {
    const q = query.value.trim().toLowerCase()
    if (!q && !tagFilter.value) return null
    return notes.value.filter(
      (n) => (!q || n.path.toLowerCase().includes(q)) && (!tagFilter.value || n.tags.includes(tagFilter.value)),
    )
  })

  const tree = computed(() => {
    const root = { name: '', path: '', folders: [], notes: [] }
    const map = { '': root }
    const ensure = (p) => {
      if (map[p]) return map[p]
      const i = p.lastIndexOf('/')
      const node = { name: p.slice(i + 1), path: p, folders: [], notes: [] }
      map[p] = node
      ensure(i < 0 ? '' : p.slice(0, i)).folders.push(node)
      return node
    }
    folders.value.forEach(ensure)
    notes.value.forEach((n) => {
      const i = n.path.lastIndexOf('/')
      ensure(i < 0 ? '' : n.path.slice(0, i)).notes.push(n)
    })
    return root
  })

  async function safe(fn) {
    try {
      return await fn()
    } catch (e) {
      ui.say(e)
    }
  }

  async function loadTree() {
    const t = await NotesService.Tree()
    notes.value = t.notes || []
    folders.value = t.folders || []
    tags.value = (await NotesService.Tags()) || []
  }

  function toggle(path) {
    expanded.value.has(path) ? expanded.value.delete(path) : expanded.value.add(path)
    localStorage.setItem('expanded', JSON.stringify([...expanded.value]))
  }

  async function open(id) {
    await flush()
    if (!id) return (cur.value = null)
    const n = await safe(() => NotesService.Get(id))
    if (!n) return (cur.value = null)
    const meta = notes.value.find((m) => m.id === id)
    cur.value = { ...n, tags: n.tags || [], updated: meta?.updated || 0 }
    tick.value++
  }

  function edit(content) {
    if (!cur.value) return
    cur.value.content = content
    unsaved = true
    clearTimeout(saveTimer)
    saveTimer = setTimeout(flush, 700)
  }

  function setTags(t) {
    if (!cur.value) return
    cur.value.tags = t
    unsaved = true
    flush()
  }

  async function flush() {
    clearTimeout(saveTimer)
    if (!unsaved || !cur.value) return
    unsaved = false
    const { id, content, tags: t } = cur.value
    const updated = await safe(() => NotesService.Save(id, content, t))
    if (updated && cur.value?.id === id) cur.value.updated = updated
    await safe(loadTree)
    scheduleSync(3000)
  }

  async function create(folder = '') {
    const name = await ui.ask('Note title', 'Untitled')
    if (!name) return null
    const n = await safe(() => NotesService.Create(folder, name))
    if (n) {
      if (folder) expanded.value.add(folder)
      await loadTree()
      scheduleSync(3000)
    }
    return n
  }

  async function move() {
    if (!cur.value) return
    const p = await ui.ask('Rename / move (Folder/Title)', cur.value.path)
    if (!p || p === cur.value.path) return
    await flush()
    const np = await safe(() => NotesService.Move(cur.value.id, p))
    if (np) {
      cur.value.path = np
      await loadTree()
      scheduleSync(3000)
    }
  }

  async function remove() {
    if (!cur.value || !(await ui.confirm(`Delete "${title(cur.value.path)}"?`))) return false
    unsaved = false
    await safe(() => NotesService.Delete(cur.value.id))
    cur.value = null
    await loadTree()
    scheduleSync(3000)
    return true
  }

  async function newFolder(parent = '') {
    const name = await ui.ask('Folder name')
    if (!name) return
    await safe(() => NotesService.CreateFolder(parent ? `${parent}/${name}` : name))
    if (parent) expanded.value.add(parent)
    await loadTree()
  }

  async function renameFolder(path) {
    const p = await ui.ask('Rename / move folder', path)
    if (!p || p === path) return
    await flush()
    if ((await safe(() => NotesService.RenameFolder(path, p))) !== undefined) {
      await loadTree()
      scheduleSync(3000)
    }
  }

  async function removeFolder(path) {
    if (!(await ui.confirm(`Delete empty folder "${path}"?`))) return
    await safe(() => NotesService.DeleteFolder(path))
    await loadTree()
  }

  function scheduleSync(ms = 0) {
    clearTimeout(syncTimer)
    syncTimer = setTimeout(sync, ms)
  }

  async function sync() {
    if (syncing.value) return
    if (!status.value.configured) {
      status.value = await NotesService.Status()
      if (!status.value.configured) return
    }
    syncing.value = true
    try {
      await flush()
      status.value = await NotesService.SyncNow()
      await loadTree()
      const m = cur.value && notes.value.find((n) => n.id === cur.value.id)
      if (cur.value && !m) await open(null)
      else if (m && m.updated !== cur.value.updated && !unsaved) await open(cur.value.id) // changed remotely
    } catch (e) {
      ui.say(e)
    } finally {
      syncing.value = false
    }
  }

  async function init() {
    await safe(loadTree)
    status.value = await NotesService.Status()
    sync()
    setInterval(sync, 30000)
    window.addEventListener('focus', () => sync())
    window.addEventListener('blur', flush)
    document.addEventListener('visibilitychange', () => document.hidden && flush())
  }

  return {
    notes, folders, tags, cur, tick, query, tagFilter, expanded, status, syncing, filtered, tree,
    init, loadTree, toggle, open, edit, setTags, flush, create, move, remove, newFolder,
    renameFolder, removeFolder, sync, title,
  }
})
