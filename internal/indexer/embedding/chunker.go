package embedding

// ChunkText splits text into chunks of at most maxChars runes
// with overlap runes of overlap between consecutive chunks.
// Returns nil for empty text.
func ChunkText(text string, maxChars, overlap int) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	if len(runes) <= maxChars {
		return []string{text}
	}

	var chunks []string
	step := maxChars - overlap
	for start := 0; start < len(runes); start += step {
		end := min(start+maxChars, len(runes))
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}
