package pdf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os/exec"
	"strings"
)

// TocItem represents a single entry in the PDF outline (table of contents).
type TocItem struct {
	Level    int        `json:"level"`              // 0=part, 1=chapter, 2=section, 3=subsection
	Page     int        `json:"page"`
	Title    string     `json:"title"`
	Children []TocItem  `json:"children,omitempty"`
}

// ExtractPageText extracts plain text from a single PDF page using pdftotext.
// Returns cleaned text with normalized whitespace.
func ExtractPageText(pdfPath string, page int) (string, error) {
	cmd := exec.Command("pdftotext",
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		"-layout",
		pdfPath,
		"-",
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftotext failed (page %d): %v\nstderr: %s", page, err, stderr.String())
	}
	return cleanText(out.String()), nil
}

// cleanText normalizes whitespace in extracted PDF text:
// - Collapses multiple spaces/tabs into single space
// - Removes leading/trailing whitespace from each line
// - Preserves paragraph breaks (blank lines)
func cleanText(s string) string {
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, line := range lines {
		// Replace tabs with spaces, collapse multiple spaces
		trimmed := strings.Join(strings.Fields(line), " ")
		cleaned = append(cleaned, trimmed)
	}
	result := strings.Join(cleaned, "\n")
	// Collapse 3+ newlines to 2 (preserve paragraph breaks, remove excess)
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// ---------- Structured text extraction with heading detection ----------

type textElement struct {
	top    int
	left   int
	width  int
	height int
	font   int
	bold   bool
	text   string
}

type textLine struct {
	top      int
	elements []textElement
	maxFont  int
	bold     bool
}

// ExtractPageTextStructured extracts text from a single page, detecting headings
// based on font size and bold markers from pdftohtml XML output.
// Returns text with markdown-style heading prefixes:
//
//	#  → chapter title    (font ≥ 3× body size, bold)
//	## → section heading  (font ≥ 1.8× body size, bold)
//	### → sub-heading     (font ≥ 1.3× body size, bold)
//
// Body text follows with no prefix. Paragraph breaks are preserved.
func ExtractPageTextStructured(pdfPath string, page int) (string, error) {
	cmd := exec.Command("pdftohtml",
		"-xml", "-stdout", "-i",
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		pdfPath,
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdftohtml failed (page %d): %v\nstderr: %s", page, err, stderr.String())
	}

	// Parse fontspecs and text elements
	fontSizes, elements, err := parsePageXML(out.Bytes())
	if err != nil {
		return "", fmt.Errorf("parse page XML failed (page %d): %v", page, err)
	}

	if len(elements) == 0 {
		return "", nil
	}

	// Group into lines by Y position (tolerance: same-line if within 3px)
	lines := groupIntoLines(elements, fontSizes)

	// Sort lines by Y position
	sortLinesByY(lines)

	// Detect body font size
	bodySize := detectBodyFontSize(lines, fontSizes)

	// Build output with heading markers
	return buildStructuredText(lines, bodySize), nil
}

// parsePageXML parses fontspec and text elements from pdftohtml XML output.
func parsePageXML(data []byte) (map[int]int, []textElement, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	fontSizes := make(map[int]int)
	var elements []textElement

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "fontspec":
				var id, size int
				for _, a := range t.Attr {
					if a.Name.Local == "id" {
						fmt.Sscanf(a.Value, "%d", &id)
					}
					if a.Name.Local == "size" {
						fmt.Sscanf(a.Value, "%d", &size)
					}
				}
				fontSizes[id] = size

			case "text":
				var el textElement
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "top":
						fmt.Sscanf(a.Value, "%d", &el.top)
					case "left":
						fmt.Sscanf(a.Value, "%d", &el.left)
					case "width":
						fmt.Sscanf(a.Value, "%d", &el.width)
					case "height":
						fmt.Sscanf(a.Value, "%d", &el.height)
					case "font":
						fmt.Sscanf(a.Value, "%d", &el.font)
					}
				}
				// Read inner content (may include <b>, <i>, <br> and char data)
				el.text, el.bold = readTextContent(decoder)
				if el.text != "" {
					elements = append(elements, el)
				}
			}
		}
	}
	return fontSizes, elements, nil
}

