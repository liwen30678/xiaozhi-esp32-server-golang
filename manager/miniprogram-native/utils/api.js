const getBaseURL = () => {
  const app = getApp()
  const baseURL = app?.globalData?.baseURL || wx.getStorageSync('baseURL')
  return (baseURL || '').replace(/\/$/, '')
}

const encodeQueryValue = (value) => encodeURIComponent(String(value))

const buildQueryString = (params = {}) => {
  const pairs = []
  Object.keys(params).forEach((key) => {
    const value = params[key]
    if (value === undefined || value === null || value === '') {
      return
    }

    if (Array.isArray(value)) {
      value.forEach((item) => {
        if (item !== undefined && item !== null && item !== '') {
          pairs.push(`${encodeURIComponent(key)}=${encodeQueryValue(item)}`)
        }
      })
      return
    }

    pairs.push(`${encodeURIComponent(key)}=${encodeQueryValue(value)}`)
  })

  return pairs.length ? `?${pairs.join('&')}` : ''
}

const buildRequestURL = (url, params) => {
  const query = buildQueryString(params)
  if (/^https?:\/\//i.test(url)) {
    return `${url}${query}`
  }
  return `${getBaseURL()}${url}${query}`
}

const handleUnauthorized = () => {
  wx.removeStorageSync('token')
  wx.removeStorageSync('user')
  wx.reLaunch({ url: '/pages/login/index' })
}

const request = ({
  url,
  method = 'GET',
  data,
  params,
  withAuth = true,
  timeout = 15000,
  header = {}
}) => {
  const token = wx.getStorageSync('token')
  const headers = {
    'Content-Type': 'application/json',
    ...header
  }

  if (withAuth && token) {
    headers.Authorization = `Bearer ${token}`
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url: buildRequestURL(url, params),
      method,
      data,
      timeout,
      header: headers,
      success: (res) => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(res.data)
          return
        }

        if (res.statusCode === 401) {
          handleUnauthorized()
        }

        reject({
          statusCode: res.statusCode,
          data: res.data,
          errMsg: res.errMsg || 'request failed'
        })
      },
      fail: (err) => {
        reject(err || { errMsg: 'network error' })
      }
    })
  })
}

const parseJSONData = (value) => {
  if (value === undefined || value === null || value === '') return {}
  if (typeof value === 'object') return value
  try {
    return JSON.parse(value)
  } catch (err) {
    return {}
  }
}

const readTempFileText = (filePath) => new Promise((resolve) => {
  if (!filePath) {
    resolve('')
    return
  }
  wx.getFileSystemManager().readFile({
    filePath,
    encoding: 'utf8',
    success: (res) => resolve(String(res?.data || '')),
    fail: () => resolve('')
  })
})

const uploadFile = ({
  url,
  filePath,
  name = 'file',
  formData = {},
  withAuth = true,
  timeout = 120000,
  header = {},
  params
}) => {
  const token = wx.getStorageSync('token')
  const headers = {
    ...header
  }

  if (withAuth && token) {
    headers.Authorization = `Bearer ${token}`
  }

  return new Promise((resolve, reject) => {
    wx.uploadFile({
      url: buildRequestURL(url, params),
      filePath,
      name,
      formData,
      timeout,
      header: headers,
      success: (res) => {
        const data = parseJSONData(res?.data)
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(data)
          return
        }

        if (res.statusCode === 401) {
          handleUnauthorized()
        }

        reject({
          statusCode: res.statusCode,
          data,
          errMsg: (data && data.error) || 'upload failed'
        })
      },
      fail: (err) => {
        reject(err || { errMsg: 'upload failed' })
      }
    })
  })
}

const downloadFile = ({
  url,
  params,
  withAuth = true,
  timeout = 120000,
  header = {}
}) => {
  const token = wx.getStorageSync('token')
  const headers = {
    ...header
  }

  if (withAuth && token) {
    headers.Authorization = `Bearer ${token}`
  }

  return new Promise((resolve, reject) => {
    wx.downloadFile({
      url: buildRequestURL(url, params),
      timeout,
      header: headers,
      success: async (res) => {
        if (res.statusCode >= 200 && res.statusCode < 300) {
          resolve(res)
          return
        }

        if (res.statusCode === 401) {
          handleUnauthorized()
        }

        const text = await readTempFileText(res?.tempFilePath)
        const parsed = parseJSONData(text)
        reject({
          statusCode: res.statusCode,
          data: parsed,
          errMsg: (parsed && parsed.error) || 'download failed'
        })
      },
      fail: (err) => {
        reject(err || { errMsg: 'download failed' })
      }
    })
  })
}

