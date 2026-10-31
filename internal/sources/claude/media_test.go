package claude

import "testing"

func TestImagesCarryMediaRefs(t *testing.T) {
	blocks := convertUserBlocks([]ContentBlock{{Type: "image", Source: &MediaSource{MediaType: "image/png", Data: "eHg="}}}, nil, nil)
	if len(blocks) != 1 || blocks[0].Media == nil || blocks[0].Media.Size != 2 || blocks[0].MediaData == "" {
		t.Fatalf("%+v", blocks)
	}
	_, children := convertToolResultContent([]byte(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"eHg="}}]`))
	if len(children) != 1 || children[0].Media == nil || children[0].Type != "image" {
		t.Fatalf("%+v", children)
	}
}
