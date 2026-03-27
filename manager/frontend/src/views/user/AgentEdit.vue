<template>
  <div class="agent-config">
    <div class="config-header">
      <div class="header-left">
        <el-button 
          @click="$router.back()" 
          :icon="ArrowLeft" 
          circle 
          size="large"
        />
        <h1>智能体配置</h1>
      </div>
      <el-button type="primary" @click="handleSave" :loading="saving" size="large">
        保存配置
      </el-button>
    </div>

    <div class="config-content">
      <div class="config-form">
        <!-- 角色快捷选择 -->
        <div class="form-section quick-config-section" v-if="hasAvailableRoles">
          <h3 class="section-title">
            快速配置
            <el-tooltip content="点击角色可快速应用其配置到智能体" placement="top">
              <el-icon class="help-icon"><QuestionFilled /></el-icon>
            </el-tooltip>
          </h3>

          <div class="role-selector role-selector-compact" v-loading="rolesLoading">
            <div class="role-inline-line role-inline-line-compact">
              <button
                v-for="role in allRoles"
                :key="role.id"
                type="button"
                class="role-inline-item"
                :class="{ active: selectedRoleId === role.id }"
                @click="applyRoleConfig(role)"
              >
                <span class="role-inline-name">{{ role.name }}</span>
                <span class="role-inline-type" :class="role.role_type === 'global' ? 'global' : 'user'">
                  {{ role.role_type === 'global' ? '全局' : '我的' }}
                </span>
              </button>
            </div>
            <div class="form-help quick-config-help">
              角色名称已平铺展示，点击任意角色会立即填充 Prompt、LLM、TTS 和音色配置（不会自动保存）
            </div>
          </div>
        </div>

        <!-- 基础信息 -->
        <div class="form-section">
          <h3 class="section-title">基础信息</h3>
          
          <div class="form-group">
            <label class="form-label">昵称</label>
            <el-input 
              v-model="form.name" 
              placeholder="请输入智能体昵称" 
              size="large"
              :maxlength="50"
              show-word-limit
            />
          </div>

          <div class="form-group">
            <label class="form-label">角色介绍(prompt)</label>
            <el-input
              v-model="form.custom_prompt"
              type="textarea"
              :rows="4"
              placeholder="请输入角色介绍/系统提示词，这将影响AI的回答风格和个性"
              :maxlength="10000"
              show-word-limit
            />
          </div>
        </div>

        <!-- 配置设置 -->
        <div class="form-section">
          <h3 class="section-title">配置设置</h3>
          
          <div class="form-group">
            <label class="form-label">语言模型</label>
            <el-select 
              v-model="form.llm_config_id" 
              placeholder="请选择语言模型" 
              size="large" 
              style="width: 100%"
              clearable
            >
              <el-option
                v-for="llmConfig in llmConfigs"
                :key="llmConfig.config_id"
                :label="llmConfig.is_default ? `${llmConfig.name} (默认)` : llmConfig.name"
                :value="llmConfig.config_id"
              >
                <div class="config-option">
                  <span class="config-name">
                    {{ llmConfig.name }}
                    <el-tag v-if="llmConfig.is_default" type="success" size="small" style="margin-left: 8px;">默认</el-tag>
                  </span>
                  <span class="config-desc">{{ llmConfig.provider || '暂无描述' }}</span>
                </div>
              </el-option>
            </el-select>
            <div class="form-help" v-if="getCurrentLlmConfigName()">
              {{ getCurrentLlmConfigInfo() }}
            </div>
          </div>

          <div class="form-group" v-if="myCloneVoices.length > 0">
            <label class="form-label">我复刻的音色</label>
            <div class="clone-voice-line" v-loading="cloneVoicesLoading">
              <button
                v-for="clone in myCloneVoices"
                :key="clone.id"
                type="button"
                class="clone-voice-item"
                :class="{ active: isCloneVoiceSelected(clone) }"
                :title="`${clone.tts_config_name || clone.tts_config_id} · ${clone.provider_voice_id}`"
                @click="applyCloneVoice(clone)"
              >
                <span class="clone-voice-name">{{ clone.name || clone.provider_voice_id }}</span>
              </button>
            </div>
            <div class="form-help">点击后会自动填充 TTS 配置和音色</div>
          </div>

          <div class="form-group">
            <label class="form-label">TTS配置</label>
            <el-select 
              v-model="form.tts_config_id" 
              placeholder="请选择TTS配置" 
              size="large" 
              style="width: 100%"
              clearable
              @change="handleTtsConfigChange"
            >
              <el-option
                v-for="ttsConfig in ttsConfigs"
                :key="ttsConfig.config_id"
                :label="ttsConfig.is_default ? `${ttsConfig.name} (默认)` : ttsConfig.name"
                :value="ttsConfig.config_id"
              >
                <div class="config-option">
                  <span class="config-name">
                    {{ ttsConfig.name }}
                    <el-tag v-if="ttsConfig.is_default" type="success" size="small" style="margin-left: 8px;">默认</el-tag>
                  </span>
                  <span class="config-desc">{{ ttsConfig.provider || '暂无描述' }}</span>
                </div>
              </el-option>
            </el-select>
            <div class="form-help" v-if="getCurrentTtsConfigName()">
              {{ getCurrentTtsConfigInfo() }}
            </div>
          </div>

          <div class="form-group" v-if="form.tts_config_id">
            <label class="form-label">音色</label>
            <el-select 
              v-model="form.voice" 
              placeholder="请选择或输入音色（支持搜索和自定义输入）" 
              size="large" 
              style="width: 100%"
              filterable
              allow-create
              default-first-option
              reserve-keyword
              clearable
              :loading="voiceLoading"
              :filter-method="filterVoice"
            >
              <el-option
                v-for="voice in filteredVoices"
                :key="voice.value"
                :label="voice.label"
                :value="voice.value"
              >
                <span>{{ voice.label }}</span>
                <span style="color: #8492a6; font-size: 13px; margin-left: 8px;">{{ voice.value }}</span>
              </el-option>
            </el-select>
            <div class="form-help">
              当前TTS配置: {{ getCurrentTtsConfigName() }}，可以搜索音色名称或值，也可以手动输入自定义音色值。
            </div>
          </div>

          <div class="form-group">
            <label class="form-label">关联知识库</label>
            <el-select
              v-model="form.knowledge_base_ids"
              multiple
              collapse-tags
              collapse-tags-tooltip
              placeholder="请选择要关联的知识库（可多选）"
              size="large"
              style="width: 100%"
            >
              <el-option
                v-for="kb in knowledgeBases"
                :key="kb.id"
                :label="kb.name"
                :value="kb.id"
              />
            </el-select>
            <div class="form-help">支持多库关联。知识库检索失败时会自动降级为普通LLM对话。</div>
          </div>

          <div class="form-group">
            <label class="form-label">语音识别速度</label>
            <el-select v-model="form.asr_speed" placeholder="请选择语音识别速度" size="large" style="width: 100%">
              <el-option label="正常" value="normal" />
              <el-option label="耐心" value="patient" />
              <el-option label="快速" value="fast" />
            </el-select>
            <div class="form-help">设置语音识别的响应速度</div>
          </div>

          <div class="form-group">
            <label class="form-label">记忆</label>
            <el-select v-model="form.memory_mode" placeholder="请选择记忆模式" size="large" style="width: 100%">
              <el-option label="无记忆" value="none" />
              <el-option label="短记忆" value="short" />
              <el-option label="长记忆" value="long" />
            </el-select>
            <div class="form-help">
              无记忆: LLM不加载历史；短记忆: 加载历史不加载长记忆；长记忆: 加载历史并加载长记忆。
            </div>
          </div>

          <div class="form-group">
            <label class="form-label">OpenClaw</label>
            <el-button type="primary" size="large" style="width: 100%" @click="showOpenClawSettings">
              查看openclaw
            </el-button>
            <div class="form-help">
              已配置: {{ form.openclaw_allowed ? '开启' : '关闭' }}，进入词 {{ form.openclaw_enter_keywords.length }} 个，退出词 {{ form.openclaw_exit_keywords.length }} 个。
            </div>
          </div>

          <div class="form-group" v-loading="mcpServiceOptionsLoading">
            <label class="form-label">MCP服务</label>
            <el-select
              v-model="selectedMcpServices"
              multiple
              filterable
              collapse-tags
              collapse-tags-tooltip
              clearable
              size="large"
              style="width: 100%"
              placeholder="留空则使用全部已启用服务"
              @change="handleMcpServiceSelectionChange"
            >
              <el-option
                v-for="serviceName in mcpServiceOptions"
                :key="serviceName"
                :label="serviceName"
                :value="serviceName"
              />
            </el-select>
            <div class="form-help">
              留空表示使用全部已启用全局MCP服务，当前可选 {{ mcpServiceOptions.length }} 个服务。
            </div>
          </div>

          <div class="form-group">
            <label class="form-label">MCP接入点</label>
            <el-button 
              type="primary" 
              @click="showMCPEndpoint" 
              size="large"
              style="width: 100%"
            >
              查看MCP接入点
            </el-button>
            <div class="form-help">获取智能体的MCP WebSocket接入点URL，可用于设备连接</div>
          </div>
        </div>
      </div>
    </div>

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
        <div class="mcp-result-box">{{ mcpCallResult || '暂无调用结果' }}</div>
      </div>

      <template #footer>
        <el-button @click="showMCPDialog = false">关闭</el-button>
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
                <div>角色配置：在 OpenClaw 控制台角色配置中使用下方三条命令，系统会自动填入当前服务的 URL 与 JWT token。</div>
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
            <el-switch v-model="form.openclaw_allowed" />
          </el-form-item>
          <el-form-item label="进入关键词">
            <el-select
              v-model="form.openclaw_enter_keywords"
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
              v-model="form.openclaw_exit_keywords"
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
                <el-button size="small" @click="fetchOpenClawEndpoint" :loading="openClawEndpointLoading">刷新</el-button>
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
          description="向openclaw发送一条文本，验证连通与回复。"
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
        >
          发送测试
        </el-button>
        <div class="mcp-result-box">{{ openClawChatTestResult || '暂无测试结果' }}</div>
      </div>
      <template #footer>
        <el-button @click="showOpenClawDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, VideoPlay, Refresh, InfoFilled, QuestionFilled } from '@element-plus/icons-vue'
