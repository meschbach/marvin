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
func (sf *SlackFormatter) ParseMessageToBlocks(message string) []slack.Block {
	message = sf.NormalizeMarkdown(message)
	var blocks []slack.Block
	lines := strings.Split(message, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		newBlocks, newIdx := sf.processLine(lines, i, line)
		blocks = append(blocks, newBlocks...)
		if newIdx > i {
			i = newIdx
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, slack.NewSectionBlock(&slack.TextBlockObject{
			Type: slack.MarkdownType,
			Text: message,
		}, nil, nil))
	}

	return blocks
}

func (sf *SlackFormatter) processLine(lines []string, i int, line string) ([]slack.Block, int) {
	if sf.isH1Header(line) {
		headerText := strings.TrimPrefix(line, "# ")
		return []slack.Block{sf.createHeaderBlock(headerText)}, i
	}

	if sf.isH2Header(line) {
		headerText := strings.TrimPrefix(line, "## ")
		return []slack.Block{sf.createHeaderBlock(headerText)}, i
	}

	if sf.isH3OrHigherHeader(line) {
		boldText := strings.TrimPrefix(strings.TrimPrefix(line, "### "), "#### ")
		return []slack.Block{sf.createBoldSectionBlock(boldText)}, i
	}

	if sf.isBlockQuote(line) {
		quoteLines, newIdx := sf.collectQuoteLines(lines, i)
		return []slack.Block{sf.createQuoteBlock(quoteLines)}, newIdx
	}

	if line == "" {
		return nil, i
	}

	contentLines, newIdx := sf.collectContentLines(lines, i)
	if len(contentLines) > 0 {
		return []slack.Block{sf.createContentBlock(contentLines)}, newIdx
	}
	return nil, i
}

func (sf *SlackFormatter) isH1Header(line string) bool {
	return strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ")
}

func (sf *SlackFormatter) isH2Header(line string) bool {
	return strings.HasPrefix(line, "## ")
}

func (sf *SlackFormatter) isH3OrHigherHeader(line string) bool {
	return strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "#### ")
}

func (sf *SlackFormatter) isBlockQuote(line string) bool {
	return strings.HasPrefix(line, "> ")
}

func (sf *SlackFormatter) createHeaderBlock(text string) slack.Block {
	return slack.NewHeaderBlock(&slack.TextBlockObject{
		Type: slack.PlainTextType,
		Text: text,
	})
}

func (sf *SlackFormatter) createBoldSectionBlock(text string) slack.Block {
	return slack.NewSectionBlock(&slack.TextBlockObject{
		Type: slack.MarkdownType,
		Text: "*" + text + "*",
	}, nil, nil)
}

func (sf *SlackFormatter) createQuoteBlock(quoteLines []string) slack.Block {
	quoteText := strings.Join(quoteLines, "\n")
	return slack.NewSectionBlock(&slack.TextBlockObject{
		Type: slack.MarkdownType,
		Text: quoteText,
	}, nil, nil)
}

func (sf *SlackFormatter) createContentBlock(contentLines []string) slack.Block {
	contentText := strings.Join(contentLines, "\n")
	return slack.NewSectionBlock(&slack.TextBlockObject{
		Type: slack.MarkdownType,
		Text: contentText,
	}, nil, nil)
}

func (sf *SlackFormatter) collectQuoteLines(lines []string, startIdx int) ([]string, int) {
	var quoteLines []string
	for i := startIdx; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "> "); i++ {
		quoteLine := strings.TrimPrefix(strings.TrimSpace(lines[i]), "> ")
		quoteLines = append(quoteLines, quoteLine)
	}
	return quoteLines, startIdx + len(quoteLines) - 1
}

func (sf *SlackFormatter) collectContentLines(lines []string, startIdx int) ([]string, int) {
	var contentLines []string
	for i := startIdx; i < len(lines); i++ {
		currentLine := strings.TrimSpace(lines[i])
		if currentLine == "" {
			break
		}
		if sf.isHeaderLine(currentLine) || strings.HasPrefix(currentLine, "> ") {
			break
		}
		contentLines = append(contentLines, lines[i])
	}
	return contentLines, startIdx + len(contentLines) - 1
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
