import Foundation
import NaturalLanguage

struct EmbedRequest: Decodable {
    let id: String
    let text: String
}

struct EmbedResponse: Encodable {
    let id: String
    let embedding: [Float]
    let dim: Int
}

// Find model
guard let model = NLContextualEmbedding(script: .latin) ??
                  NLContextualEmbedding(language: .english) else {
    fputs("ERROR: No contextual embedding model found\n", stderr)
    exit(1)
}

// Download assets if needed
if !model.hasAvailableAssets {
    fputs("Downloading embedding model assets...\n", stderr)
    let sem = DispatchSemaphore(value: 0)
    model.requestAssets { _, error in
        if let error = error {
            fputs("ERROR: Failed to download assets: \(error)\n", stderr)
            exit(1)
        }
        sem.signal()
    }
    sem.wait()
}

// Load model once
do {
    try model.load()
} catch {
    fputs("ERROR: Failed to load model: \(error)\n", stderr)
    exit(1)
}

let encoder = JSONEncoder()
let decoder = JSONDecoder()

// Process stdin line by line
while let line = readLine() {
    guard !line.isEmpty else { continue }

    guard let data = line.data(using: .utf8),
          let req = try? decoder.decode(EmbedRequest.self, from: data) else {
        fputs("WARN: Invalid JSON input, skipping\n", stderr)
        continue
    }

    guard let result = try? model.embeddingResult(for: req.text, language: .english) else {
        fputs("WARN: Failed to embed id=\(req.id), skipping\n", stderr)
        continue
    }

    // Average token vectors into a single sentence embedding
    var vector = [Float](repeating: 0, count: model.dimension)
    var tokenCount = 0
    result.enumerateTokenVectors(in: req.text.startIndex..<req.text.endIndex) { tokenVector, _ in
        for (j, v) in tokenVector.enumerated() {
            vector[j] += Float(v)
        }
        tokenCount += 1
        return true
    }
    if tokenCount > 0 {
        for j in 0..<vector.count {
            vector[j] /= Float(tokenCount)
        }
    }

    let resp = EmbedResponse(id: req.id, embedding: vector, dim: model.dimension)
    if let jsonData = try? encoder.encode(resp),
       let jsonStr = String(data: jsonData, encoding: .utf8) {
        print(jsonStr)
        fflush(stdout)
    }
}

model.unload()