const api = {
  request,
  uploadFile,
  downloadFile,
  get(url, params, options = {}) {
    return request({ ...options, url, method: 'GET', params })
  },
  post(url, data, options = {}) {
    return request({ ...options, url, method: 'POST', data })
  },
  put(url, data, options = {}) {
    return request({ ...options, url, method: 'PUT', data })
  },
  patch(url, data, options = {}) {
    return request({ ...options, url, method: 'PATCH', data })
  },
  delete(url, data, options = {}) {
    return request({ ...options, url, method: 'DELETE', data })
  },

  login(payload) {
    return api.post('/api/login', payload, { withAuth: false })
  },
  getProfile() {
    return api.get('/api/profile')
  },
  getDashboardStats() {
    return api.get('/api/dashboard/stats')
  },

  getAgents() {
    return api.get('/api/user/agents')
  },
  createAgent(payload) {
    return api.post('/api/user/agents', payload)
  },
  getAgent(agentID) {
    return api.get(`/api/user/agents/${agentID}`)
  },
  updateAgent(agentID, payload) {
    return api.put(`/api/user/agents/${agentID}`, payload)
  },
  deleteAgent(agentID) {
    return api.delete(`/api/user/agents/${agentID}`)
  },

  getDevices() {
    return api.get('/api/user/devices')
  },
  createDevice(payload) {
    return api.post('/api/user/devices', payload)
  },
  injectMessage(payload) {
    return api.post('/api/user/devices/inject-message', payload)
  },

  getAgentDevices(agentID) {
    return api.get(`/api/user/agents/${agentID}/devices`)
  },
  bindDeviceToAgent(agentID, payload) {
    return api.post(`/api/user/agents/${agentID}/devices`, payload)
  },
  removeAgentDevice(agentID, deviceID) {
    return api.delete(`/api/user/agents/${agentID}/devices/${deviceID}`)
  },
  applyDeviceRole(deviceID, roleID) {
    return api.post(`/api/devices/${deviceID}/apply-role`, { role_id: roleID || null })
  },

  getRoles() {
    return api.get('/api/user/roles')
  },
  createRole(payload) {
    return api.post('/api/user/roles', payload)
  },
  updateRole(roleID, payload) {
    return api.put(`/api/user/roles/${roleID}`, payload)
  },
  deleteRole(roleID) {
    return api.delete(`/api/user/roles/${roleID}`)
  },
  toggleRole(roleID) {
    return api.patch(`/api/user/roles/${roleID}/toggle`)
  },

  getKnowledgeBases() {
    return api.get('/api/user/knowledge-bases')
  },
  createKnowledgeBase(payload) {
    return api.post('/api/user/knowledge-bases', payload)
  },
  updateKnowledgeBase(kbID, payload) {
    return api.put(`/api/user/knowledge-bases/${kbID}`, payload)
  },
  deleteKnowledgeBase(kbID) {
    return api.delete(`/api/user/knowledge-bases/${kbID}`)
  },
  syncKnowledgeBase(kbID) {
    return api.post(`/api/user/knowledge-bases/${kbID}/sync`)
  },

  getLLMConfigs() {
    return api.get('/api/user/llm-configs')
  },
  getTTSConfigs() {
    return api.get('/api/user/tts-configs')
  },
  getVoiceOptions(params) {
    return api.get('/api/user/voice-options', params)
  },

  getAgentHistory(agentID, params) {
    return api.get(`/api/user/history/agents/${agentID}/messages`, params)
  },
  deleteHistoryMessage(messageID) {
    return api.delete(`/api/user/history/messages/${messageID}`)
  },

  getSpeakerGroups(params) {
    return api.get('/api/user/speaker-groups', params)
  },
  getSpeakerGroup(groupID) {
    return api.get(`/api/user/speaker-groups/${groupID}`)
  },
  createSpeakerGroup(payload) {
    return api.post('/api/user/speaker-groups', payload)
  },
  updateSpeakerGroup(groupID, payload) {
    return api.put(`/api/user/speaker-groups/${groupID}`, payload)
  },
  deleteSpeakerGroup(groupID) {
    return api.delete(`/api/user/speaker-groups/${groupID}`)
  },
  getSpeakerSamples(groupID) {
    return api.get(`/api/user/speaker-groups/${groupID}/samples`)
  },
  addSpeakerSample(groupID, filePath) {
    return api.uploadFile({
      url: `/api/user/speaker-groups/${groupID}/samples`,
      filePath,
      name: 'audio'
    })
  },
  verifySpeakerGroup(groupID, filePath) {
    return api.uploadFile({
      url: `/api/user/speaker-groups/${groupID}/verify`,
      filePath,
      name: 'audio'
    })
  },
  deleteSpeakerSample(groupID, sampleID) {
    return api.delete(`/api/user/speaker-groups/${groupID}/samples/${sampleID}`)
  },
  getSpeakerSampleFile(groupID, sampleID) {
    return api.downloadFile({
      url: `/api/user/speaker-groups/${groupID}/samples/${sampleID}/file`
    })
  },

  getVoiceCloneCapabilities(provider) {
    return api.get('/api/user/voice-clone/capabilities', { provider })
  },
  getVoiceClones(params) {
    return api.get('/api/user/voice-clones', params)
  },
  createVoiceClone(filePath, formData, fieldName = 'audio_file') {
    return api.uploadFile({
      url: '/api/user/voice-clones',
      filePath,
      name: fieldName,
      formData,
      timeout: 120000
    })
  },
  updateVoiceClone(cloneID, payload) {
    return api.put(`/api/user/voice-clones/${cloneID}`, payload)
  },
  retryVoiceClone(cloneID) {
    return api.post(`/api/user/voice-clones/${cloneID}/retry`)
  },
  appendVoiceCloneAudio(cloneID, filePath, formData = {}, fieldName = 'audio_file') {
    return api.uploadFile({
      url: `/api/user/voice-clones/${cloneID}/append-audio`,
      filePath,
      name: fieldName,
      formData,
      timeout: 120000
    })
  },
  getVoiceCloneAudios(cloneID) {
    return api.get(`/api/user/voice-clones/${cloneID}/audios`)
  },
  previewVoiceClone(cloneID) {
    return api.downloadFile({
      url: `/api/user/voice-clones/${cloneID}/preview`
    })
  },
  getVoiceCloneAudioFile(audioID) {
    return api.downloadFile({
      url: `/api/user/voice-clones/audios/${audioID}/file`
    })
  }
}

module.exports = api
