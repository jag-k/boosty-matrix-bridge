package boosty

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// ParseTextContent parses a Boosty content JSON string: ["text", "unstyled", [[0,0,5],...]]
func ParseTextContent(raw string) (*TextContent, error) {
	if raw == "" {
		return &TextContent{}, nil
	}

	var parts []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		return nil, fmt.Errorf("failed to parse content JSON: %w", err)
	}

	tc := &TextContent{}
	if len(parts) >= 1 {
		if err := json.Unmarshal(parts[0], &tc.Text); err != nil {
			return nil, fmt.Errorf("failed to parse text: %w", err)
		}
	}
	if len(parts) >= 2 {
		if err := json.Unmarshal(parts[1], &tc.Style); err != nil {
			return nil, fmt.Errorf("failed to parse style: %w", err)
		}
	}
	if len(parts) >= 3 {
		var rawEntities [][]json.RawMessage
		if err := json.Unmarshal(parts[2], &rawEntities); err != nil {
			// entities might be an empty array
			tc.Entities = nil
		} else {
			for _, rawEnt := range rawEntities {
				if len(rawEnt) < 3 {
					continue
				}
				var formatType, offset, length int
				if err := json.Unmarshal(rawEnt[0], &formatType); err != nil {
					continue
				}
				if err := json.Unmarshal(rawEnt[1], &offset); err != nil {
					continue
				}
				if err := json.Unmarshal(rawEnt[2], &length); err != nil {
					continue
				}
				tc.Entities = append(tc.Entities, TextEntity{
					FormatType: formatType,
					Offset:     offset,
					Length:     length,
				})
			}
		}
	}

	return tc, nil
}

// RenderContentBlocks converts a slice of ContentBlock to plain text.
func RenderContentBlocks(blocks []ContentBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case ContentTypeText:
			if block.Modificator == "BLOCK_END" {
				sb.WriteString("\n")
				continue
			}
			tc, err := ParseTextContent(block.Content)
			if err != nil {
				continue
			}
			sb.WriteString(tc.Text)
		case ContentTypeLink:
			tc, err := ParseTextContent(block.Content)
			if err != nil {
				continue
			}
			if block.URL != "" {
				sb.WriteString(fmt.Sprintf("%s (%s)", tc.Text, block.URL))
			} else {
				sb.WriteString(tc.Text)
			}
		case ContentTypeImage:
			sb.WriteString(fmt.Sprintf("[image: %s]", block.URL))
		case ContentTypeFile:
			sb.WriteString(fmt.Sprintf("[file: %s]", block.Title))
		case ContentTypeAudio:
			sb.WriteString(fmt.Sprintf("[audio: %s]", block.Title))
		case ContentTypeOKVideo:
			sb.WriteString(fmt.Sprintf("[video: %s]", block.Title))
		case ContentTypeVideo:
			sb.WriteString(fmt.Sprintf("[video: %s]", block.URL))
		case ContentTypeSmile:
			sb.WriteString(fmt.Sprintf("[sticker: %s]", block.Name))
		default:
			sb.WriteString(fmt.Sprintf("[%s]", block.Type))
		}
	}

	// Trim trailing newlines
	result := sb.String()
	result = strings.TrimRight(result, "\n")
	return result
}

// RenderContentBlocksHTML converts a slice of ContentBlock to HTML.
func RenderContentBlocksHTML(blocks []ContentBlock) string {
	var sb strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case ContentTypeText:
			if block.Modificator == "BLOCK_END" {
				sb.WriteString("<br/>")
				continue
			}
			tc, err := ParseTextContent(block.Content)
			if err != nil {
				continue
			}
			sb.WriteString(applyHTMLFormatting(tc))
		case ContentTypeLink:
			tc, err := ParseTextContent(block.Content)
			if err != nil {
				continue
			}
			if block.URL != "" {
				sb.WriteString(fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(block.URL), html.EscapeString(tc.Text)))
			} else {
				sb.WriteString(html.EscapeString(tc.Text))
			}
		case ContentTypeImage:
			sb.WriteString(fmt.Sprintf(`<img src="%s" width="%d" height="%d" alt="image"/>`, html.EscapeString(block.URL), block.Width, block.Height))
		case ContentTypeFile:
			sb.WriteString(fmt.Sprintf(`<a href="%s">📎 %s</a>`, html.EscapeString(block.URL), html.EscapeString(block.Title)))
		case ContentTypeAudio:
			sb.WriteString(fmt.Sprintf(`<a href="%s">🎵 %s</a>`, html.EscapeString(block.URL), html.EscapeString(block.Title)))
		case ContentTypeOKVideo:
			sb.WriteString(fmt.Sprintf(`[video: %s]`, html.EscapeString(block.Title)))
		case ContentTypeVideo:
			sb.WriteString(fmt.Sprintf(`<a href="%s">🎬 video</a>`, html.EscapeString(block.URL)))
		case ContentTypeSmile:
			sb.WriteString(fmt.Sprintf(`<img src="%s" alt="%s" height="32"/>`, html.EscapeString(block.LargeURL), html.EscapeString(block.Name)))
		default:
			sb.WriteString(fmt.Sprintf("[%s]", block.Type))
		}
	}

	result := sb.String()
	for strings.HasSuffix(result, "<br/>") {
		result = strings.TrimSuffix(result, "<br/>")
	}
	return result
}

// applyHTMLFormatting applies formatting entities to text.
func applyHTMLFormatting(tc *TextContent) string {
	if len(tc.Entities) == 0 {
		return html.EscapeString(tc.Text)
	}

	runes := []rune(tc.Text)

	opens := make(map[int][]string)
	closes := make(map[int][]string)

	for _, ent := range tc.Entities {
		var open, close string
		switch ent.FormatType {
		case TextFormatBold:
			open, close = "<b>", "</b>"
		case TextFormatItalic:
			open, close = "<i>", "</i>"
		case TextFormatUnderline:
			open, close = "<u>", "</u>"
		default:
			continue
		}
		start := ent.Offset
		end := ent.Offset + ent.Length
		if start > len(runes) {
			start = len(runes)
		}
		if end > len(runes) {
			end = len(runes)
		}
		opens[start] = append(opens[start], open)
		closes[end] = append(closes[end], close)
	}

	var sb strings.Builder
	for i, r := range runes {
		for _, c := range closes[i] {
			sb.WriteString(c)
		}
		for _, o := range opens[i] {
			sb.WriteString(o)
		}
		sb.WriteString(html.EscapeString(string(r)))
	}
	for _, c := range closes[len(runes)] {
		sb.WriteString(c)
	}

	return sb.String()
}
