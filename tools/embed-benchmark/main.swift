import Foundation
import NaturalLanguage

let samples = [
    "debugging the authentication timeout in the login flow",
    "refactoring the database connection pool to use lazy initialization",
    "how do I add semantic search to my CLI tool using Apple Intelligence",
    "fix the race condition in the file watcher goroutine",
    "the user reported that search results are empty when filtering by project",
]

// Find embedding model - try script-based first, then language
let embedding: NLContextualEmbedding? =
    NLContextualEmbedding(script: .latin) ??
    NLContextualEmbedding(language: .english)

guard let embedding = embedding else {
    print("ERROR: No contextual embedding model found")
    exit(1)
}

print("Found model: \(embedding.modelIdentifier)")
print("Dimension: \(embedding.dimension)")
print("Has assets: \(embedding.hasAvailableAssets)")
print()

if !embedding.hasAvailableAssets {
    print("Assets not downloaded. Requesting...")
    embedding.requestAssets { result, error in
        if let error = error {
            print("ERROR downloading assets: \(error)")
            exit(1)
        }
        print("Assets downloaded: \(result)")
        benchmark(embedding: embedding)
        exit(0)
    }
    RunLoop.main.run()
} else {
    benchmark(embedding: embedding)
}

func benchmark(embedding: NLContextualEmbedding) {
    // Measure cold load
    let loadStart = CFAbsoluteTimeGetCurrent()
    do {
        try embedding.load()
    } catch {
        print("ERROR loading model: \(error)")
        exit(1)
    }
    let loadTime = CFAbsoluteTimeGetCurrent() - loadStart
    print(String(format: "Cold load time: %.3f seconds", loadTime))

    // Measure individual embeddings
    for (i, text) in samples.enumerated() {
        let start = CFAbsoluteTimeGetCurrent()
        guard let result = try? embedding.embeddingResult(for: text, language: .english) else {
            print("ERROR: Failed to embed sample \(i)")
            continue
        }
        let elapsed = CFAbsoluteTimeGetCurrent() - start

        // Get the embedding vector for the full string
        var vector = [Double](repeating: 0, count: embedding.dimension)
        result.enumerateTokenVectors(in: text.startIndex..<text.endIndex) { tokenVector, tokenRange in
            for (j, v) in tokenVector.enumerated() {
                vector[j] += v / Double(text.count)
            }
            return true
        }

        print(String(format: "  Sample %d: %.1f ms  (len=%d, dim=%d)", i, elapsed * 1000, text.count, embedding.dimension))
    }

    // Measure batch throughput
    let batchStart = CFAbsoluteTimeGetCurrent()
    let batchSize = 100
    for i in 0..<batchSize {
        let text = samples[i % samples.count]
        _ = try? embedding.embeddingResult(for: text, language: .english)
    }
    let batchTime = CFAbsoluteTimeGetCurrent() - batchStart
    print(String(format: "\nBatch of %d: %.3f seconds (%.1f ms/item)", batchSize, batchTime, batchTime * 1000 / Double(batchSize)))

    // Measure unload + reload
    embedding.unload()
    let reloadStart = CFAbsoluteTimeGetCurrent()
    try? embedding.load()
    let reloadTime = CFAbsoluteTimeGetCurrent() - reloadStart
    print(String(format: "Warm reload time: %.3f seconds", reloadTime))
}
