package slacker

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// EnhancedIntentAnalyzer provides advanced intent failure analysis with multiple detection strategies
type EnhancedIntentAnalyzer struct {
	helpAnalyzer *HelpAnalyzer
	// Could add embedding service for semantic similarity in the future
}

// IntentMatch represents a potential intent match with confidence
type IntentMatch struct {
	Intent     *ToolManagementIntent
	Confidence float64
	MatchType  string // "exact", "semantic", "typo", "partial"
	Reason     string // Explanation of why this match was suggested
}

// SimilarCommand represents a command similar to the user's input
type SimilarCommand struct {
	Command    string
	Similarity float64
	MatchType  string
	Context    string // When/why this command might be relevant
}

// NewEnhancedIntentAnalyzer creates a new enhanced intent analyzer
func NewEnhancedIntentAnalyzer(helpAnalyzer *HelpAnalyzer) *EnhancedIntentAnalyzer {
	return &EnhancedIntentAnalyzer{
		helpAnalyzer: helpAnalyzer,
	}
}

// AnalyzeIntentFailureEnhanced provides comprehensive intent failure analysis
func (eia *EnhancedIntentAnalyzer) AnalyzeIntentFailureEnhanced(ctx context.Context, message string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	// Start with fallback analysis to avoid LLM dependency issues
	baseAnalysis := eia.helpAnalyzer.fallbackIntentAnalysis(message, helpCtx)

	// Try to get LLM analysis if available, but don't fail if it's not
	if eia.helpAnalyzer.llm != nil {
		llmAnalysis, err := eia.helpAnalyzer.AnalyzeIntentFailure(ctx, message, helpCtx)
		if err == nil {
			baseAnalysis = llmAnalysis
		}
	}

	// Enhance with additional analysis
	enhancedAnalysis := eia.enhanceIntentAnalysis(baseAnalysis, message, helpCtx)

	return enhancedAnalysis, nil
}

// enhanceIntentAnalysis adds advanced analysis to the base help analysis
func (eia *EnhancedIntentAnalyzer) enhanceIntentAnalysis(baseAnalysis *HelpAnalysis, message string, helpCtx *HelpContext) *HelpAnalysis {
	// Find similar commands using multiple strategies
	similarCommands := eia.findSimilarCommands(message, helpCtx)

	// Find potential typo corrections
	typos := eia.findTypoCorrections(message, helpCtx)

	// Find pattern completions
	completions := eia.findPatternCompletions(message, helpCtx)

	// Find contextual suggestions
	contextual := eia.findContextualSuggestions(message, helpCtx)

	// Merge all suggestions
	allSuggestions := make([]string, 0, len(baseAnalysis.Suggestions)+len(similarCommands)+len(typos)+len(completions))
	allSuggestions = append(allSuggestions, baseAnalysis.Suggestions...)

	// Add similar commands
	for _, cmd := range similarCommands {
		allSuggestions = append(allSuggestions, fmt.Sprintf("Did you mean: `%s`? (%s)", cmd.Command, cmd.Context))
	}

	// Add typo corrections
	for _, typo := range typos {
		allSuggestions = append(allSuggestions, fmt.Sprintf("Try: `%s` (corrected typo)", typo))
	}

	// Add completions
	for _, completion := range completions {
		allSuggestions = append(allSuggestions, fmt.Sprintf("Complete: `%s`", completion))
	}

	// Add contextual suggestions
	allSuggestions = append(allSuggestions, contextual...)

	// Enhance examples
	allExamples := make([]string, 0, len(baseAnalysis.Examples))
	allExamples = append(allExamples, baseAnalysis.Examples...)

	// Add examples for similar commands
	for _, cmd := range similarCommands[:min(3, len(similarCommands))] {
		if example := eia.getExampleForCommand(cmd.Command); example != "" {
			allExamples = append(allExamples, example)
		}
	}

	// Update diagnosis with more specific information
	enhancedDiagnosis := baseAnalysis.Diagnosis
	if len(similarCommands) > 0 {
		enhancedDiagnosis += fmt.Sprintf(" Found %d similar commands.", len(similarCommands))
	}
	if len(typos) > 0 {
		enhancedDiagnosis += fmt.Sprintf(" Found %d possible typo corrections.", len(typos))
	}

	return &HelpAnalysis{
		FailureType: baseAnalysis.FailureType,
		Diagnosis:   enhancedDiagnosis,
		Suggestions: allSuggestions,
		Examples:    allExamples,
		Actions:     baseAnalysis.Actions,
		ContextHelp: baseAnalysis.ContextHelp,
		Confidence:  baseAnalysis.Confidence, // Keep original confidence
	}
}

