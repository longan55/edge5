import { defineStore } from 'pinia'
import request from '@/utils/request'

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    userInfo: null,
    permissions: []
  }),

  actions: {
    async login(username, password, captchaId, captcha) {
      const res = await request.post('/login', {
        username,
        password,
        captcha_id: captchaId,
        captcha
      })

      this.token = res.data.token
      this.userInfo = res.data.user
      localStorage.setItem('token', this.token)
      return res.data
    },

    async getUserInfo() {
      try {
        const res = await request.get('/user/info')
        this.userInfo = res.data
        return res.data
      } catch (error) {
        this.logout()
        throw error
      }
    },

    logout() {
      this.token = ''
      this.userInfo = null
      localStorage.removeItem('token')
    },

    async changePassword(oldPassword, newPassword) {
      await request.post('/user/password', {
        old_password: oldPassword,
        new_password: newPassword
      })
      this.logout()
    }
  }
})
