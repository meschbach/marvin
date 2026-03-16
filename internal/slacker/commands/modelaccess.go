package commands

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/meschbach/marvin/internal/config"
)

func HandleModelAccess(ctx context.Context, deps *CommandsDependencies, msg string) error {
	userID := deps.Context.UserID()
	channelID := deps.Context.ChannelID()

	if !deps.TenantToolSet.IsAdmin(userID) {
		deps.SecurityLogger.LogError(userID, "ModelAccess", "Unauthorized model access attempt")
		response := "❌ Only administrators can manage model access."
		return send(ctx, deps, userID, channelID, response)
	}

	msg = strings.TrimSpace(msg)

	action, target, targetUser := parseModelAccessIntent(msg)

	switch action {
	case "model_access_list":
		response, err := handleModelAccessList(ctx, deps)
		if err != nil {
			deps.SecurityLogger.LogError(userID, "ModelAccess", err.Error())
			return send(ctx, deps, userID, channelID, fmt.Sprintf("❌ Error: %v", err))
		}
		return send(ctx, deps, userID, channelID, response)
	case "model_access_allow":
		response, err := handleModelAccessAllow(ctx, deps, target)
		if err != nil {
			deps.SecurityLogger.LogError(userID, "ModelAccess", err.Error())
			return send(ctx, deps, userID, channelID, fmt.Sprintf("❌ Error: %v", err))
		}
		return send(ctx, deps, userID, channelID, response)
	case "model_access_deny":
		response, err := handleModelAccessDeny(ctx, deps, target)
		if err != nil {
			deps.SecurityLogger.LogError(userID, "ModelAccess", err.Error())
			return send(ctx, deps, userID, channelID, fmt.Sprintf("❌ Error: %v", err))
		}
		return send(ctx, deps, userID, channelID, response)
	case "model_access_clear":
		response, err := handleModelAccessClear(ctx, deps, userID)
		if err != nil {
			deps.SecurityLogger.LogError(userID, "ModelAccess", err.Error())
			return send(ctx, deps, userID, channelID, fmt.Sprintf("❌ Error: %v", err))
		}
		return send(ctx, deps, userID, channelID, response)
	case "model_access_status":
		response, err := handleModelAccessStatus(ctx, deps, targetUser)
		if err != nil {
			deps.SecurityLogger.LogError(userID, "ModelAccess", err.Error())
			return send(ctx, deps, userID, channelID, fmt.Sprintf("❌ Error: %v", err))
		}
		return send(ctx, deps, userID, channelID, response)
	default:
		return send(ctx, deps, userID, channelID, "❌ Unknown model access command. Use: list, allow <model>, deny <model>, clear, or status @user")
	}
}

func parseModelAccessIntent(msg string) (action, target, targetUser string) {
	msgLower := strings.ToLower(msg)

	listPattern := regexp.MustCompile(`(?i)^(list|show|display)$`)
	if listPattern.MatchString(msgLower) || strings.Contains(msgLower, "model access list") || strings.Contains(msgLower, "list model access") {
		return "model_access_list", "", ""
	}

	allowPattern := regexp.MustCompile(`(?i)^allow\s+(.+)$`)
	if matches := allowPattern.FindStringSubmatch(msg); len(matches) > 1 {
		return "model_access_allow", strings.TrimSpace(matches[1]), ""
	}
	if strings.HasPrefix(msgLower, "model access allow ") {
		return "model_access_allow", strings.TrimSpace(msg[19:]), ""
	}

	denyPattern := regexp.MustCompile(`(?i)^deny\s+(.+)$`)
	if matches := denyPattern.FindStringSubmatch(msg); len(matches) > 1 {
		return "model_access_deny", strings.TrimSpace(matches[1]), ""
	}
	if strings.HasPrefix(msgLower, "model access deny ") {
		return "model_access_deny", strings.TrimSpace(msg[17:]), ""
	}

	clearPattern := regexp.MustCompile(`(?i)^(clear|reset)$`)
	if clearPattern.MatchString(msgLower) || strings.Contains(msgLower, "model access clear") || strings.Contains(msgLower, "clear model access") {
		return "model_access_clear", "", ""
	}

	statusPattern := regexp.MustCompile(`(?i)^status\s+@?(\S+)$`)
	if matches := statusPattern.FindStringSubmatch(msg); len(matches) > 1 {
		return "model_access_status", "", strings.TrimPrefix(matches[1], "@")
	}
	if strings.HasPrefix(msgLower, "model access status ") {
		return "model_access_status", "", strings.TrimSpace(msg[19:])
	}

	return "", "", ""
}

func handleModelAccessList(ctx context.Context, deps *CommandsDependencies) (string, error) {
	state, err := deps.Config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting model access config: %w", err)
	}
	return formatModelAccessResponse(state), nil
}

