const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

Page({
  data: {
    user: null,
    baseURL: ''
  },

  onShow() {
    if (!ensureLogin()) return
    this.loadProfile()
    this.setData({ baseURL: wx.getStorageSync('baseURL') || '' })
  },

  async loadProfile() {
    try {
      const res = await api.getProfile()
      this.setData({ user: res.user || null })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    }
  },

  onLogout() {
    wx.removeStorageSync('token')
    wx.removeStorageSync('user')
    wx.reLaunch({ url: '/pages/login/index' })
  },

  gotoRoles() {
    wx.navigateTo({ url: '/pages/roles/index' })
  },

  gotoKnowledgeBases() {
    wx.navigateTo({ url: '/pages/knowledge-bases/index' })
  },

  gotoSpeakers() {
    wx.navigateTo({ url: '/pages/speakers/index' })
  },

  gotoVoiceClones() {
    wx.navigateTo({ url: '/pages/voice-clones/index' })
  }
})
