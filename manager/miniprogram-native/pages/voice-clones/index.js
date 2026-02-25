const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const CLONE_PROVIDERS = ['minimax', 'cosyvoice', 'aliyun_qwen', 'indextts_vllm']
const PENDING_STATUSES = ['queued', 'processing']

const pickList = (res) => res?.data || res?.items || res?.list || []

const normalizeProvider = (provider) => String(provider || '').trim().toLowerCase()

const parseMetaJSON = (metaJSON) => {
  if (!metaJSON) return {}
  if (typeof metaJSON === 'object') return metaJSON
  try {
    return JSON.parse(metaJSON)
  } catch (err) {
    return {}
  }
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

const statusText = (status) => {
  if (status === 'queued') return '排队中'
  if (status === 'processing') return '处理中'
  if (status === 'active') return '成功'
  if (status === 'failed') return '失败'
  return '未知'
}

const statusClass = (status) => {
  if (status === 'active') return 'status-active'
  if (status === 'failed') return 'status-inactive'
  if (status === 'queued' || status === 'processing') return 'status-pending'
  return 'status-inactive'
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

const toDisplayError = (item, normalizedStatus) => {
  if (normalizedStatus !== 'failed') return '-'
  if (item?.task_last_error) return item.task_last_error
  const meta = parseMetaJSON(item?.meta_json)
  return meta?.last_error || '-'
}

const chargeNotice = (provider, scene = 'create') => {
  const normalized = normalizeProvider(provider)
  if (normalized === 'aliyun_qwen') {
    return scene === 'create'
      ? '计费提醒：千问声音复刻按音色收费，1分钱/个音色。是否继续？'
      : '计费提醒：千问声音复刻按音色收费，1分钱/个音色，是否继续试听？'
  }
  if (normalized === 'minimax') {
    return scene === 'create'
      ? '计费提醒：Minimax 复刻免费，首次试听该复刻音色收费 9.9 元。是否继续？'
      : '计费提醒：Minimax 该复刻音色首次试听收费 9.9 元，是否继续试听？'
  }
  return ''
}

const getAudioExtensionsByProvider = (provider) => {
  const normalized = normalizeProvider(provider)
  if (normalized === 'aliyun_qwen') return ['wav', 'mp3', 'm4a']
  if (normalized === 'indextts_vllm') return ['wav', 'mp3', 'flac', 'm4a', 'ogg']
  return ['wav']
}

const getAudioHintByProvider = (provider) => {
  const normalized = normalizeProvider(provider)
  if (normalized === 'minimax') return '要求：WAV，时长至少 10 秒'
  if (normalized === 'aliyun_qwen') return '要求：WAV/MP3/M4A，建议 10-20 秒，最长 60 秒'
  if (normalized === 'indextts_vllm') return '要求：WAV/MP3/FLAC/M4A/OGG'
  return '要求：WAV；CosyVoice 需填写音频对应文字'
}

const defaultCapability = () => ({
  enabled: true,
  requires_transcript: false,
  min_text_len: 0,
  max_text_len: 0
})

Page({
  data: {
    loading: false,
    list: [],
    polling: false,

    showCreate: false,
    saving: false,

    showRename: false,
    renaming: false,
    renameForm: {
      id: '',
      name: ''
    },

    retryingID: '',
    previewingUploadID: '',
    previewingCloneID: '',
    appendingID: '',

    cloneTTSConfigs: [],
    cloneTTSNames: [],
    configIndex: -1,

    langNames: ['中文 (zh-CN)', '英文 (en-US)'],
    langValues: ['zh-CN', 'en-US'],
    langIndex: 0,

    capability: defaultCapability(),
    audioHint: '',

    form: {
      name: '',
      tts_config_id: '',
      transcript: '',
      transcript_lang: 'zh-CN',
      source_type: 'upload',
      audio_file_path: '',
      audio_file_name: ''
    }
  },

  onShow() {
    if (!ensureLogin()) return
    this.ensureAudioContext()
    this.loadData()
  },

  onHide() {
    this.clearPollingTimer()
    this.stopAudio()
  },

  onUnload() {
    this.clearPollingTimer()
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

  clearPollingTimer() {
    if (!this.pollingTimer) return
    clearTimeout(this.pollingTimer)
    this.pollingTimer = null
  },

  schedulePolling() {
    this.clearPollingTimer()
    if (!this.data.list.some((item) => PENDING_STATUSES.includes(item.normalized_status))) return
    this.pollingTimer = setTimeout(async () => {
      if (this.data.polling) return
      this.setData({ polling: true })
      try {
        await this.loadVoiceClones(true)
      } finally {
        this.setData({ polling: false })
      }
    }, 2500)
  },

  async loadData() {
    this.setData({ loading: true })
    try {
      await Promise.all([
        this.loadCloneConfigs(),
        this.loadVoiceClones(true)
      ])
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  async loadCloneConfigs() {
    const res = await api.getTTSConfigs()
    const configs = pickList(res).filter((item) => CLONE_PROVIDERS.includes(normalizeProvider(item?.provider)))

    let configIndex = this.data.configIndex
    let selectedConfigID = this.data.form.tts_config_id
    if (!configs.length) {
      configIndex = -1
      selectedConfigID = ''
    } else {
      const existing = configs.findIndex((item) => item.config_id === selectedConfigID)
      configIndex = existing >= 0 ? existing : 0
      selectedConfigID = configs[configIndex].config_id
    }

    this.setData({
      cloneTTSConfigs: configs,
      cloneTTSNames: configs.map((item) => item.name),
      configIndex,
      'form.tts_config_id': selectedConfigID
    })

    await this.loadCapabilityByConfigID(selectedConfigID)
  },

  async loadVoiceClones(silent = false) {
    if (!silent) {
      this.setData({ loading: true })
    }
    try {
      const res = await api.getVoiceClones()
      const list = pickList(res).map((item) => {
        const normalized = normalizeStatus(item)
        const provider = normalizeProvider(item?.provider)
        return {
          ...item,
          normalized_status: normalized,
          status_text: statusText(normalized),
          status_class: statusClass(normalized),
          last_error: toDisplayError(item, normalized),
          created_text: formatDateTime(item?.created_at),
          can_retry: normalized === 'failed',
          can_preview: normalized === 'active',
          can_append: normalized === 'active' && provider === 'indextts_vllm'
        }
      })
      this.setData({ list })
      this.schedulePolling()
    } catch (err) {
      if (silent) {
        throw err
      }
      if (!silent) {
        wx.showToast({ title: err?.data?.error || '加载复刻任务失败', icon: 'none' })
      }
    } finally {
      if (!silent) {
        this.setData({ loading: false })
      }
    }
  },

  getCurrentConfig() {
    const { cloneTTSConfigs, form } = this.data
    return cloneTTSConfigs.find((item) => item.config_id === form.tts_config_id) || null
  },

  async loadCapabilityByConfigID(configID) {
    const config = this.data.cloneTTSConfigs.find((item) => item.config_id === configID)
    if (!config) {
      this.setData({
        capability: defaultCapability(),
        audioHint: ''
      })
      return
    }

    const provider = normalizeProvider(config.provider)
    try {
      const res = await api.getVoiceCloneCapabilities(provider)
      this.setData({
        capability: {
          ...defaultCapability(),
          ...(res?.data || {})
        },
        audioHint: getAudioHintByProvider(provider)
      })
    } catch (err) {
      this.setData({
        capability: defaultCapability(),
        audioHint: getAudioHintByProvider(provider)
      })
      wx.showToast({ title: err?.data?.error || '加载能力失败', icon: 'none' })
    }
  },

  openCreate() {
    if (!this.data.cloneTTSConfigs.length) {
      wx.showToast({ title: '暂无可复刻的TTS配置', icon: 'none' })
      return
    }
    const config = this.data.cloneTTSConfigs[0]
    this.setData({
      showCreate: true,
      saving: false,
      configIndex: 0,
      langIndex: 0,
      form: {
        name: '',
        tts_config_id: config.config_id,
        transcript: '',
        transcript_lang: 'zh-CN',
        source_type: 'upload',
        audio_file_path: '',
        audio_file_name: ''
      }
    })
    this.loadCapabilityByConfigID(config.config_id)
  },

  closeCreate() {
    this.setData({
      showCreate: false,
      saving: false
    })
  },

  onCreateNameInput(e) {
    this.setData({ 'form.name': e.detail.value })
  },

  onTranscriptInput(e) {
    this.setData({ 'form.transcript': e.detail.value })
  },

  onConfigChange(e) {
    const index = Number(e.detail.value)
    const config = this.data.cloneTTSConfigs[index]
    this.setData({
      configIndex: index,
      'form.tts_config_id': config?.config_id || '',
      'form.audio_file_path': '',
      'form.audio_file_name': ''
    })
    this.loadCapabilityByConfigID(config?.config_id || '')
  },

  onLangChange(e) {
    const index = Number(e.detail.value)
    this.setData({
      langIndex: index,
      'form.transcript_lang': this.data.langValues[index] || 'zh-CN'
    })
  },

  async chooseAudioFile() {
    const config = this.getCurrentConfig()
    const extensions = getAudioExtensionsByProvider(config?.provider || '')
    try {
      const res = await new Promise((resolve, reject) => {
        wx.chooseMessageFile({
          count: 1,
          type: 'file',
          extension: extensions,
          success: resolve,
          fail: reject
        })
      })
      const file = res?.tempFiles?.[0]
      if (!file?.path) return
      this.setData({
        'form.audio_file_path': file.path,
        'form.audio_file_name': file.name || 'audio'
      })
    } catch (err) {
      if (String(err?.errMsg || '').includes('cancel')) return
      wx.showToast({ title: '选择文件失败', icon: 'none' })
    }
  },

  clearAudioFile() {
    this.setData({
      'form.audio_file_path': '',
      'form.audio_file_name': ''
    })
  },

  async submitCreate() {
    const { form, capability } = this.data
    if (!form.tts_config_id) {
      wx.showToast({ title: '请选择TTS配置', icon: 'none' })
      return
    }
    if (!form.audio_file_path) {
      wx.showToast({ title: '请上传音频文件', icon: 'none' })
      return
    }
    if (capability.requires_transcript && !String(form.transcript || '').trim()) {
      wx.showToast({ title: '请填写音频对应文字', icon: 'none' })
      return
    }

    const config = this.getCurrentConfig()
    const notice = chargeNotice(config?.provider || '', 'create')
    if (notice) {
      const confirmed = await new Promise((resolve) => {
        wx.showModal({
          title: '创建复刻提醒',
          content: notice,
          success: (res) => resolve(!!res.confirm),
          fail: () => resolve(false)
        })
      })
      if (!confirmed) return
    }

    this.setData({ saving: true })
    try {
      await api.createVoiceClone(
        form.audio_file_path,
        {
          name: form.name,
          tts_config_id: form.tts_config_id,
          source_type: form.source_type,
          transcript: form.transcript,
          transcript_lang: form.transcript_lang
        },
        'audio_file'
      )
      wx.showToast({ title: '已提交复刻任务', icon: 'success' })
      this.closeCreate()
      this.loadVoiceClones()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '创建失败', icon: 'none' })
      this.setData({ saving: false })
    }
  },

  async retryClone(e) {
    const cloneID = e.currentTarget.dataset.id
    if (!cloneID) return
    this.setData({ retryingID: String(cloneID) })
    try {
      await api.retryVoiceClone(cloneID)
      wx.showToast({ title: '已提交重试', icon: 'success' })
      await this.loadVoiceClones(true)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '重试失败', icon: 'none' })
    } finally {
      this.setData({ retryingID: '' })
    }
  },

  openRename(e) {
    const cloneID = e.currentTarget.dataset.id
    const name = e.currentTarget.dataset.name || ''
    if (!cloneID) return
    this.setData({
      showRename: true,
      renaming: false,
      renameForm: {
        id: String(cloneID),
        name: String(name)
      }
    })
  },

  closeRename() {
    this.setData({
      showRename: false,
      renaming: false,
      renameForm: { id: '', name: '' }
    })
  },

  onRenameInput(e) {
    this.setData({
      'renameForm.name': e.detail.value
    })
  },

  async submitRename() {
    const cloneID = this.data.renameForm.id
    const name = String(this.data.renameForm.name || '').trim()
    if (!cloneID || !name) {
      wx.showToast({ title: '请输入名称', icon: 'none' })
      return
    }
    this.setData({ renaming: true })
    try {
      await api.updateVoiceClone(cloneID, { name })
      wx.showToast({ title: '更新成功', icon: 'success' })
      this.closeRename()
      this.loadVoiceClones(true)
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '更新失败', icon: 'none' })
      this.setData({ renaming: false })
    }
  },

  async appendAudio(e) {
    const cloneID = e.currentTarget.dataset.id
    const provider = normalizeProvider(e.currentTarget.dataset.provider)
    if (!cloneID) return

    try {
      const res = await new Promise((resolve, reject) => {
        wx.chooseMessageFile({
          count: 1,
          type: 'file',
          extension: getAudioExtensionsByProvider(provider),
          success: resolve,
          fail: reject
        })
      })
      const file = res?.tempFiles?.[0]
      if (!file?.path) return

      this.setData({ appendingID: String(cloneID) })
      await api.appendVoiceCloneAudio(cloneID, file.path, { source_type: 'upload' }, 'audio_file')
      wx.showToast({ title: '追加成功', icon: 'success' })
      this.loadVoiceClones(true)
    } catch (err) {
      if (!String(err?.errMsg || '').includes('cancel')) {
        wx.showToast({ title: err?.data?.error || '追加失败', icon: 'none' })
      }
    } finally {
      this.setData({ appendingID: '' })
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

  async previewUploadAudio(e) {
    const cloneID = e.currentTarget.dataset.id
    if (!cloneID) return
    this.setData({ previewingUploadID: String(cloneID) })
    try {
      const res = await api.getVoiceCloneAudios(cloneID)
      const audios = pickList(res)
      if (!audios.length) {
        wx.showToast({ title: '未找到原音频', icon: 'none' })
        return
      }
      const fileRes = await api.getVoiceCloneAudioFile(audios[0].id)
      this.playTempFile(fileRes?.tempFilePath)
      wx.showToast({ title: '开始播放原音频', icon: 'none' })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '播放失败', icon: 'none' })
    } finally {
      this.setData({ previewingUploadID: '' })
    }
  },

  async previewCloneAudio(e) {
    const cloneID = e.currentTarget.dataset.id
    const provider = e.currentTarget.dataset.provider
    if (!cloneID) return

    const notice = chargeNotice(provider, 'preview')
    if (notice) {
      const confirmed = await new Promise((resolve) => {
        wx.showModal({
          title: '试听提醒',
          content: notice,
          success: (res) => resolve(!!res.confirm),
          fail: () => resolve(false)
        })
      })
      if (!confirmed) return
    }

    this.setData({ previewingCloneID: String(cloneID) })
    try {
      const fileRes = await api.previewVoiceClone(cloneID)
      this.playTempFile(fileRes?.tempFilePath)
      wx.showToast({ title: '开始播放复刻音频', icon: 'none' })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '试听失败', icon: 'none' })
    } finally {
      this.setData({ previewingCloneID: '' })
    }
  },

  noop() {}
})
