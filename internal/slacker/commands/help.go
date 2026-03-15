package commands

const helpTemplate = `Available Commands:
  help          - Show this help message
  tools         - List available tools
  thinking      - Show current thinking preference
  preferences   - Manage your preferences
  reset session - Reset the current conversation session

Admin Commands:
  admin              - Show admin help
  model access       - Manage model access
  add tool           - Add a new tool
  remove tool       - Remove a tool
  list tools        - List all available tools

Use "@marvin <command>" to run a command.
`

func RenderHelp(registry *CommandRegistry) string {
	return helpTemplate
}
