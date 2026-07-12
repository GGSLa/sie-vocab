package pdf

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftotext",
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
	family string // font family from <fontspec> (e.g. "Merriweather", "OpenSans")
	text   string
}

type textLine struct {
	top      int
	elements []textElement
	maxFont  int
	bold     bool
	family   string // dominant font family for this line
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftohtml",
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
	fontFamilies := make(map[int]string)
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
				var family string
				for _, a := range t.Attr {
					if a.Name.Local == "id" {
						fmt.Sscanf(a.Value, "%d", &id)
					}
					if a.Name.Local == "size" {
						fmt.Sscanf(a.Value, "%d", &size)
					}
					if a.Name.Local == "family" {
						family = normalizeFamily(a.Value)
					}
				}
				fontSizes[id] = size
				fontFamilies[id] = family

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
				el.family = fontFamilies[el.font]
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
		// Determine dominant font family for the line
		family := ""
		familyCount := make(map[string]int)
		for _, el := range els {
			if el.family != "" {
				familyCount[el.family]++
			}
		}
		maxCount := 0
		for fam, count := range familyCount {
			if count > maxCount {
				maxCount = count
				family = fam
			}
		}
		lines = append(lines, textLine{
			top:      y,
			elements: els,
			maxFont:  maxFont,
			bold:     hasBold,
			family:   family,
		})
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
	family  string // font family for callout/body detection
}

// buildStructuredText constructs the output text with heading markers.
// It merges multi-line headings and drop caps before building the final output.
func buildStructuredText(lines []textLine, bodySize int) string {
	// Determine content boundaries to filter page-header/footer decorations.
	contentTop, contentLast := findContentBounds(lines, bodySize)

	// Phase 1: build raw lines with classification
	var raw []mergedLine
	var prevTop int = -1000

	for _, line := range lines {
		if contentTop > 0 && line.top < contentTop-20 {
			continue
		}
		vertGap := line.top - prevTop
		if vertGap > 30 && contentLast > 0 && line.top > contentLast &&
			float64(line.maxFont)/float64(bodySize) < 1.5 {
			continue
		}
		prevTop = line.top

		// Build text from elements
		var lineText strings.Builder
		lastRight := -100
		lastFont := -1
		for _, el := range line.elements {
			needSpace := false
			if lastRight > 0 {
				if el.left-lastRight > 5 {
					needSpace = true
				} else if lastFont >= 0 && el.font != lastFont {
					// Font change with tiny gap: words in different fonts
					// (e.g. italic) may overlap or touch in PDF coordinates.
					prevEndsWithLetter := lineText.Len() > 0 && isLetter(rune(lineText.String()[lineText.Len()-1]))
					currStartsWithLetter := len(el.text) > 0 && isLetter(rune(el.text[0]))
					if prevEndsWithLetter && currStartsWithLetter {
						needSpace = true
					}
				}
			}
			if needSpace {
				lineText.WriteByte(' ')
			}
			lineText.WriteString(el.text)
			lastRight = el.left + el.width
			lastFont = el.font
		}
		text := strings.TrimSpace(lineText.String())
		if text == "" {
			continue
		}

		prefix := classifyLine(line, text, bodySize)
		raw = append(raw, mergedLine{top: line.top, text: text, prefix: prefix, family: line.family})
	}

	// Phase 2: merge consecutive same-level headings and drop caps
	merged := mergeRelatedLines(raw)

	// Phase 3: output with paragraph breaks
	var out strings.Builder
	prevWasHeading := false
	lastTop := -1000
	lastFamily := "" // track font family for callout/body separation

	for _, ml := range merged {
		vertGap := ml.top - lastTop
		lastTop = ml.top

		// Detect font family switch between non-heading lines
		familySwitch := !prevWasHeading &&
			lastFamily != "" && ml.family != "" &&
			lastFamily != ml.family

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
			if familySwitch && out.Len() > 0 {
				out.WriteString("\n\n")
			} else if prevWasHeading && vertGap > 15 {
				out.WriteByte('\n')
			} else if !prevWasHeading && vertGap > 20 && out.Len() > 0 {
				out.WriteString("\n\n")
			} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
				out.WriteByte('\n')
			}
			out.WriteString(ml.text)
			prevWasHeading = false
		}

		// Track font family for switch detection
		if ml.family != "" {
			lastFamily = ml.family
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
			// Keep next.top (body text position) rather than ml.top (drop cap position)
				// to avoid false paragraph breaks caused by the drop cap's vertical offset.
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
		// Exclude header and footer zones, mirroring detectBodyFontSize.
		// Page headers (top < 80) and footers (top > 1080) should not
		// affect content boundary detection.
		if line.top < 80 || line.top > 1080 {
			continue
		}
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

// isLetter returns true if r is an ASCII letter.
func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// normalizeFamily strips the PDF subset prefix from a font family name.
// pdftohtml outputs names like "EMBDHV+Merriweather" or "JJLHZL+OpenSans"
// where the prefix is a random hash used for font subsetting.
// We strip the prefix to get the canonical family name for comparison.
func normalizeFamily(family string) string {
	if idx := strings.LastIndex(family, "+"); idx >= 0 {
		return family[idx+1:]
	}
	return family
}

// ---------- OCR fallback for scanned/image-based PDF pages ----------

const ocrCacheDir = "/tmp/sie-ocr-cache"

// ExtractPageTextOCR extracts text from a scanned/image-based PDF page
// using pdftoppm to render the page as PNG, then tesseract to OCR it.
// The rendered PNG is cached to disk to avoid re-rendering on retry.
// lang specifies the Tesseract language code(s), e.g. "eng", "chi_sim", "chi_sim+eng".
func ExtractPageTextOCR(pdfPath string, page int, lang string) (string, error) {
	if lang == "" {
		lang = "eng"
	}
	os.MkdirAll(ocrCacheDir, 0755)
	imagePath := fmt.Sprintf("%s/page-%d.png", ocrCacheDir, page)

	// Render page to PNG if not cached (pdftoppm always works, even for scanned PDFs)
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		tmpPrefix := fmt.Sprintf("%s/tmp-%d", ocrCacheDir, page)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "pdftoppm",
			"-png", "-r", "150",
			"-f", fmt.Sprintf("%d", page),
			"-l", fmt.Sprintf("%d", page),
			"-singlefile",
			pdfPath, tmpPrefix,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			log.Printf("❌ OCR: pdftoppm 渲染失败 (page=%d): %v\nstderr: %s", page, err, stderr.String())
			return "", fmt.Errorf("OCR 渲染失败: %v", err)
		}
		os.Rename(tmpPrefix+".png", imagePath)
	}

	// Run tesseract OCR on the page image
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tesseract", imagePath, "stdout", "-l", lang, "--psm", "6")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		log.Printf("❌ OCR: tesseract 失败 (page=%d): %v\nstderr: %s", page, err, stderr.String())
		return "", fmt.Errorf("OCR 识别失败: %v", err)
	}

	text := cleanText(out.String())
	if text == "" {
		return "", nil
	}

	// Apply heuristic heading detection for OCR text (no font metadata available)
	text = addOCRHeadingMarkers(text)

	log.Printf("🔍 OCR 提取: page=%d, chars=%d", page, len(text))
	return text, nil
}

