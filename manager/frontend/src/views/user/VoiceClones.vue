<template>
  <div class="voice-clones-page">
    <div class="page-header">
      <div>
        <h2>声音复刻</h2>
        <p class="subtitle">当前第一步仅支持 Minimax，支持上传音频与浏览器录音</p>
      </div>
      <el-button type="primary" @click="openCreateDialog">创建复刻音色</el-button>
    </div>

    <el-table :data="voiceClones" v-loading="loading" stripe>
      <el-table-column prop="name" label="名称" min-width="140" />
      <el-table-column prop="provider" label="提供商" width="120" />
      <el-table-column prop="tts_config_id" label="TTS Config ID" min-width="180" />
      <el-table-column prop="provider_voice_id" label="复刻音色ID" min-width="220" />
      <el-table-column label="创建时间" width="180">
        <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button link type="primary" @click="loadAudios(row)">查看音频</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="createDialogVisible" title="创建复刻音色" width="680px">
      <el-form label-width="140px">
        <el-form-item label="复刻名称">
          <el-input v-model="form.name" placeholder="可选，不填则自动使用文件名" />
        </el-form-item>
        <el-form-item label="TTS配置" required>
          <el-select v-model="form.tts_config_id" placeholder="请选择Minimax配置" style="width: 100%" @change="onConfigChange">
            <el-option v-for="cfg in minimaxConfigs" :key="cfg.config_id" :label="`${cfg.name} (${cfg.config_id})`" :value="cfg.config_id" />
          </el-select>
        </el-form-item>
        <el-form-item label="音频来源">
          <el-radio-group v-model="form.source_type">
            <el-radio label="upload">上传音频</el-radio>
            <el-radio label="record">浏览器录音</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item v-if="form.source_type === 'upload'" label="音频文件" required>
          <input type="file" accept="audio/*" @change="handleFileChange" />
        </el-form-item>

        <el-form-item v-else label="浏览器录音" required>
          <el-button :disabled="isRecording" @click="startRecording">开始录音</el-button>
          <el-button :disabled="!isRecording" type="warning" @click="stopRecording">停止录音</el-button>
          <audio v-if="recordPreviewUrl" :src="recordPreviewUrl" controls style="display:block;width:100%;margin-top:10px" />
        </el-form-item>

        <el-form-item :label="capability.requires_transcript ? '音频对应文字 *' : '音频对应文字'">
          <el-input
            v-model="form.transcript"
            type="textarea"
            :rows="4"
            :placeholder="capability.requires_transcript ? '该提供商要求填写音频对应文字' : '可选填写，不填也可提交'"
          />
          <div class="help">要求：{{ capability.min_text_len || 0 }} - {{ capability.max_text_len || 4000 }} 字符</div>
        </el-form-item>

        <el-form-item label="文字语言">
          <el-select v-model="form.transcript_lang" style="width: 220px">
            <el-option label="中文 (zh-CN)" value="zh-CN" />
            <el-option label="英文 (en-US)" value="en-US" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submitClone">提交复刻</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="audioDialogVisible" title="复刻原始音频" width="720px">
      <el-table :data="currentAudios" stripe>
        <el-table-column prop="source_type" label="来源" width="90" />
        <el-table-column prop="file_name" label="文件名" min-width="220" />
        <el-table-column prop="transcript" label="对应文字" min-width="240" show-overflow-tooltip />
        <el-table-column label="播放" width="120">
          <template #default="{ row }">
            <el-button link type="primary" @click="playAudio(row)">播放</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../../utils/api'

const loading = ref(false)
const submitting = ref(false)
const createDialogVisible = ref(false)
const audioDialogVisible = ref(false)
const voiceClones = ref([])
const currentAudios = ref([])
const ttsConfigs = ref([])

const form = ref({
  name: '',
  tts_config_id: '',
  source_type: 'upload',
  transcript: '',
  transcript_lang: 'zh-CN',
  audioFile: null,
  recordBlob: null
})

