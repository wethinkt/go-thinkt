package tui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	_ "image/jpeg"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/charmbracelet/x/ansi/sixel"
	"golang.org/x/image/draw"
)

// decodeImageDimensions reads just the image header to extract width x height
// without decoding the full pixel data. Returns "1920x1080" or "" on error.
func decodeImageDimensions(mediaType, base64Data string) string {
	// Decode a small prefix — image headers are typically < 1KB.
	// base64 encodes 3 bytes per 4 chars, so 2048 chars ≈ 1536 bytes of raw data.
	sample := base64Data
	if len(sample) > 2048 {
		sample = sample[:2048]
	}
	decoded, err := base64.StdEncoding.DecodeString(sample)
	if err != nil {
		// Try with padding adjustment for truncated base64
		decoded, err = base64.RawStdEncoding.DecodeString(sample)
		if err != nil {
			return ""
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)
}

// formatByteSize formats a byte count as a human-readable string.
func formatByteSize(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fMB", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fKB", float64(n)/1_000)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// graphicsProtocol represents which terminal image protocol to use.
type graphicsProtocol int

const (
	protocolNone graphicsProtocol = iota
	protocolSixel
	protocolKitty
)

// detectGraphicsProtocol checks the terminal for image protocol support.
func detectGraphicsProtocol() graphicsProtocol {
	term := os.Getenv("TERM")
	termProgram := os.Getenv("TERM_PROGRAM")

	// Kitty graphics protocol
	if strings.Contains(term, "kitty") || termProgram == "kitty" {
		return protocolKitty
	}
	// Ghostty supports kitty graphics protocol
	if termProgram == "ghostty" {
		return protocolKitty
	}
	// WezTerm supports both; prefer kitty
	if termProgram == "WezTerm" {
		return protocolKitty
	}

	// Sixel-capable terminals
	switch termProgram {
	case "iTerm.app", "foot", "mlterm", "contour":
		return protocolSixel
	}
	if strings.Contains(term, "xterm") {
		return protocolSixel
	}

	return protocolNone
}

// encodeImage decodes base64 image data, resizes to fit within maxWidthCells,
// and returns the terminal escape sequence string using the best available protocol.
func encodeImage(base64Data string, maxWidthCells, cellWidthPx int) (string, error) {
	proto := detectGraphicsProtocol()
	switch proto {
	case protocolKitty:
		return encodeKittyImage(base64Data, maxWidthCells, cellWidthPx)
	case protocolSixel:
		return encodeSixel(base64Data, maxWidthCells, cellWidthPx)
	default:
		return "", fmt.Errorf("no graphics protocol available")
	}
}

// encodeKittyImage encodes an image using the Kitty graphics protocol.
// It re-encodes as PNG and sends via chunked APC sequences.
func encodeKittyImage(base64Data string, maxWidthCells, cellWidthPx int) (string, error) {
	img, err := decodeAndResize(base64Data, maxWidthCells, cellWidthPx)
	if err != nil {
		return "", err
	}

	// Encode as PNG for kitty
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", fmt.Errorf("png encode: %w", err)
	}

	// Base64 encode the PNG data
	b64Data := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	// Kitty protocol requires chunking: max 4096 bytes per chunk
	const chunkSize = 4096
	var result strings.Builder

	for i := 0; i < len(b64Data); i += chunkSize {
		end := i + chunkSize
		if end > len(b64Data) {
			end = len(b64Data)
		}
		chunk := b64Data[i:end]
		isFirst := i == 0
		isLast := end >= len(b64Data)

		var opts []string
		if isFirst {
			// a=T: transmit and display, f=100: PNG, t=d: direct data
			opts = append(opts, "a=T", "f=100", "t=d")
		}
		if isLast {
			opts = append(opts, "m=0") // m=0: last chunk
		} else {
			opts = append(opts, "m=1") // m=1: more chunks follow
		}

		result.WriteString(ansi.KittyGraphics([]byte(chunk), opts...))
	}

	return result.String(), nil
}

// encodeSixel encodes an image using the Sixel graphics protocol.
func encodeSixel(base64Data string, maxWidthCells, cellWidthPx int) (string, error) {
	img, err := decodeAndResize(base64Data, maxWidthCells, cellWidthPx)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	enc := sixel.Encoder{}
	if err := enc.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("sixel encode: %w", err)
	}

	return ansi.SixelGraphics(0, 1, 0, buf.Bytes()), nil
}

// --- Kitty Virtual Placement (Unicode Placeholder) support ---

// cachedProtocol caches the detected graphics protocol (detected once).
var cachedProtocol struct {
	once     sync.Once
	protocol graphicsProtocol
}

func getGraphicsProtocol() graphicsProtocol {
	cachedProtocol.once.Do(func() {
		cachedProtocol.protocol = detectGraphicsProtocol()
	})
	return cachedProtocol.protocol
}

// pendingImage holds an image that needs to be transmitted to the terminal.
type pendingImage struct {
	ID         int32
	Base64Data string
	Columns    int
	Rows       int
}

// imageTracker assigns stable kitty image IDs and collects images for transmission.
type imageTracker struct {
	mu          sync.Mutex
	nextID      atomic.Int32
	assignments map[string]int32 // content key → image ID
	pending     []pendingImage
}

var globalImageTracker = &imageTracker{
	assignments: make(map[string]int32),
}

