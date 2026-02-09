package config

type HttpMCPBlock struct {
	Name            string                `hcl:"name,label"`
	URL             string                `hcl:"url,label"`
	Sharing         *SharingBlock         `hcl:"sharing,block"`
	AssistantPrompt *AssistantPromptBlock `hcl:"assistant_prompt,block"`
}