// findSimilarCommands finds commands that are semantically or structurally similar
func (eia *EnhancedIntentAnalyzer) findSimilarCommands(message string, helpCtx *HelpContext) []SimilarCommand {
	var similar []SimilarCommand

	// Common command patterns to check against
	commonCommands := []struct {
		cmd      string
		context  string
		keywords []string
	}{
		{"list my tools", "See what tools you have available", []string{"list", "show", "tools", "available"}},
		{"show preferences", "Display your current settings", []string{"show", "preferences", "settings", "config"}},
		{"add http tool at <url>", "Add an HTTP/MCP server tool", []string{"add", "http", "mcp", "server", "tool"}},
		{"add local program <path>", "Add a local executable tool", []string{"add", "local", "program", "executable"}},
		{"add docker tool <name>", "Add a Docker container tool", []string{"add", "docker", "container"}},
		{"thinking on", "Enable thinking display", []string{"thinking", "show", "thoughts", "verbose"}},
		{"thinking off", "Disable thinking display", []string{"thinking", "hide", "quiet"}},
		{"reset session", "Clear conversation history", []string{"reset", "clear", "new", "start"}},
	}

	messageLower := strings.ToLower(message)
	messageWords := strings.Fields(messageLower)

	for _, cmd := range commonCommands {
		// Calculate similarity based on keyword overlap
		similarity := eia.calculateKeywordSimilarity(messageWords, cmd.keywords)

		// Boost similarity if user has recently used similar commands
		if eia.hasRecentSimilarCommand(helpCtx.RecentCommands, cmd.cmd) {
			similarity += 0.1
		}

		if similarity > 0.3 { // Threshold for "similar"
			similar = append(similar, SimilarCommand{
				Command:    cmd.cmd,
				Similarity: similarity,
				MatchType:  "semantic",
				Context:    cmd.context,
			})
		}
	}

	// Sort by similarity descending
	sort.Slice(similar, func(i, j int) bool {
		return similar[i].Similarity > similar[j].Similarity
	})

	return similar[:min(5, len(similar))] // Return top 5
}

// findTypoCorrections finds likely typo corrections using edit distance
func (eia *EnhancedIntentAnalyzer) findTypoCorrections(message string, helpCtx *HelpContext) []string {
	var corrections []string

	// Common command words and their likely misspellings
	typos := map[string][]string{
		"list":        {"lst", "lis", "listt", "lits"},
		"show":        {"shwo", "sho", "showw", "shoow"},
		"tools":       {"tool", "toos", "toolls", "tolls"},
		"add":         {"ad", "add", "add", "addd"},
		"http":        {"htp", "htt", "http", "htp"},
		"docker":      {"doker", "docer", "dokcer", "dockr"},
		"thinking":    {"think", "thining", "thnking", "thinkng"},
		"preferences": {"preference", "prefs", "pref", "prefernces"},
		"session":     {"sesion", "sesson", "sesssion", "sesison"},
	}

	messageWords := strings.Fields(strings.ToLower(message))

	for _, word := range messageWords {
		for correctWord, misspellings := range typos {
			for _, misspelling := range misspellings {
				if eia.editDistance(word, misspelling) <= 1 {
					// Found a typo, suggest the corrected command
					correctedMessage := strings.ReplaceAll(strings.ToLower(message), misspelling, correctWord)
					if correctedMessage != strings.ToLower(message) {
						corrections = append(corrections, correctedMessage)
					}
				}
			}
		}
	}

	// Remove duplicates
	uniqueCorrections := make([]string, 0, len(corrections))
	seen := make(map[string]bool)
	for _, correction := range corrections {
		if !seen[correction] {
			seen[correction] = true
			uniqueCorrections = append(uniqueCorrections, correction)
		}
	}

	return uniqueCorrections
}

// findPatternCompletions finds commands that might be incomplete
func (eia *EnhancedIntentAnalyzer) findPatternCompletions(message string, helpCtx *HelpContext) []string {
	var completions []string

	messageLower := strings.ToLower(message)

	// Check for incomplete patterns
	patterns := []struct {
		prefix   string
		complete string
	}{
		{"add", "add http tool at <url>"},
		{"add http", "add http tool at <url>"},
		{"add local", "add local program <path>"},
		{"add docker", "add docker tool <name>"},
		{"show", "show preferences"},
		{"list", "list my tools"},
		{"thinking", "thinking on/off"},
		{"reset", "reset session"},
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(messageLower, pattern.prefix) && messageLower != pattern.prefix {
			completions = append(completions, pattern.complete)
		}
	}

	return completions
}

