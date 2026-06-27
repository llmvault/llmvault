package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const (
	canvasIDAttr = "data-canvas-id"
)

var canvasIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

type artifactValidationError struct {
	Result artifactValidationResult
}

func (e artifactValidationError) Error() string {
	return fmt.Sprintf("artifact validation failed with %d issue(s); inspect JSON output for fixes", len(e.Result.Issues))
}

func validateHTMLFile(relPath, fullPath, artifactType string) (artifactHTMLReport, []artifactValidationIssue) {
	report := artifactHTMLReport{Path: relPath}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return report, []artifactValidationIssue{{
			Code:     "html_file_read_failed",
			Severity: "error",
			File:     relPath,
			Message:  fmt.Sprintf("Could not read HTML file %q: %v", relPath, err),
			Fix:      "Make sure the file exists and is readable.",
		}}
	}
	source := string(data)
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return report, []artifactValidationIssue{{
			Code:     "html_parse_failed",
			Severity: "error",
			File:     relPath,
			Message:  fmt.Sprintf("HTML parser could not parse %q: %v", relPath, err),
			Fix:      "Fix malformed HTML before validating again.",
		}}
	}
	validator := htmlValidator{
		file:         relPath,
		artifactType: artifactType,
		canvasIDs:    map[string]string{},
	}
	validator.validateDocumentShape(source, doc)
	validator.walk(doc, 0)
	validator.validateMainStructure()
	validator.validateHeadingStructure()
	report.CanvasIDCount = validator.canvasIDCount
	report.SemanticElementCount = validator.semanticCount
	report.TopLevelSectionCount = validator.topLevelSectionCount
	report.MaxDepth = validator.maxDepth
	return report, validator.issues
}

type htmlValidator struct {
	file                 string
	artifactType         string
	issues               []artifactValidationIssue
	canvasIDs            map[string]string
	canvasIDCount        int
	semanticCount        int
	topLevelSectionCount int
	maxDepth             int
	body                 *html.Node
	mains                []*html.Node
	headings             []headingNode
}

type headingNode struct {
	level int
	path  string
}

func (v *htmlValidator) validateDocumentShape(source string, doc *html.Node) {
	lower := strings.ToLower(source)
	if !strings.Contains(lower, "<html") {
		v.addIssue("html_missing_html_tag", "error", "html", "", "HTML source must include an explicit <html> element.", "Wrap the document in <!doctype html><html>...</html>.")
	}
	if !strings.Contains(lower, "<body") {
		v.addIssue("html_missing_body_tag", "error", "body", "", "HTML source must include an explicit <body> element.", "Add a <body> element and put the artifact UI inside it.")
	}
	v.body = firstElement(doc, "body")
	v.mains = collectElements(doc, "main")
}

