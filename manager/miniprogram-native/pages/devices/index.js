const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const isActiveRole = (role) => role?.status === 'active' || !role?.status

const formatDateTime = (value) => {
  if (!value) return '从未'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '从未'
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  return `${y}-${m}-${day} ${h}:${min}`
}

Page({
  data: {
    list: [],
    loading: false,
    roleOptions: [],
    roleOptionNames: []
  },

  onShow() {
    if (!ensureLogin()) return
    this.loadDevices()
  },

  onPullDownRefresh() {
    this.loadDevices().finally(() => wx.stopPullDownRefresh())
  },

  async loadDevices() {
    this.setData({ loading: true })
    try {
      const [deviceRes, roleRes] = await Promise.all([
        api.getDevices(),
        api.getRoles()
      ])
      const globalRoles = roleRes?.data?.global_roles || []
      const userRoles = roleRes?.data?.user_roles || []
      const roleOptions = [{ id: null, name: '不使用角色' }, ...[...globalRoles, ...userRoles].filter(isActiveRole).map((r) => ({ id: r.id, name: r.name }))]
      const roleOptionNames = roleOptions.map((item) => item.name)

      const list = pickList(deviceRes).map((item) => {
        const index = roleOptions.findIndex((opt) => Number(opt.id) === Number(item.role_id))
        return {
          ...item,
          role_picker_index: index >= 0 ? index : 0,
          last_active_text: formatDateTime(item.last_active_at),
          created_text: formatDateTime(item.created_at)
        }
      })

      this.setData({ list, roleOptions, roleOptionNames })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载设备失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  async onRoleChange(e) {
    const deviceID = e.currentTarget.dataset.id
    const index = Number(e.detail.value)
    const option = this.data.roleOptions[index]
    if (!deviceID || !option) return

    wx.showLoading({ title: '应用中' })
    try {
      await api.applyDeviceRole(deviceID, option.id)
      wx.hideLoading()
      wx.showToast({ title: '角色已更新', icon: 'success' })
      const list = this.data.list.map((item) => {
        if (Number(item.id) === Number(deviceID)) {
          return { ...item, role_picker_index: index, role_id: option.id }
        }
        return item
      })
      this.setData({ list })
    } catch (err) {
      wx.hideLoading()
      wx.showToast({ title: err?.data?.error || '更新角色失败', icon: 'none' })
      this.loadDevices()
    }
  }
})