import api from '@/utils/api'
import { postJSONWithSSE } from '@/utils/sse'
import { buildOpenClawCommands } from '@/utils/openclaw'

const route = useRoute()
const router = useRouter()
const saving = ref(false)
const applyingRoleConfig = ref(false)

// 角色相关数据
const globalRoles = ref([])
const userRoles = ref([])
const selectedRoleId = ref(null)
const rolesLoading = ref(false)

const isRoleEnabled = (role) => role?.status === "active" || !role?.status

// 计算所有角色列表（用于选择器）
const allRoles = computed(() => {
  return [...globalRoles.value, ...userRoles.value].filter(isRoleEnabled)
})
const hasAvailableRoles = computed(() => allRoles.value.length > 0)
const OPENCLAW_DEFAULT_ENTER_KEYWORDS = ['打开龙虾', '进入龙虾']
const OPENCLAW_DEFAULT_EXIT_KEYWORDS = ['关闭龙虾', '退出龙虾']
const openClawDocURL = 'https://github.com/hackers365/xiaozhi-esp32-server-golang/blob/main/doc/openclaw_integration.md'

// 表单数据
const form = reactive({
  name: '',
  custom_prompt: '',
  llm_config_id: null,
  tts_config_id: null,
  voice: null,
  asr_speed: 'normal',
  knowledge_base_ids: [],
  memory_mode: 'short',
  mcp_service_names: '',
  openclaw_allowed: false,
  openclaw_enter_keywords: [...OPENCLAW_DEFAULT_ENTER_KEYWORDS],
  openclaw_exit_keywords: [...OPENCLAW_DEFAULT_EXIT_KEYWORDS]
})

