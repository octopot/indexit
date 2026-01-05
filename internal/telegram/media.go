package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotd/td/telegram/downloader"
	gotdpeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"go.octolab.org/toolset/indexit/internal/telegram/mapper"
	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

type MediaOptions struct {
	Peer     uid.PeerRef
	Dir      string
	Kinds    []string
	Limit    int
	PageSize int
	MinID    int
	MaxID    int
	From     time.Time
	To       time.Time

	Overwrite bool
}

// target is one downloadable file, resolved from a message.
type target struct {
	kind     string
	mime     string
	size     int64
	name     string
	location tg.InputFileLocationClass
}

// FetchMedia walks one dialog — or one forum topic — and downloads the media of
// every message into opt.Dir, emitting one manifest record per file.
//
// Download happens in the same pass as the walk on purpose: a file location is
// only usable together with the file_reference Telegram hands out with the
// message, and that reference expires. Emitting locations now and fetching them
// later would be a contract we cannot keep.
func FetchMedia(
	ctx context.Context,
	api MediaAPI,
	cache *peers.Cache,
	out Writer,
	opt MediaOptions,
	guard RateGuard,
) error {
	if strings.TrimSpace(opt.Dir) == "" {
		return fmt.Errorf("no destination directory: --dir is required")
	}
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	resolved, err := ResolvePeer(ctx, api, cache, opt.Peer, guard)
	if err != nil {
		return err
	}

	wanted := make(map[string]bool, len(opt.Kinds))
	for _, kind := range opt.Kinds {
		kind = strings.ToLower(strings.TrimSpace(kind))
		if kind != "" {
			wanted[kind] = true
		}
	}

	log := slog.Default()
	loader := downloader.NewDownloader()
	window := walkOptions{
		Limit:    opt.Limit,
		PageSize: opt.PageSize,
		MinID:    opt.MinID,
		MaxID:    opt.MaxID,
		From:     opt.From,
		To:       opt.To,
	}

	return walkMessages(ctx, api, cache, resolved, window, guard, "media",
		func(ctx context.Context, msg *tg.Message, _ gotdpeer.Entities) (bool, error) {
			media, ok := msg.GetMedia()
			if !ok {
				return false, nil
			}
			file := resolveTarget(msg.ID, media)
			if file == nil {
				return false, nil
			}
			if len(wanted) > 0 && !wanted[file.kind] {
				return false, nil
			}

			path := filepath.Join(opt.Dir, file.name)
			record := model.MediaRecord{
				Kind:      "media",
				DialogUID: resolved.UID,
				MessageID: msg.ID,
				TopicID:   resolved.TopicID,
				GroupedID: msg.GroupedID,
				Date:      time.Unix(int64(msg.Date), 0).UTC().Format(time.RFC3339),
				Type:      file.kind,
				MIME:      file.mime,
				Size:      file.size,
				Path:      path,
			}

			if !opt.Overwrite {
				if info, err := os.Stat(path); err == nil && info.Size() > 0 {
					record.Skipped = true
					record.Size = info.Size()
					return true, out.Write(record)
				}
			}

			written, err := download(ctx, loader, api, guard, file.location, path)
			if err != nil {
				// One unreachable file must not end the walk: the run is meant to
				// drain an archive, and the manifest is what tells the operator
				// which frames are missing.
				log.Warn("media: download failed", "message_id", msg.ID, "error", err)
				record.Error = err.Error()
				return true, out.Write(record)
			}
			record.Size = written
			return true, out.Write(record)
		})
}

// download writes the file through a temporary neighbour and renames it into
// place, so an interrupted run leaves no half-file that the next run would take
// for already downloaded.
func download(
	ctx context.Context,
	loader *downloader.Downloader,
	api MediaAPI,
	guard RateGuard,
	location tg.InputFileLocationClass,
	path string,
) (int64, error) {
	tmp := path + ".part"
	defer func() { _ = os.Remove(tmp) }()

	if err := guard.Do(ctx, func(ctx context.Context) error {
		_, err := loader.Download(api, location).ToPath(ctx, tmp)
		return err
	}); err != nil {
		return 0, err
	}
	info, err := os.Stat(tmp)
	if err != nil {
		return 0, fmt.Errorf("stat downloaded file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return 0, fmt.Errorf("move downloaded file into place: %w", err)
	}
	return info.Size(), nil
}

// resolveTarget turns a message media into a downloadable file, or nil when
// there is nothing to download (a web page preview, a geo point, a poll).
// The kind matches what `fetch messages` reports for the same message, so the
// --media filter and the JSONL descriptor speak the same words.
func resolveTarget(messageID int, media tg.MessageMediaClass) *target {
	descriptor := mapper.Media(media)
	if descriptor == nil {
		return nil
	}
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := m.Photo.(*tg.Photo)
		if !ok {
			return nil
		}
		size, bytes := largestPhotoSize(photo)
		if size == "" {
			return nil
		}
		return &target{
			kind: descriptor.Type,
			mime: "image/jpeg",
			size: bytes,
			name: fmt.Sprintf("%d.jpg", messageID),
			location: &tg.InputPhotoFileLocation{
				ID:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     size,
			},
		}
	case *tg.MessageMediaDocument:
		doc, ok := m.Document.(*tg.Document)
		if !ok {
			return nil
		}
		return &target{
			kind: descriptor.Type,
			mime: doc.MimeType,
			size: doc.Size,
			name: fmt.Sprintf("%d%s", messageID, documentExt(doc)),
			location: &tg.InputDocumentFileLocation{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
			},
		}
	default:
		return nil
	}
}

// largestPhotoSize picks the biggest downloadable size of a photo and reports
// the bytes Telegram claims for it. Stripped and path sizes are placeholders
// carried inside the message, not files to fetch.
func largestPhotoSize(photo *tg.Photo) (string, int64) {
	best := ""
	bestArea := 0
	var bestBytes int64
	for _, size := range photo.Sizes {
		switch s := size.(type) {
		case *tg.PhotoSize:
			if area := s.W * s.H; area > bestArea {
				best, bestArea, bestBytes = s.Type, area, int64(s.Size)
			}
		case *tg.PhotoSizeProgressive:
			area := s.W * s.H
			if area <= bestArea {
				continue
			}
			var bytes int64
			if len(s.Sizes) > 0 {
				bytes = int64(s.Sizes[len(s.Sizes)-1])
			}
			best, bestArea, bestBytes = s.Type, area, bytes
		case *tg.PhotoCachedSize:
			if area := s.W * s.H; area > bestArea {
				best, bestArea, bestBytes = s.Type, area, int64(len(s.Bytes))
			}
		}
	}
	return best, bestBytes
}

// documentExt prefers the extension of the original file name, falls back to
// the MIME type and finally to .bin — a file with no extension is worse than a
// file with a wrong one.
func documentExt(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		name, ok := attr.(*tg.DocumentAttributeFilename)
		if !ok {
			continue
		}
		if ext := filepath.Ext(name.FileName); ext != "" {
			return ext
		}
	}
	if exts, err := mime.ExtensionsByType(doc.MimeType); err == nil && len(exts) > 0 {
		// ExtensionsByType returns the whole family in alphabetical order
		// (video/mp4 → .f4v, .m4v, .mp4), so the first entry is rarely the
		// expected one. Prefer the extension that spells the MIME subtype.
		if _, subtype, ok := strings.Cut(doc.MimeType, "/"); ok {
			for _, ext := range exts {
				if ext == "."+subtype {
					return ext
				}
			}
		}
		return exts[0]
	}
	return ".bin"
}
