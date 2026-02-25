const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const clipText = (text, size = 90) => {
  const value = String(text || '')
  if (value.length <= size) return value
  return `${value.slice(0, size)}...`
}

Page({
  data: {
    loading: false,
    list: [],
    showForm: false,
    saving: false,
    isEdit: false,
    currentID: null,
    statusOptions: ['启用', '禁用'],
    statusValues: ['active', 'inactive'],
    statusIndex: 0,
    form: {
      name: '',
      description: '',
      content: '',
      status: 'active',
      inherit_global_threshold: true,
      retrieval_threshold: ''
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
      const res = await api.getKnowledgeBases()
      const list = pickList(res).map((item) => ({
        ...item,
        description_preview: clipText(item.description || item.content, 90),
        status_text: item.status === 'inactive' ? '禁用' : '启用',
        sync_status_text: item.sync_status || 'pending',
        retrieval_text:
          item.retrieval_threshold === null || item.retrieval_threshold === undefined
            ? '继承全局'
            : Number(item.retrieval_threshold).toFixed(2)
      }))
      this.setData({ list })
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
      currentID: null,
      statusIndex: 0,
      form: {
        name: '',
        description: '',
        content: '',
        status: 'active',
        inherit_global_threshold: true,
        retrieval_threshold: ''
      }
    })
  },

  openEdit(e) {
    const id = e.currentTarget.dataset.id
    const item = this.data.list.find((row) => Number(row.id) === Number(id))
    if (!item) return

    const hasThreshold = item.retrieval_threshold !== null && item.retrieval_threshold !== undefined
    const statusIndex = item.status === 'inactive' ? 1 : 0

    this.setData({
      showForm: true,
      isEdit: true,
      currentID: item.id,
      statusIndex,
      form: {
        name: item.name || '',
        description: item.description || '',
        content: item.content || '',
        status: item.status || 'active',
        inherit_global_threshold: !hasThreshold,
        retrieval_threshold: hasThreshold ? String(item.retrieval_threshold) : ''
      }
    })
  },

  closeForm() {
    this.setData({
      showForm: false,
      saving: false
    })
  },

  onNameInput(e) {
    this.setData({ 'form.name': e.detail.value })
  },

  onDescriptionInput(e) {
    this.setData({ 'form.description': e.detail.value })
  },

  onContentInput(e) {
    this.setData({ 'form.content': e.detail.value })
  },

  onStatusChange(e) {
    const statusIndex = Number(e.detail.value)
    this.setData({
      statusIndex,
      'form.status': this.data.statusValues[statusIndex] || 'active'
    })
  },

  onInheritThresholdChange(e) {
    this.setData({
      'form.inherit_global_threshold': !!e.detail.value
    })
  },

  onThresholdInput(e) {
    this.setData({ 'form.retrieval_threshold': e.detail.value })
  },

  async submitForm() {
    const { form, isEdit, currentID } = this.data
    if (!form.name) {
      wx.showToast({ title: '名称必填', icon: 'none' })
      return
    }

    const payload = {
      name: form.name,
      description: form.description,
      content: form.content,
      status: form.status,
      inherit_global_threshold: !!form.inherit_global_threshold
    }

    if (!payload.inherit_global_threshold) {
      const value = Number(form.retrieval_threshold)
      if (Number.isNaN(value) || value < 0 || value > 1) {
        wx.showToast({ title: '阈值需在0~1', icon: 'none' })
        return
      }
      payload.retrieval_threshold = value
    }

    this.setData({ saving: true })
    try {
      if (isEdit) {
        await api.updateKnowledgeBase(currentID, payload)
      } else {
        await api.createKnowledgeBase(payload)
      }
      wx.showToast({ title: '保存成功', icon: 'success' })
      this.closeForm()
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '保存失败', icon: 'none' })
      this.setData({ saving: false })
    }
  },

  async syncItem(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return
    try {
      await api.syncKnowledgeBase(id)
      wx.showToast({ title: '已提交同步', icon: 'success' })
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '同步失败', icon: 'none' })
    }
  },

  async removeItem(e) {
    const id = e.currentTarget.dataset.id
    if (!id) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '删除知识库',
        content: '确认删除该知识库？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    try {
      await api.deleteKnowledgeBase(id)
      wx.showToast({ title: '删除成功', icon: 'success' })
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
    }
  },

  noop() {}
})
