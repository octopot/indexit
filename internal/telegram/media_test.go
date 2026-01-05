package telegram

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.octolab.org/toolset/indexit/internal/telegram/model"
	"go.octolab.org/toolset/indexit/internal/telegram/peers"
	"go.octolab.org/toolset/indexit/internal/telegram/uid"
)

// fileAPI is scriptedAPI plus the upload.* surface the downloader needs: it
// serves one in-memory file, in whatever chunks the downloader asks for.
type fileAPI struct {
	scriptedAPI

	content []byte
	fail    bool
	reqs    []*tg.UploadGetFileRequest
}

func (f *fileAPI) UploadGetFile(_ context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	f.reqs = append(f.reqs, req)
	if f.fail {
		return nil, errors.New("upload.getFile refused")
	}
	offset := int(req.Offset)
	if offset > len(f.content) {
		offset = len(f.content)
	}
	end := offset + req.Limit
	if end > len(f.content) {
		end = len(f.content)
	}
	return &tg.UploadFile{Type: &tg.StorageFileJpeg{}, Bytes: f.content[offset:end]}, nil
}

func (f *fileAPI) UploadGetFileHashes(context.Context, *tg.UploadGetFileHashesRequest) ([]tg.FileHash, error) {
	return nil, errors.New("unexpected UploadGetFileHashes call")
}

func (f *fileAPI) UploadReuploadCDNFile(context.Context, *tg.UploadReuploadCDNFileRequest) ([]tg.FileHash, error) {
	return nil, errors.New("unexpected UploadReuploadCDNFile call")
}

func (f *fileAPI) UploadGetCDNFileHashes(context.Context, *tg.UploadGetCDNFileHashesRequest) ([]tg.FileHash, error) {
	return nil, errors.New("unexpected UploadGetCDNFileHashes call")
}

func (f *fileAPI) UploadGetWebFile(context.Context, *tg.UploadGetWebFileRequest) (*tg.UploadWebFile, error) {
	return nil, errors.New("unexpected UploadGetWebFile call")
}

func photoMessage(id int, groupedID int64) *tg.Message {
	msg := &tg.Message{ID: id, Date: 1700000000}
	msg.SetMedia(&tg.MessageMediaPhoto{
		Photo: &tg.Photo{
			ID:            int64(id) * 10,
			AccessHash:    7,
			FileReference: []byte{0xAA},
			Sizes: []tg.PhotoSizeClass{
				&tg.PhotoStrippedSize{Type: "i"},
				&tg.PhotoSize{Type: "m", W: 320, H: 320, Size: 10},
				&tg.PhotoSize{Type: "y", W: 1280, H: 1280, Size: 64},
			},
		},
	})
	if groupedID != 0 {
		msg.SetGroupedID(groupedID)
	}
	return msg
}

func mediaFixture(t *testing.T, messages ...*tg.Message) (*fileAPI, *peers.Cache, uid.PeerRef) {
	t.Helper()
	page := &tg.MessagesMessages{}
	for _, msg := range messages {
		page.Messages = append(page.Messages, msg)
	}
	api := &fileAPI{content: make([]byte, 64)}
	api.historyPages = []tg.MessagesMessagesClass{page, &tg.MessagesMessages{}}
	cache := peers.New()
	cache.Put(peers.Entry{Kind: uid.KindUser, ID: 1, AccessHash: 2})
	ref, err := uid.Parse("user:1")
	require.NoError(t, err)
	return api, cache, ref
}

func TestFetchMediaDownloadsAndSkips(t *testing.T) {
	dir := t.TempDir()
	api, cache, ref := mediaFixture(t, photoMessage(11, 900), photoMessage(12, 900))
	out := &recWriter{}

	require.NoError(t, FetchMedia(context.Background(), api, cache, out,
		MediaOptions{Peer: ref, Dir: dir}, RateGuard{}))
	require.Len(t, out.records, 2)

	first, ok := out.records[0].(model.MediaRecord)
	require.True(t, ok)
	assert.Equal(t, "media", first.Kind)
	assert.Equal(t, 11, first.MessageID)
	assert.EqualValues(t, 900, first.GroupedID)
	assert.Equal(t, "photo", first.Type)
	assert.Equal(t, filepath.Join(dir, "11.jpg"), first.Path)
	assert.False(t, first.Skipped)
	assert.EqualValues(t, 64, first.Size)
	assert.Empty(t, first.Error)

	for _, name := range []string{"11.jpg", "12.jpg"} {
		info, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.EqualValues(t, 64, info.Size())
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "no .part leftovers")

	// Second pass over the same directory downloads nothing.
	again, cache2, ref2 := mediaFixture(t, photoMessage(11, 900), photoMessage(12, 900))
	out2 := &recWriter{}
	require.NoError(t, FetchMedia(context.Background(), again, cache2, out2,
		MediaOptions{Peer: ref2, Dir: dir}, RateGuard{}))
	require.Len(t, out2.records, 2)
	assert.True(t, out2.records[0].(model.MediaRecord).Skipped)
	assert.Empty(t, again.reqs, "an existing file is not fetched again")

	// ...unless asked to overwrite.
	third, cache3, ref3 := mediaFixture(t, photoMessage(11, 900))
	require.NoError(t, FetchMedia(context.Background(), third, cache3, &recWriter{},
		MediaOptions{Peer: ref3, Dir: dir, Overwrite: true}, RateGuard{}))
	assert.NotEmpty(t, third.reqs)
}