// LLM配置数据
const llmConfigs = ref([])

// TTS配置数据
const ttsConfigs = ref([])

// 知识库数据
const knowledgeBases = ref([])

const loadKnowledgeBases = async () => {
  try {
    const response = await api.get('/user/knowledge-bases')
    knowledgeBases.value = response.data.data || []
  } catch (error) {
    console.error('加载知识库失败:', error)
  }
}

// 音色相关数据
const availableVoices = ref([])
const filteredVoices = ref([])
const voiceSearchKeyword = ref('')
const voiceLoading = ref(false)
const previousTtsConfigId = ref(null) // 用于跟踪TTS配置变化
const myCloneVoices = ref([])
const cloneVoicesLoading = ref(false)

// MCP服务选择
const mcpServiceOptions = ref([])
const selectedMcpServices = ref([])
const mcpServiceOptionsLoading = ref(false)

// MCP接入点相关
const showMCPDialog = ref(false)
const mcpLoading = ref(false)
const mcpEndpointData = ref({
  endpoint: ''
})
const toolsLoading = ref(false)
const mcpTools = ref([])
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
  return '暂无安装命令，请刷新后重试。'
})

// 加载LLM配置
const loadLlmConfigs = async () => {
  try {
    const response = await api.get('/user/llm-configs')
    llmConfigs.value = response.data.data || []
    // 不在这里自动选择默认配置，交给具体的使用场景处理
  } catch (error) {
    console.error('加载LLM配置失败:', error)
  }
}

// 加载TTS配置
const loadTtsConfigs = async () => {
  try {
    const response = await api.get('/user/tts-configs')
    ttsConfigs.value = response.data.data || []
    // 不在这里自动选择默认配置，交给具体的使用场景处理
  } catch (error) {
    console.error('加载TTS配置失败:', error)
  }
}