// addOCRHeadingMarkers applies lightweight heuristic heading detection to OCR text.
// Without font metadata, we rely on text patterns:
//   - All-caps lines that are short (≤ 80 chars) and not just numbers → "## "
//   - Lines starting with "Chapter", "Part", "Unit", "Section" etc. → "## "
func addOCRHeadingMarkers(text string) string {
	lines := strings.Split(text, "\n")
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			out = append(out, "")
			continue
		}

		// Skip lines that are already marked as headings
		if strings.HasPrefix(trimmed, "#") {
			out = append(out, trimmed)
			continue
		}

		if isOCRHeading(trimmed) {
			out = append(out, "## "+trimmed)
		} else {
			out = append(out, trimmed)
		}
	}

	return strings.Join(out, "\n")
}

// isOCRHeading returns true if the line looks like a heading based on text heuristics.
func isOCRHeading(line string) bool {
	// Must be relatively short (headings are rarely > 80 chars)
	if len(line) > 80 {
		return false
	}
	// Must be at least 3 characters
	if len(line) < 3 {
		return false
	}

	// Pattern 1: Starts with common heading prefixes
	lowerLine := strings.ToLower(line)
	for _, prefix := range []string{"chapter", "part ", "unit ", "section", "appendix", "module", "lesson", "topic"} {
		if strings.HasPrefix(lowerLine, prefix) {
			return true
		}
	}

	// Pattern 2: All-caps line (at least 80% uppercase letters)
	letterCount := 0
	upperCount := 0
	for _, r := range line {
		if r >= 'A' && r <= 'Z' {
			upperCount++
			letterCount++
		} else if r >= 'a' && r <= 'z' {
			letterCount++
		}
	}
	if letterCount >= 4 && float64(upperCount)/float64(letterCount) >= 0.8 {
		return true
	}

	return false
}

