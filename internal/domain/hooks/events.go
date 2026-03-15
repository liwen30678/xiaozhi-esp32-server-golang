package hooks

const (
	EventChatASROutput      = "chat.asr.output"
	EventChatLLMInput       = "chat.llm.input"
	EventChatLLMOutput      = "chat.llm.output"
	EventChatTTSInput       = "chat.tts.input"
	EventChatTTSOutputStart = "chat.tts.output.start"
	EventChatTTSOutputStop  = "chat.tts.output.stop"
	EventChatMetric         = "chat.metric"
)

const (
	MetaSession   = "session"
	MetaSessionID = "session_id"
	MetaDeviceID  = "device_id"
)
