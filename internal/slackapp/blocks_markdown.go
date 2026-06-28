package slackapp

import (
	"fmt"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

func renderSlackBlocks(blocks slacksdk.Blocks) string {
	out := make([]string, 0, len(blocks.BlockSet))
	for _, block := range blocks.BlockSet {
		if text := renderSlackBlock(block); text != "" {
			out = append(out, text)
		}
	}
	return strings.Join(out, "\n\n")
}

func renderSlackBlock(block slacksdk.Block) string {
	switch b := block.(type) {
	case *slacksdk.SectionBlock:
		return renderSectionBlock(b)
	case *slacksdk.HeaderBlock:
		if b.Text != nil {
			return "### " + cleanSlackText(b.Text.Text)
		}
	case *slacksdk.ContextBlock:
		return renderContextBlock(b)
	case *slacksdk.ImageBlock:
		title := ""
		if b.Title != nil {
			title = b.Title.Text
		}
		return renderImageReference(firstNonEmpty(title, b.AltText, "image"), slackImageBlockURL(b))
	case *slacksdk.VideoBlock:
		return renderVideoBlock(b)
	case *slacksdk.MarkdownBlock:
		return cleanSlackText(b.Text)
	case *slacksdk.RichTextBlock:
		return renderRichTextBlock(b)
	case *slacksdk.TableBlock:
		return renderTableBlock(b)
	case *slacksdk.DividerBlock:
		return "---"
	case *slacksdk.ActionBlock:
		if b.Elements != nil {
			return renderBlockElements(*b.Elements)
		}
	case *slacksdk.FileBlock:
		return "File block: " + b.ExternalID
	default:
		return compactJSON(block)
	}
	return ""
}

func renderSectionBlock(block *slacksdk.SectionBlock) string {
	var parts []string
	if block.Text != nil {
		parts = append(parts, cleanSlackText(block.Text.Text))
	}
	for _, field := range block.Fields {
		if field != nil && cleanSlackText(field.Text) != "" {
			parts = append(parts, "- "+cleanSlackText(field.Text))
		}
	}
	if block.Accessory != nil {
		if text := renderAccessory(block.Accessory); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func renderContextBlock(block *slacksdk.ContextBlock) string {
	parts := make([]string, 0, len(block.ContextElements.Elements))
	for _, element := range block.ContextElements.Elements {
		switch typed := element.(type) {
		case *slacksdk.TextBlockObject:
			parts = append(parts, cleanSlackText(typed.Text))
		case *slacksdk.ImageBlockElement:
			if url := slackImageElementURL(typed); url != "" {
				parts = append(parts, renderImageReference(firstNonEmpty(typed.AltText, "context image"), url))
			}
		default:
			if raw := compactJSON(element); raw != "" {
				parts = append(parts, raw)
			}
		}
	}
	return strings.Join(parts, " · ")
}

func renderAccessory(accessory *slacksdk.Accessory) string {
	switch {
	case accessory.ImageElement != nil:
		return renderImageReference(firstNonEmpty(accessory.ImageElement.AltText, "image"), slackImageElementURL(accessory.ImageElement))
	case accessory.ButtonElement != nil:
		text := ""
		if accessory.ButtonElement.Text != nil {
			text = accessory.ButtonElement.Text.Text
		}
		return "Button: " + markdownLinkOrText(firstNonEmpty(text, accessory.ButtonElement.Value, "button"), accessory.ButtonElement.URL)
	case accessory.SelectElement != nil:
		return "Select menu: " + accessory.SelectElement.Type
	case accessory.MultiSelectElement != nil:
		return "Multi-select menu: " + accessory.MultiSelectElement.Type
	case accessory.UnknownElement != nil:
		return compactJSON(accessory.UnknownElement)
	default:
		return compactJSON(accessory)
	}
}

func renderBlockElements(elements slacksdk.BlockElements) string {
	parts := make([]string, 0, len(elements.ElementSet))
	for _, element := range elements.ElementSet {
		if raw := compactJSON(element); raw != "" {
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, "\n")
}

func renderImageReference(label, url string) string {
	label = cleanSlackText(label)
	url = strings.TrimSpace(url)
	if url == "" {
		return "Image: " + label
	}
	return "Image: [" + label + "](" + url + ")"
}

func renderVideoBlock(block *slacksdk.VideoBlock) string {
	title := "video"
	if block.Title != nil && cleanSlackText(block.Title.Text) != "" {
		title = cleanSlackText(block.Title.Text)
	}
	var lines []string
	lines = append(lines, "Video: "+markdownLinkOrText(title, firstNonEmpty(block.TitleURL, block.VideoURL)))
	if block.Description != nil && cleanSlackText(block.Description.Text) != "" {
		lines = append(lines, cleanSlackText(block.Description.Text))
	}
	if block.ThumbnailURL != "" {
		lines = append(lines, "Thumbnail: "+block.ThumbnailURL)
	}
	if block.ProviderName != "" {
		lines = append(lines, "Provider: "+block.ProviderName)
	}
	return strings.Join(lines, "\n")
}

func renderTableBlock(block *slacksdk.TableBlock) string {
	rows := make([]string, 0, len(block.Rows))
	for _, row := range block.Rows {
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			if cell == nil {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, strings.ReplaceAll(renderRichTextBlock(cell), "\n", " "))
		}
		rows = append(rows, fmt.Sprintf("| %s |", strings.Join(cells, " | ")))
	}
	return strings.Join(rows, "\n")
}
