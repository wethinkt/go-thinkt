package embedding

import (
	"context"
	"math"
	"os"
	"testing"
)

func TestEmbedder(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s (run EnsureModel first)", modelPath)
	}

	e, err := NewEmbedder("", modelPath)
	if err != nil {
		t.Fatalf("NewEmbedder: %v", err)
	}
	defer e.Close()

	if e.Dim() <= 0 {
		t.Fatalf("Dim() = %d, want > 0", e.Dim())
	}
	if e.EmbedModelID() != DefaultModelID {
		t.Errorf("EmbedModelID() = %q, want %q", e.EmbedModelID(), DefaultModelID)
	}

	texts := []string{
		"The quick brown fox jumps over the lazy dog.",
		"Machine learning models convert text into vectors.",
	}

	result, err := e.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(result.Vectors) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(result.Vectors), len(texts))
	}
	if result.TotalTokens <= 0 {
		t.Errorf("TotalTokens = %d, want > 0", result.TotalTokens)
	}

	for i, vec := range result.Vectors {
		if len(vec) != e.Dim() {
			t.Errorf("vec[%d] dim = %d, want %d", i, len(vec), e.Dim())
		}

		// Verify L2 normalization: magnitude should be ~1.0.
		var sum float64
		for _, v := range vec {
			sum += float64(v) * float64(v)
		}
		mag := math.Sqrt(sum)
		if math.Abs(mag-1.0) > 1e-4 {
			t.Errorf("vec[%d] L2 magnitude = %f, want ~1.0", i, mag)
		}
	}

	vecs := result.Vectors
	// Sanity: the two different texts should produce different vectors.
	if len(vecs[0]) > 0 && len(vecs[1]) > 0 && vecs[0][0] == vecs[1][0] {
		// Not a definitive check, but a basic sanity signal.
		allSame := true
		for j := range vecs[0] {
			if vecs[0][j] != vecs[1][j] {
				allSame = false
				break
			}
		}
		if allSame {
			t.Error("two different texts produced identical vectors")
		}
	}
}

func TestEmbedEmpty(t *testing.T) {
	modelPath, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		t.Skipf("model not downloaded: %s", modelPath)
	}

	e, err := NewEmbedder("", modelPath)
	if err != nil {
		t.Fatalf("NewEmbedder: %v", err)
	}
	defer e.Close()

	// Empty slice should return empty result.
	result, err := e.Embed(context.Background(), nil)
	if err != nil {
		t.Fatalf("Embed(nil): %v", err)
	}
	if len(result.Vectors) != 0 {
		t.Errorf("Embed(nil).Vectors len = %d, want 0", len(result.Vectors))
	}
}

func TestDefaultModelPath(t *testing.T) {
	p, err := DefaultModelPath()
	if err != nil {
		t.Fatalf("DefaultModelPath: %v", err)
	}
	if p == "" {
		t.Fatal("DefaultModelPath returned empty string")
	}
	// Should end with the expected model filename.
	spec, _ := LookupModel("")
	if got := p[len(p)-len(spec.FileName):]; got != spec.FileName {
		t.Errorf("path ends with %q, want %q", got, spec.FileName)
	}
}

func TestNormalizeL2(t *testing.T) {
	vec := []float32{3.0, 4.0}
	normalizeL2(vec)

	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if mag := math.Sqrt(sum); math.Abs(mag-1.0) > 1e-6 {
		t.Errorf("after normalizeL2, magnitude = %f, want 1.0", mag)
	}

	// Zero vector should remain zero.
	zero := []float32{0, 0, 0}
	normalizeL2(zero)
	for i, v := range zero {
		if v != 0 {
			t.Errorf("zero[%d] = %f after normalizeL2, want 0", i, v)
		}
	}
}
