import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

export const useUserStore = defineStore('user', () => {
  const userDetails = ref({
    "name": "Alwin Doss",
    "initial": "AD",
    "email": "alwin@email.com",
    "phone": "9876543210",
  })


  return { userDetails }
})