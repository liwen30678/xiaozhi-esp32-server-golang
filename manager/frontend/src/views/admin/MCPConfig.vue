<template>
  <div class="mcp-config">
    <div class="page-header">
      <div class="header-content">
        <div class="title-section">
          <el-icon class="title-icon"><Connection /></el-icon>
          <h1 class="page-title">MCP配置管理</h1>
        </div>
      </div>
    </div>

    <div class="config-description">
      <el-alert
        title="配置说明"
        description="配置MCP (Model Context Protocol) 相关参数，包括全局MCP服务器配置和设备端MCP配置"
        type="info"
        :closable="false"
        show-icon
      />
    </div>

    <div class="form-container">
      <el-form ref="formRef" :model="form" :rules="rules" class="config-form" v-loading="loading">
        <el-card class="config-card global-mcp" shadow="never">
          <template #header>
            <div class="card-header">
              <el-icon class="card-icon"><Setting /></el-icon>
              <span class="card-title">全局MCP配置</span>
            </div>
          </template>
          
          <div class="form-grid">
            <el-form-item label="启用全局MCP" prop="mcp.global.enabled" class="form-item">
              <el-switch v-model="form.mcp.global.enabled" />
            </el-form-item>
            
            <el-form-item label="重连间隔(秒)" prop="mcp.global.reconnect_interval" class="form-item">
              <el-input-number v-model="form.mcp.global.reconnect_interval" :min="1" :max="3600" style="width: 100%" />
            </el-form-item>
            
            <el-form-item label="最大重连次数" prop="mcp.global.max_reconnect_attempts" class="form-item">
              <el-input-number v-model="form.mcp.global.max_reconnect_attempts" :min="1" :max="100" style="width: 100%" />
            </el-form-item>
          </div>

          <div class="server-list">
            <div class="server-list-header">
              <h4>全局MCP服务器</h4>
              <el-button type="primary" size="small" @click="addGlobalServer">
                <el-icon><Plus /></el-icon>添加服务器
              </el-button>
            </div>
            
            <div v-for="(server, index) in form.mcp.global.servers" :key="index" class="server-item">
              <div class="server-item-header">
                <div class="server-title-row">
                  <span>服务器 {{ index + 1 }}</span>
                  <el-tag size="small" :type="server.allowed_tools?.length ? 'warning' : 'info'">
                    {{ server.allowed_tools?.length ? `${server.allowed_tools.length}个工具` : '全部工具' }}
                  </el-tag>
                </div>
                <div class="server-actions">
                  <el-button size="small" :loading="server._tools_loading" @click="discoverGlobalServerTools(server)">
                    探测工具
                  </el-button>
                  <el-button type="danger" size="small" @click="removeGlobalServer(index)">
                    <el-icon><Delete /></el-icon>删除
                  </el-button>
                </div>
              </div>
              
              <div class="server-form-grid">
                <el-form-item :label="'服务器名称'" :prop="`mcp.global.servers.${index}.name`" class="form-item">
                  <el-input v-model="server.name" placeholder="服务器名称" />
                </el-form-item>
                
                                       <el-form-item :label="'服务器类型'" :prop="`mcp.global.servers.${index}.type`" class="form-item">
                         <el-select v-model="server.type" placeholder="选择服务器类型" style="width: 100%">
                           <el-option label="SSE" value="sse" />
                           <el-option label="StreamableHTTP" value="streamablehttp" />
                         </el-select>
                       </el-form-item>
                
                <el-form-item :label="'服务器URL'" :prop="`mcp.global.servers.${index}.url`" class="form-item">
                  <el-input v-model="server.url" placeholder="服务器URL" />
                </el-form-item>
                
                <el-form-item :label="'启用状态'" :prop="`mcp.global.servers.${index}.enabled`" class="form-item">
                  <el-switch v-model="server.enabled" />
                </el-form-item>
              </div>

              <el-form-item :label="'允许工具'" class="form-item tool-form-item">
                <div class="tool-picker">
                  <div class="tool-picker-tip">
                    留空表示允许该服务器全部工具。探测工具时会使用当前填写的类型、URL 和 Headers。
                  </div>
                  <el-select
                    v-model="server.allowed_tools"
                    multiple
                    filterable
                    clearable
                    collapse-tags
                    collapse-tags-tooltip
                    style="width: 100%"
                    placeholder="不选择则允许全部工具"
                    :loading="server._tools_loading"
                  >
                    <el-option v-for="tool in server._tool_options" :key="tool.name" :label="tool.name" :value="tool.name">
                      <div class="tool-option-row">
                        <span class="tool-option-name">{{ tool.name }}</span>
                        <span class="tool-option-desc">{{ tool.description || '无描述' }}</span>
                      </div>
                    </el-option>
                  </el-select>
                </div>
              </el-form-item>
            </div>
          </div>
        </el-card>

        <el-card class="config-card local-mcp" shadow="never">
          <template #header>
            <div class="card-header">
              <el-icon class="card-icon"><HomeFilled /></el-icon>
              <span class="card-title">本地MCP配置</span>
            </div>
          </template>
          
          <div class="form-grid">
            <el-form-item label="退出对话" prop="local_mcp.exit_conversation" class="form-item">
              <el-switch v-model="form.local_mcp.exit_conversation" />
            </el-form-item>
            
            <el-form-item label="清除对话历史" prop="local_mcp.clear_conversation_history" class="form-item">
              <el-switch v-model="form.local_mcp.clear_conversation_history" />
            </el-form-item>
            
            <el-form-item label="播放音乐" prop="local_mcp.play_music" class="form-item">
              <el-switch v-model="form.local_mcp.play_music" />
            </el-form-item>
          </div>
        </el-card>

        <div class="action-section">
          <el-button type="primary" size="large" class="save-button" :loading="saving" @click="handleSave">
            <el-icon><Check /></el-icon>保存配置
          </el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Connection, Setting, HomeFilled, Plus, Delete, Check } from '@element-plus/icons-vue'
