package slackapp

import (
	"fmt"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

func renderRichTextBlock(block *slacksdk.RichTextBlock) string {
	parts := make([]string, 0, len(block.Elements))
	for _, element := range block.Elements {
		if text := renderRichTextElement(element); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func renderRichTextElement(element slacksdk.RichTextElement) string {
	switch typed := element.(type) {
	case *slacksdk.RichTextSection:
		return renderRichTextSectionElements(typed.Elements)
	case *slacksdk.RichTextList:
		return renderRichTextList(typed)
	case *slacksdk.RichTextQuote:
		text := renderRichTextSectionElements(typed.Elements)
		if text == "" {
			return ""
		}
		return "> " + strings.ReplaceAll(text, "\n", "\n> ")
	case *slacksdk.RichTextPreformatted:
		text := renderRichTextSectionElements(typed.Elements)
		if text == "" {
			return ""
		}
		lang := strings.TrimSpace(typed.Language)
		return "```" + lang + "\n" + text + "\n```"
	default:
		return compactJSON(element)
	}
}

func renderRichTextList(list *slacksdk.RichTextList) string {
	var lines []string
	for i, element := range list.Elements {
		text := renderRichTextElement(element)
		if text == "" {
			continue
		}
		prefix := "-"
		if list.Style == slacksdk.RTEListOrdered {
			prefix = fmt.Sprintf("%d.", list.Offset+i+1)
		}
		indent := strings.Repeat("  ", max(0, list.Indent))
		lines = append(lines, indent+prefix+" "+strings.ReplaceAll(text, "\n", "\n"+indent+"  "))
	}
	return strings.Join(lines, "\n")
}

func renderRichTextSectionElements(elements []slacksdk.RichTextSectionElement) string {
	var b strings.Builder
	for _, element := range elements {
		b.WriteString(renderRichTextSectionElement(element))
	}
	return cleanSlackText(b.String())
}

func renderRichTextSectionElement(element slacksdk.RichTextSectionElement) string {
	switch typed := element.(type) {
	case *slacksdk.RichTextSectionTextElement:
		return applyRichTextStyle(typed.Text, typed.Style)
	case *slacksdk.RichTextSectionChannelElement:
		return applyRichTextStyle("<#"+typed.ChannelID+">", typed.Style)
	case *slacksdk.RichTextSectionUserElement:
		return applyRichTextStyle("<@"+typed.UserID+">", typed.Style)
	case *slacksdk.RichTextSectionEmojiElement:
		return ":" + typed.Name + ":"
	case *slacksdk.RichTextSectionLinkElement:
		label := firstNonEmpty(typed.Text, typed.URL)
		return applyRichTextStyle(markdownLinkOrText(label, typed.URL), typed.Style)
	case *slacksdk.RichTextSectionTeamElement:
		return applyRichTextStyle("<!team^"+typed.TeamID+">", typed.Style)
	case *slacksdk.RichTextSectionUserGroupElement:
		return "<!subteam^" + typed.UsergroupID + ">"
	case *slacksdk.RichTextSectionDateElement:
		if typed.Fallback != nil && *typed.Fallback != "" {
			return *typed.Fallback
		}
		return fmt.Sprintf("<!date^%d^%s>", typed.Timestamp, typed.Format)
	case *slacksdk.RichTextSectionBroadcastElement:
		return "<!" + typed.Range + ">"
	case *slacksdk.RichTextSectionColorElement:
		return typed.Value
	default:
		return compactJSON(element)
	}
}

func applyRichTextStyle(text string, style *slacksdk.RichTextSectionTextStyle) string {
	text = cleanSlackText(text)
	if text == "" || style == nil {
		return text
	}
	if style.Code {
		text = "`" + text + "`"
	}
	if style.Bold {
		text = "**" + text + "**"
	}
	if style.Italic {
		text = "_" + text + "_"
	}
	if style.Strike {
		text = "~~" + text + "~~"
	}
	return text
}
