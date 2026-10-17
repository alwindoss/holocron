<script setup>
import { onBeforeUnmount } from 'vue'
import { useEditor, EditorContent } from '@tiptap/vue-3'
import StarterKit from '@tiptap/starter-kit'
import Placeholder from '@tiptap/extension-placeholder'
import { Markdown } from 'tiptap-markdown'

const props = defineProps({ content: { type: String, default: '' } })
const emit = defineEmits(['change'])

// Remounted (via :key) whenever another note opens, so content is only read once.
const editor = useEditor({
  content: props.content,
  extensions: [
    StarterKit,
    Placeholder.configure({ placeholder: 'Start writing…' }),
    Markdown.configure({ html: false, tightLists: true, linkify: true, breaks: false }),
  ],
  onUpdate: ({ editor }) => emit('change', editor.storage.markdown.getMarkdown()),
})
onBeforeUnmount(() => editor.value?.destroy())

const btns = [
  ['B', 'toggleBold', 'bold'],
  ['I', 'toggleItalic', 'italic'],
  ['S', 'toggleStrike', 'strike'],
  ['</>', 'toggleCode', 'code'],
  ['H1', 'toggleHeading', 'heading', { level: 1 }],
  ['H2', 'toggleHeading', 'heading', { level: 2 }],
  ['•', 'toggleBulletList', 'bulletList'],
  ['1.', 'toggleOrderedList', 'orderedList'],
  ['❝', 'toggleBlockquote', 'blockquote'],
  ['{ }', 'toggleCodeBlock', 'codeBlock'],
]
const run = (cmd, arg) => editor.value.chain().focus()[cmd](arg).run()
</script>

<template>
  <div v-if="editor" class="editor">
    <div class="toolbar">
      <button
        v-for="[label, cmd, name, arg] in btns"
        :key="label"
        :class="{ on: editor.isActive(name, arg) }"
        @mousedown.prevent
        @click="run(cmd, arg)"
      >{{ label }}</button>
      <span class="sep" />
      <button :disabled="!editor.can().undo()" @mousedown.prevent @click="editor.chain().focus().undo().run()">↶</button>
      <button :disabled="!editor.can().redo()" @mousedown.prevent @click="editor.chain().focus().redo().run()">↷</button>
    </div>
    <EditorContent :editor="editor" class="content" />
  </div>
</template>
