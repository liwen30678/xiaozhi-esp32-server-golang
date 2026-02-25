const api = require('../../utils/api')
const { ensureLogin } = require('../../utils/auth')

const pickList = (res) => res?.data || res?.items || res?.list || []

const normalizeStats = (raw) => ({
  deviceCount: raw?.totalDevices ?? raw?.device_count ?? 0,
  agentCount: raw?.totalAgents ?? raw?.agent_count ?? 0,
  onlineDeviceCount: raw?.onlineDevices ?? raw?.online_device_count ?? 0
})

Page({
  data: {
    user: null,
    stats: {
      deviceCount: 0,
      agentCount: 0,
      onlineDeviceCount: 0
    },
    agents: [],
    devices: [],
    loading: false,
    showAddDevice: false,
    addDeviceSaving: false,
    selectedAddAgentName: '',
    addDeviceForm: {
      device_name: '',
      agent_id: ''
    },
    addDeviceAgentIndex: -1,
    showInject: false,
    injecting: false,
    selectedInjectDeviceName: '',
    injectForm: {
      device_id: '',
      message: '',
      skip_llm: false
    },
    injectDeviceIndex: -1
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
      const [profileRes, statsRes, agentsRes, devicesRes] = await Promise.all([
        api.getProfile(),
        api.getDashboardStats(),
        api.getAgents(),
        api.getDevices()
      ])
      const agents = pickList(agentsRes)
      const devices = pickList(devicesRes)
      this.setData({
        user: profileRes?.user || null,
        stats: normalizeStats(statsRes || {}),
        agents,
        devices
      })
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '加载失败', icon: 'none' })
    } finally {
      this.setData({ loading: false })
    }
  },

  goAgents() {
    wx.switchTab({ url: '/pages/agents/index' })
  },

  openAddDevice() {
    if (!this.data.agents.length) {
      wx.showToast({ title: '请先创建智能体', icon: 'none' })
      return
    }

    this.setData({
      showAddDevice: true,
      addDeviceAgentIndex: -1,
      selectedAddAgentName: '',
      addDeviceForm: {
        device_name: '',
        agent_id: ''
      }
    })
  },

  closeAddDevice() {
    this.setData({
      showAddDevice: false,
      addDeviceSaving: false
    })
  },

  onAddDeviceNameInput(e) {
    this.setData({
      'addDeviceForm.device_name': e.detail.value
    })
  },

  onAddDeviceAgentChange(e) {
    const index = Number(e.detail.value)
    const agent = this.data.agents[index]
    this.setData({
      addDeviceAgentIndex: index,
      selectedAddAgentName: agent?.name || '',
      'addDeviceForm.agent_id': agent?.id || ''
    })
  },

  async submitAddDevice() {
    const { addDeviceForm } = this.data
    if (!addDeviceForm.device_name || !addDeviceForm.agent_id) {
      wx.showToast({ title: '请填写完整', icon: 'none' })
      return
    }

    this.setData({ addDeviceSaving: true })
    try {
      await api.createDevice({
        device_name: addDeviceForm.device_name,
        agent_id: Number(addDeviceForm.agent_id)
      })
      wx.showToast({ title: '添加成功', icon: 'success' })
      this.closeAddDevice()
      this.loadData()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '添加失败', icon: 'none' })
      this.setData({ addDeviceSaving: false })
    }
  },

  openInject() {
    if (!this.data.devices.length) {
      wx.showToast({ title: '暂无可用设备', icon: 'none' })
      return
    }

    this.setData({
      showInject: true,
      injectDeviceIndex: -1,
      selectedInjectDeviceName: '',
      injectForm: {
        device_id: '',
        message: '',
        skip_llm: false
      }
    })
  },

  closeInject() {
    this.setData({
      showInject: false,
      injecting: false
    })
  },

  onInjectDeviceChange(e) {
    const index = Number(e.detail.value)
    const device = this.data.devices[index]
    this.setData({
      injectDeviceIndex: index,
      selectedInjectDeviceName: device?.device_name || '',
      'injectForm.device_id': device?.device_name || ''
    })
  },

  onInjectMessageInput(e) {
    this.setData({
      'injectForm.message': e.detail.value
    })
  },

  onInjectSkipChange(e) {
    this.setData({
      'injectForm.skip_llm': !!e.detail.value
    })
  },

  async submitInject() {
    const { injectForm } = this.data
    if (!injectForm.device_id || !injectForm.message) {
      wx.showToast({ title: '请选择设备并填写消息', icon: 'none' })
      return
    }

    this.setData({ injecting: true })
    try {
      await api.injectMessage(injectForm)
      wx.showToast({ title: '注入成功', icon: 'success' })
      this.closeInject()
    } catch (err) {
      wx.showToast({ title: err?.data?.error || '注入失败', icon: 'none' })
      this.setData({ injecting: false })
    }
  },

  noop() {},

  onHide() {
    this.setData({
      showAddDevice: false,
      showInject: false
    })
  }
})
