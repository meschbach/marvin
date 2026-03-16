package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

func HandleHelp(ctx context.Context, deps *CommandsDependencies, msg string) error {
	helpText := RenderHelp(nil)
	return send(ctx, deps, "", deps.Context.ChannelID(), helpText)
}

func HandleThinking(ctx context.Context, deps *CommandsDependencies, msg string) error {
	msg = strings.ToLower(strings.TrimSpace(msg))

	currentPrefs, hasPrefs := deps.SessionManager.GetPreferences(deps.Context.UserID())
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	parts := strings.Fields(msg)
	if len(parts) == 0 {
		return send(ctx, deps, deps.Context.UserID(), "", formatPreferences(currentPrefs))
	}

	subCmd := parts[0]

	switch subCmd {
	case "on":
		return handleToggleThinking(ctx, deps, currentPrefs, true)
	case "off":
		return handleToggleThinking(ctx, deps, currentPrefs, false)
	case "format":
		if len(parts) < 2 {
			return send(ctx, deps, deps.Context.UserID(), "", "🤖 Please specify a format: `plain`, `markdown`, or `collapsed`.")
		}
		return handleThinkingFormat(ctx, deps, currentPrefs, parts[1])
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "🤖 Unknown thinking command. Use `on`, `off`, or `format [plain|markdown|collapsed]`.")
	}
}

func HandleTools(ctx context.Context, deps *CommandsDependencies, msg string) error {
	msg = strings.ToLower(strings.TrimSpace(msg))

	currentPrefs, hasPrefs := deps.SessionManager.GetPreferences(deps.Context.UserID())
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	parts := strings.Fields(msg)
	if len(parts) == 0 {
		return send(ctx, deps, deps.Context.UserID(), "", formatPreferences(currentPrefs))
	}

	subCmd := parts[0]

	switch subCmd {
	case "on":
		return handleToggleTools(ctx, deps, currentPrefs, true)
	case "off":
		return handleToggleTools(ctx, deps, currentPrefs, false)
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "🔧 Unknown tools command. Use `on` or `off`.")
	}
}

func HandleDone(ctx context.Context, deps *CommandsDependencies, msg string) error {
	msg = strings.ToLower(strings.TrimSpace(msg))

	currentPrefs, hasPrefs := deps.SessionManager.GetPreferences(deps.Context.UserID())
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	parts := strings.Fields(msg)
	if len(parts) == 0 {
		return send(ctx, deps, deps.Context.UserID(), "", formatPreferences(currentPrefs))
	}

	subCmd := parts[0]

	switch subCmd {
	case "on":
		return handleToggleDone(ctx, deps, currentPrefs, true)
	case "off":
		return handleToggleDone(ctx, deps, currentPrefs, false)
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "✅ Unknown done command. Use `on` or `off`.")
	}
}

func HandleVerbose(ctx context.Context, deps *CommandsDependencies, msg string) error {
	msg = strings.ToLower(strings.TrimSpace(msg))

	currentPrefs, hasPrefs := deps.SessionManager.GetPreferences(deps.Context.UserID())
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	parts := strings.Fields(msg)
	if len(parts) == 0 {
		return send(ctx, deps, deps.Context.UserID(), "", formatPreferences(currentPrefs))
	}

	subCmd := parts[0]

	switch subCmd {
	case "on":
		return handleToggleVerbose(ctx, deps, currentPrefs, true)
	case "off":
		return handleToggleVerbose(ctx, deps, currentPrefs, false)
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "🔍 Unknown verbose command. Use `on` or `off`.")
	}
}

func HandlePreferences(ctx context.Context, deps *CommandsDependencies, msg string) error {
	msg = strings.ToLower(strings.TrimSpace(msg))

	currentPrefs, hasPrefs := deps.SessionManager.GetPreferences(deps.Context.UserID())
	if !hasPrefs {
		currentPrefs = DefaultUserPreferences()
	}

	updatedPrefs := currentPrefs

	switch {
	case msg == "" || msg == "show":
		return send(ctx, deps, deps.Context.UserID(), "", formatPreferences(updatedPrefs))
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "To manage your preferences, please use natural language like: \"show my preferences\"")
	}
}

