<template>
  <el-form ref="formRef" :model="model" :rules="rules" label-width="120px">
    <el-form-item label="提供商" prop="provider">
      <el-select v-model="model.provider" placeholder="请选择提供商" style="width: 100%" @change="onProviderChange">
        <el-option label="OpenAI" value="openai" />
        <el-option label="Azure OpenAI" value="azure" />
        <el-option label="Anthropic" value="anthropic" />
        <el-option label="智谱AI" value="zhipu" />
        <el-option label="阿里云" value="aliyun" />
        <el-option label="豆包" value="doubao" />
        <el-option label="SiliconFlow" value="siliconflow" />
        <el-option label="DeepSeek" value="deepseek" />
      </el-select>
    </el-form-item>
    <el-form-item label="配置名称" prop="name">
      <el-input v-model="model.name" placeholder="请输入配置名称" />
    </el-form-item>
    <el-form-item label="配置ID" prop="config_id">
      <el-input v-model="model.config_id" placeholder="请输入唯一的配置ID" />
    </el-form-item>
    <el-form-item label="模型类型" prop="type">
      <el-select v-model="model.type" placeholder="请选择模型类型" style="width: 100%">
        <el-option label="OpenAI" value="openai" />
        <el-option label="Ollama" value="ollama" />
      </el-select>
    </el-form-item>
    <el-form-item label="模型名称" prop="model_name">
      <el-input v-model="model.model_name" placeholder="请输入模型名称" />
    </el-form-item>
    <el-form-item label="API密钥" prop="api_key">
      <el-input v-model="model.api_key" type="password" placeholder="请输入API密钥" show-password />
    </el-form-item>
    <el-form-item label="基础URL" prop="base_url">
      <el-input v-model="model.base_url" placeholder="请输入基础URL" style="width: 100%" />
    </el-form-item>
    <el-form-item label="max_tokens" prop="max_tokens">
      <el-input-number v-model="model.max_tokens" :min="1" :max="100000" placeholder="max_tokens" style="width: 100%" />
    </el-form-item>
    <el-form-item label="温度" prop="temperature">
      <el-input-number v-model="model.temperature" :min="0" :max="2" :step="0.1" placeholder="温度" style="width: 100%" />
    </el-form-item>
    <el-form-item label="Top P" prop="top_p">
      <el-input-number v-model="model.top_p" :min="0" :max="1" :step="0.1" placeholder="Top P" style="width: 100%" />
    </el-form-item>
  </el-form>
</template>

<script setup>
import { ref, watch } from 'vue'

const quickUrls = {
  openai: 'https://api.openai.com/v1',
  azure: 'https://your-resource-name.openai.azure.com',
  anthropic: 'https://api.anthropic.com',
  zhipu: 'https://open.bigmodel.cn/api/paas/v4',
  aliyun: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  doubao: 'https://ark.cn-beijing.volces.com/api/v3',
  siliconflow: 'https://api.siliconflow.cn/v1',
  deepseek: 'https://api.deepseek.com/v1'
}

const props = defineProps({
  model: { type: Object, required: true },
  rules: { type: Object, default: () => ({}) }
})

const formRef = ref()

watch(() => props.model?.provider, (value) => {
  if (value && quickUrls[value] && props.model) {
    props.model.base_url = quickUrls[value]
  }
}, { immediate: true })

function onProviderChange(value) {
  if (value && quickUrls[value] && props.model) {
    props.model.base_url = quickUrls[value]
  }
}

function getJsonData() {
  const m = props.model
  const config = {
    type: m.type,
    model_name: m.model_name,
    api_key: m.api_key,
    base_url: m.base_url,
    max_tokens: m.max_tokens
  }
  if (m.temperature !== undefined && m.temperature !== null) config.temperature = m.temperature
  if (m.top_p !== undefined && m.top_p !== null) config.top_p = m.top_p
  return JSON.stringify(config, null, 2)
}

function validate() {
  return formRef.value?.validate()
}

function resetFields() {
  formRef.value?.resetFields()
}

defineExpose({ validate, getJsonData, resetFields })
</script>
