const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const clipText = (text, size = 56) => {
  const value = String(text || '')
  if (value.length <= size) return value
  return `${value.slice(0, size)}...`
}

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

const formatFileSize = (bytes) => {
  const num = Number(bytes || 0)
  if (!Number.isFinite(num) || num <= 0) return '-'
  if (num < 1024) return `${num}B`
  if (num < 1024 * 1024) return `${(num / 1024).toFixed(1)}KB`
  return `${(num / 1024 / 1024).toFixed(2)}MB`
}

const toNullableString = (value) => {
  if (value === undefined || value === null || value === '') return null
  return String(value)
}

const normalizeStatus = (item) => {
  const status = String(item?.status || '').trim().toLowerCase()
  const taskStatus = String(item?.task_status || '').trim().toLowerCase()
  if (status === 'failed' || taskStatus === 'failed') return 'failed'
  if (status === 'active' || taskStatus === 'succeeded') return 'active'
  if (taskStatus === 'queued' || taskStatus === 'processing') return taskStatus
  if (status === 'queued' || status === 'processing') return status
  return status || taskStatus || 'unknown'
}

const normalizeVoiceOptions = (items, currentVoice = '') => {
  const result = []
  const seen = new Set()

  ;(items || []).forEach((item) => {
    let value = ''
    let label = ''
    if (typeof item === 'string') {
      value = item.trim()
      label = value
    } else if (item && typeof item === 'object') {
      value = String(item.value || '').trim()
      label = String(item.label || item.value || '').trim()
    }

    if (!value || seen.has(value)) return
    seen.add(value)
    result.push({ value, label: label || value })
  })

  const safeCurrent = String(currentVoice || '').trim()
  if (safeCurrent && !seen.has(safeCurrent)) {
    result.unshift({
      value: safeCurrent,
      label: `当前值：${safeCurrent}`
    })
  }

  return result
}

