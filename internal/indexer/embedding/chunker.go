package embedding

// ChunkText splits text into chunks of at most maxChars characters
// with overlap characters of overlap between consecutive chunks.
// Returns nil for empty text.
func ChunkText(text string, maxChars, overlap int) []string {
	if len(text) == 0 {
		return nil
	}
	if len(text) <= maxChars {
		return []string{text}
	}

	var chunks []string
	step := maxChars - overlap
	for start := 0; start < len(text); start += step {
		end := min(start+maxChars, len(text))
		chunks = append(chunks, text[start:end])
		if end == len(text) {
			break
		}
	}
	return chunks
}
