<template>
  <div class="config-page">
    <div class="page-header">
      <h2>我的知识库</h2>
      <el-button type="primary" @click="openDialog()">新增知识库</el-button>
    </div>

    <el-table :data="items" v-loading="loading" style="width: 100%">
      <el-table-column prop="id" label="ID" width="80" />
      <el-table-column prop="name" label="名称" width="180" />
      <el-table-column prop="description" label="描述" />
      <el-table-column label="内容预览">
        <template #default="scope">
          {{ (scope.row.content || '').slice(0, 100) }}{{ (scope.row.content || '').length > 100 ? '...' : '' }}
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="scope">
          <el-tag :type="scope.row.status === 'active' ? 'success' : 'info'">{{ scope.row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="scope">
          <el-button size="small" @click="openDialog(scope.row)">编辑</el-button>
          <el-button size="small" type="danger" @click="removeItem(scope.row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="editing ? '编辑知识库' : '新增知识库'" width="680px">
      <el-form :model="form" label-width="90px">
        <el-form-item label="名称">
          <el-input v-model="form.name" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="form.content" type="textarea" :rows="10" placeholder="请输入纯文本知识内容" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status" style="width: 100%">
            <el-option value="active" label="active" />
            <el-option value="inactive" label="inactive" />
          </el-select>
        </el-form-item>
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

const loading = ref(false)
const items = ref([])
const dialogVisible = ref(false)
const editing = ref(false)
const currentId = ref(null)

const form = reactive({
  name: '',
  description: '',
  content: '',
  status: 'active'
})

const loadData = async () => {
  loading.value = true
  try {
    const res = await api.get('/user/knowledge-bases')
    items.value = res.data.data || []
  } finally {
    loading.value = false
  }
}

const openDialog = (row = null) => {
  editing.value = !!row
  currentId.value = row?.id || null
  form.name = row?.name || ''
  form.description = row?.description || ''
  form.content = row?.content || ''
  form.status = row?.status || 'active'
  dialogVisible.value = true
}

const submit = async () => {
  if (!form.name.trim()) {
    ElMessage.error('名称不能为空')
    return
  }
  try {
    if (editing.value) {
      await api.put(`/user/knowledge-bases/${currentId.value}`, form)
    } else {
      await api.post('/user/knowledge-bases', form)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await loadData()
  } catch (e) {
    ElMessage.error('保存失败')
  }
}

const removeItem = async (id) => {
  try {
    await ElMessageBox.confirm('确认删除该知识库吗？', '提示', { type: 'warning' })
    await api.delete(`/user/knowledge-bases/${id}`)
    ElMessage.success('删除成功')
    await loadData()
  } catch {}
}

onMounted(loadData)
</script>