// ExtractPageTextHybrid extracts text from a PDF page, preferring the existing
// text layer (pdftohtml structured extraction) and falling back to OCR when
// the page is image-only (no extractable text elements).
//
// This ensures the system works with both text-based and scanned PDFs without
// requiring any pre-processing.
func ExtractPageTextHybrid(pdfPath string, page int, lang string) (string, error) {
	// Try structured text extraction first (gives font metadata for heading detection)
	text, err := ExtractPageTextStructured(pdfPath, page)
	if err != nil {
		// pdftohtml failed entirely — try OCR as fallback
		log.Printf("⚠️ pdftohtml 失败 (page=%d): %v — 尝试 OCR 回退", page, err)
		ocrText, ocrErr := ExtractPageTextOCR(pdfPath, page, lang)
		if ocrErr != nil {
			return "", ocrErr
		}
		return mergeHardWrappedLines(ocrText), nil
	}

	// If text is empty or nearly empty, this is likely a scanned/image-only page
	if len(strings.TrimSpace(text)) < 50 {
		log.Printf("🔍 页面 %d 文字层不足 (%d chars)，启用 OCR 回退...", page, len(text))
		ocrText, ocrErr := ExtractPageTextOCR(pdfPath, page, lang)
		if ocrErr != nil {
			log.Printf("⚠️ OCR 回退也失败 (page=%d): %v — 返回原始文本", page, ocrErr)
			return text, nil // return whatever little text we got
		}
		if ocrText != "" {
			return mergeHardWrappedLines(ocrText), nil
		}
	}

	return mergeHardWrappedLines(text), nil
}

// mergeHardWrappedLines merges PDF hard-wrapped lines within paragraphs.
// PDF text extraction often breaks lines mid-sentence where the original
// layout ran out of horizontal space. This function detects those breaks
// and joins them, while keeping real paragraph breaks (blank lines),
// headings (# markers), and callout/sidebar lines (» markers) intact.
func mergeHardWrappedLines(text string) string {
	// Split into paragraph blocks (blank-line separated)
	blocks := strings.Split(text, "\n\n")
	var resultBlocks []string

	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		var merged []string
		var current string

		for _, rawLine := range lines {
			line := strings.TrimSpace(rawLine)
			if line == "" {
				if current != "" {
					merged = append(merged, current)
					current = ""
				}
				continue
			}

			// Heading markers: strip if the heading content is just a bullet/list marker
			// (pdftohtml sometimes mislabels list items as headings, e.g. "### •")
			if isHeadingLine(line) {
				rest := stripHeadingPrefix(line)
				if isListItem(rest) || isJustBulletMarker(rest) {
					// This is actually a list item — strip heading markers, merge with next line
					if current != "" {
						merged = append(merged, current)
						current = ""
					}
					current = rest
					continue
				}
				if current != "" {
					merged = append(merged, current)
					current = ""
				}
				merged = append(merged, line)
				continue
			}

			// Callout/sidebar lines: start new callout, merge continuation lines
			if isCalloutLine(line) {
				if current != "" {
					merged = append(merged, current)
					current = ""
				}
				current = line
				continue
			}

			// Bullet points and numbered lists — keep standalone
			if isListItem(line) {
				if current != "" {
					merged = append(merged, current)
					current = ""
				}
				merged = append(merged, line)
				continue
			}

			// Body text: merge with previous line
			if current == "" {
				current = line
			} else {
				// Hyphenated word break: merge without space
				if strings.HasSuffix(current, "-") {
					current = current[:len(current)-1] + line
				} else {
					current += " " + line
				}
			}
		}

		if current != "" {
			merged = append(merged, current)
		}

		if len(merged) > 0 {
			resultBlocks = append(resultBlocks, strings.Join(merged, "\n"))
		} else {
			// Preserve intentional blank paragraphs
			resultBlocks = append(resultBlocks, "")
		}
	}

	result := strings.Join(resultBlocks, "\n\n")

	// Second pass: merge paragraph blocks that are clearly continuations.
	// The structured text extractor may insert false paragraph breaks
	// when it detects unusual vertical gaps. If a block ends without
	// sentence-ending punctuation and the next block starts with a
	// lowercase letter or a common continuation word, merge them.
	result = mergeFalseParagraphBreaks(result)

	return strings.TrimSpace(result)
}

