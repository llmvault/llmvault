package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/imagedescription"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/system"
)

func (e *slackMediaEnricher) enrichSlackImage(ctx context.Context, token string, item slackapp.SlackMediaItem) string {
	data, mimeType, notice := e.downloadSlackMedia(ctx, token, item)
	if notice != "" {
		return slackMediaNoticeXML("image", item, item.MimeType, notice)
	}
	if e.gateway == nil || e.kms == nil {
		return slackMediaNoticeXML("image", item, mimeType, "image description service unavailable")
	}
	route, ok := e.registry.ResolveModel(slackImageDescribeProvider, slackImageDescribeModel)
	if !ok {
		return slackMediaNoticeXML("image", item, mimeType, "image description model unavailable")
	}
	cred, err := slackMediaCredential(ctx, e.db, slackImageDescribeProvider)
	if err != nil {
		return slackMediaNoticeXML("image", item, mimeType, "image description credential unavailable")
	}
	apiKey, err := decryptSlackMediaCredential(ctx, e.kms, cred)
	if err != nil {
		return slackMediaNoticeXML("image", item, mimeType, "image description credential decrypt failed")
	}
	defer zeroBytes(apiKey)

	temp := float32(0)
	req := &system.LLMRequest{
		Model: route.UpstreamID,
		Messages: []system.LLMMessage{
			{Role: "system", Content: imagedescription.SystemPrompt},
			{Role: "user", Parts: []system.LLMPart{
				{Kind: system.LLMPartText, Text: fmt.Sprintf("Filename: %s\nContent type: %s\nAlt text: %s", item.Name, mimeType, item.AltText)},
				{Kind: system.LLMPartMedia, ContentType: mimeType, Text: dataURL(mimeType, data)},
			}},
		},
		MaxTokens:      imagedescription.MaxTokens,
		Temperature:    &temp,
		ResponseFormat: system.JSONResponseSpec(),
	}
	res, err := e.gateway.Complete(ctx, system.ForwardCall{
		ProviderID: slackImageDescribeProvider,
		BaseURL:    cred.BaseURL,
		APIKey:     string(apiKey),
		AuthScheme: cred.AuthScheme,
		Request:    req,
		Stream:     false,
	})
	if err != nil {
		return slackMediaNoticeXML("image", item, mimeType, "image description failed: "+err.Error())
	}
	return slackImageXML(item, mimeType, parseSlackImageAnalysis(res.Text))
}

func slackImageXML(item slackapp.SlackMediaItem, mimeType string, analysis map[string]any) string {
	var sections []string
	if details := compactSlackMediaValue(analysis["important_details"]); details != "" {
		sections = append(sections, "<important_details>\n"+escapeXMLText(details)+"\n</important_details>")
	}
	if visible := compactSlackMediaValue(analysis["visible_text"]); visible != "" {
		sections = append(sections, "<visible_text>\n"+escapeXMLText(visible)+"\n</visible_text>")
	}
	full := slackImageFullDescription(analysis)
	if full != "" {
		sections = append(sections, "<full_description>\n"+escapeXMLText(full)+"\n</full_description>")
	}
	if summary := compactSlackMediaValue(analysis["summary"]); summary != "" {
		sections = append(sections, "<short_description>\n"+escapeXMLText(summary)+"\n</short_description>")
	}
	if len(sections) == 0 {
		sections = append(sections, "<notice>Image was downloaded but no description was produced.</notice>")
	}
	return slackAttachmentXML("image", item, mimeType, strings.Join(sections, "\n"))
}

func slackImageFullDescription(analysis map[string]any) string {
	var lines []string
	for _, key := range []string{"category", "confidence", "summary", "visible_text", "important_details", "limitations"} {
		if text := compactSlackMediaValue(analysis[key]); text != "" {
			lines = append(lines, key+": "+text)
		}
	}
	return strings.Join(lines, "\n")
}
