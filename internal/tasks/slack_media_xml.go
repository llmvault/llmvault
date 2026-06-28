package tasks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/usehivy/hivy/internal/slackapp"
)

func slackAttachmentXML(kind string, item slackapp.SlackMediaItem, mimeType, body string) string {
	var attrs []string
	attrs = append(attrs, `type="`+escapeXMLText(kind)+`"`)
	for _, attr := range []struct {
		name  string
		value string
	}{
		{"name", item.Name},
		{"url", item.URL},
		{"mime_type", mimeType},
		{"source", item.Source},
		{"alt_text", item.AltText},
	} {
		if strings.TrimSpace(attr.value) != "" {
			attrs = append(attrs, attr.name+`="`+escapeXMLText(strings.TrimSpace(attr.value))+`"`)
		}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = "<notice>No attachment details were produced.</notice>"
	}
	return "<attachment " + strings.Join(attrs, " ") + ">\n" + body + "\n</attachment>"
}

func slackMediaNoticeXML(kind string, item slackapp.SlackMediaItem, mimeType, notice string) string {
	return slackAttachmentXML(kind, item, mimeType, "<notice>"+escapeXMLText(notice)+"</notice>")
}

func compactSlackMediaValue(value any) string {
	return strings.TrimSpace(compactSlackMediaValueIndent(value, 0))
}

func compactSlackMediaValueIndent(value any, indent int) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case []any:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := compactSlackMediaValueIndent(item, indent+2); text != "" {
				lines = append(lines, strings.Repeat(" ", indent)+"- "+indentSlackMediaMultiline(text, indent+2))
			}
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			if text := compactSlackMediaValueIndent(typed[key], indent+2); text != "" {
				lines = append(lines, strings.Repeat(" ", indent)+key+": "+indentSlackMediaMultiline(text, indent+len(key)+2))
			}
		}
		return strings.Join(lines, "\n")
	default:
		return fmt.Sprint(typed)
	}
}

func indentSlackMediaMultiline(text string, indent int) string {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "\n") {
		return text
	}
	return strings.ReplaceAll(text, "\n", "\n"+strings.Repeat(" ", indent))
}
