package server

import (
	"encoding/json"
	"fmt"

	"github.com/wethinkt/go-thinkt/internal/indexer/rpc"
	"github.com/wethinkt/go-thinkt/internal/indexer/search"
)

var errIndexerUnavailable = fmt.Errorf("indexer server is not running (start it with 'thinkt-indexer serve')")

// indexerSearch calls the indexer RPC search method.
func indexerSearch(params rpc.SearchParams) ([]search.SessionResult, int, error) {
	if !rpc.ServerAvailable() {
		return nil, 0, errIndexerUnavailable
	}
	resp, err := rpc.Call(rpc.MethodSearch, params, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("rpc search: %w", err)
	}
	if !resp.OK {
		return nil, 0, fmt.Errorf("rpc search: %s", resp.Error)
	}
	var data rpc.SearchData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, 0, fmt.Errorf("unmarshal search response: %w", err)
	}
	if data.Results == nil {
		data.Results = []search.SessionResult{}
	}
	return data.Results, data.TotalMatches, nil
}

// indexerSemanticSearch calls the indexer RPC semantic_search method.
func indexerSemanticSearch(params rpc.SemanticSearchParams) ([]search.SemanticResult, error) {
	if !rpc.ServerAvailable() {
		return nil, errIndexerUnavailable
	}
	resp, err := rpc.Call(rpc.MethodSemanticSearch, params, nil)
	if err != nil {
		return nil, fmt.Errorf("rpc semantic search: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("rpc semantic search: %s", resp.Error)
	}
	var data rpc.SemanticSearchData
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("unmarshal semantic search response: %w", err)
	}
	if data.Results == nil {
		data.Results = []search.SemanticResult{}
	}
	return data.Results, nil
}

// indexerStats calls the indexer RPC stats method and returns raw JSON.
func indexerStats() (json.RawMessage, error) {
	if !rpc.ServerAvailable() {
		return nil, errIndexerUnavailable
	}
	resp, err := rpc.Call(rpc.MethodStats, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("rpc stats: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("rpc stats: %s", resp.Error)
	}
	return resp.Data, nil
}
