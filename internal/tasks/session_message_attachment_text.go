package tasks

import "strings"

func runtimeAttachmentDetails(record map[string]any) string {
	sections := make([]string, 0, 3)
	for _, section := range []struct {
		tag  string
		text string
	}{
		{tag: "important_details", text: firstRuntimeString(record, "important_details")},
		{tag: "full_description", text: firstRuntimeString(record, "full_description", "rendered_description", "description")},
		{tag: "short_description", text: firstRuntimeString(record, "short_description", "summary")},
	} {
		if strings.TrimSpace(section.text) == "" {
			continue
		}
		sections = append(sections, "<"+section.tag+">\n"+escapeXMLText(section.text)+"\n</"+section.tag+">")
	}
	return strings.Join(sections, "\n")
}