import api from '@/utils/api'

const loading = ref(false)
const saving = ref(false)
const configId = ref(null)
const formRef = ref()

const form = reactive({
  mcp: {
    global: {
      enabled: true,
      servers: [],
      reconnect_interval: 300,
      max_reconnect_attempts: 10
    }
  },
  local_mcp: {
    exit_conversation: true,
    clear_conversation_history: true,
    play_music: false
  }
})

const rules = {
  'mcp.global.enabled': [{ required: true, message: '请选择是否启用全局MCP', trigger: 'change' }],
  'mcp.global.reconnect_interval': [
    { required: true, message: '请输入重连间隔', trigger: 'blur' },
    { type: 'number', min: 1, max: 3600, message: '重连间隔必须在1-3600之间', trigger: 'blur' }
  ],
  'mcp.global.max_reconnect_attempts': [
    { required: true, message: '请输入最大重连次数', trigger: 'blur' },
    { type: 'number', min: 1, max: 100, message: '最大重连次数必须在1-100之间', trigger: 'blur' }
  ],
  'local_mcp.exit_conversation': [{ required: true, message: '请选择是否退出对话', trigger: 'change' }],
  'local_mcp.clear_conversation_history': [{ required: true, message: '请选择是否清除对话历史', trigger: 'change' }],
  'local_mcp.play_music': [{ required: true, message: '请选择是否播放音乐', trigger: 'change' }]
}

const createGlobalServer = () => ({
  name: '',
  type: 'streamablehttp',
  url: '',
  enabled: true,
  allowed_tools: [],
  _tool_options: [],
  _tools_loading: false
})

const mergeServerToolOptions = (server, tools = []) => {
  const merged = new Map()

  ;(tools || []).forEach((tool) => {
    if (!tool?.name) return
    merged.set(tool.name, {
      name: tool.name,
      description: tool.description || ''
    })
  })

  ;(server.allowed_tools || []).forEach((name) => {
    if (!name || merged.has(name)) return
    merged.set(name, {
      name,
      description: '当前已选择'
    })
  })

  server._tool_options = Array.from(merged.values()).sort((a, b) => a.name.localeCompare(b.name))
}

