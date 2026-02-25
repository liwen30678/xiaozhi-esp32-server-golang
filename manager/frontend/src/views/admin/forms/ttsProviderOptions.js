export const TTS_PROVIDERS_WITH_VOICE_CLONE = ['minimax', 'cosyvoice', 'aliyun_qwen', 'indextts_vllm']

const voiceCloneProviderSet = new Set(TTS_PROVIDERS_WITH_VOICE_CLONE)

export const TTS_PROVIDER_OPTIONS = [
  { label: '豆包 WebSocket', value: 'doubao_ws' },
  { label: 'Edge TTS', value: 'edge' },
  { label: 'Edge 离线', value: 'edge_offline' },
  { label: 'CosyVoice', value: 'cosyvoice' },
  { label: 'OpenAI', value: 'openai' },
  { label: '千问', value: 'aliyun_qwen' },
  { label: '智谱', value: 'zhipu' },
  { label: 'Minimax', value: 'minimax' },
  { label: 'IndexTTS(vLLM)', value: 'indextts_vllm' }
].map((item) => ({
  ...item,
  supports_voice_clone: voiceCloneProviderSet.has(item.value)
}))

export const TTS_PROVIDERS_WITH_VOICES = ['minimax', 'edge', 'doubao', 'doubao_ws', 'zhipu', 'openai', 'indextts_vllm']
