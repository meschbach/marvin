package config

type LocalProgramBlock struct {
	Name            string                `hcl:"name,label"`
	Program         string                `hcl:"program"`
	Args            []string              `hcl:"args,optional"`
	Sharing         *SharingBlock         `hcl:"sharing,block"`
	AssistantPrompt *AssistantPromptBlock `hcl:"assistant_prompt,block"`
}
