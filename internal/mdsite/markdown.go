// Package mdsite renders the docs/ markdown tree into a static HTML site using
// only the standard library, so `qorm docs` cross-compiles like everything else
// (no node, no cgo). The markdown converter supports the subset the docs use:
// ATX headings, fenced code, inline code/bold/italic/links, unordered/ordered
// lists, blockquotes, horizontal rules and GFM tables.
package mdsite

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	codeRe     = regexp.MustCompile("`[^`]+`")
	linkRe     = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	boldRe     = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe   = regexp.MustCompile(`\*([^*]+)\*`)
	orderedRe  = regexp.MustCompile(`^\s*\d+\.\s+`)
	tableSepRe = regexp.MustCompile(`^\s*\|?[\s:|-]*-[\s:|-]*\|?\s*$`)
)

// isHR reports whether a line is a horizontal rule (>=3 of the same -, * or _).
// A function, not a regexp: Go's RE2 has no backreferences.
func isHR(line string) bool {
	t := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if len(t) < 3 {
		return false
	}
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(t); i++ {
		if t[i] != c {
			return false
		}
	}
	return true
}

// RenderMarkdown converts a markdown document to an HTML fragment.
func RenderMarkdown(src string) string {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out strings.Builder
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "```"):
			i++
			var code []string
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				code = append(code, lines[i])
				i++
			}
			i++ // closing fence
			out.WriteString("<pre><code>" + html.EscapeString(strings.Join(code, "\n")) + "</code></pre>\n")
		case isTableHeader(lines, i):
			i = renderTable(&out, lines, i)
		case headingLevel(line) > 0:
			lvl := headingLevel(line)
			text := strings.TrimSpace(line[lvl:])
			fmt.Fprintf(&out, "<h%d id=%q>%s</h%d>\n", lvl, slug(text), inlineMD(text), lvl)
			i++
		case isHR(line):
			out.WriteString("<hr>\n")
			i++
		case isUnordered(line):
			i = renderList(&out, lines, i, false)
		case orderedRe.MatchString(line):
			i = renderList(&out, lines, i, true)
		case strings.HasPrefix(strings.TrimSpace(line), ">"):
			i = renderQuote(&out, lines, i)
		case strings.TrimSpace(line) == "":
			i++
		default:
			var para []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "" && !isBlockStart(lines, i) {
				para = append(para, strings.TrimSpace(lines[i]))
				i++
			}
			out.WriteString("<p>" + inlineMD(strings.Join(para, " ")) + "</p>\n")
		}
	}
	return out.String()
}

func isBlockStart(lines []string, i int) bool {
	line := lines[i]
	return strings.HasPrefix(line, "```") || headingLevel(line) > 0 || isHR(line) ||
		isUnordered(line) || orderedRe.MatchString(line) ||
		strings.HasPrefix(strings.TrimSpace(line), ">") || isTableHeader(lines, i)
}

func headingLevel(line string) int {
	n := 0
	for n < len(line) && line[n] == '#' {
		n++
	}
	if n > 0 && n <= 6 && n < len(line) && line[n] == ' ' {
		return n
	}
	return 0
}

func isUnordered(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ")
}

func renderList(out *strings.Builder, lines []string, i int, ordered bool) int {
	tag := "ul"
	if ordered {
		tag = "ol"
	}
	out.WriteString("<" + tag + ">\n")
	for i < len(lines) {
		if ordered && orderedRe.MatchString(lines[i]) {
			out.WriteString("<li>" + inlineMD(orderedRe.ReplaceAllString(lines[i], "")) + "</li>\n")
		} else if !ordered && isUnordered(lines[i]) {
			t := strings.TrimSpace(lines[i])
			out.WriteString("<li>" + inlineMD(strings.TrimSpace(t[2:])) + "</li>\n")
		} else {
			break
		}
		i++
	}
	out.WriteString("</" + tag + ">\n")
	return i
}

func renderQuote(out *strings.Builder, lines []string, i int) int {
	var q []string
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
		q = append(q, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")))
		i++
	}
	out.WriteString("<blockquote>" + inlineMD(strings.Join(q, " ")) + "</blockquote>\n")
	return i
}

func isTableHeader(lines []string, i int) bool {
	return i+1 < len(lines) && strings.Contains(lines[i], "|") && tableSepRe.MatchString(lines[i+1])
}

func renderTable(out *strings.Builder, lines []string, i int) int {
	header := splitRow(lines[i])
	i += 2 // header + separator
	out.WriteString("<table>\n<thead><tr>")
	for _, c := range header {
		out.WriteString("<th>" + inlineMD(c) + "</th>")
	}
	out.WriteString("</tr></thead>\n<tbody>\n")
	for i < len(lines) && strings.Contains(lines[i], "|") && strings.TrimSpace(lines[i]) != "" {
		out.WriteString("<tr>")
		for _, c := range splitRow(lines[i]) {
			out.WriteString("<td>" + inlineMD(c) + "</td>")
		}
		out.WriteString("</tr>\n")
		i++
	}
	out.WriteString("</tbody></table>\n")
	return i
}

func splitRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	for j := range parts {
		parts[j] = strings.TrimSpace(parts[j])
	}
	return parts
}

// inlineMD processes inline markdown on a line: escape, then code/link/bold/italic.
func inlineMD(s string) string {
	s = html.EscapeString(s)
	// protect code spans from further processing
	var codes []string
	s = codeRe.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, m[1:len(m)-1])
		return fmt.Sprintf("\x00c%d\x00", len(codes)-1)
	})
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	s = boldRe.ReplaceAllString(s, `<strong>$1</strong>`)
	s = italicRe.ReplaceAllString(s, `<em>$1</em>`)
	for idx, c := range codes {
		s = strings.Replace(s, fmt.Sprintf("\x00c%d\x00", idx), "<code>"+c+"</code>", 1)
	}
	return s
}

// slug builds an anchor id, keeping letters/digits (incl. CJK) and dashes.
func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
