package inferenceextension

type openAIEndpoint struct {
	api    string
	prompt string
}

// buildOpenAITests returns the list of request shapes we want to exercise.
func buildOpenAITests() []openAIEndpoint {
	return []openAIEndpoint{
		{
			api:    "/v1/completions",
			prompt: "Write as if you were a critic: San Francisco",
		},
		{
			api:    "/v1/chat/completions",
			prompt: `[{"role":"user","content":"Write as if you were a critic: San Francisco"}]`,
		},
		{
			api: "/v1/chat/completions",
			prompt: `[{"role":"user","content":"Write as if you were a critic: San Francisco"},` +
				`{"role":"assistant","content":"Okay, let's see..."},` +
				`{"role":"user","content":"Now summarize your thoughts."}]`,
		},
	}
}