// readTextContent reads the text content inside a <text> element,
// detecting <b> tags and collecting character data.
func readTextContent(d *xml.Decoder) (string, bool) {
	var buf strings.Builder
	bold := false
	for {
		token, err := d.Token()
		if err != nil {
			break
		}
		switch t := token.(type) {
		case xml.StartElement:
			if t.Name.Local == "b" {
				bold = true
			}
		case xml.EndElement:
			if t.Name.Local == "text" {
				return strings.TrimSpace(buf.String()), bold
			}
		case xml.CharData:
			buf.WriteString(string(t))
		}
	}
	return strings.TrimSpace(buf.String()), bold
}

// groupIntoLines groups text elements by their Y position into lines.
// Elements within yTolerance pixels vertically are considered same-line.
func groupIntoLines(elements []textElement, fontSizes map[int]int) []textLine {
	const yTolerance = 3

	// Build map: Y position → list of elements
	yBuckets := make(map[int][]textElement)
	var yKeys []int
	for _, el := range elements {
		matchedY := -1
		for y := range yBuckets {
			if abs(el.top-y) <= yTolerance {
				matchedY = y
				break
			}
		}
		if matchedY < 0 {
			yBuckets[el.top] = append(yBuckets[el.top], el)
			yKeys = append(yKeys, el.top)
		} else {
			yBuckets[matchedY] = append(yBuckets[matchedY], el)
		}
	}

	var lines []textLine
	for _, y := range yKeys {
		els := yBuckets[y]
		// Sort within line by X position
		sortElementsByX(els)
		maxFont := 0
		hasBold := false
		for _, el := range els {
			if sz, ok := fontSizes[el.font]; ok && sz > maxFont {
				maxFont = sz
			}
			if el.bold {
				hasBold = true
			}
		}
		lines = append(lines, textLine{top: y, elements: els, maxFont: maxFont, bold: hasBold})
	}
	return lines
}

// sortLinesByY sorts lines by their Y position.
func sortLinesByY(lines []textLine) {
	for i := 0; i < len(lines); i++ {
		for j := i + 1; j < len(lines); j++ {
			if lines[i].top > lines[j].top {
				lines[i], lines[j] = lines[j], lines[i]
			}
		}
	}
}

// sortElementsByX sorts text elements by their X position.
func sortElementsByX(els []textElement) {
	for i := 0; i < len(els); i++ {
		for j := i + 1; j < len(els); j++ {
			if els[i].left > els[j].left {
				els[i], els[j] = els[j], els[i]
			}
		}
	}
}

// detectBodyFontSize finds the most common font size among substantial text lines.
// Very short text (< 5 chars), extreme sizes, and lines in header/footer zones are excluded.
func detectBodyFontSize(lines []textLine, fontSizes map[int]int) int {
	sizeCount := make(map[int]int)
	totalChars := make(map[int]int)

	for _, line := range lines {
		// Skip header zone (top of page) and footer zone (bottom of page)
		if line.top < 80 || line.top > 1080 {
			continue
		}
		for _, el := range line.elements {
			if len(el.text) < 5 {
				continue
			}
			if sz, ok := fontSizes[el.font]; ok {
				sizeCount[sz]++
				totalChars[sz] += len(el.text)
			}
		}
	}

	bestSize, bestChars := 0, 0
	for sz, chars := range totalChars {
		if chars > bestChars {
			bestChars = chars
			bestSize = sz
		}
	}
	if bestSize == 0 {
		bestSize = 12 // default fallback
	}
	return bestSize
}

// mergedLine is a pre-processed line ready for output.
type mergedLine struct {
	top     int
	text    string
	prefix  string // heading prefix: "# ", "## ", "### ", or ""
}

