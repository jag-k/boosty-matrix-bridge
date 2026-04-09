package connector

import (
	"context"
	"fmt"
	"html"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
)

// convertToMatrix converts a Boosty message to a bridgev2 ConvertedMessage.
func (b *BoostyClient) convertToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, msg boosty.Message) (*bridgev2.ConvertedMessage, error) {
	log := zerolog.Ctx(ctx)
	var cm bridgev2.ConvertedMessage

	// Separate text and media blocks
	var textBlocks []boosty.ContentBlock
	var mediaBlocks []boosty.ContentBlock

	for _, block := range msg.Data {
		switch block.Type {
		case boosty.ContentTypeText, boosty.ContentTypeLink:
			textBlocks = append(textBlocks, block)
		default:
			mediaBlocks = append(mediaBlocks, block)
		}
	}

	// Convert text blocks into a single text part
	if len(textBlocks) > 0 {
		plainText := boosty.RenderContentBlocks(textBlocks)
		htmlText := boosty.RenderContentBlocksHTML(textBlocks)

		content := &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    plainText,
		}
		if htmlText != plainText {
			content.Format = event.FormatHTML
			content.FormattedBody = htmlText
		}

		cm.Parts = append(cm.Parts, &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: content,
		})
	}

	// Convert each media block into a separate part
	for _, block := range mediaBlocks {
		var part *bridgev2.ConvertedMessagePart
		var err error

		switch block.Type {
		case boosty.ContentTypeImage:
			part, err = b.convertImageToMatrix(ctx, portal, intent, block)
		case boosty.ContentTypeFile:
			part, err = b.convertFileToMatrix(ctx, portal, intent, block)
		case boosty.ContentTypeAudio:
			part, err = b.convertAudioToMatrix(ctx, portal, intent, block)
		case boosty.ContentTypeOKVideo:
			part, err = b.convertOKVideoToMatrix(ctx, block)
		case boosty.ContentTypeVideo:
			part, err = b.convertVideoLinkToMatrix(ctx, block)
		case boosty.ContentTypeSmile:
			part, err = b.convertSmileToMatrix(ctx, portal, intent, block)
		default:
			log.Warn().Str("type", block.Type).Msg("Unknown content block type, skipping")
			continue
		}

		if err != nil {
			log.Err(err).Str("type", block.Type).Msg("Failed to convert media block")
			continue
		}
		if part != nil {
			cm.Parts = append(cm.Parts, part)
		}
	}

	cm.MergeCaption()

	// Fallback: if no parts at all, create a placeholder
	if len(cm.Parts) == 0 {
		cm.Parts = append(cm.Parts, &bridgev2.ConvertedMessagePart{
			Type: event.EventMessage,
			Content: &event.MessageEventContent{
				MsgType: event.MsgNotice,
				Body:    "[empty message]",
			},
		})
	}

	return &cm, nil
}

func (b *BoostyClient) convertImageToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	if block.URL == "" {
		return nil, fmt.Errorf("image block has no URL")
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgImage,
		Body:    "image.jpg",
		Info: &event.FileInfo{
			Width:  block.Width,
			Height: block.Height,
			Size:   block.Size,
		},
	}

	var err error
	content.URL, content.File, err = intent.UploadMediaStream(ctx, portal.MXID, int64(block.Size), true, func(w io.Writer) (*bridgev2.FileStreamResult, error) {
		return b.downloadBoostyMedia(ctx, w, block.URL)
	})
	if err != nil {
		return nil, err
	}

	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
	}, nil
}

func (b *BoostyClient) convertFileToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	if block.URL == "" {
		return nil, fmt.Errorf("file block has no URL")
	}

	fileName := block.Title
	if fileName == "" {
		fileName = "file"
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgFile,
		Body:    fileName,
		Info: &event.FileInfo{
			Size: block.Size,
		},
	}

	var err error
	content.URL, content.File, err = intent.UploadMediaStream(ctx, portal.MXID, int64(block.Size), true, func(w io.Writer) (*bridgev2.FileStreamResult, error) {
		result, err := b.downloadBoostyMedia(ctx, w, block.URL)
		if err != nil {
			return nil, err
		}
		result.FileName = fileName
		return result, nil
	})
	if err != nil {
		return nil, err
	}

	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
	}, nil
}

