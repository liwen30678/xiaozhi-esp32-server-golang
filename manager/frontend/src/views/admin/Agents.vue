<template>
  <div class="admin-agents">
    <div class="page-header">
      <h2>智能体管理</h2>
      <p class="page-subtitle">管理系统中的所有智能体</p>
    </div>

    <div class="toolbar">
      <el-button type="primary" @click="showAddDialog = true">
        <el-icon><Plus /></el-icon>
        添加智能体
      </el-button>
      <el-button @click="loadAgents">
        <el-icon><Refresh /></el-icon>
        刷新
      </el-button>
    </div>

    <el-table :data="agents" v-loading="loading" stripe>
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="昵称" width="150" />
      <el-table-column prop="user_id" label="用户ID" width="100" />
      <el-table-column label="角色介绍" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.custom_prompt || '未设置' }}
        </template>
      </el-table-column>
      <el-table-column label="语言模型" width="150">
        <template #default="{ row }">
          {{ row.llm_config?.name || '未设置' }}
        </template>
      </el-table-column>
      <el-table-column label="音色" width="150">
        <template #default="{ row }">
          {{ row.tts_config?.name || '未设置' }}
        </template>
      </el-table-column>
      <el-table-column label="语音识别速度" width="120">
        <template #default="{ row }">
          <el-tag :type="getASRSpeedType(row.asr_speed)">
            {{ getASRSpeedText(row.asr_speed) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="记忆模式" width="120">
        <template #default="{ row }">
          <el-tag :type="getMemoryModeType(row.memory_mode)">
            {{ getMemoryModeText(row.memory_mode) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : 'info'">
            {{ row.status === 'active' ? '活跃' : '非活跃' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="280">
        <template #default="{ row }">
          <el-button size="small" @click="editAgent(row)">
            编辑
          </el-button>
          <el-button size="small" type="primary" @click="showMCPEndpoint(row)">
            MCP接入点
          </el-button>
          <el-button size="small" type="danger" @click="deleteAgent(row)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 添加/编辑智能体对话框 -->
    <el-dialog
      v-model="showAddDialog"
      :title="editingAgent ? '编辑智能体' : '添加智能体'"
      width="600px"
    >
      <el-form :model="agentForm" :rules="agentRules" ref="agentFormRef" label-width="120px">
        <el-form-item label="用户ID" prop="user_id">
          <el-input-number v-model="agentForm.user_id" :min="1" style="width: 100%" />
        </el-form-item>
        <el-form-item label="昵称" prop="name">
          <el-input v-model="agentForm.name" placeholder="请输入智能体昵称" />
        </el-form-item>
        <el-form-item label="角色介绍" prop="custom_prompt">
          <el-input
            v-model="agentForm.custom_prompt"
            type="textarea"
            :rows="4"
            placeholder="请输入角色介绍/系统提示词"
          />
        </el-form-item>
        <el-form-item label="语言模型" prop="llm_config_id">
          <el-select v-model="agentForm.llm_config_id" placeholder="请选择语言模型" style="width: 100%">
            <el-option 
              v-for="config in llmConfigs" 
              :key="config.config_id" 
              :label="config.name" 
              :value="config.config_id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="音色" prop="tts_config_id">
          <el-select v-model="agentForm.tts_config_id" placeholder="请选择音色" style="width: 100%">
            <el-option 
              v-for="config in ttsConfigs" 
              :key="config.config_id" 
              :label="config.name" 
              :value="config.config_id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="语音识别速度" prop="asr_speed">
          <el-select v-model="agentForm.asr_speed" style="width: 100%">
            <el-option label="正常" value="normal" />
            <el-option label="耐心" value="patient" />
            <el-option label="快速" value="fast" />
          </el-select>
        </el-form-item>
        <el-form-item label="记忆模式" prop="memory_mode">
          <el-select v-model="agentForm.memory_mode" style="width: 100%">
            <el-option label="无记忆" value="none" />
            <el-option label="短记忆" value="short" />
            <el-option label="长记忆" value="long" />
          </el-select>
        </el-form-item>
        <el-form-item label="OpenClaw">
          <el-button type="primary" size="large" style="width: 100%" @click="showOpenClawSettings">
            查看openclaw
          </el-button>
          <div style="margin-top: 6px; color: #909399; font-size: 12px;">
            已配置: {{ agentForm.openclaw_allowed ? '开启' : '关闭' }}，进入词 {{ agentForm.openclaw_enter_keywords.length }} 个，退出词 {{ agentForm.openclaw_exit_keywords.length }} 个。
          </div>
        </el-form-item>
        <el-form-item label="状态" prop="status">
          <el-select v-model="agentForm.status" style="width: 100%">
            <el-option label="活跃" value="active" />
            <el-option label="非活跃" value="inactive" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="saveAgent" :loading="saving">
          {{ editingAgent ? '更新' : '添加' }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showOpenClawDialog"
      title="OpenClaw设置"
      width="680px"
    >
      <div>
        <div class="openclaw-tip-row">
          <span class="openclaw-tip-title">接入tips</span>
          <el-tooltip effect="light" placement="top-start" :show-after="200" :enterable="true" popper-class="openclaw-tip-popper">
            <template #content>
              <div class="openclaw-tip-content">
                <div>架构：设备语音 -> 服务端路由 -> OpenClaw 会话 -> xiaozhi 插件。</div>
                <div>角色配置：在 OpenClaw 控制台角色配置中使用下方四条命令，最后执行 `openclaw gateway restart` 使配置生效。</div>
                <div>进入逻辑：命中进入词（默认“打开龙虾/进入龙虾”）后进入 OpenClaw 模式，后续文本优先走 OpenClaw。</div>
                <div>退出逻辑：在 OpenClaw 模式下命中退出词（默认“关闭龙虾/退出龙虾”）即退出，恢复普通 LLM 对话。</div>
                <el-link :href="openClawDocURL" target="_blank" type="primary" :underline="false">
                  查看完整文档
                </el-link>
              </div>
            </template>
            <el-icon class="openclaw-tip-icon"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>

        <el-form label-width="100px">
          <el-form-item label="开关">
            <el-switch v-model="agentForm.openclaw_allowed" />
          </el-form-item>
          <el-form-item label="进入关键词">
            <el-select
              v-model="agentForm.openclaw_enter_keywords"
              multiple
              filterable
              allow-create
              default-first-option
              clearable
              style="width: 100%"
              placeholder="输入后回车，可添加多个关键词"
            />
          </el-form-item>
          <el-form-item label="退出关键词">
            <el-select
              v-model="agentForm.openclaw_exit_keywords"
              multiple
              filterable
              allow-create
              default-first-option
              clearable
              style="width: 100%"
              placeholder="输入后回车，可添加多个关键词"
            />
          </el-form-item>
        </el-form>

        <el-divider />

        <div v-loading="openClawEndpointLoading">
          <div class="openclaw-status-bar">
            <div class="endpoint-label">连接状态：</div>
            <el-tag :type="openClawStatusTagType">{{ openClawStatusText }}</el-tag>
          </div>
          <div v-if="openClawEndpointData.status_message" class="openclaw-status-message">
            {{ openClawEndpointData.status_message }}
          </div>
          <div class="mcp-endpoint-display">
            <div class="endpoint-header">
              <div class="endpoint-label">OpenClaw角色配置命令：</div>
              <div class="endpoint-actions">
                <el-button size="small" @click="fetchOpenClawEndpoint" :disabled="!editingAgent" :loading="openClawEndpointLoading">刷新</el-button>
                <el-button size="small" type="primary" @click="copyOpenClawCommands" :disabled="!openClawCommandData.ready">复制命令</el-button>
              </div>
            </div>
            <div v-if="openClawCommandData.ready" class="openclaw-command-hint">在 OpenClaw 控制台角色配置中依次执行以下命令：</div>
            <div v-if="openClawCommandData.ready" class="openclaw-command-steps">
              <div
                v-for="(step, index) in openClawCommandData.steps"
                :key="`${step.title}-${index}`"
                class="openclaw-command-step"
              >
                <div class="openclaw-command-step-title">第 {{ index + 1 }} 行：{{ step.title }}</div>
                <pre class="openclaw-command-content">{{ step.command }}</pre>
              </div>
            </div>
            <pre v-else class="openclaw-command-content">{{ openClawCommandDisplayText }}</pre>
          </div>
        </div>

        <el-divider />
        <el-alert
          title="对话测试"
          description="向openclaw发送文本测试请求并查看回复。"
          type="info"
          :closable="false"
          show-icon
          style="margin-bottom: 12px"
        />
        <el-form label-width="100px">
          <el-form-item label="测试消息">
            <el-input
              v-model="openClawChatTestForm.message"
              type="textarea"
              :rows="3"
              placeholder="请输入测试消息"
            />
          </el-form-item>
        </el-form>
        <el-button
          type="primary"
          @click="testOpenClawChat"
          :loading="openClawChatTesting"
          :disabled="!editingAgent"
        >
          发送测试
        </el-button>
        <div class="mcp-result-box">{{ openClawChatTestResult || '暂无测试结果' }}</div>
      </div>
      <template #footer>
        <el-button @click="showOpenClawDialog = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- MCP接入点对话框 -->
    <el-dialog
      v-model="showMCPDialog"
      title="MCP接入点"
      width="700px"
    >
      <div v-loading="mcpLoading">
        <!-- 工具列表区域 -->
        <div class="mcp-tools-section">
          <div class="tools-header">
            <div class="tools-title">MCP工具列表</div>
            <el-button 
              size="small" 
              type="primary" 
              @click="refreshMcpTools"
              :loading="toolsLoading"
            >
              <el-icon><Refresh /></el-icon>
              刷新工具列表
            </el-button>
          </div>
          
          <div class="tools-list">
            <div v-if="mcpTools.length === 0" class="tools-empty">
              <el-tag type="info" size="large" class="tool-tag">
                暂无工具数据
              </el-tag>
            </div>
            
            <div v-else class="tools-tags">
              <el-tag
                v-for="tool in mcpTools"
                :key="tool.name"
                :type="tool.schema ? 'success' : 'info'"
                size="large"
                class="tool-tag"
                :title="tool.description"
              >
                {{ tool.name }}
                <el-tooltip
                  v-if="tool.description"
                  :content="tool.description"
                  placement="top"
                  :show-after="500"
                >
                  <el-icon class="tool-info-icon"><InfoFilled /></el-icon>
                </el-tooltip>
              </el-tag>
            </div>
          </div>
        </div>

        <el-alert
          title="接入点信息"
          description="这是智能体的MCP WebSocket接入点URL，可用于设备连接"
          type="info"
          :closable="false"
          show-icon
          style="margin-bottom: 20px; margin-top: 24px;"
        />
        
        <div class="mcp-endpoint-display">
          <div class="endpoint-header">
            <div class="endpoint-label">MCP接入点URL：</div>
            <el-button size="small" type="primary" @click="copyMCPEndpoint">复制URL</el-button>
          </div>
          <div class="endpoint-content">
            {{ mcpEndpointData.endpoint }}
          </div>
        </div>

        <el-divider />
        <el-form :model="mcpCallForm" label-width="90px">
          <el-form-item label="工具">
            <el-select v-model="mcpCallForm.tool_name" placeholder="请选择工具" style="width: 100%" @change="handleMcpToolChange">
              <el-option v-for="tool in mcpTools" :key="tool.name" :label="tool.name" :value="tool.name" />
            </el-select>
          </el-form-item>
          <el-form-item label="参数JSON">
            <el-input v-model="mcpCallForm.argumentsText" type="textarea" :rows="6" placeholder='例如: {"query":"hello"}' />
          </el-form-item>
        </el-form>
        <el-button type="primary" @click="callAgentMcpTool" :loading="callingTool">调用工具</el-button>
        <div class="endpoint-content" style="margin-top: 12px">{{ mcpCallResult || "暂无调用结果" }}</div>

      </div>
      
      <template #footer>
        <el-button @click="showMCPDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh, InfoFilled, QuestionFilled } from '@element-plus/icons-vue'
import api from '../../utils/api'
import { postJSONWithSSE } from '../../utils/sse'
import { buildOpenClawCommands } from '../../utils/openclaw'

const agents = ref([])
const llmConfigs = ref([])
const ttsConfigs = ref([])
const loading = ref(false)
const showAddDialog = ref(false)
const editingAgent = ref(null)
const saving = ref(false)
const agentFormRef = ref()

// MCP接入点相关
const showMCPDialog = ref(false)
const mcpLoading = ref(false)
const mcpEndpointData = ref({
  endpoint: ''
})

// MCP工具相关
const toolsLoading = ref(false)
const mcpTools = ref([])
const currentAgentId = ref(null)
const callingTool = ref(false)
const mcpCallResult = ref('')
const mcpCallForm = ref({ tool_name: '', argumentsText: '{}' })
const showOpenClawDialog = ref(false)
const openClawEndpointLoading = ref(false)
const openClawEndpointData = ref({
  endpoint: '',
  connected: false,
  status: 'unknown',
  status_message: ''
})
const openClawChatTesting = ref(false)
const openClawChatTestResult = ref('')
const openClawChatTestForm = ref({
  message: ''
})
const openClawStatusText = computed(() => {
  const status = String(openClawEndpointData.value.status || '').toLowerCase()
  if (status === 'online') return '已连接'
  if (status === 'offline') return '未连接'
  return '状态未知'
})
const openClawStatusTagType = computed(() => {
  const status = String(openClawEndpointData.value.status || '').toLowerCase()
  if (status === 'online') return 'success'
  if (status === 'offline') return 'danger'
  return 'info'
})
const openClawCommandData = computed(() => buildOpenClawCommands(openClawEndpointData.value.endpoint))
const openClawCommandDisplayText = computed(() => {
  if (openClawCommandData.value.ready) {
    return openClawCommandData.value.copyText
  }
  if (!editingAgent.value?.id) {
    return '新建智能体时尚未生成安装命令，保存后可查看。'
  }
  return '暂无安装命令，请刷新后重试。'
})
const OPENCLAW_DEFAULT_ENTER_KEYWORDS = ['打开龙虾', '进入龙虾']
const OPENCLAW_DEFAULT_EXIT_KEYWORDS = ['关闭龙虾', '退出龙虾']
const openClawDocURL = 'https://github.com/hackers365/xiaozhi-esp32-server-golang/blob/main/doc/openclaw_integration.md'

const agentForm = ref({
  user_id: null,
  name: '',
  custom_prompt: '',
  llm_config_id: null,
  tts_config_id: null,
  asr_speed: 'normal',
  memory_mode: 'short',
  openclaw_allowed: false,
  openclaw_enter_keywords: [...OPENCLAW_DEFAULT_ENTER_KEYWORDS],
  openclaw_exit_keywords: [...OPENCLAW_DEFAULT_EXIT_KEYWORDS],
  status: 'active'
})

const agentRules = {
  user_id: [{ required: true, message: '请输入用户ID', trigger: 'blur' }],
  name: [{ required: true, message: '请输入智能体昵称', trigger: 'blur' }],
  asr_speed: [{ required: true, message: '请选择语音识别速度', trigger: 'change' }],
  memory_mode: [{ required: true, message: '请选择记忆模式', trigger: 'change' }],
  status: [{ required: true, message: '请选择状态', trigger: 'change' }]
}

const loadAgents = async () => {
  loading.value = true
  try {
    const response = await api.get('/admin/agents')
    agents.value = response.data.data || []
  } catch (error) {
    ElMessage.error('加载智能体列表失败')
    console.error('Error loading agents:', error)
  } finally {
    loading.value = false
  }
}

const loadConfigs = async () => {
  try {
    const [llmResponse, ttsResponse] = await Promise.all([
      api.get('/admin/llm-configs'),
      api.get('/admin/tts-configs')
    ])
    llmConfigs.value = llmResponse.data.data || []
    ttsConfigs.value = ttsResponse.data.data || []
    
    // 对配置进行排序，默认配置排在前面
    llmConfigs.value.sort((a, b) => {
      if (a.is_default && !b.is_default) return -1
      if (!a.is_default && b.is_default) return 1
      return a.name.localeCompare(b.name)
    })
    
    ttsConfigs.value.sort((a, b) => {
      if (a.is_default && !b.is_default) return -1
      if (!a.is_default && b.is_default) return 1
      return a.name.localeCompare(b.name)
    })
  } catch (error) {
    console.error('Error loading configs:', error)
  }
}

const editAgent = (agent) => {
  editingAgent.value = agent
  const openclawConfig = parseOpenClawConfigFromAgent(agent)
  agentForm.value = {
    user_id: agent.user_id,
    name: agent.name,
    custom_prompt: agent.custom_prompt || '',
    llm_config_id: agent.llm_config_id,
    tts_config_id: agent.tts_config_id,
    asr_speed: agent.asr_speed || 'normal',
    memory_mode: agent.memory_mode || 'short',
    openclaw_allowed: !!openclawConfig.allowed,
    openclaw_enter_keywords: normalizeKeywordList(openclawConfig.enter_keywords),
    openclaw_exit_keywords: normalizeKeywordList(openclawConfig.exit_keywords),
    status: agent.status
  }
  showAddDialog.value = true
  openClawEndpointData.value = {
    endpoint: '',
    connected: false,
    status: 'unknown',
    status_message: ''
  }
}

const saveAgent = async () => {
  if (!agentFormRef.value) return
  
  const valid = await agentFormRef.value.validate().catch(() => false)
  if (!valid) return

  saving.value = true
  try {
    const payload = {
      ...agentForm.value,
      openclaw: {
        allowed: !!agentForm.value.openclaw_allowed,
        enter_keywords: normalizeKeywordList(agentForm.value.openclaw_enter_keywords),
        exit_keywords: normalizeKeywordList(agentForm.value.openclaw_exit_keywords)
      }
    }
    delete payload.openclaw_allowed
    delete payload.openclaw_enter_keywords
    delete payload.openclaw_exit_keywords

    if (editingAgent.value) {
      await api.put(`/admin/agents/${editingAgent.value.id}`, payload)
      ElMessage.success('智能体更新成功')
    } else {
      await api.post('/admin/agents', payload)
      ElMessage.success('智能体添加成功')
    }
    showAddDialog.value = false
    resetForm()
    loadAgents()
  } catch (error) {
    ElMessage.error(editingAgent.value ? '智能体更新失败' : '智能体添加失败')
    console.error('Error saving agent:', error)
  } finally {
    saving.value = false
  }
}

const deleteAgent = async (agent) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除智能体 "${agent.name}" 吗？`,
      '确认删除',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    
    await api.delete(`/admin/agents/${agent.id}`)
    ElMessage.success('智能体删除成功')
    loadAgents()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('智能体删除失败')
      console.error('Error deleting agent:', error)
    }
  }
}

const resetForm = () => {
  editingAgent.value = null
  openClawEndpointData.value = {
    endpoint: '',
    connected: false,
    status: 'unknown',
    status_message: ''
  }
  openClawChatTestResult.value = ''
  openClawChatTestForm.value.message = ''
  agentForm.value = {
    user_id: null,
    name: '',
    custom_prompt: '',
    llm_config_id: null,
    tts_config_id: null,
    asr_speed: 'normal',
    memory_mode: 'short',
    openclaw_allowed: false,
    openclaw_enter_keywords: [...OPENCLAW_DEFAULT_ENTER_KEYWORDS],
    openclaw_exit_keywords: [...OPENCLAW_DEFAULT_EXIT_KEYWORDS],
    status: 'active'
  }
  
  // 为新建智能体自动选择默认配置
  if (!editingAgent.value) {
    const defaultLlmConfig = llmConfigs.value.find(config => config.is_default)
    const defaultTtsConfig = ttsConfigs.value.find(config => config.is_default)
    
    if (defaultLlmConfig) {
      agentForm.value.llm_config_id = defaultLlmConfig.config_id
    }
    if (defaultTtsConfig) {
      agentForm.value.tts_config_id = defaultTtsConfig.config_id
    }
  }
  
  if (agentFormRef.value) {
    agentFormRef.value.resetFields()
  }
}

const getASRSpeedText = (speed) => {
  const speedMap = {
    'normal': '正常',
    'patient': '耐心',
    'fast': '快速'
  }
  return speedMap[speed] || '正常'
}

const getASRSpeedType = (speed) => {
  const typeMap = {
    'normal': '',
    'patient': 'warning',
    'fast': 'success'
  }
  return typeMap[speed] || ''
}

const getMemoryModeText = (mode) => {
  const modeMap = {
    none: '无记忆',
    short: '短记忆',
    long: '长记忆'
  }
  return modeMap[mode] || '短记忆'
}

const getMemoryModeType = (mode) => {
  const typeMap = {
    none: 'info',
    short: '',
    long: 'success'
  }
  return typeMap[mode] || ''
}

const normalizeKeywordList = (keywords) => {
  if (!Array.isArray(keywords)) return []
  const unique = []
  const seen = new Set()
  for (const item of keywords) {
    const keyword = String(item || '').trim()
    if (!keyword || seen.has(keyword)) continue
    seen.add(keyword)
    unique.push(keyword)
  }
  return unique
}

const buildDefaultOpenClawConfig = () => ({
  allowed: false,
  enter_keywords: [...OPENCLAW_DEFAULT_ENTER_KEYWORDS],
  exit_keywords: [...OPENCLAW_DEFAULT_EXIT_KEYWORDS]
})

const normalizeOpenClawConfig = (raw) => {
  const enterKeywords = normalizeKeywordList(raw?.enter_keywords)
  const exitKeywords = normalizeKeywordList(raw?.exit_keywords)
  return {
    allowed: !!raw?.allowed,
    enter_keywords: enterKeywords.length > 0 ? enterKeywords : [...OPENCLAW_DEFAULT_ENTER_KEYWORDS],
    exit_keywords: exitKeywords.length > 0 ? exitKeywords : [...OPENCLAW_DEFAULT_EXIT_KEYWORDS]
  }
}

const parseOpenClawConfigFromAgent = (agent) => {
  if (agent && agent.openclaw && typeof agent.openclaw === 'object') {
    return normalizeOpenClawConfig(agent.openclaw)
  }

  if (!agent || !agent.openclaw_config || typeof agent.openclaw_config !== 'string') {
    return buildDefaultOpenClawConfig()
  }

  try {
    const parsed = JSON.parse(agent.openclaw_config)
    if (parsed && typeof parsed === 'object') {
      return normalizeOpenClawConfig(parsed)
    }
  } catch (_) {
    // ignore invalid payload
  }

  return buildDefaultOpenClawConfig()
}

const fetchOpenClawEndpoint = async () => {
  if (!editingAgent.value?.id) {
    openClawEndpointData.value = {
      endpoint: '',
      connected: false,
      status: 'unknown',
      status_message: '新建智能体时尚未生成接入点，保存后可查看。'
    }
    return
  }
  openClawEndpointLoading.value = true
  try {
    const response = await api.get(`/admin/agents/${editingAgent.value.id}/openclaw-endpoint`)
    const data = response.data?.data || {}
    const connected = !!data.connected
    const status = String(data.status || '').trim().toLowerCase()
    openClawEndpointData.value.endpoint = data.endpoint || ''
    openClawEndpointData.value.connected = connected
    openClawEndpointData.value.status = status || (connected ? 'online' : 'offline')
    openClawEndpointData.value.status_message = typeof data.status_message === 'string' ? data.status_message : ''
  } catch (error) {
    console.error('获取OpenClaw接入点失败:', error)
    openClawEndpointData.value.endpoint = ''
    openClawEndpointData.value.connected = false
    openClawEndpointData.value.status = 'unknown'
    openClawEndpointData.value.status_message = error.response?.data?.error || ''
    ElMessage.error('获取OpenClaw接入点失败')
  } finally {
    openClawEndpointLoading.value = false
  }
}

const copyOpenClawCommands = async () => {
  const commands = openClawCommandData.value.copyText
  if (!commands) {
    ElMessage.warning('暂无可复制的 OpenClaw 角色配置命令')
    return
  }
  try {
    await navigator.clipboard.writeText(commands)
    ElMessage.success('OpenClaw 角色配置命令已复制')
  } catch (error) {
    console.error('复制 OpenClaw 角色配置命令失败:', error)
    ElMessage.error('复制失败，请手动复制')
  }
}

const showOpenClawSettings = async () => {
  showOpenClawDialog.value = true
  openClawChatTestResult.value = ''
  if (editingAgent.value?.id) {
    await fetchOpenClawEndpoint()
  } else {
    openClawEndpointData.value = {
      endpoint: '',
      connected: false,
      status: 'unknown',
      status_message: '新建智能体时尚未生成接入点，保存后可查看。'
    }
  }
}

const formatOpenClawChatResult = (reply, latency) => {
  const lines = [`回复: ${String(reply || '') || '(空)'}`]
  if (Number.isFinite(latency)) {
    lines.push(`耗时: ${latency}ms`)
  }
  return lines.join('\n')
}

const testOpenClawChat = async () => {
  if (!editingAgent.value?.id) {
    ElMessage.warning('请先保存智能体后再测试')
    return
  }

  const message = String(openClawChatTestForm.value.message || '').trim()
  if (!message) {
    ElMessage.warning('请输入测试消息')
    return
  }

  openClawChatTesting.value = true
  openClawChatTestResult.value = '连接中...'
  try {
    const requestTimeoutMs = 610000
    const timeoutMs = 600000
    const token = String(localStorage.getItem('token') || '')
    const chunks = []
    let finalData = null
    let streamError = ''

    const normalizePayload = (payload) => (payload && typeof payload === 'object' ? payload : {})

    const response = await postJSONWithSSE({
      url: `/api/admin/agents/${editingAgent.value.id}/openclaw-chat-test?stream=1`,
      body: {
        message,
        timeout_ms: timeoutMs
      },
      timeoutMs: requestTimeoutMs,
      token,
      onEvent: (event, payload) => {
        const envelope = normalizePayload(payload)
        if (event === 'start') {
          openClawChatTestResult.value = '已连接，等待回复...'
          return
        }
        if (event === 'chunk') {
          const data = normalizePayload(envelope.data)
          const chunk = typeof data.chunk === 'string' ? data.chunk : ''
          if (chunk) {
            chunks.push(chunk)
          }
          const reply = String(data.reply || chunks.join(''))
          const latency = Number(data.latency_ms)
          openClawChatTestResult.value = `流式回复中...\n${formatOpenClawChatResult(reply, latency)}`
          return
        }
        if (event === 'result') {
          finalData = normalizePayload(envelope.data)
          const reply = String(finalData.reply || chunks.join(''))
          const latency = Number(finalData.latency_ms)
          openClawChatTestResult.value = formatOpenClawChatResult(reply, latency)
          return
        }
        if (event === 'error') {
          const data = normalizePayload(envelope.data)
          const messageText = String(envelope.error || data.error || 'OpenClaw对话测试失败')
          const partialReply = String(data.reply || chunks.join(''))
          streamError = messageText
          openClawChatTestResult.value = partialReply
            ? `错误: ${messageText}\n已接收: ${partialReply}`
            : `错误: ${messageText}`
          return
        }
        if (event === 'done') {
          if (!finalData) {
            finalData = normalizePayload(envelope.data)
          }
          if (envelope.ok === false && !streamError) {
            streamError = 'OpenClaw对话测试失败'
          }
        }
      }
    })

    if (response.mode === 'json') {
      const data = response.payload?.data || {}
      const reply = String(data.reply || '')
      const latency = Number(data.latency_ms)
      openClawChatTestResult.value = formatOpenClawChatResult(reply, latency)
      ElMessage.success('OpenClaw对话测试成功')
      return
    }

    if (streamError) {
      throw new Error(streamError)
    }

    if (finalData && typeof finalData === 'object') {
      const reply = String(finalData.reply || chunks.join(''))
      const latency = Number(finalData.latency_ms)
      openClawChatTestResult.value = formatOpenClawChatResult(reply, latency)
    } else if (chunks.length > 0) {
      openClawChatTestResult.value = formatOpenClawChatResult(chunks.join(''), Number.NaN)
    } else {
      throw new Error('未收到OpenClaw返回内容')
    }

    ElMessage.success('OpenClaw对话测试成功')
  } catch (error) {
    const msg = error.response?.data?.error || error.message || 'OpenClaw对话测试失败'
    openClawChatTestResult.value = `错误: ${msg}`
    ElMessage.error(msg)
  } finally {
    openClawChatTesting.value = false
    await fetchOpenClawEndpoint()
  }
}

// 显示MCP接入点
const showMCPEndpoint = async (agent) => {
  showMCPDialog.value = true
  mcpLoading.value = true
  currentAgentId.value = agent.id
  mcpCallResult.value = ""
  mcpCallForm.value = { tool_name: "", argumentsText: "{}" }
  
  try {
    const response = await api.get(`/admin/agents/${agent.id}/mcp-endpoint`)
    mcpEndpointData.value = response.data.data
    
    // 自动刷新工具列表
    await refreshMcpTools()
  } catch (error) {
    ElMessage.error('获取MCP接入点失败')
    console.error('Error getting MCP endpoint:', error)
    showMCPDialog.value = false
  } finally {
    mcpLoading.value = false
  }
}

// 刷新MCP工具列表
const refreshMcpTools = async () => {
  if (!currentAgentId.value) {
    ElMessage.warning('未选择智能体')
    return
  }
  
  toolsLoading.value = true
  try {
    const response = await api.get(`/admin/agents/${currentAgentId.value}/mcp-tools`)
    if (response.data.data && response.data.data.tools) {
      mcpTools.value = response.data.data.tools
      if (mcpTools.value.length > 0) {
        if (!mcpCallForm.value.tool_name) {
          mcpCallForm.value.tool_name = mcpTools.value[0].name
        }
        updateMcpExampleByTool(mcpCallForm.value.tool_name)
      }
      ElMessage.success(`成功获取 ${mcpTools.value.length} 个工具`)
    } else {
      mcpTools.value = []
      ElMessage.info('未找到工具数据')
    }
  } catch (error) {
    ElMessage.error('获取工具列表失败: ' + (error.response?.data?.error || error.message))
    console.error('Error refreshing MCP tools:', error)
    mcpTools.value = []
  } finally {
    toolsLoading.value = false
  }
}







const buildExampleFromSchema = (schema = {}) => {
  if (!schema || typeof schema !== 'object') return {}
  if (Array.isArray(schema.enum) && schema.enum.length > 0) return schema.enum[0]

  const type = schema.type || 'object'
  if (type === 'object') {
    const props = schema.properties || {}
    const result = {}
    Object.keys(props).sort().forEach((key) => {
      result[key] = buildExampleFromSchema(props[key])
    })
    return result
  }
  if (type === 'array') {
    return [buildExampleFromSchema(schema.items || {})]
  }
  if (type === 'number') return 0.1
  if (type === 'integer') return 0
  if (type === 'boolean') return false
  return ''
}

const updateMcpExampleByTool = (toolName) => {
  const selectedTool = mcpTools.value.find(item => item.name === toolName)
  if (!selectedTool) return

  const example = buildExampleFromSchema(selectedTool.input_schema || {})
  mcpCallForm.value.argumentsText = JSON.stringify(example ?? {}, null, 2)
}

const handleMcpToolChange = (toolName) => {
  updateMcpExampleByTool(toolName)
}

const formatMcpCallResult = (payload) => {
  const MAX_PARSE_DEPTH = 8

  const tryParseJSONString = (value) => {
    if (typeof value !== 'string') return { parsed: false, value }
    let text = value.trim()
    if (!text) return { parsed: false, value }

    const fenced = text.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i)
    if (fenced) {
      text = fenced[1].trim()
    }

    const looksLikeJSON =
      (text.startsWith('{') && text.endsWith('}')) ||
      (text.startsWith('[') && text.endsWith(']'))
    if (!looksLikeJSON) return { parsed: false, value }

    try {
      return { parsed: true, value: JSON.parse(text) }
    } catch (_) {
      return { parsed: false, value }
    }
  }

  const deepParseJSONStrings = (value, depth = 0) => {
    if (depth >= MAX_PARSE_DEPTH || value == null) return value

    if (typeof value === 'string') {
      const parsed = tryParseJSONString(value)
      if (!parsed.parsed) return value
      return deepParseJSONStrings(parsed.value, depth + 1)
    }

    if (Array.isArray(value)) {
      return value.map((item) => deepParseJSONStrings(item, depth + 1))
    }

    if (typeof value === 'object') {
      const out = {}
      Object.keys(value).forEach((key) => {
        out[key] = deepParseJSONStrings(value[key], depth + 1)
      })

      if (Array.isArray(out.content) && out.content.length === 1) {
        const first = out.content[0]
        if (first && typeof first === 'object' && !Array.isArray(first) && first.type === 'text' && Object.prototype.hasOwnProperty.call(first, 'text')) {
          const textValue = first.text
          if (textValue && typeof textValue === 'object') {
            return textValue
          }
        }
      }

      return out
    }

    return value
  }

  const data = payload ?? {}
  const raw = (data && typeof data === 'object' && !Array.isArray(data) && Object.prototype.hasOwnProperty.call(data, 'result'))
    ? data.result
    : data

  return JSON.stringify(deepParseJSONStrings(raw), null, 2)
}

const callAgentMcpTool = async () => {
  if (!currentAgentId.value || !mcpCallForm.value.tool_name) {
    ElMessage.warning('请选择工具')
    return
  }

  let argumentsObj = {}
  try {
    argumentsObj = mcpCallForm.value.argumentsText ? JSON.parse(mcpCallForm.value.argumentsText) : {}
  } catch (e) {
    ElMessage.error('参数JSON格式错误')
    return
  }

  callingTool.value = true
  try {
    const response = await api.post(`/admin/agents/${currentAgentId.value}/mcp-call`, {
      tool_name: mcpCallForm.value.tool_name,
      arguments: argumentsObj
    })
    mcpCallResult.value = formatMcpCallResult(response.data.data || {})
    ElMessage.success('MCP工具调用成功')
  } catch (error) {
    mcpCallResult.value = JSON.stringify(error.response?.data || { error: error.message }, null, 2)
    ElMessage.error('MCP工具调用失败')
  } finally {
    callingTool.value = false
  }
}

// 复制MCP接入点URL
const copyMCPEndpoint = async () => {
  try {
    await navigator.clipboard.writeText(mcpEndpointData.value.endpoint)
    ElMessage.success('MCP接入点URL已复制到剪贴板')
  } catch (error) {
    ElMessage.error('复制失败')
    console.error('Error copying to clipboard:', error)
  }
}

onMounted(() => {
  loadAgents()
  loadConfigs()
})
</script>

<style scoped>
.admin-agents {
  padding: 20px;
}

.page-header {
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0 0 8px 0;
  color: #303133;
  font-size: 24px;
  font-weight: 600;
}

.page-subtitle {
  margin: 0;
  color: #909399;
  font-size: 14px;
}

.toolbar {
  margin-bottom: 20px;
  display: flex;
  gap: 12px;
}

.mcp-endpoint-display {
  margin: 20px 0;
}

.openclaw-status-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.openclaw-status-message {
  margin-bottom: 12px;
  color: #6b7280;
  font-size: 12px;
  line-height: 1.4;
}

.openclaw-tip-row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 12px;
}

.openclaw-tip-title {
  font-size: 13px;
  color: #6b7280;
}

.openclaw-tip-icon {
  font-size: 16px;
  color: #1d4ed8;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
  border-radius: 999px;
  padding: 2px;
  cursor: help;
  transition: all 0.2s ease;
}

.openclaw-tip-icon:hover {
  color: #1e40af;
  background: #dbeafe;
  border-color: #93c5fd;
}

.openclaw-tip-content {
  max-width: 420px;
  color: #111827;
  font-size: 12px;
  line-height: 1.7;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.openclaw-tip-content code {
  background: #f3f4f6;
  border-radius: 4px;
  padding: 0 4px;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
}

:deep(.openclaw-tip-popper) {
  max-width: 460px;
  background: #ffffff !important;
  border: 1px solid #dbeafe !important;
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12) !important;
}

:deep(.openclaw-tip-popper .el-popper__arrow::before) {
  background: #ffffff !important;
  border: 1px solid #dbeafe !important;
}

:deep(.openclaw-tip-popper .el-link) {
  color: #2563eb !important;
}

.endpoint-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}

.endpoint-header .endpoint-label {
  margin-bottom: 0;
}

.endpoint-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.endpoint-label {
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  margin-bottom: 8px;
}

.openclaw-command-hint {
  margin-bottom: 8px;
  color: #6b7280;
  font-size: 12px;
}

.openclaw-command-steps {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.openclaw-command-step-title {
  margin-bottom: 6px;
  color: #374151;
  font-size: 13px;
  font-weight: 500;
}

.openclaw-command-content {
  margin: 0;
  padding: 12px 16px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 13px;
  color: #1e293b;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-all;
}

.endpoint-content {
  padding: 12px 16px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
  font-size: 13px;
  color: #1e293b;
  word-break: break-all;
  line-height: 1.5;
  min-height: 60px;
  display: flex;
  align-items: center;
}

.mcp-tools-section {
  margin-top: 24px;
  border-top: 1px solid #e2e8f0;
  padding-top: 20px;
}

.tools-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.tools-title {
  font-size: 16px;
  font-weight: 600;
  color: #374151;
}



.tools-empty {
  margin: 20px 0;
  text-align: center;
}

.tools-list {
  margin-top: 16px;
}

.tools-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 16px;
}

.tool-tag {
  position: relative;
  padding: 8px 16px;
  font-size: 14px;
  border-radius: 20px;
  cursor: pointer;
  transition: all 0.3s ease;
}

.tool-tag:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}

.tool-info-icon {
  margin-left: 6px;
  font-size: 12px;
  opacity: 0.7;
}

.mcp-result-box {
  margin-top: 12px;
  white-space: pre-wrap;
  font-family: monospace;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 10px;
  min-height: 80px;
}

.tool-tag:hover .tool-info-icon {
  opacity: 1;
}
</style>