const normalizeGlobalServer = (server = {}) => {
  const normalized = {
    ...server,
    name: server.name || '',
    type: server.type || 'streamablehttp',
    url: server.url || '',
    enabled: server.enabled !== false,
    allowed_tools: Array.isArray(server.allowed_tools) ? [...server.allowed_tools] : [],
    _tool_options: [],
    _tools_loading: false
  }
  mergeServerToolOptions(normalized)
  return normalized
}

const addGlobalServer = () => {
  form.mcp.global.servers.push(createGlobalServer())
}

const removeGlobalServer = (index) => {
  form.mcp.global.servers.splice(index, 1)
}

const sanitizeGlobalServers = () => {
  return form.mcp.global.servers.map((server) => {
    const sanitized = { ...server }
    delete sanitized._tool_options
    delete sanitized._tools_loading
    return sanitized
  })
}

const generateConfig = () => {
  return JSON.stringify({
    mcp: {
      global: {
        ...form.mcp.global,
        servers: sanitizeGlobalServers()
      }
    },
    local_mcp: form.local_mcp
  }, null, 2)
}

const discoverGlobalServerTools = async (server) => {
  if (!server?.url) {
    ElMessage.warning('请先填写服务器URL')
    return
  }

  server._tools_loading = true
  try {
    const response = await api.post('/admin/mcp-configs/discover-tools', {
      transport: server.type,
      url: server.url,
      headers: server.headers || null
    })
    mergeServerToolOptions(server, response.data?.data?.tools || [])
    ElMessage.success(`探测到 ${server._tool_options.length} 个工具`)
  } catch (error) {
    mergeServerToolOptions(server)
    ElMessage.error(error.response?.data?.error || '探测工具失败')
  } finally {
    server._tools_loading = false
  }
}

const loadConfig = async () => {
  loading.value = true
  try {
    const response = await api.get('/admin/mcp-configs')
    const configs = response.data.data
    
    if (configs && configs.length > 0) {
      const config = configs.find(c => c.is_default) || configs[0]
      configId.value = config.id
      
             try {
           const configData = JSON.parse(config.json_data)
           // 兼容旧格式：如果存在global配置，则转换为新格式
           if (configData.global && !configData.mcp) {
             form.mcp.global = {
               ...form.mcp.global,
               ...configData.global,
               servers: Array.isArray(configData.global?.servers)
                 ? configData.global.servers.map(normalizeGlobalServer)
                 : []
             }
           } else if (configData.mcp) {
             const globalConfig = configData.mcp.global || {}
             form.mcp.global = {
               ...form.mcp.global,
               ...globalConfig,
               servers: Array.isArray(globalConfig.servers)
                 ? globalConfig.servers.map(normalizeGlobalServer)
                 : []
             }
           }
           if (configData.local_mcp) Object.assign(form.local_mcp, configData.local_mcp)
         } catch (error) {
          console.error('Parse config failed:', error)
          ElMessage.warning('Config format error, reset to default values')
        }
    } else {
      form.mcp.global.servers.push(normalizeGlobalServer({
        name: '默认MCP服务器',
        type: 'streamablehttp',
        url: 'http://192.168.208.214:3001/mcp',
        enabled: true
      }))
    }
  } catch (error) {
    console.error('加载配置失败:', error)
    ElMessage.error('加载配置失败')
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  if (!formRef.value) return
  
  await formRef.value.validate(async (valid) => {
    if (valid) {
      saving.value = true
      try {
        const configData = {
          name: 'MCP全局配置',
          config_id: 'mcp_global_config',
          is_default: true,
          json_data: generateConfig()
        }
        
        if (configId.value) {
          await api.put(`/admin/mcp-configs/${configId.value}`, configData)
          ElMessage.success('更新成功')
        } else {
          const response = await api.post('/admin/mcp-configs', configData)
          configId.value = response.data.data.id
          ElMessage.success('保存成功')
        }
      } catch (error) {
        ElMessage.error(error.response?.data?.message || '保存失败')
      } finally {
        saving.value = false
      }
    }
  })
}

onMounted(() => {
  loadConfig()
})
</script>

<style scoped>
.mcp-config {
  min-height: 100vh;
  background: #f8f9fa;
  padding: 24px;
}

.page-header {
  margin-bottom: 24px;
}

.header-content {
  max-width: 1200px;
  margin: 0 auto;
}

.title-section {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 8px;
}

.title-icon {
  font-size: 32px;
  color: #409eff;
}

.page-title {
  font-size: 28px;
  font-weight: 600;
  color: #1f2937;
  margin: 0;
  background: linear-gradient(135deg, #409eff 0%, #67c23a 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.config-description {
  max-width: 1200px;
  margin: 0 auto 24px;
}

.form-container {
  max-width: 1200px;
  margin: 0 auto;
}

.config-form {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.config-card {
  background: rgba(255, 255, 255, 0.95);
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  transition: all 0.3s ease;
  overflow: hidden;
}

.config-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 25px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
}

.global-mcp {
  border-left: 4px solid #409eff;
}



.local-mcp {
  border-left: 4px solid #e6a23c;
}

.card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0;
}

.card-icon {
  font-size: 20px;
  color: #409eff;
}

.card-title {
  font-size: 18px;
  font-weight: 600;
  color: #1f2937;
  margin: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 24px;
  padding: 24px;
}

.form-item {
  margin-bottom: 0;
}

.server-list {
  padding: 0 24px 24px;
}

.server-list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.server-list-header h4 {
  margin: 0;
  color: #374151;
  font-size: 16px;
  font-weight: 600;
}

.server-item {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
  background: #f9fafb;
}

.server-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 8px;
  border-bottom: 1px solid #e5e7eb;
}