// mergeFalseParagraphBreaks does a second pass over paragraph-separated
// blocks and merges those that are clearly false breaks (mid-sentence).
// Also handles callout continuation lines that were incorrectly split into
// separate blocks by the structured text extractor.
func mergeFalseParagraphBreaks(text string) string {
	blocks := strings.Split(text, "\n\n")
	if len(blocks) < 2 {
		return text
	}

	var merged []string
	current := blocks[0]

	for i := 1; i < len(blocks); i++ {
		next := blocks[i]

		// Never merge if next block starts a new section (heading)
		if looksLikeHeading(next) {
			merged = append(merged, current)
			current = next
			continue
		}

		// If current is a callout/list that doesn't end with sentence-ending
		// punctuation, allow merging with a continuation next block.
		// This handles bullet points split across PDF lines with false paragraph breaks.
		currentLastLine := getLastLine(current)
		nextIsContinuation := isContinuation(currentLastLine, next)

		if looksLikeCalloutOrList(current) && nextIsContinuation {
			if strings.HasSuffix(strings.TrimSpace(current), "-") {
				current = strings.TrimRight(current, "- \t") + strings.TrimLeft(next, " \t")
			} else {
				current += " " + strings.TrimLeft(next, " \t")
			}
			continue
		}

		// Never merge across headings
		if looksLikeSpecialBlock(current) || looksLikeSpecialBlock(next) {
			merged = append(merged, current)
			current = next
			continue
		}

		// Merge if the next block looks like a continuation of the current one
		if nextIsContinuation {
			// Hyphenated word break across blocks
			if strings.HasSuffix(strings.TrimSpace(current), "-") {
				current = strings.TrimRight(current, "- \t") + strings.TrimLeft(next, " \t")
			} else {
				current += " " + strings.TrimLeft(next, " \t")
			}
			continue
		}

		merged = append(merged, current)
		current = next
	}

	merged = append(merged, current)
	return strings.Join(merged, "\n\n")
}

// getLastLine returns the last line of a multi-line block.
func getLastLine(block string) string {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

// looksLikeHeading returns true if the block starts with a markdown heading marker.
func looksLikeHeading(block string) bool {
	return strings.HasPrefix(strings.TrimSpace(block), "#")
}

// looksLikeCalloutOrList returns true if the block starts with a callout marker or list item.
func looksLikeCalloutOrList(block string) bool {
	firstLine := strings.TrimSpace(block)
	return strings.HasPrefix(firstLine, "»") || isListItem(firstLine)
}

// looksLikeSpecialBlock returns true if the block is a heading, callout, or list item.
func looksLikeSpecialBlock(block string) bool {
	firstLine := strings.TrimSpace(block)
	if strings.HasPrefix(firstLine, "#") || strings.HasPrefix(firstLine, "»") {
		return true
	}
	if isListItem(firstLine) {
		return true
	}
	return false
}

// isContinuation returns true if `next` is a continuation of `prev`.
// Heuristic: previous block doesn't end with sentence-ending punctuation
// OR next block starts with lowercase.
func isContinuation(prev, next string) bool {
	prev = strings.TrimSpace(prev)
	next = strings.TrimSpace(next)
	if prev == "" || next == "" {
		return false
	}

	prevLast := prev[len(prev)-1]
	nextFirst := rune(next[0])

	// If prev ends with sentence-ending punctuation, it's a real break
	if prevLast == '.' || prevLast == '!' || prevLast == '?' {
		return false
	}

	// If prev ends with colon or comma, merge
	if prevLast == ',' || prevLast == ';' || prevLast == ':' {
		return true
	}

	// If next starts with lowercase letter, it's clearly a continuation
	if nextFirst >= 'a' && nextFirst <= 'z' {
		return true
	}

	// If prev ends with hyphen, merge
	if prevLast == '-' {
		return true
	}

	// If prev doesn't end with any punctuation and next starts with
	// a common lowercase continuation word, merge
	commonContinuations := []string{"and", "or", "but", "the", "a", "an", "in", "on", "at",
		"to", "for", "of", "with", "from", "by", "as", "is", "are", "was", "were",
		"that", "this", "these", "those", "it", "they", "not", "also", "which",
		"who", "whom", "whose", "can", "may", "will", "shall", "would", "could",
		"should", "has", "have", "had", "been", "being", "does", "did", "its",
		"their", "our", "your", "my", "his", "her", "up", "out", "about"}
	lowerNext := strings.ToLower(next)
	for _, w := range commonContinuations {
		if lowerNext == w || strings.HasPrefix(lowerNext, w+" ") {
			return true
		}
	}

	return false
}

func isHeadingLine(line string) bool {
	return strings.HasPrefix(line, "#")
}

func isCalloutLine(line string) bool {
	return strings.HasPrefix(line, "»")
}

func isListItem(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 2 {
		return false
	}
	// Bullet markers
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "• ") ||
		strings.HasPrefix(trimmed, "· ") || strings.HasPrefix(trimmed, "– ") {
		return true
	}
	// Numbered list: "1.", "1)", "(1)", "a)", "A.", etc.
	if len(trimmed) >= 3 {
		r := []rune(trimmed)
		// "1. ", "1) ", "a) ", "A. "
		if (r[0] >= '0' && r[0] <= '9') && (r[1] == '.' || r[1] == ')') && r[2] == ' ' {
			return true
		}
		// "(1) "
		if r[0] == '(' && (r[1] >= '0' && r[1] <= '9') && r[2] == ')' {
			return true
		}
		// "a) ", "A. "
		if ((r[0] >= 'a' && r[0] <= 'z') || (r[0] >= 'A' && r[0] <= 'Z')) &&
			(r[1] == '.' || r[1] == ')') && r[2] == ' ' {
			return true
		}
	}
	return false
}