// 加载智能体数据
const loadAgent = async () => {
  try {
    const response = await api.get(`/user/agents/${route.params.id}`)
    const agent = response.data.data
    const openclawConfig = parseOpenClawConfigFromAgent(agent)
    
    // 映射基本字段
    Object.assign(form, {
      name: agent.name || '',
      custom_prompt: agent.custom_prompt || '',
      asr_speed: agent.asr_speed || 'normal',
      voice: agent.voice || null,
      knowledge_base_ids: agent.knowledge_base_ids || [],
      memory_mode: agent.memory_mode || 'short',
      mcp_service_names: agent.mcp_service_names || '',
      openclaw_allowed: !!openclawConfig.allowed,
      openclaw_enter_keywords: normalizeKeywordList(openclawConfig.enter_keywords),
      openclaw_exit_keywords: normalizeKeywordList(openclawConfig.exit_keywords)
    })
    selectedMcpServices.value = normalizeMcpServiceNames((form.mcp_service_names || '').split(','))
    syncMcpServiceNamesToForm()
    
    // 处理LLM配置关联
    const hasValidLlmConfigId = agent.llm_config_id && 
                               agent.llm_config_id !== '' && 
                               agent.llm_config_id !== 'null' && 
                               agent.llm_config_id !== 'undefined'
    
    if (hasValidLlmConfigId) {
      // 验证config_id是否在可用配置中
      const llmConfig = llmConfigs.value.find(config => config.config_id === agent.llm_config_id)
      if (llmConfig) {
        form.llm_config_id = agent.llm_config_id
        console.log(`✅ 智能体使用LLM配置: ${llmConfig.name}`)
      } else {
        console.warn(`⚠️ 智能体的LLM配置ID ${agent.llm_config_id} 不存在，将使用默认配置`)
        // 如果config_id无效，使用默认配置
        const defaultLlmConfig = llmConfigs.value.find(config => config.is_default)
        form.llm_config_id = defaultLlmConfig ? defaultLlmConfig.config_id : null
        if (defaultLlmConfig) {
          console.log(`🔄 已切换到默认LLM配置: ${defaultLlmConfig.name}`)
        }
      }
    } else {
      // 如果没有配置，使用默认配置
      const defaultLlmConfig = llmConfigs.value.find(config => config.is_default)
      form.llm_config_id = defaultLlmConfig ? defaultLlmConfig.config_id : null
      if (defaultLlmConfig) {
        console.log(`🎯 智能体LLM配置为空，使用默认配置: ${defaultLlmConfig.name}`)
      } else {
        console.warn(`❌ 没有找到默认LLM配置`)
      }
    }
    
    // 处理TTS配置关联
    const hasValidTtsConfigId = agent.tts_config_id && 
                               agent.tts_config_id !== '' && 
                               agent.tts_config_id !== 'null' && 
                               agent.tts_config_id !== 'undefined'
    
    if (hasValidTtsConfigId) {
      // 验证config_id是否在可用配置中
      const ttsConfig = ttsConfigs.value.find(config => config.config_id === agent.tts_config_id)
      if (ttsConfig) {
        form.tts_config_id = agent.tts_config_id
        console.log(`✅ 智能体使用TTS配置: ${ttsConfig.name}`)
      } else {
        console.warn(`⚠️ 智能体的TTS配置ID ${agent.tts_config_id} 不存在，将使用默认配置`)
        // 如果config_id无效，使用默认配置
        const defaultTtsConfig = ttsConfigs.value.find(config => config.is_default)
        form.tts_config_id = defaultTtsConfig ? defaultTtsConfig.config_id : null
        if (defaultTtsConfig) {
          console.log(`🔄 已切换到默认TTS配置: ${defaultTtsConfig.name}`)
        }
      }
    } else {
      // 如果没有配置，使用默认配置
      const defaultTtsConfig = ttsConfigs.value.find(config => config.is_default)
      form.tts_config_id = defaultTtsConfig ? defaultTtsConfig.config_id : null
      if (defaultTtsConfig) {
        console.log(`🎯 智能体TTS配置为空，使用默认配置: ${defaultTtsConfig.name}`)
      } else {
        console.warn(`❌ 没有找到默认TTS配置`)
      }
    }
  } catch (error) {
    console.error('加载智能体失败:', error)
    ElMessage.error('加载智能体失败')
  }
}

