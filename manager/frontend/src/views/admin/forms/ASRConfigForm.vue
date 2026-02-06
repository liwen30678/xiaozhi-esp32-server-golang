<template>
  <el-form ref="formRef" :model="model" :rules="rules" label-width="120px">
    <el-form-item label="提供商" prop="provider">
      <el-select v-model="model.provider" placeholder="请选择提供商" style="width: 100%" @change="onProviderChange">
        <el-option label="FunASR" value="funasr" />
        <el-option label="豆包" value="doubao" />
      </el-select>
    </el-form-item>
    <el-form-item label="配置名称" prop="name">
      <el-input v-model="model.name" placeholder="请输入配置名称" />
    </el-form-item>
    <el-form-item label="配置ID" prop="config_id">
      <el-input v-model="model.config_id" placeholder="请输入唯一的配置ID" />
    </el-form-item>
    <div v-if="model.provider === 'funasr'">
      <el-form-item label="主机地址" prop="funasr.host">
        <el-input v-model="model.funasr.host" placeholder="请输入主机地址" />
      </el-form-item>
      <el-form-item label="端口" prop="funasr.port">
        <el-input-number v-model="model.funasr.port" :min="1" :max="65535" style="width: 100%" />
      </el-form-item>
      <el-form-item label="模式" prop="funasr.mode">
        <el-select v-model="model.funasr.mode" placeholder="请选择模式" style="width: 100%">
          <el-option label="2pass" value="2pass" />
          <el-option label="offline" value="offline" />
          <el-option label="online" value="online" />
        </el-select>
      </el-form-item>
      <el-form-item label="采样率" prop="funasr.sample_rate">
        <el-select v-model="model.funasr.sample_rate" placeholder="请选择采样率" style="width: 100%">
          <el-option label="8000" :value="8000" />
          <el-option label="16000" :value="16000" />
          <el-option label="44100" :value="44100" />
          <el-option label="48000" :value="48000" />
        </el-select>
      </el-form-item>
      <el-form-item label="块大小" prop="funasr.chunk_size">
        <div style="display: flex; gap: 8px; width: 100%">
          <el-input-number v-model="model.funasr.chunk_size[0]" :min="1" placeholder="前向" style="flex: 1" />
          <el-input-number v-model="model.funasr.chunk_size[1]" :min="1" placeholder="中间" style="flex: 1" />
          <el-input-number v-model="model.funasr.chunk_size[2]" :min="1" placeholder="后向" style="flex: 1" />
        </div>
        <div class="form-tip">
          <el-icon><InfoFilled /></el-icon>
          格式：[前向, 中间, 后向]，例如：[5, 10, 5]
        </div>
      </el-form-item>
      <el-form-item label="块间隔" prop="funasr.chunk_interval">
        <el-input-number v-model="model.funasr.chunk_interval" :min="1" style="width: 100%" />
      </el-form-item>
      <el-form-item label="最大连接数" prop="funasr.max_connections">
        <el-input-number v-model="model.funasr.max_connections" :min="1" style="width: 100%" />
      </el-form-item>
      <el-form-item label="超时时间(秒)" prop="funasr.timeout">
        <el-input-number v-model="model.funasr.timeout" :min="1" style="width: 100%" />
      </el-form-item>
      <el-form-item label="自动结束" prop="funasr.auto_end">
        <el-switch v-model="model.funasr.auto_end" />
        <div class="form-tip">
          <el-icon><InfoFilled /></el-icon>
          确保FunASR已进行相应配置
        </div>
      </el-form-item>
    </div>
    <div v-if="model.provider === 'doubao'">
      <el-form-item label="应用ID" prop="doubao.appid">
        <el-input v-model="model.doubao.appid" placeholder="请输入应用ID" />
      </el-form-item>
      <el-form-item label="访问令牌" prop="doubao.access_token">
        <el-input v-model="model.doubao.access_token" type="password" placeholder="请输入访问令牌" show-password />
      </el-form-item>
      <el-form-item label="WebSocket URL" prop="doubao.ws_url">
        <el-input v-model="model.doubao.ws_url" placeholder="请输入WebSocket URL" />
      </el-form-item>
      <el-form-item label="模型名称" prop="doubao.model_name">
        <el-input v-model="model.doubao.model_name" placeholder="请输入模型名称" />
      </el-form-item>
      <el-form-item label="结束窗口大小" prop="doubao.end_window_size">
        <el-input-number v-model="model.doubao.end_window_size" :min="1" style="width: 100%" />
      </el-form-item>
      <el-form-item label="启用标点符号" prop="doubao.enable_punc">
        <el-switch v-model="model.doubao.enable_punc" />
      </el-form-item>
      <el-form-item label="启用反向文本标准化" prop="doubao.enable_itn">
        <el-switch v-model="model.doubao.enable_itn" />
      </el-form-item>
      <el-form-item label="启用数字检测修正" prop="doubao.enable_ddc">
        <el-switch v-model="model.doubao.enable_ddc" />
      </el-form-item>
      <el-form-item label="分块时长(毫秒)" prop="doubao.chunk_duration">
        <el-input-number v-model="model.doubao.chunk_duration" :min="1" style="width: 100%" />
      </el-form-item>
      <el-form-item label="超时时间(秒)" prop="doubao.timeout">
        <el-input-number v-model="model.doubao.timeout" :min="1" style="width: 100%" />
      </el-form-item>
    </div>
  </el-form>
</template>

<script setup>
import { ref, watch } from 'vue'
import { InfoFilled } from '@element-plus/icons-vue'

const props = defineProps({
  model: { type: Object, required: true },
  rules: { type: Object, default: () => ({}) }
})

const formRef = ref()

watch(() => props.model?.provider, (provider) => {
  if (provider === 'funasr' && props.model.funasr) {
    props.model.funasr.mode = 'offline'
  }
}, { immediate: true })

function onProviderChange() {
  if (props.model.provider === 'funasr' && props.model.funasr) {
    props.model.funasr.mode = 'offline'
  }
}

function getJsonData() {
  const m = props.model
  if (m.provider === 'funasr') return JSON.stringify(m.funasr || {})
  if (m.provider === 'doubao') return JSON.stringify(m.doubao || {})
  return '{}'
}

function validate() {
  return formRef.value?.validate()
}

function resetFields() {
  formRef.value?.resetFields()
}

defineExpose({ validate, getJsonData, resetFields })
</script>

<style scoped>
.form-tip {
  margin-top: 8px;
  font-size: 12px;
  color: #909399;
  display: flex;
  align-items: center;
  gap: 4px;
}
.form-tip .el-icon {
  font-size: 14px;
  color: #409eff;
}
</style>
