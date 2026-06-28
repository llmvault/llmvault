package slackapp

import (
	"encoding/json"
	"fmt"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

func RenderSlackMessageMarkdown(message slacksdk.Message) string {
	var parts []string
	if text := cleanSlackText(message.Text); text != "" {
		parts = append(parts, "Top-level text:\n"+text)
	}
	if blockText := renderSlackBlocks(message.Blocks); blockText != "" {
		parts = append(parts, "Blocks:\n"+blockText)
	}
	if attachmentText := renderSlackAttachments(message.Attachments); attachmentText != "" {
		parts = append(parts, "Attachments:\n"+attachmentText)
	}
	if fileText := renderSlackFiles(message.Files); fileText != "" {
		parts = append(parts, "Files:\n"+fileText)
	}
	if len(parts) == 0 {
		return "(no text)"
	}
	return strings.Join(parts, "\n\n")
}

func renderSlackAttachments(attachments []slacksdk.Attachment) string {
	sections := make([]string, 0, len(attachments))
	for i, attachment := range attachments {
		var lines []string
		if attachment.Title != "" {
			lines = append(lines, "### "+markdownLinkOrText(attachment.Title, attachment.TitleLink))
		}
		if attachment.Pretext != "" {
			lines = append(lines, cleanSlackText(attachment.Pretext))
		}
		if attachment.AuthorName != "" {
			lines = append(lines, "Author: "+markdownLinkOrText(attachment.AuthorName, attachment.AuthorLink))
		}
		if attachment.Text != "" {
			lines = append(lines, cleanSlackText(attachment.Text))
		}
		for _, field := range attachment.Fields {
			if strings.TrimSpace(field.Title) == "" && strings.TrimSpace(field.Value) == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- **%s:** %s", cleanSlackText(field.Title), cleanSlackText(field.Value)))
		}
		if blockText := renderSlackBlocks(attachment.Blocks); blockText != "" {
			lines = append(lines, blockText)
		}
		if attachment.ImageURL != "" {
			lines = append(lines, "Image: "+attachment.ImageURL)
		}
		if attachment.ThumbURL != "" {
			lines = append(lines, "Thumbnail: "+attachment.ThumbURL)
		}
		if attachment.Footer != "" {
			lines = append(lines, "Footer: "+cleanSlackText(attachment.Footer))
		}
		if attachment.Color != "" {
			lines = append(lines, "Color: "+attachment.Color)
		}
		if len(lines) == 0 {
			if raw := compactJSON(attachment); raw != "" {
				lines = append(lines, raw)
			}
		}
		if len(lines) > 0 {
			sections = append(sections, fmt.Sprintf("Attachment %d:\n%s", i+1, strings.Join(lines, "\n")))
		}
	}
	return strings.Join(sections, "\n\n")
}

func renderSlackFiles(files []slacksdk.File) string {
	sections := make([]string, 0, len(files))
	for _, file := range files {
		name := firstNonEmpty(file.Title, file.Name, file.ID, "file")
		var lines []string
		lines = append(lines, "- name: "+name)
		if file.Mimetype != "" {
			lines = append(lines, "- mime_type: "+file.Mimetype)
		}
		if file.PrettyType != "" {
			lines = append(lines, "- type: "+file.PrettyType)
		}
		if file.Size > 0 {
			lines = append(lines, fmt.Sprintf("- size_bytes: %d", file.Size))
		}
		if file.URLPrivateDownload != "" {
			lines = append(lines, "- url_private_download: "+file.URLPrivateDownload)
		} else if file.URLPrivate != "" {
			lines = append(lines, "- url_private: "+file.URLPrivate)
		}
		if file.Permalink != "" {
			lines = append(lines, "- permalink: "+file.Permalink)
		}
		if file.PreviewPlainText != "" {
			lines = append(lines, "\nPreview:\n"+cleanSlackText(file.PreviewPlainText))
		} else if file.PlainText != "" {
			lines = append(lines, "\nPlain text:\n"+cleanSlackText(file.PlainText))
		} else if file.Preview != "" {
			lines = append(lines, "\nPreview:\n"+cleanSlackText(file.Preview))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func markdownLinkOrText(text, url string) string {
	text = cleanSlackText(text)
	url = strings.TrimSpace(url)
	if text == "" {
		return url
	}
	if url == "" {
		return text
	}
	return "[" + text + "](" + url + ")"
}

func cleanSlackText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}

func compactJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" || string(raw) == "{}" || string(raw) == "[]" {
		return ""
	}
	return string(raw)
}