func TestFetchMediaReportsFailureAndContinues(t *testing.T) {
	dir := t.TempDir()
	api, cache, ref := mediaFixture(t, photoMessage(11, 0))
	api.fail = true
	out := &recWriter{}

	require.NoError(t, FetchMedia(context.Background(), api, cache, out,
		MediaOptions{Peer: ref, Dir: dir}, RateGuard{}))
	require.Len(t, out.records, 1)
	record := out.records[0].(model.MediaRecord)
	assert.NotEmpty(t, record.Error)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a failed download leaves nothing behind")
}

func TestFetchMediaFiltersByKind(t *testing.T) {
	dir := t.TempDir()
	api, cache, ref := mediaFixture(t, photoMessage(11, 0))
	out := &recWriter{}

	require.NoError(t, FetchMedia(context.Background(), api, cache, out,
		MediaOptions{Peer: ref, Dir: dir, Kinds: []string{"video"}}, RateGuard{}))
	assert.Empty(t, out.records)
	assert.Empty(t, api.reqs)
}

func TestFetchMediaLimitCountsFiles(t *testing.T) {
	dir := t.TempDir()
	text := &tg.Message{ID: 13, Date: 1700000000, Message: "no media here"}
	api, cache, ref := mediaFixture(t, text, photoMessage(11, 0), photoMessage(12, 0))
	out := &recWriter{}

	require.NoError(t, FetchMedia(context.Background(), api, cache, out,
		MediaOptions{Peer: ref, Dir: dir, Limit: 1}, RateGuard{}))
	require.Len(t, out.records, 1, "the text message must not consume the budget")
	assert.Equal(t, 11, out.records[0].(model.MediaRecord).MessageID)
}

func TestFetchMediaRequiresDir(t *testing.T) {
	api, cache, ref := mediaFixture(t, photoMessage(11, 0))
	err := FetchMedia(context.Background(), api, cache, &recWriter{},
		MediaOptions{Peer: ref}, RateGuard{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--dir")
}

func TestResolveTargetPicksLargestPhotoSize(t *testing.T) {
	msg := photoMessage(11, 0)
	media, ok := msg.GetMedia()
	require.True(t, ok)

	file := resolveTarget(msg.ID, media)
	require.NotNil(t, file)
	assert.Equal(t, "photo", file.kind)
	assert.Equal(t, "11.jpg", file.name)
	assert.EqualValues(t, 64, file.size)

	location, ok := file.location.(*tg.InputPhotoFileLocation)
	require.True(t, ok)
	assert.Equal(t, "y", location.ThumbSize, "the stripped preview is not a file to fetch")
	assert.EqualValues(t, 110, location.ID)
	assert.Equal(t, []byte{0xAA}, location.FileReference)
}

func TestResolveTargetDocumentNaming(t *testing.T) {
	doc := &tg.Document{ID: 5, AccessHash: 6, MimeType: "video/mp4", Size: 128}
	doc.Attributes = []tg.DocumentAttributeClass{
		&tg.DocumentAttributeVideo{Duration: 3},
		&tg.DocumentAttributeFilename{FileName: "IMG_0001.MOV"},
	}
	file := resolveTarget(21, &tg.MessageMediaDocument{Document: doc})
	require.NotNil(t, file)
	assert.Equal(t, "video", file.kind)
	assert.Equal(t, "21.MOV", file.name)
	assert.Equal(t, "video/mp4", file.mime)

	bare := &tg.Document{ID: 5, AccessHash: 6, MimeType: "video/mp4", Size: 128}
	file = resolveTarget(22, &tg.MessageMediaDocument{Document: bare})
	require.NotNil(t, file)
	assert.Equal(t, ".mp4", filepath.Ext(file.name), "the MIME type names the file when the original name is gone")
}

func TestResolveTargetIgnoresNonFiles(t *testing.T) {
	assert.Nil(t, resolveTarget(1, &tg.MessageMediaWebPage{}))
	assert.Nil(t, resolveTarget(2, &tg.MessageMediaGeo{}))
	assert.Nil(t, resolveTarget(3, &tg.MessageMediaPoll{}))
}