const capability = ref({ enabled: true, requires_transcript: false, min_text_len: 0, max_text_len: 0 })

const minimaxConfigs = computed(() => ttsConfigs.value.filter(item => item.provider === 'minimax'))

const isRecording = ref(false)
const mediaRecorder = ref(null)
const recordChunks = ref([])
const recordPreviewUrl = ref('')

const formatDate = (value) => (value ? new Date(value).toLocaleString() : '-')

const loadVoiceClones = async () => {
  loading.value = true
  try {
    const res = await api.get('/user/voice-clones')
    voiceClones.value = res.data.data || []
  } finally {
    loading.value = false
  }
}

const loadTtsConfigs = async () => {
  const res = await api.get('/user/tts-configs')
  ttsConfigs.value = res.data.data || []
}

const openCreateDialog = async () => {
  createDialogVisible.value = true
  await loadTtsConfigs()
}

const onConfigChange = async (configId) => {
  const cfg = minimaxConfigs.value.find(item => item.config_id === configId)
  if (!cfg) return
  const res = await api.get('/user/voice-clone/capabilities', { params: { provider: cfg.provider } })
  capability.value = res.data.data || capability.value
}

const handleFileChange = (event) => {
  form.value.audioFile = event.target.files?.[0] || null
}

const startRecording = async () => {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
  recordChunks.value = []
  const recorder = new MediaRecorder(stream)
  mediaRecorder.value = recorder
  recorder.ondataavailable = (evt) => {
    if (evt.data && evt.data.size > 0) recordChunks.value.push(evt.data)
  }
  recorder.onstop = () => {
    const blob = new Blob(recordChunks.value, { type: 'audio/webm' })
    form.value.recordBlob = blob
    recordPreviewUrl.value = URL.createObjectURL(blob)
    stream.getTracks().forEach(t => t.stop())
  }
  recorder.start()
  isRecording.value = true
}

const stopRecording = () => {
  if (mediaRecorder.value) mediaRecorder.value.stop()
  isRecording.value = false
}

const submitClone = async () => {
  if (!form.value.tts_config_id) {
    ElMessage.warning('请选择 Minimax TTS 配置')
    return
  }
  if (capability.value.requires_transcript && !form.value.transcript.trim()) {
    ElMessage.warning('该提供商要求填写音频对应文字')
    return
  }

  const fd = new FormData()
  fd.append('name', form.value.name)
  fd.append('tts_config_id', form.value.tts_config_id)
  fd.append('source_type', form.value.source_type)
  fd.append('transcript', form.value.transcript)
  fd.append('transcript_lang', form.value.transcript_lang)

  if (form.value.source_type === 'upload') {
    if (!form.value.audioFile) {
      ElMessage.warning('请上传音频文件')
      return
    }
    fd.append('audio_file', form.value.audioFile)
  } else {
    if (!form.value.recordBlob) {
      ElMessage.warning('请先录音')
      return
    }
    fd.append('audio_blob', form.value.recordBlob, 'record.webm')
  }

  submitting.value = true
  try {
    await api.post('/user/voice-clones', fd)
    ElMessage.success('复刻音色创建成功')
    createDialogVisible.value = false
    await loadVoiceClones()
  } finally {
    submitting.value = false
  }
}

const loadAudios = async (clone) => {
  const res = await api.get(`/user/voice-clones/${clone.id}/audios`)
  currentAudios.value = res.data.data || []
  audioDialogVisible.value = true
}

const playAudio = async (audio) => {
  const response = await api.get(`/user/voice-clones/audios/${audio.id}/file`, { responseType: 'blob' })
  const audioPlayer = new Audio(URL.createObjectURL(response.data))
  await audioPlayer.play()
}

onMounted(async () => {
  await loadVoiceClones()
})
</script>

<style scoped>
.voice-clones-page {
  padding: 20px;
}
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.subtitle {
  color: #666;
  margin-top: 4px;
}
.help {
  color: #999;
  font-size: 12px;
  margin-top: 4px;
}
</style>
