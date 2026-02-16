<template>
  <div class="mcp-market-page">
    <div class="page-header">
      <h2>MCP市场</h2>
      <p class="subtitle">连接多个MCP市场并导入可用的SSE/StreamableHTTP服务</p>
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :lg="11">
        <el-card shadow="never" class="panel-card">
          <template #header>
            <div class="panel-header">
              <span>市场连接管理</span>
              <div>
                <el-button type="primary" size="small" @click="openCreateDialog">新增连接</el-button>
                <el-button size="small" @click="loadMarkets">
                  <el-icon><Refresh /></el-icon>
                </el-button>
              </div>
            </div>
          </template>

          <el-table :data="markets" stripe v-loading="marketsLoading" height="520">
            <el-table-column prop="name" label="名称" min-width="140" />
            <el-table-column prop="catalog_url" label="目录URL" min-width="220" show-overflow-tooltip />
            <el-table-column label="鉴权" width="120">
              <template #default="{ row }">
                <el-tag size="small" :type="row.has_token ? 'success' : 'info'">
                  {{ row.auth_type || 'none' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag size="small" :type="row.enabled ? 'success' : 'info'">
                  {{ row.enabled ? '启用' : '禁用' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="210" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click="openEditDialog(row)">编辑</el-button>
                <el-button link type="success" @click="testMarket(row)">测试</el-button>
                <el-button link type="danger" @click="deleteMarket(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="13">
        <el-card shadow="never" class="panel-card">
          <template #header>
            <div class="panel-header">
              <span>聚合服务列表</span>
              <div class="search-actions">
                <el-input
                  v-model="serviceQuery"
                  placeholder="搜索服务名/描述/ID"
                  clearable
                  size="small"
                  style="width: 240px"
                  @keyup.enter="loadServices(1)"
                >
                  <template #append>
                    <el-button @click="loadServices(1)">
                      <el-icon><Search /></el-icon>
                    </el-button>
                  </template>
                </el-input>
                <el-button size="small" @click="loadServices(servicePage)">
                  <el-icon><Refresh /></el-icon>
                </el-button>
              </div>
            </div>
          </template>

          <el-table :data="services" stripe v-loading="servicesLoading" height="460" @row-click="handleSelectService">
            <el-table-column prop="name" label="服务" min-width="180" show-overflow-tooltip />
            <el-table-column prop="market_name" label="来源市场" min-width="120" show-overflow-tooltip />
            <el-table-column prop="service_id" label="Service ID" min-width="180" show-overflow-tooltip />
            <el-table-column label="操作" width="90" fixed="right">
              <template #default="{ row }">
                <el-button link type="primary" @click.stop="loadServiceDetail(row)">详情</el-button>
              </template>
            </el-table-column>
          </el-table>

          <div class="pagination-wrap">
            <el-pagination
              layout="prev, pager, next, total"
              :current-page="servicePage"
              :page-size="servicePageSize"
              :total="serviceTotal"
              @current-change="loadServices"
            />
          </div>

          <el-alert
            v-if="serviceWarnings.length > 0"
            type="warning"
            :closable="false"
            title="部分市场拉取失败"
            class="warning-alert"
          >
            <template #default>
              <div v-for="(warn, idx) in serviceWarnings" :key="idx">{{ warn }}</div>
            </template>
          </el-alert>
        </el-card>
      </el-col>
    </el-row>

    <el-card shadow="never" class="panel-card detail-panel" v-loading="detailLoading">
      <template #header>
        <div class="panel-header">
          <span>服务详情与导入</span>
        </div>
      </template>

      <div v-if="!serviceDetail" class="empty-tip">请选择一个服务查看详情</div>
      <template v-else>
        <div class="detail-grid">
          <div><strong>服务：</strong>{{ serviceDetail.name }}</div>
          <div><strong>来源市场：</strong>{{ serviceDetail.market_name }}</div>
          <div><strong>Service ID：</strong>{{ serviceDetail.service_id }}</div>
        </div>

        <div class="desc" v-if="serviceDetail.description">{{ serviceDetail.description }}</div>

        <el-table :data="serviceDetail.endpoints || []" size="small" stripe>
          <el-table-column prop="name" label="资源名" min-width="120" show-overflow-tooltip />
          <el-table-column prop="transport" label="传输" width="140" />
          <el-table-column prop="url" label="URL" min-width="300" show-overflow-tooltip />
        </el-table>

        <div class="import-actions">
          <el-input
            v-model="nameOverride"
            placeholder="可选：导入名称覆盖（默认服务名）"
            clearable
            style="max-width: 320px"
          />
          <el-button type="primary" :loading="importing" @click="importService">一键导入并热更新</el-button>
        </div>
      </template>
    </el-card>

    <el-dialog v-model="marketDialogVisible" :title="editingMarket ? '编辑市场连接' : '新增市场连接'" width="640px">
      <el-form ref="marketFormRef" :model="marketForm" :rules="marketRules" label-width="130px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="marketForm.name" placeholder="例如：魔搭MCP广场" />
        </el-form-item>
        <el-form-item label="目录URL" prop="catalog_url">
          <el-input v-model="marketForm.catalog_url" placeholder="https://example.com/api/services" />
        </el-form-item>
        <el-form-item label="详情URL模板" prop="detail_url_template">
          <el-input v-model="marketForm.detail_url_template" placeholder="https://example.com/api/services/{id}（可选）" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="marketForm.enabled" />
        </el-form-item>

        <el-divider>鉴权配置</el-divider>

        <el-form-item label="鉴权类型">
          <el-select v-model="marketForm.auth.type" style="width: 100%">
            <el-option label="none" value="none" />
            <el-option label="bearer" value="bearer" />
            <el-option label="header" value="header" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="marketForm.auth.type === 'header'" label="Header名">
          <el-input v-model="marketForm.auth.header_name" placeholder="Authorization" />
        </el-form-item>
        <el-form-item label="Token">
          <el-input
            v-model="marketForm.auth.token"
            :placeholder="editingMarket ? `留空则保持原值（当前：${editingMarket.token_mask || '未设置'}）` : '输入Token（可选）'"
            show-password
            clearable
          />
        </el-form-item>
        <el-form-item label="额外Headers(JSON)">
          <el-input
            v-model="extraHeadersText"
            type="textarea"
            :rows="4"
            placeholder='例如：{"X-API-Key":"xxx"}'
          />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="marketDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="marketSaving" @click="saveMarket">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import api from '@/utils/api'

const markets = ref([])
const marketsLoading = ref(false)
const marketDialogVisible = ref(false)
const marketSaving = ref(false)
const editingMarket = ref(null)
const marketFormRef = ref()
const extraHeadersText = ref('')

const marketForm = reactive({
  name: '',
  catalog_url: '',
  detail_url_template: '',
  enabled: true,
  auth: {
    type: 'none',
    token: '',
    header_name: 'Authorization'
  }
})

const marketRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  catalog_url: [{ required: true, message: '请输入目录URL', trigger: 'blur' }]
}

const services = ref([])
const servicesLoading = ref(false)
const serviceWarnings = ref([])
const servicePage = ref(1)
const servicePageSize = ref(20)
const serviceTotal = ref(0)
const serviceQuery = ref('')

const detailLoading = ref(false)
const serviceDetail = ref(null)
const importing = ref(false)
const nameOverride = ref('')

const loadMarkets = async () => {
  marketsLoading.value = true
  try {
    const resp = await api.get('/admin/mcp-markets')
    markets.value = resp.data.data || []
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '加载市场连接失败')
  } finally {
    marketsLoading.value = false
  }
}

const resetMarketForm = () => {
  marketForm.name = ''
  marketForm.catalog_url = ''
  marketForm.detail_url_template = ''
  marketForm.enabled = true
  marketForm.auth.type = 'none'
  marketForm.auth.token = ''
  marketForm.auth.header_name = 'Authorization'
  extraHeadersText.value = ''
}

const openCreateDialog = () => {
  editingMarket.value = null
  resetMarketForm()
  marketDialogVisible.value = true
}

const openEditDialog = (row) => {
  editingMarket.value = row
  marketForm.name = row.name
  marketForm.catalog_url = row.catalog_url
  marketForm.detail_url_template = row.detail_url_template || ''
  marketForm.enabled = !!row.enabled
  marketForm.auth.type = row.auth_type || 'none'
  marketForm.auth.header_name = row.header_name || 'Authorization'
  marketForm.auth.token = ''
  extraHeadersText.value = ''
  marketDialogVisible.value = true
}

const parseExtraHeaders = () => {
  const txt = extraHeadersText.value.trim()
  if (!txt) return null
  try {
    const parsed = JSON.parse(txt)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      throw new Error('extra_headers 必须是JSON对象')
    }
    return parsed
  } catch (e) {
    throw new Error('额外Headers不是合法JSON对象')
  }
}

const saveMarket = async () => {
  if (!marketFormRef.value) return
  const valid = await marketFormRef.value.validate().catch(() => false)
  if (!valid) return

  let extraHeaders = null
  try {
    extraHeaders = parseExtraHeaders()
  } catch (e) {
    ElMessage.error(e.message)
    return
  }

  const payload = {
    name: marketForm.name,
    catalog_url: marketForm.catalog_url,
    detail_url_template: marketForm.detail_url_template,
    enabled: marketForm.enabled,
    auth: {
      type: marketForm.auth.type,
      token: marketForm.auth.token,
      header_name: marketForm.auth.header_name,
      extra_headers: extraHeaders
    }
  }

  marketSaving.value = true
  try {
    if (editingMarket.value) {
      await api.put(`/admin/mcp-markets/${editingMarket.value.id}`, payload)
      ElMessage.success('更新成功')
    } else {
      await api.post('/admin/mcp-markets', payload)
      ElMessage.success('创建成功')
    }
    marketDialogVisible.value = false
    await loadMarkets()
    await loadServices(1)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    marketSaving.value = false
  }
}

const deleteMarket = async (row) => {
  try {
    await ElMessageBox.confirm(`确认删除市场连接「${row.name}」？`, '提示', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    await api.delete(`/admin/mcp-markets/${row.id}`)
    ElMessage.success('删除成功')
    await loadMarkets()
    await loadServices(1)
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.error || '删除失败')
    }
  }
}

