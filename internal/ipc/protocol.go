package ipc

type DaemonRequest struct {
	ID string `json:"id"`
	Op string `json:"op"`
}

type DaemonResponse struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

type WorkerRequest struct {
	ID           string `json:"id"`
	Op           string `json:"op"`
	AudioFormat  string `json:"audio_format,omitempty"`
	SampleRateHz int    `json:"sample_rate_hz,omitempty"`
	AudioB64     string `json:"audio_b64,omitempty"`
}

type WorkerResponse struct {
	ID         string `json:"id"`
	OK         bool   `json:"ok"`
	Transcript string `json:"transcript,omitempty"`
	Cleaned    string `json:"cleaned,omitempty"`
	Stage      string `json:"stage,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Device     string `json:"device,omitempty"`
	ASRModel   string `json:"asr_model,omitempty"`
	LLMModel   string `json:"llm_model,omitempty"`
}