// findContextualSuggestions provides suggestions based on user's context
func (eia *EnhancedIntentAnalyzer) findContextualSuggestions(message string, helpCtx *HelpContext) []string {
	var suggestions []string

	// If user recently used tool commands, suggest more tool management
	if eia.hasRecentToolCommands(helpCtx.RecentCommands) {
		suggestions = append(suggestions, "💡 Try: `list my tools` to see what's available")
		suggestions = append(suggestions, "💡 Try: `add http tool at <url>` to add a new tool")
	}

	// If user is admin, suggest admin commands
	if helpCtx.IsAdmin {
		suggestions = append(suggestions, "👑 Admin: Try `model access list` to manage model permissions")
		suggestions = append(suggestions, "👑 Admin: Try `status model access for @user` to check user permissions")
	}

	// If message contains URLs but is malformed, suggest correct syntax
	if strings.Contains(message, "http") && !strings.Contains(strings.ToLower(message), "add http") {
		suggestions = append(suggestions, "🔗 Found URL - try: `add http tool at <your-url>`")
	}

	// If message mentions files or paths, suggest local program tool
	if strings.Contains(message, "/") || strings.Contains(message, ".exe") || strings.Contains(message, ".sh") {
		suggestions = append(suggestions, "📁 Found path - try: `add local program <path-to-program>`")
	}

	return suggestions
}

// calculateKeywordSimilarity calculates similarity based on keyword overlap
func (eia *EnhancedIntentAnalyzer) calculateKeywordSimilarity(messageWords []string, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	matches := 0
	for _, word := range messageWords {
		for _, keyword := range keywords {
			if strings.Contains(word, keyword) || strings.Contains(keyword, word) {
				matches++
				break
			}
		}
	}

	return float64(matches) / float64(len(keywords))
}

// hasRecentSimilarCommand checks if user has recently used similar commands
func (eia *EnhancedIntentAnalyzer) hasRecentSimilarCommand(recentCommands []string, command string) bool {
	commandWords := strings.Fields(strings.ToLower(command))

	for _, recent := range recentCommands {
		recentWords := strings.Fields(strings.ToLower(recent))
		// If commands share 2+ words, consider them similar
		commonWords := 0
		for _, cmdWord := range commandWords {
			for _, recentWord := range recentWords {
				if cmdWord == recentWord {
					commonWords++
					break
				}
			}
		}
		if commonWords >= 2 {
			return true
		}
	}

	return false
}

// hasRecentToolCommands checks if user recently used tool-related commands
func (eia *EnhancedIntentAnalyzer) hasRecentToolCommands(recentCommands []string) bool {
	for _, cmd := range recentCommands {
		if strings.Contains(strings.ToLower(cmd), "tool") ||
			strings.Contains(strings.ToLower(cmd), "add") ||
			strings.Contains(strings.ToLower(cmd), "list") {
			return true
		}
	}
	return false
}

// editDistance calculates the Levenshtein distance between two strings
func (eia *EnhancedIntentAnalyzer) editDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	if s1[0] == s2[0] {
		return eia.editDistance(s1[1:], s2[1:])
	}

	return 1 + min3(
		eia.editDistance(s1[1:], s2),
		eia.editDistance(s1, s2[1:]),
		eia.editDistance(s1[1:], s2[1:]),
	)
}

// getExampleForCommand returns an example usage for a command
func (eia *EnhancedIntentAnalyzer) getExampleForCommand(command string) string {
	examples := map[string]string{
		"list my tools":            "list my tools",
		"show preferences":         "show preferences",
		"add http tool at <url>":   "add http tool at https://api.example.com/mcp",
		"add local program <path>": "add local program /usr/bin/git",
		"add docker tool <name>":   "add docker tool nginx",
		"thinking on":              "thinking on",
		"thinking off":             "thinking off",
		"reset session":            "reset session",
	}

	if example, exists := examples[command]; exists {
		return example
	}

	// Return a generic example for unknown commands
	return fmt.Sprintf("Example: %s", command)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func min3(a, b, c int) int {
	return min(min(a, b), c)
}
