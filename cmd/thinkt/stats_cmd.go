package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wethinkt/go-thinkt/internal/analytics"
	"github.com/wethinkt/go-thinkt/internal/sources/claude"
)

// Search and stats command flags
var (
	searchProject string
	searchLimit   int
	statsProject  string
	statsLimit    int
	statsDays     int
	outputJSON    bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search across sessions using DuckDB",
	Long: `Full-text search across all Claude Code sessions.

Uses DuckDB to efficiently search through JSONL session files.
Searches in user messages and assistant responses.

Examples:
  thinkt search "authentication"
  thinkt search -p ./myproject "error handling"
  thinkt search --limit 100 "database"
  thinkt search --json "api" | jq .`,
	Args: cobra.ExactArgs(1),
	RunE: runSearch,
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Analytics and statistics using DuckDB",
	Long: `Analyze Claude Code sessions using DuckDB.

Provides various analytics including token usage, tool frequency,
word frequency, activity timelines, and more.

Examples:
  thinkt stats tokens
  thinkt stats tools -p ./myproject
  thinkt stats words --limit 100
  thinkt stats activity --days 7`,
}

var statsTokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Token usage by session",
	RunE:  runStatsTokens,
}

var statsToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Tool usage frequency",
	RunE:  runStatsTools,
}

var statsWordsCmd = &cobra.Command{
	Use:   "words",
	Short: "Word frequency in user prompts",
	RunE:  runStatsWords,
}

var statsActivityCmd = &cobra.Command{
	Use:   "activity",
	Short: "Daily activity timeline",
	RunE:  runStatsActivity,
}

var statsModelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Model usage statistics",
	RunE:  runStatsModels,
}

var statsErrorsCmd = &cobra.Command{
	Use:   "errors",
	Short: "Tool errors and failures",
	RunE:  runStatsErrors,
}

var queryCmd = &cobra.Command{
	Use:   "query <sql>",
	Short: "Run raw SQL query using DuckDB",
	Long: `Execute a raw SQL query against session data.

DuckDB can read JSONL files directly using read_json_auto().
The base directory pattern is available as a placeholder.

Examples:
  thinkt query "SELECT COUNT(*) FROM read_json_auto('~/.claude/projects/*/*.jsonl')"
  thinkt query "SELECT DISTINCT json_extract_string(entry, '$.model') FROM read_json_auto('~/.claude/projects/*/*.jsonl')"`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

// getAnalyticsBaseDir returns the base directory for analytics queries.
// TODO: Update analytics to support multi-source
func getAnalyticsBaseDir() (string, error) {
	return claude.DefaultDir()
}

func runSearch(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	results, err := engine.Search(context.Background(), args[0], searchProject, searchLimit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(results)
	}

	if len(results) == 0 {
		fmt.Println("No results found")
		return nil
	}

	for _, r := range results {
		// Truncate content for display
		content := r.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")

		fmt.Printf("[%s] %s\n", r.EntryType, r.SessionPath)
		fmt.Printf("  %s\n\n", content)
	}

	return nil
}

func runStatsTokens(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	stats, err := engine.GetTokenStats(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get token stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	if len(stats) == 0 {
		fmt.Println("No token data found")
		return nil
	}

	fmt.Printf("%-40s %10s %10s %10s %10s\n", "SESSION", "INPUT", "OUTPUT", "CACHE", "TOTAL")
	fmt.Println(strings.Repeat("-", 84))
	for _, s := range stats {
		sessionID := s.SessionID
		if len(sessionID) > 38 {
			sessionID = sessionID[:38] + ".."
		}
		fmt.Printf("%-40s %10d %10d %10d %10d\n",
			sessionID, s.InputTokens, s.OutputTokens, s.CacheTokens, s.TotalTokens)
	}

	return nil
}

func runStatsTools(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	stats, err := engine.GetToolStats(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get tool stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(stats)
	}

	if len(stats) == 0 {
		fmt.Println("No tool usage data found")
		return nil
	}

	fmt.Printf("%-30s %10s\n", "TOOL", "COUNT")
	fmt.Println(strings.Repeat("-", 42))
	for _, t := range stats {
		fmt.Printf("%-30s %10d\n", t.ToolName, t.UsageCount)
	}

	return nil
}

func runStatsWords(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	words, err := engine.GetWordFrequency(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get word frequency: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(words)
	}

	if len(words) == 0 {
		fmt.Println("No word data found")
		return nil
	}

	fmt.Printf("%-20s %10s\n", "WORD", "COUNT")
	fmt.Println(strings.Repeat("-", 32))
	for _, w := range words {
		fmt.Printf("%-20s %10d\n", w.Word, w.Count)
	}

	return nil
}

func runStatsActivity(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	activity, err := engine.GetActivity(context.Background(), statsProject, statsDays)
	if err != nil {
		return fmt.Errorf("get activity: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(activity)
	}

	if len(activity) == 0 {
		fmt.Println("No activity data found")
		return nil
	}

	fmt.Printf("%-12s %10s %10s\n", "DATE", "SESSIONS", "MESSAGES")
	fmt.Println(strings.Repeat("-", 34))
	for _, a := range activity {
		fmt.Printf("%-12s %10d %10d\n", a.Date.Format("2006-01-02"), a.Sessions, a.Messages)
	}

	return nil
}

func runStatsModels(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	models, err := engine.GetModelStats(context.Background(), statsProject)
	if err != nil {
		return fmt.Errorf("get model stats: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(models)
	}

	if len(models) == 0 {
		fmt.Println("No model data found")
		return nil
	}

	fmt.Printf("%-40s %10s %15s\n", "MODEL", "RESPONSES", "AVG OUTPUT")
	fmt.Println(strings.Repeat("-", 67))
	for _, m := range models {
		model := m.Model
		if len(model) > 38 {
			model = model[:38] + ".."
		}
		fmt.Printf("%-40s %10d %15.0f\n", model, m.Responses, m.AvgOutputTokens)
	}

	return nil
}

func runStatsErrors(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	errors, err := engine.GetErrors(context.Background(), statsProject, statsLimit)
	if err != nil {
		return fmt.Errorf("get errors: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(errors)
	}

	if len(errors) == 0 {
		fmt.Println("No errors found")
		return nil
	}

	for _, e := range errors {
		content := e.Content
		if len(content) > 100 {
			content = content[:100] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")

		fmt.Printf("[%s] %s\n", e.ToolName, e.Timestamp.Format("2006-01-02 15:04"))
		fmt.Printf("  %s\n", e.SessionPath)
		fmt.Printf("  %s\n\n", content)
	}

	return nil
}

func runQuery(cmd *cobra.Command, args []string) error {
	dir, err := getAnalyticsBaseDir()
	if err != nil {
		return fmt.Errorf("get base dir: %w", err)
	}

	engine, err := analytics.NewEngine(dir)
	if err != nil {
		return fmt.Errorf("create analytics engine: %w", err)
	}
	defer engine.Close()

	results, err := engine.Query(context.Background(), args[0])
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(results)
	}

	if len(results) == 0 {
		fmt.Println("No results")
		return nil
	}

	// Print as JSON by default for raw queries (easier to read)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}
