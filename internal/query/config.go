package query

type Config struct {
	Model         string         `hcl:"model"`
	LocalPrograms []LocalProgram `hcl:"local_program,block"`
	SystemPrompt  *SystemPrompt  `hcl:"system_prompt,block"`
}

type LocalProgram struct {
	Name    string   `hcl:"name,label"`
	Program string   `hcl:"program"`
	Args    []string `hcl:"args,optional"`
}

type SystemPrompt struct {
	FromString string `hcl:"from_string,optional"`
	FromFile   string `hcl:"from_file,optional"`
}