.server-item-header span {
  font-weight: 600;
  color: #374151;
}

.server-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.server-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.server-form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 16px;
}

.tool-form-item {
  margin-top: 16px;
}

.tool-picker {
  width: 100%;
}

.tool-picker-tip {
  margin-bottom: 8px;
  color: #6b7280;
  font-size: 12px;
  line-height: 1.5;
}

.tool-option-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.35;
}

.tool-option-name {
  color: #111827;
}

.tool-option-desc {
  color: #6b7280;
  font-size: 12px;
}

.action-section {
  display: flex;
  justify-content: center;
  padding: 32px 0;
}

.save-button {
  padding: 12px 32px;
  font-size: 16px;
  font-weight: 500;
  border-radius: 8px;
  background: linear-gradient(135deg, #409eff 0%, #67c23a 100%);
  border: none;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  transition: all 0.3s ease;
}

.save-button:hover {
  transform: translateY(-1px);
  box-shadow: 0 10px 25px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
}

:deep(.el-form-item__label) {
  font-weight: 500;
  color: #374151;
  font-size: 14px;
}

:deep(.el-input__wrapper) {
  border-radius: 8px;
  box-shadow: 0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06);
  transition: all 0.2s ease;
}

:deep(.el-input__wrapper:hover) {
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
}

:deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 0 0 3px rgba(64, 158, 255, 0.1);
}

:deep(.el-select .el-input__wrapper) {
  border-radius: 8px;
}

:deep(.el-input-number .el-input__wrapper) {
  border-radius: 8px;
}

:deep(.el-switch) {
  --el-switch-on-color: #409eff;
}

:deep(.el-card__header) {
  background: linear-gradient(135deg, #f8fafc 0%, #f1f5f9 100%);
  border-bottom: 1px solid #e2e8f0;
  padding: 20px 24px;
}

:deep(.el-card__body) {
  padding: 0;
}

@media (max-width: 768px) {
  .mcp-config {
    padding: 16px;
  }
  
  .page-title {
    font-size: 24px;
  }
  
  .title-icon {
    font-size: 28px;
  }
  
  .form-grid {
    grid-template-columns: 1fr;
    gap: 16px;
    padding: 16px;
  }
  
  .server-form-grid {
    grid-template-columns: 1fr;
  }

  .server-item-header {
    align-items: flex-start;
    flex-direction: column;
    gap: 12px;
  }
}

@media (max-width: 480px) {
  .title-section {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
  
  .page-title {
    font-size: 20px;
  }
}
</style>
