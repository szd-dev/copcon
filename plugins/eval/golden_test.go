package eval

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type goldenEntry struct {
	Query        string   `json:"query"`
	RelevantDocs []string `json:"relevant_docs"`
	Category     string   `json:"category"`
}

func loadGoldenSet(path string) []RetrievalTestCase {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("open golden set: %v", err))
	}
	defer f.Close()

	var cases []RetrievalTestCase
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry goldenEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			panic(fmt.Sprintf("parse golden entry: %v", err))
		}
		cases = append(cases, RetrievalTestCase{
			Query:          entry.Query,
			RelevantDocIDs: entry.RelevantDocs,
		})
	}
	if err := scanner.Err(); err != nil {
		panic(fmt.Sprintf("scan golden set: %v", err))
	}
	return cases
}

func buildKeywordIndex(fixturesDir string) (map[string]string, []string, error) {
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read fixtures dir: %w", err)
	}

	index := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixturesDir, entry.Name()))
		if err != nil {
			return nil, nil, fmt.Errorf("read fixture %s: %w", entry.Name(), err)
		}
		index[entry.Name()] = string(data)
	}

	filenames := make([]string, 0, len(index))
	for name := range index {
		filenames = append(filenames, name)
	}
	sort.Strings(filenames)

	return index, filenames, nil
}

func tokenizeForMatching(s string) []string {
	seen := make(map[string]bool)
	var tokens []string

	add := func(t string) {
		if !seen[t] && t != "" {
			seen[t] = true
			tokens = append(tokens, t)
		}
	}

	s = strings.ToLower(s)
	for _, word := range strings.Fields(s) {
		add(word)
	}

	runes := []rune(s)
	for i := 0; i < len(runes)-1; i++ {
		if runes[i] != ' ' && runes[i+1] != ' ' {
			add(string(runes[i]) + string(runes[i+1]))
		}
	}

	return tokens
}

func keywordRetriever(query string, k int, index map[string]string, filenames []string) []string {
	tokens := tokenizeForMatching(query)
	if len(tokens) == 0 {
		return nil
	}

	type scoredDoc struct {
		name  string
		score int
	}

	var scored []scoredDoc
	for _, name := range filenames {
		content, ok := index[name]
		if !ok {
			continue
		}
		contentLower := strings.ToLower(content)
		score := 0
		for _, tok := range tokens {
			if strings.Contains(contentLower, tok) {
				score++
			}
		}
		scored = append(scored, scoredDoc{name, score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].name < scored[j].name
	})

	if k > len(scored) {
		k = len(scored)
	}
	result := make([]string, k)
	for i := 0; i < k; i++ {
		result[i] = scored[i].name
	}
	return result
}

func TestGoldenEval(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "eval", "testdata", "golden_set.jsonl")
	fixturesDir := filepath.Join("..", "..", "eval", "testdata", "fixtures")

	testCases := loadGoldenSet(goldenPath)
	require.NotEmpty(t, testCases, "golden set must not be empty")

	index, filenames, err := buildKeywordIndex(fixturesDir)
	require.NoError(t, err)
	require.NotEmpty(t, index, "fixture index must not be empty")

	retriever := func(query string, k int) []string {
		return keywordRetriever(query, k, index, filenames)
	}

	kValues := []int{3, 5, 10}
	result := EvaluateRetrieval(testCases, retriever, kValues)

	jsonPath := "/tmp/retrieval_eval.json"
	require.NoError(t, WriteJSON(result, jsonPath))

	PrintSummary(result, os.Stdout)

	var nonAdversarialCases []RetrievalTestCase
	for _, tc := range testCases {
		if len(tc.RelevantDocIDs) > 0 {
			nonAdversarialCases = append(nonAdversarialCases, tc)
		}
	}
	nonAdvResult := EvaluateRetrieval(nonAdversarialCases, retriever, kValues)

	t.Logf("Non-adversarial queries: %d / %d", len(nonAdversarialCases), len(testCases))
	t.Logf("Recall@5 (non-adversarial): %.4f", nonAdvResult.MeanRecallAtK[5])
	t.Logf("MRR (non-adversarial): %.4f", nonAdvResult.MeanMRR)

	assert.GreaterOrEqual(t, nonAdvResult.MeanRecallAtK[5], 0.50,
		"Recall@5 should be >= 0.50 for keyword retriever")
	assert.GreaterOrEqual(t, nonAdvResult.MeanMRR, 0.40,
		"MRR should be >= 0.40 for keyword retriever")
}

func TestGoldenEval_PerfectMock(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "eval", "testdata", "golden_set.jsonl")
	testCases := loadGoldenSet(goldenPath)
	require.NotEmpty(t, testCases, "golden set must not be empty")

	queryMap := make(map[string][]string, len(testCases))
	for _, tc := range testCases {
		queryMap[tc.Query] = tc.RelevantDocIDs
	}

	retriever := func(query string, k int) []string {
		docs := queryMap[query]
		if k < len(docs) {
			return docs[:k]
		}
		return docs
	}

	kValues := []int{3, 5, 10}
	result := EvaluateRetrieval(testCases, retriever, kValues)

	var nonAdversarialCases []RetrievalTestCase
	for _, tc := range testCases {
		if len(tc.RelevantDocIDs) > 0 {
			nonAdversarialCases = append(nonAdversarialCases, tc)
		}
	}
	nonAdvResult := EvaluateRetrieval(nonAdversarialCases, retriever, kValues)

	t.Logf("Perfect mock — Recall@5: %.4f", nonAdvResult.MeanRecallAtK[5])
	t.Logf("Perfect mock — MRR: %.4f", nonAdvResult.MeanMRR)

	assert.InDelta(t, 1.0, nonAdvResult.MeanRecallAtK[5], 1e-9,
		"Perfect retriever Recall@5 should be 1.0")
	assert.InDelta(t, 1.0, nonAdvResult.MeanMRR, 1e-9,
		"Perfect retriever MRR should be 1.0")

	assert.Greater(t, result.NumQueries, 0)
	t.Logf("Total queries (including adversarial): %d", result.NumQueries)
	PrintSummary(result, os.Stdout)
}