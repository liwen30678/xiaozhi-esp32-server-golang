const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const formatDateTime = (value) => {
  if (!value) return '未知'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '未知'
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  const h = String(d.getHours()).padStart(2, '0')
  const min = String(d.getMinutes()).padStart(2, '0')
  const sec = String(d.getSeconds()).padStart(2, '0')
  return `${y}-${m}-${day} ${h}:${min}:${sec}`
}

Page({
  data: {
    agentID: '',
    loading: false,
    loadingMore: false,
    deleting: false,
    list: [],
    total: 0,
    page: 1,
    pageSize: 20,
    roleOptions: ['全部', '用户', '助手'],
    roleValues: ['', 'user', 'assistant'],
    roleIndex: 0,
    hasMore: false
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
    this.loadMessages(true)
  },

  onPullDownRefresh() {
    this.loadMessages(true).finally(() => wx.stopPullDownRefresh())
  },

  async loadMessages(reset = false) {
    const page = reset ? 1 : this.data.page + 1
    const { roleValues, roleIndex, pageSize, agentID } = this.data
    const params = {
      page,
      page_size: pageSize
    }
    const role = roleValues[roleIndex]
    if (role) params.role = role

    this.setData(reset ? { loading: true } : { loadingMore: true })
    try {
      const res = await api.getAgentHistory(agentID, params)
      const rows = (res?.data || []).map((item) => ({
        ...item,
        created_text: formatDateTime(item.created_at),
        role_text: item.role === 'assistant' ? '助手' : item.role === 'user' ? '用户' : item.role
      }))

      const list = reset ? rows : this.data.list.concat(rows)
      const total = Number(res?.total || 0)
      const hasMore = list.length < total

      this.setData({
        list,
        total,
        page,
        hasMore
      })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载历史失败', icon: 'none' })
    } finally {
      this.setData({
        loading: false,
        loadingMore: false
      })
    }
  },

  onRoleChange(e) {
    const roleIndex = Number(e.detail.value)
    this.setData({ roleIndex })
    this.loadMessages(true)
  },

  loadMore() {
    if (!this.data.hasMore || this.data.loadingMore) return
    this.loadMessages(false)
  },

  async removeMessage(e) {
    const id = e.currentTarget.dataset.id
    if (!id || this.data.deleting) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '删除消息',
        content: '删除后无法恢复，是否继续？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    this.setData({ deleting: true })
    try {
      await api.deleteHistoryMessage(id)
      wx.showToast({ title: '删除成功', icon: 'success' })
      this.loadMessages(true)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
      this.setData({ deleting: false })
    } finally {
      this.setData({ deleting: false })
    }
  }
})