Page({
  data: {
    loading: false,
    list: [],

    filterAgentOptions: [{ id: '', name: '全部智能体' }],
    filterAgentNames: ['全部智能体'],
    filterAgentIndex: 0,
    filterAgentID: '',

    agents: [],
    agentNames: [],

    ttsOptions: [{ config_id: '', name: '不设置', provider: '' }],
    ttsNames: ['不设置'],

    cloneVoices: [],

    showForm: false,
    isEdit: false,
    saving: false,
    currentGroupID: null,

    formAgentIndex: -1,
    formTTSIndex: 0,
    voiceLoading: false,
    voiceOptions: [],
    voiceOptionNames: [],
    voiceIndex: -1,

    form: {
      agent_id: '',
      name: '',
      prompt: '',
      description: '',
      tts_config_id: '',
      voice: ''
    },

    showSamples: false,
    selectedGroup: null,
    samplesLoading: false,
    uploadingSample: false,
    deletingSampleID: '',
    playingSampleID: '',
    verifyingGroupID: '',
    samples: [],
    verifyResult: null
  },

  onShow() {
    if (!ensureLogin()) return
    this.ensureAudioContext()
    this.loadData()
  },

  onHide() {
    this.stopAudio()
  },

  onUnload() {
    this.stopAudio()
    if (this.audioContext) {
      try {
        this.audioContext.destroy()
      } catch (err) {
        // ignore
      }
      this.audioContext = null
    }
  },

  onPullDownRefresh() {
    this.loadData().finally(() => wx.stopPullDownRefresh())
  },

  ensureAudioContext() {
    if (this.audioContext) return
    const ctx = wx.createInnerAudioContext()
    ctx.obeyMuteSwitch = false
    ctx.onError(() => {
      wx.showToast({ title: '音频播放失败', icon: 'none' })
    })
    this.audioContext = ctx
  },

  stopAudio() {
    if (!this.audioContext) return
    try {
      this.audioContext.stop()
    } catch (err) {
      // ignore
    }
  },

  async loadData() {
    this.setData({ loading: true })
    try {
      await this.loadBaseData()
      await this.loadSpeakerGroups(true)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  async loadBaseData() {
    const [agentsRes, ttsRes, clonesRes] = await Promise.all([
      api.getAgents(),
      api.getTTSConfigs(),
      api.getVoiceClones()
    ])

    const agents = pickList(agentsRes)
    const filterAgentOptions = [{ id: '', name: '全部智能体' }, ...agents.map((item) => ({ id: item.id, name: item.name }))]
    const filterAgentNames = filterAgentOptions.map((item) => item.name)

    const ttsOptions = [{ config_id: '', name: '不设置', provider: '' }, ...pickList(ttsRes)]
    const ttsNames = ttsOptions.map((item) => item.name)

    const cloneVoices = pickList(clonesRes)
      .filter((item) => normalizeStatus(item) === 'active')
      .map((item) => ({
        id: item.id,
        name: item.name || item.provider_voice_id,
        tts_config_id: item.tts_config_id,
        provider_voice_id: item.provider_voice_id,
        provider: item.provider
      }))

    this.setData({
      agents,
      agentNames: agents.map((item) => item.name),
      filterAgentOptions,
      filterAgentNames,
      ttsOptions,
      ttsNames,
      cloneVoices
    })
  },

  async loadSpeakerGroups(silent = false) {
    if (!silent) {
      this.setData({ loading: true })
    }
    try {
      const params = {}
      if (this.data.filterAgentID) {
        params.agent_id = this.data.filterAgentID
      }
      const res = await api.getSpeakerGroups(params)
      const list = pickList(res).map((item) => ({
        ...item,
        prompt_preview: clipText(item?.prompt || '', 66),
        created_text: formatDateTime(item?.created_at),
        tts_text: item?.tts_config_id || '未设置',
        voice_text: item?.voice || '未设置'
      }))
      this.setData({ list })
    } catch (err) {
      if (silent) {
        throw err
      }
      if (!silent) {
        wx.showToast({ title: err?.data?.error || '加载声纹组失败', icon: 'none' })
      }
    } finally {
      if (!silent) {
        this.setData({ loading: false })
      }
    }
  },

  onFilterAgentChange(e) {
    const filterAgentIndex = Number(e.detail.value)
    const selected = this.data.filterAgentOptions[filterAgentIndex]
    this.setData({
      filterAgentIndex,
      filterAgentID: selected?.id || ''
    })
    this.loadSpeakerGroups()
  },

  openCreate() {
    if (!this.data.agents.length) {
      wx.showToast({ title: '请先创建智能体', icon: 'none' })
      return
    }
    this.setData({
      showForm: true,
      isEdit: false,
      saving: false,
      currentGroupID: null,
      formAgentIndex: 0,
      formTTSIndex: 0,
      voiceOptions: [],
      voiceOptionNames: [],
      voiceIndex: -1,
      form: {
        agent_id: this.data.agents[0].id,
        name: '',
        prompt: '',
        description: '',
        tts_config_id: '',
        voice: ''
      }
    })
  },

  async openEdit(e) {
    const groupID = e.currentTarget.dataset.id
    const group = this.data.list.find((item) => Number(item.id) === Number(groupID))
    if (!group) return

    const formAgentIndex = Math.max(0, this.data.agents.findIndex((item) => Number(item.id) === Number(group.agent_id)))
    const formTTSIndex = Math.max(0, this.data.ttsOptions.findIndex((item) => item.config_id === (group.tts_config_id || '')))
    const voice = group.voice || ''

    this.setData({
      showForm: true,
      isEdit: true,
      saving: false,
      currentGroupID: group.id,
      formAgentIndex,
      formTTSIndex,
      form: {
        agent_id: group.agent_id,
        name: group.name || '',
        prompt: group.prompt || '',
        description: group.description || '',
        tts_config_id: group.tts_config_id || '',
        voice
      }
    })
    await this.loadVoiceOptions(group.tts_config_id || '', voice)
  },

  closeForm() {
    this.setData({
      showForm: false,
      saving: false,
      voiceLoading: false,
      voiceOptions: [],
      voiceOptionNames: [],
      voiceIndex: -1
    })
  },

  onFormAgentChange(e) {
    const formAgentIndex = Number(e.detail.value)
    const selected = this.data.agents[formAgentIndex]
    this.setData({
      formAgentIndex,
      'form.agent_id': selected?.id || ''
    })
  },

  onNameInput(e) {
    this.setData({ 'form.name': e.detail.value })
  },

  onPromptInput(e) {
    this.setData({ 'form.prompt': e.detail.value })
  },

  onDescriptionInput(e) {
    this.setData({ 'form.description': e.detail.value })
  },

  onTTSChange(e) {
    const formTTSIndex = Number(e.detail.value)
    const selected = this.data.ttsOptions[formTTSIndex]
    this.setData({
      formTTSIndex,
      'form.tts_config_id': selected?.config_id || '',
      'form.voice': '',
      voiceIndex: -1
    })
    this.loadVoiceOptions(selected?.config_id || '', '')
  },

  onVoiceChange(e) {
    const voiceIndex = Number(e.detail.value)
    const selected = this.data.voiceOptions[voiceIndex]
    this.setData({
      voiceIndex,
      'form.voice': selected?.value || ''
    })
  },

  onVoiceInput(e) {
    const voice = e.detail.value
    const voiceIndex = this.data.voiceOptions.findIndex((item) => item.value === voice)
    this.setData({
      'form.voice': voice,
      voiceIndex
    })
  },

  async loadVoiceOptions(ttsConfigID, currentVoice = '') {
    const selectedConfigID = String(ttsConfigID || '')
    if (!selectedConfigID) {
      this.setData({
        voiceOptions: [],
        voiceOptionNames: [],
        voiceIndex: -1
      })
      return
    }

    const config = this.data.ttsOptions.find((item) => item.config_id === selectedConfigID)
    const provider = String(config?.provider || '')
    if (!provider) {
      this.setData({
        voiceOptions: [],
        voiceOptionNames: [],
        voiceIndex: -1
      })
      return
    }

    this.setData({ voiceLoading: true })
    try {
      const res = await api.getVoiceOptions({
        provider,
        config_id: selectedConfigID
      })
      const voices = normalizeVoiceOptions(pickList(res), currentVoice)
      let voiceIndex = voices.findIndex((item) => item.value === String(currentVoice || ''))
      const nextData = {
        voiceOptions: voices,
        voiceOptionNames: voices.map((item) => item.label),
        voiceIndex
      }
      if (!currentVoice && voices.length) {
        voiceIndex = 0
        nextData.voiceIndex = 0
        nextData['form.voice'] = voices[0].value
      }
      this.setData(nextData)
    } catch (err) {
      this.setData({
        voiceOptions: [],
        voiceOptionNames: [],
        voiceIndex: -1
      })
      wx.showToast({ title: err?.data?.error || '加载音色失败', icon: 'none' })
    } finally {
      this.setData({ voiceLoading: false })
    }
  },

  async applyCloneVoice(e) {
    const cloneID = e.currentTarget.dataset.id
    const clone = this.data.cloneVoices.find((item) => Number(item.id) === Number(cloneID))
    if (!clone) return

    const formTTSIndex = Math.max(0, this.data.ttsOptions.findIndex((item) => item.config_id === (clone.tts_config_id || '')))
    this.setData({
      formTTSIndex,
      'form.tts_config_id': clone.tts_config_id || '',
      'form.voice': clone.provider_voice_id || ''
    })
    await this.loadVoiceOptions(clone.tts_config_id || '', clone.provider_voice_id || '')
  },

  async submitForm() {
    const { isEdit, currentGroupID, form } = this.data
    if (!form.agent_id) {
      wx.showToast({ title: '请选择智能体', icon: 'none' })
      return
    }
    if (!String(form.name || '').trim()) {
      wx.showToast({ title: '请输入声纹名称', icon: 'none' })
      return
    }

    const payload = {
      agent_id: Number(form.agent_id),
      name: String(form.name || '').trim(),
      prompt: form.prompt || '',
      description: form.description || '',
      tts_config_id: toNullableString(form.tts_config_id),
      voice: toNullableString(form.voice)
    }

    this.setData({ saving: true })
    try {
      if (isEdit) {
        await api.updateSpeakerGroup(currentGroupID, payload)
      } else {
        await api.createSpeakerGroup(payload)
      }
      wx.showToast({ title: '保存成功', icon: 'success' })
      this.closeForm()
      this.loadSpeakerGroups()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '保存失败', icon: 'none' })
      this.setData({ saving: false })
    }
  },

  async deleteGroup(e) {
    const groupID = e.currentTarget.dataset.id
    const name = e.currentTarget.dataset.name || '该声纹组'
    if (!groupID) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '删除确认',
        content: `确定删除“${name}”吗？该组下样本会一并删除。`,
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    try {
      await api.deleteSpeakerGroup(groupID)
      wx.showToast({ title: '删除成功', icon: 'success' })
      if (this.data.selectedGroup && Number(this.data.selectedGroup.id) === Number(groupID)) {
        this.closeSamples()
      }
      this.loadSpeakerGroups()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
    }
  },

  openSamples(e) {
    const groupID = e.currentTarget.dataset.id
    const group = this.data.list.find((item) => Number(item.id) === Number(groupID))
    if (!group) return
    this.setData({
      showSamples: true,
      selectedGroup: group,
      verifyResult: null
    })
    this.loadSamples(group.id)
  },

  closeSamples() {
    this.setData({
      showSamples: false,
      selectedGroup: null,
      samples: [],
      verifyResult: null,
      samplesLoading: false
    })
  },

  async loadSamples(groupID) {
    this.setData({ samplesLoading: true })
    try {
      const res = await api.getSpeakerSamples(groupID)
      const samples = pickList(res).map((item) => ({
        ...item,
        file_size_text: formatFileSize(item.file_size),
        created_text: formatDateTime(item.created_at)
      }))
      this.setData({ samples })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载样本失败', icon: 'none' })
    } finally {
      this.setData({ samplesLoading: false })
    }
  },

  async chooseWavFile() {
    const res = await new Promise((resolve, reject) => {
      wx.chooseMessageFile({
        count: 1,
        type: 'file',
        extension: ['wav'],
        success: resolve,
        fail: reject
      })
    })
    return res?.tempFiles?.[0] || null
  },

  async addSample() {
    const groupID = this.data.selectedGroup?.id
    if (!groupID) return
    try {
      const file = await this.chooseWavFile()
      if (!file?.path) return
      this.setData({ uploadingSample: true })
      await api.addSpeakerSample(groupID, file.path)
      wx.showToast({ title: '样本上传成功', icon: 'success' })
      await Promise.all([
        this.loadSamples(groupID),
        this.loadSpeakerGroups(true)
      ])
    } catch (err) {
      if (!String(err?.errMsg || '').includes('cancel')) {
        wx.showToast({ title: err?.data?.error || '上传失败', icon: 'none' })
      }
    } finally {
      this.setData({ uploadingSample: false })
    }
  },

  async deleteSample(e) {
    const sampleID = e.currentTarget.dataset.id
    const groupID = this.data.selectedGroup?.id
    if (!sampleID || !groupID) return

    const confirmed = await new Promise((resolve) => {
      wx.showModal({
        title: '删除样本',
        content: '确认删除该样本吗？',
        success: (res) => resolve(!!res.confirm),
        fail: () => resolve(false)
      })
    })
    if (!confirmed) return

    this.setData({ deletingSampleID: String(sampleID) })
    try {
      await api.deleteSpeakerSample(groupID, sampleID)
      wx.showToast({ title: '删除成功', icon: 'success' })
      await Promise.all([
        this.loadSamples(groupID),
        this.loadSpeakerGroups(true)
      ])
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '删除失败', icon: 'none' })
    } finally {
      this.setData({ deletingSampleID: '' })
    }
  },

  playTempFile(tempFilePath) {
    if (!tempFilePath) return
    this.ensureAudioContext()
    try {
      this.audioContext.stop()
    } catch (err) {
      // ignore
    }
    this.audioContext.src = tempFilePath
    this.audioContext.play()
  },

  async playSample(e) {
    const sampleID = e.currentTarget.dataset.id
    const groupID = this.data.selectedGroup?.id
    if (!sampleID || !groupID) return
    this.setData({ playingSampleID: String(sampleID) })
    try {
      const fileRes = await api.getSpeakerSampleFile(groupID, sampleID)
      this.playTempFile(fileRes?.tempFilePath)
      wx.showToast({ title: '开始播放样本', icon: 'none' })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '播放失败', icon: 'none' })
    } finally {
      this.setData({ playingSampleID: '' })
    }
  },

  async quickVerify(e) {
    const groupID = e.currentTarget.dataset.id
    const group = this.data.list.find((item) => Number(item.id) === Number(groupID))
    if (!group) return
    this.verifyGroup(group)
  },

  async verifyInSamples() {
    if (!this.data.selectedGroup) return
    this.verifyGroup(this.data.selectedGroup)
  },

  async verifyGroup(group) {
    if (!group?.id) return
    try {
      const file = await this.chooseWavFile()
      if (!file?.path) return
      this.setData({ verifyingGroupID: String(group.id) })
      const res = await api.verifySpeakerGroup(group.id, file.path)
      const result = res?.data || {}
      const verified = !!result.verified
      const confidence = Number(result.confidence || 0)
      const threshold = Number(result.threshold || 0)
      const message = result.message || (verified ? '验证通过' : '验证未通过')

      this.setData({
        verifyResult: {
          verified,
          confidence,
          confidence_text: (confidence * 100).toFixed(1),
          threshold,
          threshold_text: (threshold * 100).toFixed(1),
          message
        }
      })

      wx.showModal({
        title: verified ? '验证通过' : '验证未通过',
        content: `${message}\n置信度：${(confidence * 100).toFixed(1)}%`,
        showCancel: false
      })
    } catch (err) {
      if (!String(err?.errMsg || '').includes('cancel')) {
        wx.showToast({ title: err?.data?.error || '验证失败', icon: 'none' })
      }
    } finally {
      this.setData({ verifyingGroupID: '' })
    }
  },

  noop() {}
})
