package main

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

func collectElements(node *html.Node, name string) []*html.Node {
	var out []*html.Node
	if node.Type == html.ElementNode && node.Data == name {
		out = append(out, node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		out = append(out, collectElements(child, name)...)
	}
	return out
}

func isSemanticElement(name string) bool {
	switch strings.ToLower(name) {
	case "main", "section", "article", "nav", "aside", "header", "footer", "form", "figure", "figcaption", "table", "thead", "tbody", "tfoot", "tr", "th", "td", "ul", "ol", "li", "dl", "dt", "dd":
		return true
	default:
		return headingLevel(name) > 0
	}
}

func isTopLevelContentElement(name string) bool {
	switch strings.ToLower(name) {
	case "section", "article", "nav", "aside", "form", "figure", "table", "header", "footer":
		return true
	default:
		return false
	}
}

func headingLevel(name string) int {
	if len(name) != 2 || name[0] != 'h' || name[1] < '1' || name[1] > '6' {
		return 0
	}
	return int(name[1] - '0')
}

func nodePath(node *html.Node) string {
	if node == nil {
		return ""
	}
	var parts []string
	for current := node; current != nil && current.Type == html.ElementNode; current = current.Parent {
		part := current.Data
		if index := elementIndexOfType(current); index > 1 {
			part = fmt.Sprintf("%s:nth-of-type(%d)", part, index)
		}
		parts = append([]string{part}, parts...)
	}
	return strings.Join(parts, " > ")
}

func elementIndexOfType(node *html.Node) int {
	if node.Parent == nil {
		return 1
	}
	index := 0
	for sibling := node.Parent.FirstChild; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode && sibling.Data == node.Data {
			index++
		}
		if sibling == node {
			return index
		}
	}
	return 1
}

func suggestedCanvasID(node *html.Node) string {
	name := strings.ToLower(node.Data)
	switch name {
	case "main":
		return "page"
	case "section":
		if title := firstHeadingText(node); title != "" {
			return slugify(title)
		}
		return "section-name"
	case "article":
		if title := firstHeadingText(node); title != "" {
			return slugify(title)
		}
		return "article-name"
	case "li":
		return "item-name"
	default:
		return name + "-name"
	}
}

func firstHeadingText(node *html.Node) string {
	if node.Type == html.ElementNode && headingLevel(node.Data) > 0 {
		return strings.TrimSpace(textContent(node))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if text := firstHeadingText(child); text != "" {
			return text
		}
	}
	return ""
}

func textContent(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var parts []string
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if text := strings.TrimSpace(textContent(child)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func countElementChildrenNamed(node *html.Node, name string) int {
	count := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == name {
			count++
		}
	}
	return count
}

func countDirectElementChildren(node *html.Node) int {
	count := 0
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			count++
		}
	}
	return count
}

func hasDescendantNamed(node *html.Node, names map[string]bool) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && names[child.Data] {
			return true
		}
		if hasDescendantNamed(child, names) {
			return true
		}
	}
	return false
}

func hasArtifactValidationErrors(issues []artifactValidationIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

func artifactValidationGuidance() []artifactValidationAdvice {
	return []artifactValidationAdvice{
		{
			Topic:  "canvas ids",
			Detail: "Use data-canvas-id on current-session comment targets. IDs should be unique, semantic, lowercase kebab-case, and written into source HTML.",
		},
		{
			Topic:  "semantic structure",
			Detail: "Use body > main, then section/article/nav/aside/form/figure/table for major content. Anchor top-level semantic blocks and rich repeated items.",
		},
		{
			Topic:  "headings",
			Detail: "Start web pages with h1 and avoid skipping heading levels so agents and users can understand the document outline.",
		},
	}
}
