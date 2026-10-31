package thinkt

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestMediaBlockInline(t *testing.T) {
	raw := []byte("png bytes")
	sum := sha256.Sum256(raw)
	b := MediaBlock("image/png", base64.StdEncoding.EncodeToString(raw))
	if b.Type != "image" || b.MediaType != "image/png" || b.MediaData == "" || b.Media == nil {
		t.Fatalf("%+v", b)
	}
	if b.Media.SHA256 != hex.EncodeToString(sum[:]) || b.Media.Size != int64(len(raw)) || b.Media.URI != "" {
		t.Fatalf("%+v", b.Media)
	}
}

func TestMediaBlockDocumentType(t *testing.T) {
	if b := MediaBlock("application/pdf", "eA=="); b.Type != "document" {
		t.Fatalf("%+v", b)
	}
}

func TestMediaBlockBadBase64StillHasIdentity(t *testing.T) {
	b := MediaBlock("image/png", "not base64!")
	if b.Media == nil || b.Media.SHA256 == "" {
		t.Fatalf("%+v", b)
	}
}

func TestMediaBlockSameBytesSameHash(t *testing.T) {
	a := MediaBlock("image/png", "eHg=")
	c, ok := MediaBlockURL("data:image/png;base64,eHg=")
	if !ok || a.Media.SHA256 != c.Media.SHA256 {
		t.Fatalf("%+v %+v", a.Media, c.Media)
	}
}

func TestMediaBlockURI(t *testing.T) {
	for in, want := range map[string]string{
		"file:///b/x.png":     "/b/x.png",
		"file:///b/a%20b.png": "/b/a b.png",
		"/b/x.png":            "/b/x.png",
		"https://e.com/x.png": "https://e.com/x.png",
	} {
		b := MediaBlockURI("image/png", in)
		if b.Media == nil || b.Media.URI != want || b.MediaData != "" || b.Type != "image" {
			t.Errorf("%s: %+v", in, b.Media)
		}
	}
}

func TestMediaBlockURL(t *testing.T) {
	if b, ok := MediaBlockURL("https://e.com/x.png"); !ok || b.Media.URI != "https://e.com/x.png" || b.Type != "image" {
		t.Fatalf("%+v", b)
	}
	if _, ok := MediaBlockURL(""); ok {
		t.Fatal("empty URL")
	}
	if _, ok := MediaBlockURL("data:nonsense"); ok {
		t.Fatal("malformed data URL")
	}
	if b, ok := MediaBlockURL("data:application/pdf;base64,eA=="); !ok || b.Type != "document" || b.MediaType != "application/pdf" {
		t.Fatalf("%+v", b)
	}
}
