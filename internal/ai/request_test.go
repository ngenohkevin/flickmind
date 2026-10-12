package ai

import "testing"

func TestBuildOpenAIBody_ReasoningEffort(t *testing.T) {
	body := buildOpenAIBody(openAIConfig{Model: "openai/gpt-oss-120b", Prompt: "p", MaxTokens: 3000, JSONMode: true, ReasoningEffort: "low"})

	if body["reasoning_effort"] != "low" {
		t.Errorf("reasoning_effort = %v, want low", body["reasoning_effort"])
	}
	if body["max_tokens"] != 3000 {
		t.Errorf("max_tokens = %v, want 3000", body["max_tokens"])
	}
	if _, ok := body["response_format"]; !ok {
		t.Error("expected response_format for JSON mode")
	}
}

func TestBuildOpenAIBody_Defaults(t *testing.T) {
	body := buildOpenAIBody(openAIConfig{Model: "deepseek-chat", Prompt: "p"})

	if _, ok := body["reasoning_effort"]; ok {
		t.Error("reasoning_effort should be omitted when unset")
	}
	if _, ok := body["response_format"]; ok {
		t.Error("response_format should be omitted without JSON mode")
	}
	if body["max_tokens"] != 4000 {
		t.Errorf("max_tokens = %v, want default 4000", body["max_tokens"])
	}
}

func TestBuildGeminiBody_ThinkingLevel(t *testing.T) {
	gc := buildGeminiBody("p")["generationConfig"].(map[string]interface{})
	tc, ok := gc["thinkingConfig"].(map[string]string)
	if !ok || tc["thinkingLevel"] != "low" {
		t.Errorf("thinkingConfig = %v, want thinkingLevel low", gc["thinkingConfig"])
	}
}

func TestGeminiResponseText(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{"single part with signature", `{"candidates":[{"content":{"parts":[{"text":"{\"a\":1}","thoughtSignature":"x"}]}}]}`, `{"a":1}`, false},
		{"skips thought part", `{"candidates":[{"content":{"parts":[{"text":"thinking...","thought":true},{"text":"answer"}]}}]}`, "answer", false},
		{"no candidates", `{"candidates":[]}`, "", true},
		{"only thoughts", `{"candidates":[{"content":{"parts":[{"text":"hmm","thought":true}]}}]}`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := geminiResponseText([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