const testMarket = async (row) => {
  try {
    const resp = await api.post(`/admin/mcp-markets/${row.id}/test`)
    const count = resp.data?.data?.service_count ?? 0
    ElMessage.success(`连接成功，可发现 ${count} 个服务`)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '连接测试失败')
  }
}

const loadServices = async (page = 1) => {
  servicePage.value = page
  servicesLoading.value = true
  try {
    const resp = await api.get('/admin/mcp-market/services', {
      params: {
        q: serviceQuery.value,
        page: servicePage.value,
        page_size: servicePageSize.value
      }
    })
    const data = resp.data.data || {}
    services.value = data.items || []
    serviceTotal.value = data.total || 0
    serviceWarnings.value = data.warnings || []
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '加载聚合服务失败')
  } finally {
    servicesLoading.value = false
  }
}

const handleSelectService = (row) => {
  loadServiceDetail(row)
}

const loadServiceDetail = async (row) => {
  detailLoading.value = true
  try {
    const resp = await api.get(`/admin/mcp-market/services/${row.market_id}/${encodeURIComponent(row.service_id)}`)
    serviceDetail.value = resp.data.data
    nameOverride.value = ''
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '加载服务详情失败')
  } finally {
    detailLoading.value = false
  }
}

const importService = async () => {
  if (!serviceDetail.value) {
    ElMessage.warning('请先选择一个服务')
    return
  }

  importing.value = true
  try {
    const payload = {
      market_id: serviceDetail.value.market_id,
      service_id: serviceDetail.value.service_id,
      name_override: nameOverride.value || ''
    }
    const resp = await api.post('/admin/mcp-market/import', payload)
    const result = resp.data.data || {}
    ElMessage.success(`导入成功：${result.imported_count || 0} 个服务已应用`)
    await loadServices(servicePage.value)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '导入失败')
  } finally {
    importing.value = false
  }
}

onMounted(async () => {
  await loadMarkets()
  await loadServices(1)
})
</script>

<style scoped>
.mcp-market-page {
  padding: 20px;
}

.page-header {
  margin-bottom: 16px;
}

.page-header h2 {
  margin: 0;
  color: #1f2937;
}

.subtitle {
  margin-top: 6px;
  color: #6b7280;
  font-size: 14px;
}

.panel-card {
  margin-bottom: 16px;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.search-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.pagination-wrap {
  margin-top: 10px;
  display: flex;
  justify-content: flex-end;
}

.warning-alert {
  margin-top: 12px;
}

.detail-panel {
  margin-top: 4px;
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 8px 12px;
  margin-bottom: 12px;
}

.desc {
  margin-bottom: 12px;
  color: #4b5563;
  line-height: 1.6;
}

.import-actions {
  margin-top: 12px;
  display: flex;
  gap: 10px;
  align-items: center;
}

.empty-tip {
  color: #9ca3af;
  font-size: 14px;
  padding: 12px 0;
}

@media (max-width: 992px) {
  .search-actions {
    flex-wrap: wrap;
  }
}
</style>