func (v *htmlValidator) walk(node *html.Node, depth int) {
	if node.Type == html.ElementNode {
		if depth > v.maxDepth {
			v.maxDepth = depth
		}
		name := strings.ToLower(node.Data)
		if isSemanticElement(name) {
			v.semanticCount++
		}
		if headingLevel(name) > 0 {
			v.headings = append(v.headings, headingNode{level: headingLevel(name), path: nodePath(node)})
		}
		if name == "main" {
			v.validateCanvasID(node, true)
		} else if requiresCanvasID(node) {
			v.validateCanvasID(node, true)
		} else {
			v.validateCanvasID(node, false)
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		v.walk(child, depth+1)
	}
}

func (v *htmlValidator) validateCanvasID(node *html.Node, required bool) {
	canvasID := strings.TrimSpace(attrValue(node, canvasIDAttr))
	path := nodePath(node)
	if canvasID == "" {
		if required {
			v.addIssue("missing_canvas_id", "error", node.Data, path, fmt.Sprintf("%s needs %s for Canvas targeting.", node.Data, canvasIDAttr), fmt.Sprintf("Add %s=\"%s\" with a stable kebab-case semantic name.", canvasIDAttr, suggestedCanvasID(node)))
		}
		return
	}
	v.canvasIDCount++
	if !canvasIDPattern.MatchString(canvasID) || len(canvasID) > 80 {
		v.issues = append(v.issues, artifactValidationIssue{
			Code:     "invalid_canvas_id",
			Severity: "error",
			File:     v.file,
			Element:  node.Data,
			NodePath: path,
			CanvasID: canvasID,
			Message:  fmt.Sprintf("%s value %q is not a valid Canvas ID.", canvasIDAttr, canvasID),
			Fix:      "Use lowercase kebab-case, start with a letter, avoid spaces/random IDs, and keep it under 80 characters.",
		})
	}
	if existingPath, exists := v.canvasIDs[canvasID]; exists {
		v.issues = append(v.issues, artifactValidationIssue{
			Code:     "duplicate_canvas_id",
			Severity: "error",
			File:     v.file,
			Element:  node.Data,
			NodePath: path,
			CanvasID: canvasID,
			Message:  fmt.Sprintf("%s value %q is duplicated; first seen at %s.", canvasIDAttr, canvasID, existingPath),
			Fix:      "Give each anchored element a unique semantic ID, such as hero-title or pricing-card-pro.",
		})
		return
	}
	v.canvasIDs[canvasID] = path
}

func (v *htmlValidator) validateMainStructure() {
	if v.body == nil {
		return
	}
	if len(v.mains) == 0 {
		v.addIssue("missing_main", "error", "body", nodePath(v.body), "HTML must include a primary <main> element.", fmt.Sprintf("Add <main %s=\"page\"> around the artifact content.", canvasIDAttr))
		return
	}
	if len(v.mains) > 1 {
		v.addIssue("multiple_main_elements", "error", "main", "", "HTML must not include multiple <main> elements.", "Keep one primary <main> and convert other main elements to section or article.")
	}
	main := v.mains[0]
	if main.Parent != v.body {
		v.addIssue("main_not_body_child", "error", "main", nodePath(main), "<main> should be a direct child of <body>.", "Move the primary <main> so the structure is body > main > section/article.")
	}
	for child := main.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if isTopLevelContentElement(child.Data) {
			v.topLevelSectionCount++
		}
	}
	if v.topLevelSectionCount == 0 {
		v.addIssue("main_missing_semantic_sections", "error", "main", nodePath(main), "<main> must contain semantic content sections.", "Wrap major content blocks in section, article, nav, aside, form, figure, or table elements with data-canvas-id.")
	}
}

func (v *htmlValidator) validateHeadingStructure() {
	if len(v.headings) == 0 {
		v.addIssue("missing_heading", "warning", "body", "", "HTML should include at least one heading.", "Add an h1 title and use h2/h3 headings for major sections.")
		return
	}
	if v.artifactType == artifactTypeWebPage && v.headings[0].level != 1 {
		v.addIssue("missing_primary_h1", "warning", "h1", v.headings[0].path, "web_page artifacts should start their heading outline with h1.", "Add one h1 near the top of the main content.")
	}
	previous := v.headings[0].level
	for _, heading := range v.headings[1:] {
		if heading.level > previous+1 {
			v.addIssue("heading_level_jump", "warning", fmt.Sprintf("h%d", heading.level), heading.path, fmt.Sprintf("Heading jumps from h%d to h%d.", previous, heading.level), "Do not skip heading levels; use the next level in the document outline.")
		}
		previous = heading.level
	}
}

func (v *htmlValidator) addIssue(code, severity, element, path, message, fix string) {
	v.issues = append(v.issues, artifactValidationIssue{
		Code:     code,
		Severity: severity,
		File:     v.file,
		Element:  element,
		NodePath: path,
		Message:  message,
		Fix:      fix,
	})
}

func requiresCanvasID(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "main", "section", "article", "nav", "aside", "form", "figure", "table":
		return true
	case "header", "footer":
		return node.Parent != nil && node.Parent.Type == html.ElementNode && (node.Parent.Data == "body" || node.Parent.Data == "main")
	case "li":
		return isRichRepeatedListItem(node)
	default:
		return false
	}
}

func isRichRepeatedListItem(node *html.Node) bool {
	if node.Parent == nil || (node.Parent.Data != "ul" && node.Parent.Data != "ol") {
		return false
	}
	if countElementChildrenNamed(node.Parent, "li") < 2 {
		return false
	}
	return hasDescendantNamed(node, map[string]bool{"article": true, "section": true, "figure": true, "form": true, "table": true}) || countDirectElementChildren(node) >= 2
}

func attrValue(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func firstElement(node *html.Node, name string) *html.Node {
	if node.Type == html.ElementNode && node.Data == name {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := firstElement(child, name); found != nil {
			return found
		}
	}
	return nil
}
