package slacker

import (
	"context"
	"strings"

	"github.com/slack-go/slack"
)

// SlackFormatter formats content for Slack display
type SlackFormatter struct{}

// NewSlackFormatter creates a new Slack formatter
func NewSlackFormatter() *SlackFormatter {
	return &SlackFormatter{}
}

// Format converts content to Slack blocks based on content type
func (sf *SlackFormatter) Format(ctx context.Context, content string, contentType ContentType) ([]slack.Block, error) {
	if contentType == ContentIgnore {
		return nil, nil
	}
	return sf.ParseMessageToBlocks(content), nil
}

// ParseMessageToBlocks converts markdown message to Slack blocks with proper header handling
//
//nolint:gocyclo
func (sf *SlackFormatter) ParseMessageToBlocks(message string) []slack.Block {
	// Normalize markdown for Slack compatibility
	message = sf.NormalizeMarkdown(message)
	var blocks []slack.Block
	lines := strings.Split(message, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Handle H1 headers (# Header)
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			headerText := strings.TrimPrefix(line, "# ")
			headerBlock := slack.NewHeaderBlock(&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: headerText,
			})
			blocks = append(blocks, headerBlock)
			continue
		}

		// Handle H2 headers (## Header)
		if strings.HasPrefix(line, "## ") {
			headerText := strings.TrimPrefix(line, "## ")
			headerBlock := slack.NewHeaderBlock(&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: headerText,
			})
			blocks = append(blocks, headerBlock)
			continue
		}

		// Handle H3+ headers (### Header, #### Header) - convert to bold text
		if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ") {
			boldText := strings.TrimPrefix(strings.TrimPrefix(line, "### "), "#### ")
			sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "*" + boldText + "*",
			}, nil, nil)
			blocks = append(blocks, sectionBlock)
			continue
		}

		// Handle block quotes (> Quote text)
		if strings.HasPrefix(line, "> ") {
			// Collect consecutive quote lines
			var quoteLines []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> ") {
				quoteLine := strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")
				quoteLines = append(quoteLines, quoteLine)
				i++
			}
			i-- // Back up one since we'll increment in the outer loop

			if len(quoteLines) > 0 {
				quoteText := strings.Join(quoteLines, "\n")
				sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: quoteText,
				}, nil, nil)
				blocks = append(blocks, sectionBlock)
			}
			continue
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		// For regular content, collect consecutive non-empty lines that aren't special
		var contentLines []string
		for i < len(lines) {
			currentLine := strings.TrimSpace(lines[i])
			if currentLine == "" {
				break // Empty line breaks content collection
			}
			if sf.isHeaderLine(currentLine) || strings.HasPrefix(currentLine, "> ") {
				break // Header or quote breaks content collection
			}
			contentLines = append(contentLines, lines[i])
			i++
		}
		i-- // Back up one since we'll increment in the outer loop

		if len(contentLines) > 0 {
			contentText := strings.Join(contentLines, "\n")
			sectionBlock := slack.NewSectionBlock(&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: contentText,
			}, nil, nil)
			blocks = append(blocks, sectionBlock)
		}
	}

	// If no blocks were created (message was empty or only had unsupported content),
	// create a simple section block
	if len(blocks) == 0 {
		blocks = append(blocks, slack.NewSectionBlock(&slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: message,
		}, nil, nil))
	}

	return blocks
}

// NormalizeMarkdown converts standard markdown to Slack's mrkdwn format
func (sf *SlackFormatter) NormalizeMarkdown(text string) string {
	result := text

	// Convert **bold** to *bold* (Slack uses single asterisks for bold)
	result = strings.ReplaceAll(result, "**", "*")

	// Handle edge case where replacement might create ***bold***
	// Replace *** with * (which will become bold)
	result = strings.ReplaceAll(result, "***", "*")

	// Convert __bold__ to *bold* (another common markdown variant)
	result = strings.ReplaceAll(result, "__", "*")

	// Convert *italic* to _italic_ (Slack prefers underscores for italics)
	// Be careful not to interfere with bold formatting
	result = strings.ReplaceAll(result, "*_", "_")
	result = strings.ReplaceAll(result, "_*", "_")

	return result
}

// isHeaderLine checks if a line is a markdown header
func (sf *SlackFormatter) isHeaderLine(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ") ||
		strings.HasPrefix(line, "#### ")
}