// buildStructuredText constructs the output text with heading markers.
// It merges multi-line headings and drop caps before building the final output.
func buildStructuredText(lines []textLine, bodySize int) string {
	// Determine content top boundary to filter page-header decorations.
	contentTop, _ := findContentBounds(lines, bodySize)

	// Phase 1: build raw lines with classification
	var raw []mergedLine
	var prevTop int = -1000

	for _, line := range lines {
		if contentTop > 0 && line.top < contentTop-20 {
			continue
		}
		vertGap := line.top - prevTop
		if vertGap > 30 && line.top > 1000 &&
			float64(line.maxFont)/float64(bodySize) < 1.5 {
			continue
		}
		prevTop = line.top

		// Build text from elements
		var lineText strings.Builder
		lastRight := -100
		for _, el := range line.elements {
			if lastRight > 0 && el.left-lastRight > 5 {
				lineText.WriteByte(' ')
			}
			lineText.WriteString(el.text)
			lastRight = el.left + el.width
		}
		text := strings.TrimSpace(lineText.String())
		if text == "" {
			continue
		}

		prefix := classifyLine(line, text, bodySize)
		raw = append(raw, mergedLine{top: line.top, text: text, prefix: prefix})
	}

	// Phase 2: merge consecutive same-level headings and drop caps
	merged := mergeRelatedLines(raw)

	// Phase 3: output with paragraph breaks
	var out strings.Builder
	prevWasHeading := false
	lastTop := -1000

	for _, ml := range merged {
		vertGap := ml.top - lastTop
		lastTop = ml.top

		if ml.prefix != "" {
			if prevWasHeading {
				out.WriteByte('\n')
			} else if vertGap > 30 && out.Len() > 0 {
				out.WriteString("\n\n")
			}
			out.WriteString(ml.prefix)
			out.WriteString(ml.text)
			out.WriteByte('\n')
			prevWasHeading = true
		} else {
			if prevWasHeading && vertGap > 15 {
				out.WriteByte('\n')
			} else if !prevWasHeading && vertGap > 20 && out.Len() > 0 {
				out.WriteString("\n\n")
			} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
				out.WriteByte('\n')
			}
			out.WriteString(ml.text)
			prevWasHeading = false
		}
	}

	return strings.TrimSpace(out.String())
}

// mergeRelatedLines merges consecutive heading lines of the same level,
// and merges drop-cap characters into the following body text.
func mergeRelatedLines(raw []mergedLine) []mergedLine {
	if len(raw) == 0 {
		return raw
	}
	var out []mergedLine

	for i := 0; i < len(raw); i++ {
		ml := raw[i]

		// Merge consecutive same-level headings into one line
		if ml.prefix != "" {
			for i+1 < len(raw) && raw[i+1].prefix == ml.prefix {
				ml.text += " " + raw[i+1].text
				// Keep the top position of the first line
				i++
			}
			out = append(out, ml)
			continue
		}

		// Merge drop cap: single short text with huge font followed by body text
		if isDropCap(ml) && i+1 < len(raw) && raw[i+1].prefix == "" {
			next := raw[i+1]
			// Prepend the drop cap to the next line
			next.text = ml.text + next.text
			next.top = ml.top // use drop cap's position
			out = append(out, next)
			i++ // skip the next line since we merged it
			continue
		}

		out = append(out, ml)
	}

	return out
}

// isDropCap returns true if the line looks like a drop cap:
// very short text (1-2 chars) that appears to be a single large decorated letter.
func isDropCap(ml mergedLine) bool {
	if ml.prefix != "" {
		return false
	}
	return len(ml.text) <= 2
}