func (b *BoostyClient) convertAudioToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	if block.URL == "" {
		return nil, fmt.Errorf("audio block has no URL")
	}

	fileName := block.Title
	if fileName == "" {
		fileName = "audio"
	}

	content := &event.MessageEventContent{
		MsgType: event.MsgAudio,
		Body:    fileName,
		Info: &event.FileInfo{
			Size:     block.Size,
			Duration: block.Duration * 1000, // seconds to ms
		},
	}

	var err error
	content.URL, content.File, err = intent.UploadMediaStream(ctx, portal.MXID, int64(block.Size), true, func(w io.Writer) (*bridgev2.FileStreamResult, error) {
		result, err := b.downloadBoostyMedia(ctx, w, block.URL)
		if err != nil {
			return nil, err
		}
		result.FileName = fileName
		return result, nil
	})
	if err != nil {
		return nil, err
	}

	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventMessage,
		Content: content,
	}, nil
}

func (b *BoostyClient) convertOKVideoToMatrix(_ context.Context, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	// ok_video has playerUrls with HLS/DASH streams which can't be easily
	// reuploaded to Matrix. Send as a text notice with the best available URL.
	var bestURL string
	for _, pu := range block.PlayerURLs {
		if pu.URL != "" {
			bestURL = pu.URL
			// Prefer full_hd or high
			if pu.Type == "full_hd" || pu.Type == "high" {
				break
			}
		}
	}

	title := block.Title
	if title == "" {
		title = "video"
	}

	body := fmt.Sprintf("[video: %s]", title)
	htmlBody := body
	if bestURL != "" {
		body = fmt.Sprintf("%s\n%s", title, bestURL)
		htmlBody = fmt.Sprintf(`<a href="%s">🎬 %s</a>`, html.EscapeString(bestURL), html.EscapeString(title))
	}

	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          body,
			Format:        event.FormatHTML,
			FormattedBody: htmlBody,
		},
	}, nil
}

func (b *BoostyClient) convertVideoLinkToMatrix(_ context.Context, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	body := block.URL
	htmlBody := fmt.Sprintf(`<a href="%s">🎬 video</a>`, html.EscapeString(block.URL))

	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Body:          body,
			Format:        event.FormatHTML,
			FormattedBody: htmlBody,
		},
	}, nil
}

func (b *BoostyClient) convertSmileToMatrix(ctx context.Context, portal *bridgev2.Portal, intent bridgev2.MatrixAPI, block boosty.ContentBlock) (*bridgev2.ConvertedMessagePart, error) {
	stickerURL := block.LargeURL
	if stickerURL == "" {
		stickerURL = block.MediumURL
	}
	if stickerURL == "" {
		stickerURL = block.SmallURL
	}
	if stickerURL == "" {
		return nil, fmt.Errorf("smile block has no URL")
	}

	content := &event.MessageEventContent{
		Body: block.Name,
		Info: &event.FileInfo{},
	}

	var err error
	content.URL, content.File, err = intent.UploadMediaStream(ctx, portal.MXID, 0, true, func(w io.Writer) (*bridgev2.FileStreamResult, error) {
		return b.downloadBoostyMedia(ctx, w, stickerURL)
	})
	if err != nil {
		return nil, err
	}

	return &bridgev2.ConvertedMessagePart{
		Type:    event.EventSticker,
		Content: content,
	}, nil
}

// downloadBoostyMedia downloads a URL using the authenticated Boosty client and writes it to w.
func (b *BoostyClient) downloadBoostyMedia(ctx context.Context, w io.Writer, mediaURL string) (*bridgev2.FileStreamResult, error) {
	resp, err := b.client.DownloadMedia(ctx, mediaURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		return nil, err
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = guessMimeFromURL(mediaURL)
	}
	if idx := strings.IndexByte(mimeType, ';'); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	return &bridgev2.FileStreamResult{
		MimeType: mimeType,
	}, nil
}

func guessMimeFromURL(url string) string {
	ext := path.Ext(url)
	if ext == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}