// assignImageID returns a stable image ID for the given content.
// New images are added to the pending list for transmission.
func (t *imageTracker) assignImageID(base64Data string, columns, rows int) int32 {
	// Use first 64 chars of base64 as key (sufficient for uniqueness)
	key := base64Data
	if len(key) > 64 {
		key = key[:64]
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if id, ok := t.assignments[key]; ok {
		return id
	}

	id := t.nextID.Add(1)
	t.assignments[key] = id
	t.pending = append(t.pending, pendingImage{
		ID:         id,
		Base64Data: base64Data,
		Columns:    columns,
		Rows:       rows,
	})
	return id
}

// drainPending returns and clears all pending images.
func (t *imageTracker) drainPending() []pendingImage {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.pending
	t.pending = nil
	return p
}

// imageDisplaySize calculates terminal columns and rows for an image.
func imageDisplaySize(base64Data string, maxColumns int) (columns, rows int) {
	dims := decodeImageDimensions("", base64Data)
	if dims == "" {
		return 0, 0
	}
	var w, h int
	if _, err := fmt.Sscanf(dims, "%dx%d", &w, &h); err != nil || w <= 0 || h <= 0 {
		return 0, 0
	}

	// Typical cell dimensions: 8px wide, 16px tall
	const cellW, cellH = 8, 16

	columns = w / cellW
	if columns > maxColumns {
		columns = maxColumns
	}
	if columns < 1 {
		columns = 1
	}

	// Scale height proportionally
	scaledH := h * columns * cellW / w
	rows = scaledH / cellH
	if rows < 1 {
		rows = 1
	}
	// Cap rows to avoid massive images
	if rows > 80 {
		rows = 80
	}

	return columns, rows
}

// kittyPlaceholderGrid generates a grid of Unicode placeholder characters
// that the terminal replaces with the actual image.
func kittyPlaceholderGrid(imageID int32, columns, rows int) string {
	r := byte((imageID >> 16) & 0xFF)
	g := byte((imageID >> 8) & 0xFF)
	b := byte(imageID & 0xFF)

	fgColor := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	reset := "\x1b[39m"
	placeholder := string(kitty.Placeholder)

	var sb strings.Builder
	for row := 0; row < rows; row++ {
		sb.WriteString(fgColor)
		diacritic := string(kitty.Diacritic(row))
		for col := 0; col < columns; col++ {
			sb.WriteString(placeholder)
			sb.WriteString(diacritic)
		}
		sb.WriteString(reset)
		if row < rows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// kittyTransmitCmd creates a tea.Cmd that transmits pending images
// to the terminal using the kitty graphics protocol with virtual placement.
func kittyTransmitCmd(images []pendingImage) tea.Cmd {
	if len(images) == 0 {
		return nil
	}

	var cmds []tea.Cmd
	for _, img := range images {
		seq, err := kittyTransmitSequence(img)
		if err != nil {
			continue
		}
		cmds = append(cmds, tea.Raw(seq))
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// kittyTransmitSequence generates the escape sequence to transmit an image
// using the kitty graphics protocol with virtual placement (U=1).
func kittyTransmitSequence(img pendingImage) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(img.Base64Data)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	goImg, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("image decode: %w", err)
	}

	var buf bytes.Buffer
	err = kitty.EncodeGraphics(&buf, goImg, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		Format:           kitty.PNG,
		Transmission:     kitty.Direct,
		ID:               int(img.ID),
		Columns:          img.Columns,
		Rows:             img.Rows,
		VirtualPlacement: true,
		Chunk:            true,
		Quite:            1,
	})
	if err != nil {
		return "", fmt.Errorf("kitty encode: %w", err)
	}

	return buf.String(), nil
}

// renderImageBlock renders an image content block, using kitty placeholders
// when available, falling back to text placeholder otherwise.
func renderImageBlock(mediaType, mediaData string, width int) string {
	if mediaData == "" {
		return ""
	}

	label := imageLabel.Render("Image")
	rawBytes := len(mediaData) * 3 / 4
	sizeStr := formatByteSize(rawBytes)
	dims := decodeImageDimensions(mediaType, mediaData)

	// Try kitty placeholder rendering
	if getGraphicsProtocol() == protocolKitty {
		columns, rows := imageDisplaySize(mediaData, width)
		if columns > 0 && rows > 0 {
			imageID := globalImageTracker.assignImageID(mediaData, columns, rows)
			grid := kittyPlaceholderGrid(imageID, columns, rows)
			var detail string
			if dims != "" {
				detail = fmt.Sprintf("[%s %s, %s]", mediaType, dims, sizeStr)
			} else {
				detail = fmt.Sprintf("[%s %s]", mediaType, sizeStr)
			}
			return label + "\n" + grid + "\n" + imageBlockStyle.Width(width).Render(detail)
		}
	}

	// Fallback to text placeholder
	var detail string
	if dims != "" {
		detail = fmt.Sprintf("[%s %s, %s]", mediaType, dims, sizeStr)
	} else {
		detail = fmt.Sprintf("[%s %s]", mediaType, sizeStr)
	}
	content := imageBlockStyle.Width(width).Render(detail)
	return label + "\n" + content
}

// decodeAndResize decodes base64 image data and resizes to fit terminal width.
func decodeAndResize(base64Data string, maxWidthCells, cellWidthPx int) (image.Image, error) {
	raw, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("image decode: %w", err)
	}

	if cellWidthPx <= 0 {
		cellWidthPx = 8
	}
	maxWidthPx := maxWidthCells * cellWidthPx
	bounds := img.Bounds()
	if bounds.Dx() > maxWidthPx {
		ratio := float64(maxWidthPx) / float64(bounds.Dx())
		newW := maxWidthPx
		newH := int(float64(bounds.Dy()) * ratio)
		if newH < 1 {
			newH = 1
		}
		resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
		draw.BiLinear.Scale(resized, resized.Bounds(), img, bounds, draw.Over, nil)
		return resized, nil
	}

	return img, nil
}