// classifyLine determines if a line is a heading and returns its markdown prefix.
// Returns "" for body text.
func classifyLine(line textLine, text string, bodySize int) string {
	if bodySize <= 0 {
		return ""
	}
	// Only classify as heading if bold and font size significantly larger than body
	if !line.bold {
		return ""
	}
	// Skip single-word short text that may be drop-caps or labels
	if len(text) < 3 {
		return ""
	}
	ratio := float64(line.maxFont) / float64(bodySize)
	if ratio >= 3.0 {
		return "# "
	}
	if ratio >= 1.8 {
		return "## "
	}
	if ratio >= 1.3 && line.maxFont > bodySize {
		return "### "
	}
	return ""
}

// findContentBounds finds the Y range of main body content by looking for
// substantial blocks of body-sized text. Short decoration lines and sidebar
// callouts at page top/bottom are excluded by requiring a minimum line length.
func findContentBounds(lines []textLine, bodySize int) (int, int) {
	if bodySize <= 0 {
		return 0, 0
	}
	first, last := 0, 0
	for _, line := range lines {
		if line.maxFont < bodySize-1 || line.maxFont > bodySize+4 {
			continue
		}
		// Compute total text length for this line
		totalLen := 0
		for _, el := range line.elements {
			totalLen += len(el.text)
		}
		// Skip short decoration lines (page numbers, labels, sidebar bullets)
		if totalLen < 20 {
			continue
		}
		if first == 0 || line.top < first {
			first = line.top
		}
		if line.top > last {
			last = line.top
		}
	}
	return first, last
}

// allElementsInRightColumn returns true if all text elements are positioned
// in the right half of the page (left > 400). Used to detect sidebar/callout boxes.
func allElementsInRightColumn(elements []textElement) bool {
	if len(elements) == 0 {
		return false
	}
	for _, el := range elements {
		if el.left <= 400 {
			return false
		}
	}
	return true
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---------- PDF outline extraction ----------

// ExtractOutline extracts the PDF's built-in outline (bookmarks / table of contents)
// using pdftohtml -xml and parsing the <outline> section.
func ExtractOutline(pdfPath string) ([]TocItem, error) {
	cmd := exec.Command("pdftohtml", "-xml", "-stdout", "-i", pdfPath)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftohtml failed: %v\nstderr: %s", err, stderr.String())
	}

	decoder := xml.NewDecoder(bytes.NewReader(out.Bytes()))

	// Find the root <outline> element (skip <page> content)
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("outline not found in PDF: %v", err)
		}
		if se, ok := token.(xml.StartElement); ok && se.Name.Local == "outline" {
			return parseOutline(decoder, 0)
		}
	}
}

// parseOutline recursively parses <outline> content, preserving interleaved order
// of <item> and nested <outline> elements.
func parseOutline(d *xml.Decoder, level int) ([]TocItem, error) {
	var items []TocItem
	var lastItem *TocItem

	for {
		token, err := d.Token()
		if err != nil {
			return items, nil
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "item":
				var page int
				for _, a := range t.Attr {
					if a.Name.Local == "page" {
						fmt.Sscanf(a.Value, "%d", &page)
					}
				}
				// Read text content (may be split across multiple CharData)
				var text strings.Builder
				for {
					inner, err := d.Token()
					if err != nil {
						break
					}
					if cd, ok := inner.(xml.CharData); ok {
						text.WriteString(string(cd))
					}
					if _, ok := inner.(xml.EndElement); ok {
						break
					}
				}
				title := strings.TrimSpace(text.String())
				if title == "" {
					title = fmt.Sprintf("(第 %d 页)", page)
				}
				item := TocItem{Level: level, Page: page, Title: title}
				items = append(items, item)
				lastItem = &items[len(items)-1]

			case "outline":
				children, err := parseOutline(d, level+1)
				if err != nil {
					return items, err
				}
				if lastItem != nil {
					lastItem.Children = children
				} else {
					// Orphan outline — append children directly
					items = append(items, children...)
				}
			}

		case xml.EndElement:
			if t.Name.Local == "outline" {
				return items, nil
			}
		}
	}
}
