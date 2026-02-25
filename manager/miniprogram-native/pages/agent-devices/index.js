const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

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

const isActiveRole = (role) => role?.status === 'active' || !role?.status

Page({
  data: {
    agentID: '',
    loading: false,
    list: [],
    roleOptions: [],
    roleOptionNames: [],
    showBind: false,
    bindCode: '',
    binding: false
  },

  onLoad(options) {
    const agentID = options?.id || ''
    if (!agentID) {
      wx.showToast({ title: '缺少智能体ID', icon: 'none' })
      setTimeout(() => wx.navigateBack(), 300)
      return
    }
    this.setData({ agentID })
  },

  onShow() {
    if (!ensureLogin()) return
    if (!this.data.agentID) return
    this.loadData()
  },

  onPullDownRefresh() {
    this.loadData().finally(() => wx.stopPullDownRefresh())
  },

  async loadData() {
    this.setData({ loading: true })
    try {
      const { agentID } = this.data
      const [deviceRes, roleRes] = await Promise.all([
        api.getAgentDevices(agentID),
        api.getRoles()
      ])

      const globalRoles = roleRes?.data?.global_roles || []
      const userRoles = roleRes?.data?.user_roles || []
      const mergedRoles = [...globalRoles, ...userRoles].filter(isActiveRole)
      const roleOptions = [{ id: null, name: '不使用角色' }, ...mergedRoles.map((r) => ({ id: r.id, name: r.name }))]
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

      this.setData({
        list,
        roleOptions,
        roleOptionNames
      })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  openBind() {
    this.setData({
      showBind: true,
      bindCode: '',
      binding: false
    })
  },

  closeBind() {
    this.setData({
      showBind: false,
      binding: false
    })
  },

  onBindCodeInput(e) {
    this.setData({ bindCode: e.detail.value })
  },

  async submitBind() {
    const { bindCode, agentID } = this.data
    if (!bindCode || bindCode.length !== 6) {
      wx.showToast({ title: '请输入6位验证码', icon: 'none' })
      return
    }

    this.setData({ binding: true })
    try {
      await api.bindDeviceToAgent(agentID, { code: bindCode })
      wx.showToast({ title: '绑定成功', icon: 'success' })
      this.closeBind()
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '绑定失败', icon: 'none' })
      this.setData({ binding: false })
    }
  },

  async removeDevice(e) {
    const deviceID = e.currentTarget.dataset.id
    if (!deviceID) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '移除设备',
        content: '确认将设备从当前智能体移除？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    try {
      await api.removeAgentDevice(this.data.agentID, deviceID)
      wx.showToast({ title: '移除成功', icon: 'success' })
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '移除失败', icon: 'none' })
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
      this.loadData()
    }
  },

  noop() {}
})
