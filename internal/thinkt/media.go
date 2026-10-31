package thinkt

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"strings"
)

// MediaRef says where an image or document block's bytes are, and identifies
// them when they are known. URI is a file path or URL when the bytes live
// elsewhere; SHA256 and Size describe the decoded bytes.
type MediaRef struct {
	URI    string `json:"uri,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
}

// mediaBlockType is "image" for image/* and "document" otherwise.
func mediaBlockType(mediaType string) string {
	if strings.HasPrefix(mediaType, "image/") {
		return "image"
	}
	return "document"
}

// MediaBlock returns a block for inline base64 data. Data that is not valid
// base64 is hashed as text, so the block still has an identity.
func MediaBlock(mediaType, data string) ContentBlock {
	b := ContentBlock{Type: mediaBlockType(mediaType), MediaType: mediaType, MediaData: data}
	b.Media = inlineRef(data)
	return b
}

func inlineRef(data string) *MediaRef {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		raw = []byte(data)
	}
	sum := sha256.Sum256(raw)
	return &MediaRef{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(raw))}
}

// MediaBlockURI returns a block whose bytes live at uri, a path or URL. A
// file:// URI becomes a plain path.
func MediaBlockURI(mediaType, uri string) ContentBlock {
	if rest, ok := strings.CutPrefix(uri, "file://"); ok {
		if p, err := url.PathUnescape(rest); err == nil {
			rest = p
		}
		uri = rest
	}
	return ContentBlock{Type: mediaBlockType(mediaType), MediaType: mediaType, Media: &MediaRef{URI: uri}}
}

// MediaBlockURL returns a block for a base64 data: URL, or for any other URL
// as a reference, assumed to be an image. It reports false for an empty URL.
func MediaBlockURL(u string) (ContentBlock, bool) {
	if u == "" {
		return ContentBlock{}, false
	}
	if rest, ok := strings.CutPrefix(u, "data:"); ok {
		if head, data, ok := strings.Cut(rest, ";base64,"); ok {
			return MediaBlock(head, data), true
		}
	}
	return ContentBlock{Type: "image", Media: &MediaRef{URI: u}}, true
}
