package slackapp

import (
	"path"
	"strings"

	slacksdk "github.com/slack-go/slack"
)

type SlackMediaItem struct {
	Kind     string
	URL      string
	Name     string
	MimeType string
	AltText  string
	Source   string
	Size     int64
}

func SlackMessageMediaItems(message slacksdk.Message) []SlackMediaItem {
	var out []SlackMediaItem
	for _, attachment := range message.Attachments {
		out = appendImageURL(out, attachment.ImageURL, firstNonEmpty(attachment.Title, "attachment-image"), "attachment.image_url")
		out = appendImageURL(out, attachment.ThumbURL, firstNonEmpty(attachment.Title, "attachment-thumb"), "attachment.thumb_url")
		out = append(out, slackBlocksMediaItems(attachment.Blocks)...)
	}
	out = append(out, slackBlocksMediaItems(message.Blocks)...)
	for _, file := range message.Files {
		item := slackFileMediaItem(file)
		if item.URL != "" && item.Kind != "" {
			out = append(out, item)
		}
	}
	return dedupeSlackMediaItems(out)
}

func slackBlocksMediaItems(blocks slacksdk.Blocks) []SlackMediaItem {
	var out []SlackMediaItem
	for _, block := range blocks.BlockSet {
		switch typed := block.(type) {
		case *slacksdk.ImageBlock:
			out = appendImageURL(out, slackImageBlockURL(typed), firstNonEmpty(typed.AltText, "image"), "block.image")
		case *slacksdk.VideoBlock:
			out = appendImageURL(out, typed.ThumbnailURL, firstNonEmpty(typed.AltText, "video-thumbnail"), "block.video.thumbnail")
		case *slacksdk.SectionBlock:
			if typed.Accessory != nil {
				out = append(out, accessoryMediaItems(typed.Accessory)...)
			}
		case *slacksdk.ContextBlock:
			for _, element := range typed.ContextElements.Elements {
				if image, ok := element.(*slacksdk.ImageBlockElement); ok {
					out = appendImageURL(out, slackImageElementURL(image), firstNonEmpty(image.AltText, "context-image"), "block.context.image")
				}
			}
		}
	}
	return out
}

func accessoryMediaItems(accessory *slacksdk.Accessory) []SlackMediaItem {
	if accessory.ImageElement == nil {
		return nil
	}
	return appendImageURL(nil, slackImageElementURL(accessory.ImageElement), firstNonEmpty(accessory.ImageElement.AltText, "image"), "block.accessory.image")
}

func slackImageBlockURL(block *slacksdk.ImageBlock) string {
	if block == nil {
		return ""
	}
	if strings.TrimSpace(block.ImageURL) != "" {
		return strings.TrimSpace(block.ImageURL)
	}
	if block.SlackFile != nil {
		return strings.TrimSpace(block.SlackFile.URL)
	}
	return ""
}

func slackImageElementURL(element *slacksdk.ImageBlockElement) string {
	if element == nil {
		return ""
	}
	if element.ImageURL != nil && strings.TrimSpace(*element.ImageURL) != "" {
		return strings.TrimSpace(*element.ImageURL)
	}
	if element.SlackFile != nil {
		return strings.TrimSpace(element.SlackFile.URL)
	}
	return ""
}

func appendImageURL(items []SlackMediaItem, rawURL, name, source string) []SlackMediaItem {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return items
	}
	return append(items, SlackMediaItem{
		Kind:     "image",
		URL:      rawURL,
		Name:     mediaName(name, rawURL, "image"),
		MimeType: mimeFromImageURL(rawURL),
		AltText:  cleanSlackText(name),
		Source:   source,
	})
}

func slackFileMediaItem(file slacksdk.File) SlackMediaItem {
	mimeType := strings.TrimSpace(file.Mimetype)
	kind := mediaKindFromMime(mimeType)
	if kind == "" {
		return SlackMediaItem{}
	}
	rawURL := firstNonEmpty(file.URLPrivateDownload, file.URLPrivate)
	return SlackMediaItem{
		Kind:     kind,
		URL:      rawURL,
		Name:     mediaName(firstNonEmpty(file.Title, file.Name, file.ID), rawURL, kind),
		MimeType: mimeType,
		AltText:  firstNonEmpty(file.Title, file.Name),
		Source:   "file",
		Size:     int64(file.Size),
	}
}

func mediaKindFromMime(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "audio/"), mimeType == "video/mp4", mimeType == "video/webm":
		return "audio"
	default:
		return ""
	}
}

func mediaName(raw, rawURL, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	if base := path.Base(strings.TrimSpace(rawURL)); base != "." && base != "/" && base != "" {
		return base
	}
	return fallback
}

func mimeFromImageURL(rawURL string) string {
	lower := strings.ToLower(strings.Split(rawURL, "?")[0])
	switch {
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return ""
	}
}

func dedupeSlackMediaItems(items []SlackMediaItem) []SlackMediaItem {
	out := make([]SlackMediaItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := item.Kind + "\x00" + item.URL
		if item.URL == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
