// src/router/index.js
import { createRouter, createWebHashHistory } from 'vue-router'
import NotesView from '@/views/NotesView.vue'
import SettingsView from '@/views/SettingsView.vue'

const routes = [
  // {
  //   path: '/home',
  //   name: 'home',
  //   component: HomeView,
  //   meta: { title: 'Good Morning, James', subtitle: 'Continue your journey', icon: '⌂' },
  // },
  // {
  //   path: '/about',
  //   name: 'about',
  //   component: AboutView,
  //   meta: { title: 'Good Morning, James', subtitle: 'Continue your journey', icon: '⌂' },
  // },
  { path: '/', name: 'home', component: NotesView },
  { path: '/n/:id', name: 'note', component: NotesView },
  { path: '/settings', name: 'settings', component: SettingsView },

]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
})

export default router