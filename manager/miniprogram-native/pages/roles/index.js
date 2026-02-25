const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const clipText = (text, size = 80) => {
  const value = String(text || '')
  if (value.length <= size) return value
  return `${value.slice(0, size)}...`
}

Page({
  data: {
    loading: false,
    list: [],
    llmOptions: [{ config_id: '', name: '不设置' }],
    ttsOptions: [{ config_id: '', name: '不设置' }],
    llmNames: ['不设置'],
    ttsNames: ['不设置'],
    showForm: false,
    saving: false,
    isEdit: false,
    currentRoleID: null,
    llmIndex: 0,
    ttsIndex: 0,
    statusOptions: ['启用', '禁用'],
    statusValues: ['active', 'inactive'],
    statusIndex: 0,
    form: {
      name: '',
      description: '',
      prompt: '',
      llm_config_id: '',
      tts_config_id: '',
      voice: '',
      status: 'active'
    }
  },

  onShow() {
    if (!ensureLogin()) return
    this.loadData()
  },

  onPullDownRefresh() {
    this.loadData().finally(() => wx.stopPullDownRefresh())
  },

  async loadData() {
    this.setData({ loading: true })
    try {
      const [rolesRes, llmRes, ttsRes] = await Promise.all([
        api.getRoles(),
        api.getLLMConfigs(),
        api.getTTSConfigs()
      ])

      const globalRoles = rolesRes?.data?.global_roles || []
      const userRoles = rolesRes?.data?.user_roles || []
      const list = [...userRoles, ...globalRoles].map((item) => ({
        ...item,
        role_type_text: item.role_type === 'global' ? '全局' : '我的',
        status_text: item.status === 'inactive' ? '禁用' : '启用',
        prompt_preview: clipText(item.prompt, 90),
        editable: item.role_type === 'user'
      }))

      const llmOptions = [{ config_id: '', name: '不设置' }, ...pickList(llmRes)]
      const ttsOptions = [{ config_id: '', name: '不设置' }, ...pickList(ttsRes)]

      this.setData({
        list,
        llmOptions,
        ttsOptions,
        llmNames: llmOptions.map((item) => item.name),
        ttsNames: ttsOptions.map((item) => item.name)
      })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  openCreate() {
    this.setData({
      showForm: true,
      isEdit: false,
      currentRoleID: null,
      llmIndex: 0,
      ttsIndex: 0,
      statusIndex: 0,
      form: {
        name: '',
        description: '',
        prompt: '',
        llm_config_id: '',
        tts_config_id: '',
        voice: '',
        status: 'active'
      }
    })
  },

  closeForm() {
    this.setData({
      showForm: false,
      saving: false
    })
  },

  openEdit(e) {
    const roleID = e.currentTarget.dataset.id
    const role = this.data.list.find((item) => Number(item.id) === Number(roleID))
    if (!role || !role.editable) return

    const llmIndex = Math.max(0, this.data.llmOptions.findIndex((item) => item.config_id === (role.llm_config_id || '')))
    const ttsIndex = Math.max(0, this.data.ttsOptions.findIndex((item) => item.config_id === (role.tts_config_id || '')))
    const statusIndex = role.status === 'inactive' ? 1 : 0

    this.setData({
      showForm: true,
      isEdit: true,
      currentRoleID: role.id,
      llmIndex,
      ttsIndex,
      statusIndex,
      form: {
        name: role.name || '',
        description: role.description || '',
        prompt: role.prompt || '',
        llm_config_id: role.llm_config_id || '',
        tts_config_id: role.tts_config_id || '',
        voice: role.voice || '',
        status: role.status || 'active'
      }
    })
  },

  onNameInput(e) {
    this.setData({ 'form.name': e.detail.value })
  },

  onDescriptionInput(e) {
    this.setData({ 'form.description': e.detail.value })
  },

  onPromptInput(e) {
    this.setData({ 'form.prompt': e.detail.value })
  },

  onVoiceInput(e) {
    this.setData({ 'form.voice': e.detail.value })
  },

  onLLMChange(e) {
    const index = Number(e.detail.value)
    const option = this.data.llmOptions[index]
    this.setData({
      llmIndex: index,
      'form.llm_config_id': option?.config_id || ''
    })
  },

  onTTSChange(e) {
    const index = Number(e.detail.value)
    const option = this.data.ttsOptions[index]
    this.setData({
      ttsIndex: index,
      'form.tts_config_id': option?.config_id || ''
    })
  },

  onStatusChange(e) {
    const statusIndex = Number(e.detail.value)
    this.setData({
      statusIndex,
      'form.status': this.data.statusValues[statusIndex] || 'active'
    })
  },

  async submitForm() {
    const { form, isEdit, currentRoleID } = this.data
    if (!form.name || !form.prompt) {
      wx.showToast({ title: '名称和提示词必填', icon: 'none' })
      return
    }

    const payload = {
      name: form.name,
      description: form.description,
      prompt: form.prompt,
      llm_config_id: form.llm_config_id || null,
      tts_config_id: form.tts_config_id || null,
      voice: form.voice || null,
      status: form.status
    }

    this.setData({ saving: true })
    try {
      if (isEdit) {
        await api.updateRole(currentRoleID, payload)
      } else {
        await api.createRole(payload)
      }
      wx.showToast({ title: '保存成功', icon: 'success' })
      this.closeForm()
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '保存失败', icon: 'none' })
      this.setData({ saving: false })
    }
  },

  async toggleRole(e) {
    const roleID = e.currentTarget.dataset.id
    if (!roleID) return
    try {
      await api.toggleRole(roleID)
      wx.showToast({ title: '状态已更新', icon: 'success' })
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '更新失败', icon: 'none' })
    }
  },

  async removeRole(e) {
    const roleID = e.currentTarget.dataset.id
    if (!roleID) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '删除角色',
        content: '确认删除该角色？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    try {
      await api.deleteRole(roleID)
      wx.showToast({ title: '删除成功', icon: 'success' })
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
    }
  },

  noop() {}
})