func handleModelAccessAllow(ctx context.Context, deps *CommandsDependencies, model string) (string, error) {
	state, err := deps.Config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting current model access config: %w", err)
	}

	deniedModels := []string{}
	for _, denied := range state.DeniedModels {
		if denied != model {
			deniedModels = append(deniedModels, denied)
		}
	}

	allowedModels := state.AllowedModels
	for _, allowed := range allowedModels {
		if allowed == model {
			return fmt.Sprintf("ℹ️ Model '%s' is already allowed.", model), nil
		}
	}
	allowedModels = append(allowedModels, model)

	newState := &config.ModelAccessState{
		AllowedModels: allowedModels,
		DeniedModels:  deniedModels,
		DefaultModel:  state.DefaultModel,
		LastUpdated:   "",
		UpdatedBy:     "",
	}

	err = deps.Config.SaveModelAccessState(newState, deps.Context.UserID())
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	deps.SecurityLogger.LogConfigChange(deps.Context.UserID(), "model_access",
		fmt.Sprintf("Allowed model: %s", model))

	return fmt.Sprintf("✅ Model '%s' has been added to the allowed list.", model), nil
}

func handleModelAccessDeny(ctx context.Context, deps *CommandsDependencies, model string) (string, error) {
	state, err := deps.Config.GetEffectiveModelAccess()
	if err != nil {
		return "", fmt.Errorf("getting current model access config: %w", err)
	}

	allowedModels := []string{}
	for _, allowed := range state.AllowedModels {
		if allowed != model {
			allowedModels = append(allowedModels, allowed)
		}
	}

	deniedModels := state.DeniedModels
	for _, denied := range deniedModels {
		if denied == model {
			return fmt.Sprintf("ℹ️ Model '%s' is already denied.", model), nil
		}
	}
	deniedModels = append(deniedModels, model)

	newState := &config.ModelAccessState{
		AllowedModels: allowedModels,
		DeniedModels:  deniedModels,
		DefaultModel:  state.DefaultModel,
		LastUpdated:   "",
		UpdatedBy:     "",
	}

	err = deps.Config.SaveModelAccessState(newState, deps.Context.UserID())
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	deps.SecurityLogger.LogConfigChange(deps.Context.UserID(), "model_access",
		fmt.Sprintf("Denied model: %s", model))

	return fmt.Sprintf("❌ Model '%s' has been added to the denied list.", model), nil
}

func handleModelAccessClear(ctx context.Context, deps *CommandsDependencies, userID string) (string, error) {
	newState := &config.ModelAccessState{
		AllowedModels: []string{},
		DeniedModels:  []string{},
		DefaultModel:  config.DefaultLanguageModel,
		LastUpdated:   "",
		UpdatedBy:     "",
	}

	err := deps.Config.SaveModelAccessState(newState, userID)
	if err != nil {
		return "", fmt.Errorf("saving model access state: %w", err)
	}

	deps.SecurityLogger.LogConfigChange(userID, "model_access", "Cleared all restrictions")

	return "✅ All model access restrictions have been cleared. All models are now allowed.", nil
}

func handleModelAccessStatus(ctx context.Context, deps *CommandsDependencies, targetUserID string) (string, error) {
	user, err := deps.SlackClient.GetUserInfo(targetUserID)
	if err != nil {
		return "", fmt.Errorf("getting user info: %w", err)
	}

	isAdmin := deps.TenantToolSet.IsAdmin(targetUserID)

	response := fmt.Sprintf("👤 **Model Access Status for @%s**\n\n", user.Name)

	if isAdmin {
		response += "👑 **Administrator** - Can bypass all model access restrictions.\n"
	} else {
		response += "👤 **Regular User** - Subject to model access restrictions.\n"
	}

	model := deps.Config.LanguageModel()
	allowed, reason := deps.Config.ValidateModelAccess(model, targetUserID)

	response += fmt.Sprintf("🤖 **Current Model:** %s\n", model)

	if allowed {
		response += "✅ **Access:** Allowed\n"
	} else {
		response += fmt.Sprintf("❌ **Access:** Denied\n📝 **Reason:** %s\n", reason)
		response += fmt.Sprintf("🔄 **Fallback:** Would use %s\n", config.DefaultLanguageModel)
	}

	return response, nil
}

func formatModelAccessResponse(state *config.ModelAccessState) string {
	var response strings.Builder
	fmt.Fprintf(&response, "🤖 **Model Access Configuration**\n\n")

	hasNoRestrictions := len(state.AllowedModels) == 0 && len(state.DeniedModels) == 0
	if hasNoRestrictions {
		response.WriteString("No restrictions in place - all models are allowed.\n")
	} else {
		formatModelList(&response, state.AllowedModels, "✅ **Allowed Models:**")
		formatModelList(&response, state.DeniedModels, "❌ **Denied Models:**")
	}

	fmt.Fprintf(&response, "\n🔧 **Default Model:** %s\n", state.DefaultModel)

	if state.UpdatedBy != "" && state.LastUpdated != "" {
		fmt.Fprintf(&response, "📝 **Last Updated:** %s by %s\n", state.LastUpdated, state.UpdatedBy)
	}

	return response.String()
}

func formatModelList(builder *strings.Builder, models []string, header string) {
	if len(models) == 0 {
		return
	}
	fmt.Fprintf(builder, "%s\n", header)
	for _, model := range models {
		fmt.Fprintf(builder, "  • %s\n", model)
	}
}
