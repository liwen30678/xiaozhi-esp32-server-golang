<template>
  <div class="config-page">
    <div class="page-header">
      <h2>知识库检索配置</h2>
      <el-button type="primary" @click="openDialog()">添加配置</el-button>
    </div>

    <el-table :data="items" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="name" label="名称" width="160" />
      <el-table-column prop="config_id" label="配置ID" width="170" />
      <el-table-column prop="provider" label="Provider" width="130" />
      <el-table-column label="启用" width="80">
        <template #default="scope"><el-tag :type="scope.row.enabled ? 'success' : 'info'">{{ scope.row.enabled ? '是' : '否' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="默认" width="80">
        <template #default="scope"><el-tag :type="scope.row.is_default ? 'success' : 'info'">{{ scope.row.is_default ? '是' : '否' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="scope">
          <el-button size="small" @click="openDialog(scope.row)">编辑</el-button>
          <el-button size="small" @click="toggle(scope.row.id)">{{ scope.row.enabled ? '禁用' : '启用' }}</el-button>
          <el-button size="small" type="danger" @click="remove(scope.row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑配置' : '新增配置'" width="700px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="配置ID"><el-input v-model="form.config_id" /></el-form-item>
        <el-form-item label="Provider"><el-input v-model="form.provider" placeholder="例如 saas_api" /></el-form-item>
        <el-form-item label="Endpoint"><el-input v-model="form.endpoint" placeholder="外部SaaS检索接口地址" /></el-form-item>
        <el-form-item label="API Key"><el-input v-model="form.api_key" type="password" show-password /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
        <el-form-item label="默认"><el-switch v-model="form.is_default" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '@/utils/api'

const items = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref(false)
const currentId = ref(null)

const form = reactive({
  name: '',
  config_id: '',
  provider: '',
  endpoint: '',
  api_key: '',
  enabled: true,
  is_default: false
})

const loadData = async () => {
  loading.value = true
  try {
    const res = await api.get('/admin/knowledge-search-configs')
    items.value = res.data.data || []
  } finally {
    loading.value = false
  }
}

const openDialog = (row = null) => {
  editing.value = !!row
  currentId.value = row?.id || null
  const data = row?.json_data ? JSON.parse(row.json_data || '{}') : {}
  form.name = row?.name || ''
  form.config_id = row?.config_id || ''
  form.provider = row?.provider || ''
  form.endpoint = data.endpoint || ''
  form.api_key = data.api_key || ''
  form.enabled = row?.enabled ?? true
  form.is_default = row?.is_default ?? false
  dialogVisible.value = true
}

const submit = async () => {
  const payload = {
    type: 'knowledge_search',
    name: form.name,
    config_id: form.config_id,
    provider: form.provider,
    enabled: form.enabled,
    is_default: form.is_default,
    json_data: JSON.stringify({
      endpoint: form.endpoint,
      api_key: form.api_key
    })
  }
  try {
    if (editing.value) {
      await api.put(`/admin/knowledge-search-configs/${currentId.value}`, payload)
    } else {
      await api.post('/admin/knowledge-search-configs', payload)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

const toggle = async (id) => {
  await api.post(`/admin/configs/${id}/toggle`)
  await loadData()
}

const remove = async (id) => {
  try {
    await ElMessageBox.confirm('确认删除该配置吗？', '提示', { type: 'warning' })
    await api.delete(`/admin/knowledge-search-configs/${id}`)
    ElMessage.success('删除成功')
    await loadData()
  } catch {}
}

onMounted(loadData)
</script>
