const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const toNullableString = (value) => {
  if (value === undefined || value === null || value === '') return null
  return String(value)
}

const buildKnowledgeList = (items, selectedIDs) => {
  const idSet = new Set((selectedIDs || []).map((id) => Number(id)))
  return items.map((item) => ({
    ...item,
    checked: idSet.has(Number(item.id))
  }))
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
    agentID: '',
    loading: false,
    saving: false,
    voiceLoading: false,
    llmConfigs: [],
    ttsConfigs: [],
    knowledgeBases: [],
    llmNames: [],
    ttsNames: [],
    voiceOptions: [],
    voiceOptionNames: [],
    llmIndex: -1,
    ttsIndex: -1,
    voiceIndex: -1,
    asrModes: ['正常', '耐心', '快速'],
    asrValues: ['normal', 'patient', 'fast'],
    asrIndex: 0,
    memoryModes: ['无记忆', '短记忆', '长记忆'],
    memoryValues: ['none', 'short', 'long'],
    memoryIndex: 1,
    form: {
      name: '',
      custom_prompt: '',
      llm_config_id: '',
      tts_config_id: '',
      voice: '',
      asr_speed: 'normal',
      memory_mode: 'short',
      knowledge_base_ids: []
    }
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

  async loadData() {
    this.setData({ loading: true })
    try {
      const { agentID } = this.data
      const [agentRes, llmRes, ttsRes, kbRes] = await Promise.all([
        api.getAgent(agentID),
        api.getLLMConfigs(),
        api.getTTSConfigs(),
        api.getKnowledgeBases()
      ])

      const agent = agentRes?.data || {}
      const llmConfigs = pickList(llmRes)
      const ttsConfigs = pickList(ttsRes)
      const knowledgeBaseRaw = pickList(kbRes)
      const selectedKnowledgeIDs = agent?.knowledge_base_ids || []

      const llmConfigID = agent?.llm_config_id || ''
      const ttsConfigID = agent?.tts_config_id || ''
      const asrSpeed = agent?.asr_speed || 'normal'
      const memoryMode = agent?.memory_mode || 'short'
      const voice = agent?.voice || ''

      const llmIndex = llmConfigs.findIndex((item) => item.config_id === llmConfigID)
      const ttsIndex = ttsConfigs.findIndex((item) => item.config_id === ttsConfigID)
      const asrIndex = Math.max(0, this.data.asrValues.findIndex((v) => v === asrSpeed))
      const memoryIndex = Math.max(0, this.data.memoryValues.findIndex((v) => v === memoryMode))

      this.setData({
        llmConfigs,
        ttsConfigs,
        llmNames: llmConfigs.map((item) => item.name),
        ttsNames: ttsConfigs.map((item) => item.name),
        llmIndex,
        ttsIndex,
        asrIndex,
        memoryIndex,
        knowledgeBases: buildKnowledgeList(knowledgeBaseRaw, selectedKnowledgeIDs),
        form: {
          name: agent?.name || '',
          custom_prompt: agent?.custom_prompt || '',
          llm_config_id: llmConfigID,
          tts_config_id: ttsConfigID,
          voice,
          asr_speed: asrSpeed,
          memory_mode: memoryMode,
          knowledge_base_ids: selectedKnowledgeIDs
        }
      })

      await this.loadVoiceOptions(ttsConfigID, voice)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  onNameInput(e) {
    this.setData({ 'form.name': e.detail.value })
  },

  onPromptInput(e) {
    this.setData({ 'form.custom_prompt': e.detail.value })
  },

  onVoiceInput(e) {
    const voice = e.detail.value
    const voiceIndex = this.data.voiceOptions.findIndex((item) => item.value === voice)
    this.setData({
      'form.voice': voice,
      voiceIndex
    })
  },

  onLLMChange(e) {
    const llmIndex = Number(e.detail.value)
    const config = this.data.llmConfigs[llmIndex]
    this.setData({
      llmIndex,
      'form.llm_config_id': config?.config_id || ''
    })
  },

  async onTTSChange(e) {
    const ttsIndex = Number(e.detail.value)
    const config = this.data.ttsConfigs[ttsIndex]
    this.setData({
      ttsIndex,
      'form.tts_config_id': config?.config_id || ''
    })
    await this.loadVoiceOptions(config?.config_id || '', this.data.form.voice || '')
  },

  onVoiceChange(e) {
    const voiceIndex = Number(e.detail.value)
    const option = this.data.voiceOptions[voiceIndex]
    this.setData({
      voiceIndex,
      'form.voice': option?.value || ''
    })
  },

  onAsrChange(e) {
    const asrIndex = Number(e.detail.value)
    this.setData({
      asrIndex,
      'form.asr_speed': this.data.asrValues[asrIndex] || 'normal'
    })
  },

  onMemoryChange(e) {
    const memoryIndex = Number(e.detail.value)
    this.setData({
      memoryIndex,
      'form.memory_mode': this.data.memoryValues[memoryIndex] || 'short'
    })
  },

  onKnowledgeChange(e) {
    const selectedIDs = (e.detail.value || []).map((id) => Number(id))
    this.setData({
      'form.knowledge_base_ids': selectedIDs,
      knowledgeBases: buildKnowledgeList(this.data.knowledgeBases, selectedIDs)
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

    const config = this.data.ttsConfigs.find((item) => item.config_id === selectedConfigID)
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
      const voiceOptionNames = voices.map((item) => item.label)
      let voiceIndex = voices.findIndex((item) => item.value === String(currentVoice || ''))

      const nextData = {
        voiceOptions: voices,
        voiceOptionNames,
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

  async submitSave() {
    const { form, agentID } = this.data
    if (!form.name) {
      wx.showToast({ title: '请输入名称', icon: 'none' })
      return
    }

    const payload = {
      name: form.name,
      custom_prompt: form.custom_prompt,
      llm_config_id: toNullableString(form.llm_config_id),
      tts_config_id: toNullableString(form.tts_config_id),
      voice: toNullableString(form.voice),
      asr_speed: form.asr_speed,
      memory_mode: form.memory_mode,
      knowledge_base_ids: form.knowledge_base_ids || []
    }

    this.setData({ saving: true })
    try {
      await api.updateAgent(agentID, payload)
      wx.showToast({ title: '保存成功', icon: 'success' })
      setTimeout(() => wx.navigateBack(), 300)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '保存失败', icon: 'none' })
      this.setData({ saving: false })
    }
  }
})
