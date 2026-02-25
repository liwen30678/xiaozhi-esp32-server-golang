const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const DEFAULT_PROMPT = '我是一个叫{{assistant_name}}的台湾女孩，说话机车，声音好听，习惯简短表达，爱用网络梗。'

const pickList = (res) => res?.data || res?.items || res?.list || []

const formatDateTime = (value) => {
  if (!value) return '未知'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '未知'

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
    showCreate: false,
    creating: false,
    form: {
      name: '',
      custom_prompt: DEFAULT_PROMPT,
      memory_mode: 'short'
    },
    memoryModes: ['无记忆', '短记忆', '长记忆'],
    memoryValues: ['none', 'short', 'long'],
    memoryIndex: 1
  },

  onShow() {
    if (!ensureLogin()) return
    this.loadAgents()
  },

  onPullDownRefresh() {
    this.loadAgents().finally(() => wx.stopPullDownRefresh())
  },

  async loadAgents() {
    this.setData({ loading: true })
    try {
      const res = await api.getAgents()
      const list = pickList(res).map((item) => ({
        ...item,
        llm_name: item?.llm_config?.name || '未设置',
        tts_name: item?.tts_config?.name || '未设置',
        updated_text: formatDateTime(item.updated_at)
      }))
      this.setData({ list })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载智能体失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  openCreate() {
    this.setData({
      showCreate: true,
      memoryIndex: 1,
      form: {
        name: '',
        custom_prompt: DEFAULT_PROMPT,
        memory_mode: 'short'
      }
    })
  },

  closeCreate() {
    this.setData({
      showCreate: false,
      creating: false
    })
  },

  onNameInput(e) {
    this.setData({
      'form.name': e.detail.value
    })
  },

  onPromptInput(e) {
    this.setData({
      'form.custom_prompt': e.detail.value
    })
  },

  onMemoryChange(e) {
    const index = Number(e.detail.value)
    this.setData({
      memoryIndex: index,
      'form.memory_mode': this.data.memoryValues[index] || 'short'
    })
  },

  async submitCreate() {
    const { form } = this.data
    if (!form.name) {
      wx.showToast({ title: '请输入智能体名称', icon: 'none' })
      return
    }

    this.setData({ creating: true })
    try {
      const [llmRes, ttsRes] = await Promise.all([
        api.getLLMConfigs(),
        api.getTTSConfigs()
      ])

      const llmDefault = pickList(llmRes).find((item) => item.is_default)
      const ttsDefault = pickList(ttsRes).find((item) => item.is_default)

      const payload = {
        name: form.name,
        custom_prompt: form.custom_prompt,
        memory_mode: form.memory_mode
      }
      if (llmDefault?.config_id) payload.llm_config_id = llmDefault.config_id
      if (ttsDefault?.config_id) payload.tts_config_id = ttsDefault.config_id

      await api.createAgent(payload)
      wx.showToast({ title: '创建成功', icon: 'success' })
      this.closeCreate()
      this.loadAgents()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '创建失败', icon: 'none' })
      this.setData({ creating: false })
    }
  },

  gotoEdit(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return
    wx.navigateTo({ url: `/pages/agent-edit/index?id=${id}` })
  },

  gotoDevices(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return
    wx.navigateTo({ url: `/pages/agent-devices/index?id=${id}` })
  },

  gotoHistory(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return
    wx.navigateTo({ url: `/pages/agent-history/index?id=${id}` })
  },

  async removeAgent(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '确认删除',
        content: '删除后无法恢复，是否继续？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })

    if (!confirmed) return

    try {
      await api.deleteAgent(id)
      wx.showToast({ title: '删除成功', icon: 'success' })
      this.loadAgents()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
    }
  },

  noop() {},

  onHide() {
    this.setData({ showCreate: false })
  }
})