const normalizeMcpServiceNames = (names) => {
  if (!Array.isArray(names)) return []
  const unique = []
  const seen = new Set()
  for (const item of names) {
    const name = String(item || '').trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    unique.push(name)
  }
  return unique
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

const syncMcpServiceNamesToForm = () => {
  selectedMcpServices.value = normalizeMcpServiceNames(selectedMcpServices.value)
  form.mcp_service_names = selectedMcpServices.value.join(',')
}

const handleMcpServiceSelectionChange = (values) => {
  selectedMcpServices.value = normalizeMcpServiceNames(values || [])
  syncMcpServiceNamesToForm()
}

const loadMcpServiceOptions = async () => {
  if (!route.params.id) return

  mcpServiceOptionsLoading.value = true
  try {
    const response = await api.get(`/user/agents/${route.params.id}/mcp-services/options`)
    const data = response.data.data || {}

    mcpServiceOptions.value = Array.isArray(data.options)
      ? normalizeMcpServiceNames(data.options)
      : []

    if (Array.isArray(data.selected)) {
      selectedMcpServices.value = normalizeMcpServiceNames(data.selected)
    } else if (typeof data.mcp_service_names === 'string') {
      selectedMcpServices.value = normalizeMcpServiceNames(data.mcp_service_names.split(','))
    } else {
      selectedMcpServices.value = normalizeMcpServiceNames((form.mcp_service_names || '').split(','))
    }
    syncMcpServiceNamesToForm()
  } catch (error) {
    console.error('加载MCP服务选项失败:', error)
    ElMessage.warning('加载MCP服务选项失败')
  } finally {
    mcpServiceOptionsLoading.value = false
  }
}

const fetchOpenClawEndpoint = async () => {
  openClawEndpointLoading.value = true
  try {
    const response = await api.get(`/user/agents/${route.params.id}/openclaw-endpoint`)
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
  await fetchOpenClawEndpoint()
}

const formatOpenClawChatResult = (reply, latency) => {
  const lines = [`回复: ${String(reply || '') || '(空)'}`]
  if (Number.isFinite(latency)) {
    lines.push(`耗时: ${latency}ms`)
  }
  return lines.join('\n')
}

const testOpenClawChat = async () => {
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
      url: `/api/user/agents/${route.params.id}/openclaw-chat-test?stream=1`,
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

// 加载角色列表（全局+用户角色）
const loadRoles = async () => {
  rolesLoading.value = true
  try {
    const response = await api.get('/user/roles')
    globalRoles.value = response.data.data?.global_roles || []
    userRoles.value = response.data.data?.user_roles || []
  } catch (error) {
    console.error('加载角色列表失败:', error)
  } finally {
    rolesLoading.value = false
  }
}

const normalizeCloneStatus = (clone) => {
  const status = String(clone?.status || '').trim().toLowerCase()
  const taskStatus = String(clone?.task_status || '').trim().toLowerCase()
  if (status === 'failed' || taskStatus === 'failed') return 'failed'
  if (status === 'active' || taskStatus === 'succeeded') return 'active'
  if (taskStatus === 'queued' || taskStatus === 'processing') return taskStatus
  if (status === 'queued' || status === 'processing') return status
  return status || taskStatus || 'unknown'
}

const loadMyCloneVoices = async () => {
  cloneVoicesLoading.value = true
  try {
    const response = await api.get('/user/voice-clones')
    const cloneList = response.data.data || []
    myCloneVoices.value = cloneList
      .filter((clone) => normalizeCloneStatus(clone) === 'active')
      .filter((clone) => clone?.provider_voice_id && clone?.tts_config_id)
      .map((clone) => ({
        id: clone.id,
        name: clone.name || clone.provider_voice_id,
        provider_voice_id: clone.provider_voice_id,
        tts_config_id: clone.tts_config_id,
        tts_config_name: clone.tts_config_name || ''
      }))
  } catch (error) {
    console.error('加载复刻音色失败:', error)
    myCloneVoices.value = []
  } finally {
    cloneVoicesLoading.value = false
  }
}

const isCloneVoiceSelected = (clone) => {
  return form.tts_config_id === clone?.tts_config_id && form.voice === clone?.provider_voice_id
}

const applyCloneVoice = async (clone) => {
  if (!clone) return
  const ttsConfig = ttsConfigs.value.find(config => config.config_id === clone.tts_config_id)
  if (!ttsConfig) {
    return
  }

  form.tts_config_id = clone.tts_config_id
  await handleTtsConfigChange()
  form.voice = clone.provider_voice_id
}

// 应用角色配置到智能体表单
const applyRoleConfig = async (role) => {
  if (!role) return
  applyingRoleConfig.value = true
  try {
    selectedRoleId.value = role.id

    // 填充配置到表单
    form.custom_prompt = role.prompt || ''

    // LLM 配置
    if (role.llm_config_id) {
      const llmConfig = llmConfigs.value.find(c => c.config_id === role.llm_config_id)
      if (llmConfig) {
        form.llm_config_id = role.llm_config_id
      }
    }

    // TTS 配置
    if (role.tts_config_id) {
      const ttsConfig = ttsConfigs.value.find(c => c.config_id === role.tts_config_id)
      if (ttsConfig) {
        form.tts_config_id = role.tts_config_id
      } else {
        form.tts_config_id = null
      }
    } else {
      form.tts_config_id = null
    }

    // 按 TTS 配置刷新音色列表，再填充角色音色
    await handleTtsConfigChange()
    form.voice = role.voice || null
  } finally {
    applyingRoleConfig.value = false
  }
}

// 保存智能体
const handleSave = async () => {
  if (applyingRoleConfig.value) {
    ElMessage.info('当前仅填充角色配置，不会自动保存，请点击“保存配置”提交')
    return
  }

  if (!form.name.trim()) {
    ElMessage.error('请输入智能体昵称')
    return
  }
  
  try {
    saving.value = true
    syncMcpServiceNamesToForm()

    const payload = {
      ...form,
      openclaw: {
        allowed: !!form.openclaw_allowed,
        enter_keywords: normalizeKeywordList(form.openclaw_enter_keywords),
        exit_keywords: normalizeKeywordList(form.openclaw_exit_keywords)
      }
    }
    delete payload.openclaw_allowed
    delete payload.openclaw_enter_keywords
    delete payload.openclaw_exit_keywords

    await api.put(`/user/agents/${route.params.id}`, payload)
    
    ElMessage.success('保存成功')
    router.push('/user/agents')
  } catch (error) {
    console.error('保存失败:', error)
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}



// 获取当前LLM配置名称
const getCurrentLlmConfigName = () => {
  if (!form.llm_config_id) return null
  const config = llmConfigs.value.find(c => c.config_id === form.llm_config_id)
  return config ? config.name : null
}

// 获取当前LLM配置信息
const getCurrentLlmConfigInfo = () => {
  if (!form.llm_config_id) return ''
  const config = llmConfigs.value.find(c => c.config_id === form.llm_config_id)
  if (!config) return ''
  
  if (config.is_default) {
    return `当前使用默认LLM配置: ${config.name}`
  } else {
    return `当前使用LLM配置: ${config.name}`
  }
}

// 获取当前TTS配置名称
const getCurrentTtsConfigName = () => {
  if (!form.tts_config_id) return null
  const config = ttsConfigs.value.find(c => c.config_id === form.tts_config_id)
  return config ? config.name : null
}

// 获取当前TTS配置信息
const getCurrentTtsConfigInfo = () => {
  if (!form.tts_config_id) return ''
  const config = ttsConfigs.value.find(c => c.config_id === form.tts_config_id)
  if (!config) return ''
  
  if (config.is_default) {
    return `当前使用默认TTS配置: ${config.name}`
  } else {
    return `当前使用TTS配置: ${config.name}`
  }
}

// 自动选择默认配置
const autoSelectDefaultConfigs = () => {
  // 选择默认LLM配置
  if (!form.llm_config_id && llmConfigs.value.length > 0) {
    const defaultLlmConfig = llmConfigs.value.find(config => config.is_default)
    if (defaultLlmConfig) {
      form.llm_config_id = defaultLlmConfig.config_id
    }
  }
  
  // 选择默认TTS配置
  if (!form.tts_config_id && ttsConfigs.value.length > 0) {
    const defaultTtsConfig = ttsConfigs.value.find(config => config.is_default)
    if (defaultTtsConfig) {
      form.tts_config_id = defaultTtsConfig.config_id
    }
  }
}

// 显示MCP接入点
const showMCPEndpoint = async () => {
  showMCPDialog.value = true
  mcpLoading.value = true
  mcpCallResult.value = ""
  mcpCallForm.value = { tool_name: "", argumentsText: "{}" }
  
  try {
    const response = await api.get(`/user/agents/${route.params.id}/mcp-endpoint`)
    mcpEndpointData.value = response.data.data
    
    // 获取工具列表
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
  toolsLoading.value = true
  try {
    const response = await api.get(`/user/agents/${route.params.id}/mcp-tools`)
    mcpTools.value = response.data.data.tools || []
    if (!mcpCallForm.value.tool_name && mcpTools.value.length > 0) {
      mcpCallForm.value.tool_name = mcpTools.value[0].name
    }
  } catch (error) {
    console.error('获取MCP工具列表失败:', error)
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
  if (!mcpCallForm.value.tool_name) {
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
    const response = await api.post(`/user/agents/${route.params.id}/mcp-call`, {
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

// 处理TTS配置变化，加载对应的音色列表
const handleTtsConfigChange = async () => {
  // 获取之前的provider（如果有）
  let previousProvider = null
  if (previousTtsConfigId.value) {
    const prevConfig = ttsConfigs.value.find(config => config.config_id === previousTtsConfigId.value)
    previousProvider = prevConfig?.provider
  }
  
  if (!form.tts_config_id) {
    availableVoices.value = []
    filteredVoices.value = []
    form.voice = null // 清空音色
    previousTtsConfigId.value = null
    return
  }
  
  // 获取当前TTS配置的provider
  const ttsConfig = ttsConfigs.value.find(config => config.config_id === form.tts_config_id)
  if (!ttsConfig || !ttsConfig.provider) {
    availableVoices.value = []
    filteredVoices.value = []
    form.voice = null // 清空音色
    previousTtsConfigId.value = form.tts_config_id
    return
  }
  
  // 如果provider发生变化，清空当前的voice值
  if (previousProvider && previousProvider !== ttsConfig.provider) {
    form.voice = null
  }
  
  // 加载音色列表
  await loadVoices(ttsConfig.provider)
  
  // 如果当前voice值在新列表中不存在，也清空它
  if (form.voice && availableVoices.value.length > 0) {
    const voiceExists = availableVoices.value.some(v => v.value === form.voice)
    if (!voiceExists) {
      form.voice = null
    }
  }
  
  // 更新previousTtsConfigId
  previousTtsConfigId.value = form.tts_config_id
}

// 音色搜索过滤函数
const filterVoice = (val) => {
  voiceSearchKeyword.value = val
  if (!val) {
    filteredVoices.value = availableVoices.value
    return
  }
  
  const keyword = val.toLowerCase()
  filteredVoices.value = availableVoices.value.filter(voice => {
    // 同时搜索 label 和 value
    return voice.label.toLowerCase().includes(keyword) || 
           voice.value.toLowerCase().includes(keyword)
  })
}

// 加载音色列表
const loadVoices = async (provider) => {
  if (!provider) {
    availableVoices.value = []
    filteredVoices.value = []
    return
  }
  
  voiceLoading.value = true
  try {
    const params = { provider }
    // 如果有TTS配置ID，总是带上config_id参数
    if (form.tts_config_id) {
      params.config_id = form.tts_config_id
    }
    const response = await api.get('/user/voice-options', { params })
    availableVoices.value = response.data.data || []
    filteredVoices.value = availableVoices.value
  } catch (error) {
    console.error('加载音色列表失败:', error)
    availableVoices.value = []
    filteredVoices.value = []
  } finally {
    voiceLoading.value = false
  }
}

onMounted(async () => {
  // 先加载配置数据和角色列表
  await Promise.all([
    loadLlmConfigs(),
    loadTtsConfigs(),
    loadRoles(),
    loadKnowledgeBases(),
    loadMyCloneVoices()
  ])
  
  if (route.params.id) {
    // 编辑现有智能体，加载智能体数据
    await loadAgent()
    await loadMcpServiceOptions()
    // 如果已有TTS配置，加载对应的音色列表
    if (form.tts_config_id) {
      previousTtsConfigId.value = form.tts_config_id
      const ttsConfig = ttsConfigs.value.find(config => config.config_id === form.tts_config_id)
      if (ttsConfig && ttsConfig.provider) {
        await loadVoices(ttsConfig.provider)
      }
    }
  } else {
    // 新建智能体，自动选择默认配置
    autoSelectDefaultConfigs()
    // 如果自动选择了TTS配置，记录它
    if (form.tts_config_id) {
      previousTtsConfigId.value = form.tts_config_id
    }
  }
})
</script>

<style scoped>
.agent-config {
  min-height: 100vh;
  background: #f8fafc;
  padding: 24px;
}

.config-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 32px;
  background: white;
  padding: 20px 24px;
  border-radius: 12px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}

.header-left h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: #1f2937;
}

.config-content {
  max-width: 800px;
  margin: 0 auto;
}

.config-form {
  background: white;
  border-radius: 12px;
  padding: 32px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

.form-section {
  margin-bottom: 40px;
  padding-bottom: 32px;
  border-bottom: 1px solid #e5e7eb;
}

.quick-config-section {
  margin-bottom: 24px;
  padding-bottom: 18px;
}

/* 角色选择器相关样式 */
.help-icon {
  margin-left: 8px;
  font-size: 16px;
  color: #909399;
  cursor: help;
}

.role-selector {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.role-selector-compact {
  gap: 8px;
}

.role-inline-line {
  display: flex;
  flex-wrap: nowrap;
  gap: 10px;
  overflow-x: auto;
  padding: 4px 2px 6px;
}

.role-inline-line-compact {
  gap: 8px;
  padding: 2px 0;
}

.role-inline-line::-webkit-scrollbar {
  height: 6px;
}

.role-inline-line::-webkit-scrollbar-thumb {
  background: #d1d5db;
  border-radius: 999px;
}

.role-inline-item {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
  padding: 6px 10px;
  border: 1px solid #d1d5db;
  border-radius: 999px;
  background: #fff;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
}

.role-inline-item:hover {
  border-color: #93c5fd;
  background: #f8fbff;
}

.role-inline-item.active {
  border-color: #3b82f6;
  background: #eff6ff;
  color: #1d4ed8;
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.15);
}

.role-inline-name {
  font-size: 12px;
  font-weight: 600;
}

.role-inline-type {
  font-size: 10px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 999px;
  border: 1px solid transparent;
}

.role-inline-type.global {
  color: #166534;
  background: #dcfce7;
  border-color: #86efac;
}

.role-inline-type.user {
  color: #7c2d12;
  background: #ffedd5;
  border-color: #fdba74;
}

.form-section:last-child {
  margin-bottom: 0;
  border-bottom: none;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  color: #1f2937;
  margin: 0 0 24px 0;
  padding-bottom: 8px;
  border-bottom: 2px solid #3b82f6;
  display: inline-block;
}

.form-group {
  margin-bottom: 24px;
}

.form-group:last-child {
  margin-bottom: 0;
}

.form-label {
  display: block;
  font-size: 14px;
  font-weight: 600;
  color: #374151;
  margin-bottom: 8px;
}

.form-help {
  font-size: 12px;
  color: #6b7280;
  margin-top: 4px;
}

.quick-config-help {
  margin-top: 2px;
}

.clone-voice-line {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.clone-voice-item {
  display: inline-flex;
  align-items: center;
  max-width: 220px;
  min-width: 0;
  padding: 4px 10px;
  border: 1px solid #d1d5db;
  border-radius: 999px;
  background: #f8fafc;
  color: #374151;
  cursor: pointer;
  transition: all 0.2s ease;
  line-height: 1.2;
  outline: none;
}

.clone-voice-item:hover {
  border-color: #93c5fd;
  background: #f1f7ff;
}

.clone-voice-item.active {
  border-color: #3b82f6;
  background: #e9f2ff;
  color: #1d4ed8;
  box-shadow: 0 0 0 1px rgba(59, 130, 246, 0.1);
}

.clone-voice-name {
  font-size: 12px;
  font-weight: 500;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.switch-group {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.switch-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: #f9fafb;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.switch-item span {
  font-size: 14px;
  font-weight: 500;
  color: #374151;
}

.template-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}

.template-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 16px;
  border: 2px solid #e5e7eb;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s ease;
  background: #fafafa;
}

.template-card:hover {
  border-color: #3b82f6;
  background: #f0f9ff;
}

.template-card.active {
  border-color: #3b82f6;
  background: #eff6ff;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}

.template-icon {
  font-size: 32px;
  margin-bottom: 8px;
}

.template-name {
  font-size: 14px;
  font-weight: 500;
  color: #374151;
  text-align: center;
}



.memory-settings {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.memory-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  background: #f9fafb;
  border-radius: 8px;
  border: 1px solid #e5e7eb;
}

.memory-item span {
  font-size: 14px;
  font-weight: 500;
  color: #374151;
}

.config-option {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.config-name {
  font-weight: 500;
  color: #374151;
}

.config-desc {
  font-size: 12px;
  color: #6b7280;
}

/* MCP工具列表相关样式 */
.mcp-tools-section {
  margin-bottom: 24px;
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
  color: #1f2937;
}

.tools-list {
  min-height: 60px;
}

.tools-empty {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 20px;
}

.tools-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tool-tag {
  position: relative;
  padding: 8px 12px;
  font-size: 13px;
  border-radius: 6px;
  cursor: default;
}

.tool-info-icon {
  margin-left: 6px;
  font-size: 12px;
  color: #6b7280;
  cursor: help;
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

@media (max-width: 768px) {
  .agent-config {
    padding: 16px;
  }
  
  .config-header {
    flex-direction: column;
    gap: 16px;
    align-items: stretch;
  }
  
  .header-left {
    justify-content: center;
  }
  
  .config-form {
    padding: 24px 16px;
  }
  
  .template-grid {
    grid-template-columns: repeat(2, 1fr);
  }
  
  .memory-item {
    flex-direction: column;
    gap: 12px;
    align-items: stretch;
  }
}
</style>