func send(ctx context.Context, deps *CommandsDependencies, userID, channelID, message string) error {
	if channelID == "" {
		channelID = deps.Context.ChannelID()
	}
	if userID == "" {
		userID = deps.Context.UserID()
	}

	_, _, err := deps.SlackClient.PostMessage(
		channelID,
		slack.MsgOptionText(message, false),
		slack.MsgOptionUsername("Marvin"),
		slack.MsgOptionIconURL("https://api.slack.com/img/icons/icon-32.png"),
	)
	return err
}

func handleToggleThinking(ctx context.Context, deps *CommandsDependencies, prefs UserPreferences, enable bool) error {
	prefs.ShowThinking = enable
	deps.SessionManager.UpdatePreferences(deps.Context.UserID(), prefs)

	response := "🤖 Thinking display has been " + map[bool]string{true: "enabled", false: "disabled"}[enable] + "."
	return send(ctx, deps, deps.Context.UserID(), "", response)
}

func handleThinkingFormat(ctx context.Context, deps *CommandsDependencies, prefs UserPreferences, format string) error {
	switch format {
	case "plain", "markdown", "collapsed":
		prefs.ThinkingFormat = format
		deps.SessionManager.UpdatePreferences(deps.Context.UserID(), prefs)
		return send(ctx, deps, deps.Context.UserID(), "", "🤖 Thinking format set to: "+format)
	default:
		return send(ctx, deps, deps.Context.UserID(), "", "🤖 Invalid format. Use: `plain`, `markdown`, or `collapsed`.")
	}
}

func handleToggleTools(ctx context.Context, deps *CommandsDependencies, prefs UserPreferences, enable bool) error {
	prefs.ShowTools = enable
	deps.SessionManager.UpdatePreferences(deps.Context.UserID(), prefs)

	response := "🔧 Tool display has been " + map[bool]string{true: "enabled", false: "disabled"}[enable] + "."
	return send(ctx, deps, deps.Context.UserID(), "", response)
}

func handleToggleDone(ctx context.Context, deps *CommandsDependencies, prefs UserPreferences, enable bool) error {
	prefs.ShowDone = enable
	deps.SessionManager.UpdatePreferences(deps.Context.UserID(), prefs)

	response := "✅ Done display has been " + map[bool]string{true: "enabled", false: "disabled"}[enable] + "."
	return send(ctx, deps, deps.Context.UserID(), "", response)
}

func handleToggleVerbose(ctx context.Context, deps *CommandsDependencies, prefs UserPreferences, enable bool) error {
	prefs.Verbose = enable
	deps.SessionManager.UpdatePreferences(deps.Context.UserID(), prefs)

	response := "🔍 Verbose output has been " + map[bool]string{true: "enabled", false: "disabled"}[enable] + "."
	return send(ctx, deps, deps.Context.UserID(), "", response)
}

func formatPreferences(prefs UserPreferences) string {
	var b strings.Builder
	b.WriteString("📊 **Your Preferences:**\n\n")
	b.WriteString(fmt.Sprintf("  • 🤖 Thinking: `%v`\n", prefs.ShowThinking))
	b.WriteString(fmt.Sprintf("  • 🔧 Tools: `%v`\n", prefs.ShowTools))
	b.WriteString(fmt.Sprintf("  • ✅ Done: `%v`\n", prefs.ShowDone))
	b.WriteString(fmt.Sprintf("  • 🔍 Verbose: `%v`\n", prefs.Verbose))
	b.WriteString(fmt.Sprintf("  • 📝 Thinking Format: `%s`\n", prefs.ThinkingFormat))
	b.WriteString(fmt.Sprintf("  • 🔨 Tool Format: `%s`\n", prefs.ToolFormat))
	return b.String()
}
