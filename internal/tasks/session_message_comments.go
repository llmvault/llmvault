package tasks

import (
	"fmt"
	"strings"
)

func runtimeStructuredCommentsBlock(value any, kind string) string {
	items := anySlice(value)
	if len(items) == 0 {
		return ""
	}
	entries := make([]string, 0, len(items))
	for _, item := range items {
		record, ok := mapRecord(item)
		if !ok {
			continue
		}
		location, body := runtimeStructuredComment(record, kind)
		if body == "" || location == "" {
			continue
		}
		entries = append(entries, fmt.Sprintf("%d. %s\n%s", len(entries)+1, location, indentRuntimeMessageBody(body)))
	}
	if len(entries) == 0 {
		return ""
	}
	if kind == "artifact" {
		return "Artifact comments to address:\n\n" + strings.Join(entries, "\n\n")
	}
	return "Code comments to address:\n\n" + strings.Join(entries, "\n\n")
}

func runtimeStructuredComment(record map[string]any, kind string) (string, string) {
	if kind == "artifact" {
		return artifactCommentLocation(record), strings.TrimSpace(firstRuntimeString(record, "body", "text"))
	}
	body := strings.TrimSpace(firstRuntimeString(record, "body"))
	path := firstRuntimeString(record, "display_path", "path")
	lineNumber := runtimeInt(record["line_number"])
	if body == "" || path == "" || lineNumber <= 0 {
		return "", ""
	}
	return fmt.Sprintf("%s:%s", path, runtimeLineLabel(lineNumber, firstRuntimeString(record, "side"))), body
}

func runtimeArtifactCommentsBlock(value any) string {
	return runtimeStructuredCommentsBlock(value, "artifact")
}

func artifactCommentLocation(record map[string]any) string {
	artifact := firstRuntimeString(record, "artifact_name", "artifact_slug", "artifact_id")
	if artifact == "" {
		artifact = "artifact"
	}
	target := firstRuntimeString(record, "target", "selector", "data_hivy_id", "element_id")
	if target == "" {
		return artifact
	}
	return artifact + " @ " + target
}