// stripHeadingPrefix removes leading "#" characters and whitespace from a heading line.
// E.g. "### •" → "•", "## - text" → "- text"
func stripHeadingPrefix(line string) string {
	s := strings.TrimSpace(line)
	for strings.HasPrefix(s, "#") {
		s = strings.TrimPrefix(s, "#")
	}
	return strings.TrimSpace(s)
}

// isJustBulletMarker returns true if the line is only a bullet/list marker character
// (no text content after it). E.g. "•", "·", "–", "-"
func isJustBulletMarker(line string) bool {
	s := strings.TrimSpace(line)
	return s == "•" || s == "·" || s == "–" || s == "-" || s == "--" || s == "---"
}

// ---------- PDF outline extraction ----------

// ExtractOutline extracts the PDF's built-in outline (bookmarks / table of contents)
// using pdftohtml -xml and parsing the <outline> section.
func ExtractOutline(pdfPath string) ([]TocItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pdftohtml", "-xml", "-stdout", "-i", pdfPath)
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

// ───────── Cross-page paragraph helpers ─────────

// GetLastParagraph returns the last paragraph block (text after the last \n\n).
func GetLastParagraph(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	blocks := strings.Split(text, "\n\n")
	for i := len(blocks) - 1; i >= 0; i-- {
		b := strings.TrimSpace(blocks[i])
		if b != "" {
			return b
		}
	}
	return ""
}

// GetFirstParagraph returns the first paragraph block (text before the first \n\n),
// stripping heading markers (#, ##, ###) from the beginning.
func GetFirstParagraph(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	blocks := strings.Split(text, "\n\n")
	for _, b := range blocks {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		// Strip heading markers from the beginning of lines
		lines := strings.Split(b, "\n")
		for len(lines) > 0 {
			first := strings.TrimSpace(lines[0])
			if strings.HasPrefix(first, "### ") || strings.HasPrefix(first, "## ") || strings.HasPrefix(first, "# ") {
				lines = lines[1:]
			} else {
				break
			}
		}
		result := strings.TrimSpace(strings.Join(lines, "\n"))
		if result != "" {
			return result
		}
	}
	return ""
}

// RemoveFirstParagraph strips the first paragraph block from the text.
// Used when the first paragraph is a continuation from the previous page
// and should only appear on the page where it starts.
func RemoveFirstParagraph(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	idx := strings.Index(text, "\n\n")
	if idx < 0 {
		return "" // entire page is one continuation paragraph
	}
	return strings.TrimSpace(text[idx+2:])
}

// IsParagraphContinued returns true if the paragraph does not end with
// sentence-ending punctuation, indicating it likely continues to the next page.
func IsParagraphContinued(para string) bool {
	para = strings.TrimSpace(para)
	if para == "" {
		return false
	}
	last := para[len(para)-1]
	return last != '.' && last != '!' && last != '?' && last != '"' && last != ')' && last != ':'
}
