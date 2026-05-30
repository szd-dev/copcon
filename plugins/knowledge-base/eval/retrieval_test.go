package kbeval

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultKValues = []int{3, 5, 10}

func TestPerfectRetrieval(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "golang concurrency", RelevantDocIDs: []string{"doc1", "doc2", "doc3"}},
		{Query: "python async", RelevantDocIDs: []string{"doc4", "doc5"}},
	}

	retriever := func(query string, k int) []string {
		switch query {
		case "golang concurrency":
			return []string{"doc1", "doc2", "doc3"}
		case "python async":
			return []string{"doc4", "doc5"}
		default:
			return nil
		}
	}

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	assert.Equal(t, 2, result.NumQueries)
	assert.Equal(t, defaultKValues, result.KValues)

	for _, k := range defaultKValues {
		assert.InDelta(t, 1.0, result.MeanRecallAtK[k], 1e-9, "Recall@%d", k)
		assert.InDelta(t, 1.0, result.MeanNDCGAtK[k], 1e-9, "nDCG@%d", k)
		assert.InDelta(t, 1.0, result.MeanHitRateAtK[k], 1e-9, "HitRate@%d", k)
	}
	assert.InDelta(t, 1.0, result.MeanMRR, 1e-9)

	assert.Equal(t, 2, len(result.QueryBreakdown))
	for _, qs := range result.QueryBreakdown {
		assert.InDelta(t, 1.0, qs.MRR, 1e-9, "query %s MRR", qs.Query)
		assert.Equal(t, 1, qs.FirstHitRank, "query %s FirstHitRank", qs.Query)
	}
}

func TestEmptyRetrieval(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "golang concurrency", RelevantDocIDs: []string{"doc1", "doc2"}},
		{Query: "python async", RelevantDocIDs: []string{"doc3"}},
	}

	retriever := func(_ string, _ int) []string {
		return nil
	}

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	assert.Equal(t, 2, result.NumQueries)

	for _, k := range defaultKValues {
		assert.InDelta(t, 0.0, result.MeanRecallAtK[k], 1e-9, "Recall@%d", k)
		assert.InDelta(t, 0.0, result.MeanPrecisionAtK[k], 1e-9, "Precision@%d", k)
		assert.InDelta(t, 0.0, result.MeanNDCGAtK[k], 1e-9, "nDCG@%d", k)
		assert.InDelta(t, 0.0, result.MeanHitRateAtK[k], 1e-9, "HitRate@%d", k)
	}
	assert.InDelta(t, 0.0, result.MeanMRR, 1e-9)

	for _, qs := range result.QueryBreakdown {
		assert.InDelta(t, 0.0, qs.MRR, 1e-9)
		assert.Equal(t, 0, qs.FirstHitRank)
	}
}

func TestPartialRetrieval(t *testing.T) {
	// Hand-calculated expected values:
	//
	// Query 1: "distributed systems"
	//   Relevant: {doc1, doc2, doc3, doc4}
	//   Retrieved: [doc1, doc5, doc3, doc6, doc7, doc8, doc2, doc9, doc10]
	//
	//   Hits@3: doc1, doc3 → 2 hits
	//   Hits@5: doc1, doc3 → 2 hits
	//   Hits@10: doc1, doc3, doc2 → 3 hits
	//
	//   Recall@3  = 2/4 = 0.5
	//   Recall@5  = 2/4 = 0.5
	//   Recall@10 = 3/4 = 0.75
	//
	//   Precision@3  = 2/3 ≈ 0.6667
	//   Precision@5  = 2/5 = 0.4
	//   Precision@10 = 3/10 = 0.3
	//
	//   MRR = 1/1 = 1.0 (doc1 at rank 1)
	//   FirstHitRank = 1
	//
	//   nDCG@3:
	//     DCG = 1/log2(2) + 0 + 1/log2(4) = 1/1 + 1/2 = 1.5
	//     IDCG = 1/log2(2) + 1/log2(3) + 1/log2(4) = 1 + 0.6309 + 0.5 = 2.1309
	//     nDCG = 1.5 / 2.1309 ≈ 0.7040
	//
	//   nDCG@5:
	//     DCG = 1/log2(2) + 0 + 1/log2(4) + 0 + 0 = 1.5
	//     IDCG = 1/log2(2) + 1/log2(3) + 1/log2(4) + 1/log2(5) = 2.562
	//     nDCG = 1.5 / 2.562 ≈ 0.5856
	//
	//   nDCG@10:
	//     DCG = 1/log2(2) + 0 + 1/log2(4) + 0 + 0 + 0 + 1/log2(8) + 0 + 0 = 1 + 0.5 + 1/3 ≈ 1.8333
	//     IDCG = same as @5 = 2.562 (only 4 relevant docs)
	//     nDCG = 1.8333 / 2.562 ≈ 0.7153
	//
	//   HitRate@3 = 1, HitRate@5 = 1, HitRate@10 = 1
	//
	// Query 2: "machine learning"
	//   Relevant: {doc4, doc5}
	//   Retrieved: [doc6, doc7, doc4, doc8, doc9]
	//
	//   Hits@3: doc4 → 1 hit
	//   Hits@5: doc4 → 1 hit
	//   Hits@10: doc4 → 1 hit (only 5 retrieved)
	//
	//   Recall@3  = 1/2 = 0.5
	//   Recall@5  = 1/2 = 0.5
	//   Recall@10 = 1/2 = 0.5
	//
	//   Precision@3  = 1/3 ≈ 0.3333
	//   Precision@5  = 1/5 = 0.2
	//   Precision@10 = 1/10 = 0.1
	//
	//   MRR = 1/3 ≈ 0.3333 (doc4 at rank 3)
	//   FirstHitRank = 3
	//
	//   nDCG@3:
	//     DCG = 0 + 0 + 1/log2(4) = 0.5
	//     IDCG = 1/log2(2) + 1/log2(3) = 1 + 0.6309 = 1.6309
	//     nDCG = 0.5 / 1.6309 ≈ 0.3066
	//
	//   nDCG@5:
	//     DCG = 0 + 0 + 1/log2(4) + 0 + 0 = 0.5
	//     IDCG = same = 1.6309
	//     nDCG = 0.5 / 1.6309 ≈ 0.3066
	//
	//   nDCG@10:
	//     DCG = same = 0.5 (only 5 docs retrieved, doc4 at rank 3)
	//     IDCG = same = 1.6309
	//     nDCG = 0.5 / 1.6309 ≈ 0.3066
	//
	//   HitRate@3 = 1, HitRate@5 = 1, HitRate@10 = 1
	//
	// Mean values (2 queries):
	//   MeanRecall@3  = (0.5 + 0.5) / 2 = 0.5
	//   MeanRecall@5  = (0.5 + 0.5) / 2 = 0.5
	//   MeanRecall@10 = (0.75 + 0.5) / 2 = 0.625
	//   MeanPrecision@3  = (0.6667 + 0.3333) / 2 = 0.5
	//   MeanPrecision@5  = (0.4 + 0.2) / 2 = 0.3
	//   MeanPrecision@10 = (0.3 + 0.1) / 2 = 0.2
	//   MeanMRR = (1.0 + 0.3333) / 2 ≈ 0.6667
	//   MeanHitRate all K = 1.0

	cases := []RetrievalTestCase{
		{Query: "distributed systems", RelevantDocIDs: []string{"doc1", "doc2", "doc3", "doc4"}},
		{Query: "machine learning", RelevantDocIDs: []string{"doc4", "doc5"}},
	}

	retriever := func(query string, k int) []string {
		switch query {
		case "distributed systems":
			return []string{"doc1", "doc5", "doc3", "doc6", "doc7", "doc8", "doc2", "doc9", "doc10"}
		case "machine learning":
			return []string{"doc6", "doc7", "doc4", "doc8", "doc9"}
		default:
			return nil
		}
	}

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	assert.Equal(t, 2, result.NumQueries)

	// Recall
	assert.InDelta(t, 0.5, result.MeanRecallAtK[3], 1e-4)
	assert.InDelta(t, 0.5, result.MeanRecallAtK[5], 1e-4)
	assert.InDelta(t, 0.625, result.MeanRecallAtK[10], 1e-4)

	// Precision
	assert.InDelta(t, 0.5, result.MeanPrecisionAtK[3], 1e-4)
	assert.InDelta(t, 0.3, result.MeanPrecisionAtK[5], 1e-4)
	assert.InDelta(t, 0.2, result.MeanPrecisionAtK[10], 1e-4)

	// MRR
	assert.InDelta(t, 2.0/3.0, result.MeanMRR, 1e-4)

	// nDCG — hand-calculated values
	// Query 1 has 4 relevant docs, so IDCG uses min(K, 4) terms.
	idcg4at3 := 1.0/math.Log2(2) + 1.0/math.Log2(3) + 1.0/math.Log2(4)
	idcg4at5 := idcg4at3 + 1.0/math.Log2(5)
	idcg4at10 := idcg4at5
	dcg4at3 := 1.0/math.Log2(2) + 1.0/math.Log2(4)
	dcg4at5 := dcg4at3
	dcg4at10 := dcg4at3 + 1.0/math.Log2(8)
	ndcg4at3 := dcg4at3 / idcg4at3
	ndcg4at5 := dcg4at5 / idcg4at5
	ndcg4at10 := dcg4at10 / idcg4at10

	// Query 2 has 2 relevant docs, so IDCG uses min(K, 2) terms.
	idcg2 := 1.0/math.Log2(2) + 1.0/math.Log2(3)
	dcg2 := 1.0 / math.Log2(4)
	ndcg2 := dcg2 / idcg2

	assert.InDelta(t, (ndcg4at3+ndcg2)/2, result.MeanNDCGAtK[3], 1e-4)
	assert.InDelta(t, (ndcg4at5+ndcg2)/2, result.MeanNDCGAtK[5], 1e-4)
	assert.InDelta(t, (ndcg4at10+ndcg2)/2, result.MeanNDCGAtK[10], 1e-4)

	// HitRate
	for _, k := range defaultKValues {
		assert.InDelta(t, 1.0, result.MeanHitRateAtK[k], 1e-9, "HitRate@%d", k)
	}

	// Per-query breakdown
	require.Len(t, result.QueryBreakdown, 2)

	q1 := result.QueryBreakdown[0]
	assert.Equal(t, "distributed systems", q1.Query)
	assert.InDelta(t, 1.0, q1.MRR, 1e-9)
	assert.Equal(t, 1, q1.FirstHitRank)

	q2 := result.QueryBreakdown[1]
	assert.Equal(t, "machine learning", q2.Query)
	assert.InDelta(t, 1.0/3.0, q2.MRR, 1e-9)
	assert.Equal(t, 3, q2.FirstHitRank)
}

func TestNoRelevantDocuments(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "empty query", RelevantDocIDs: []string{}},
	}

	retriever := func(_ string, _ int) []string {
		return []string{"doc1", "doc2"}
	}

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	for _, k := range defaultKValues {
		assert.InDelta(t, 0.0, result.MeanRecallAtK[k], 1e-9)
		assert.InDelta(t, 0.0, result.MeanNDCGAtK[k], 1e-9)
	}
}

func TestEmptyTestCases(t *testing.T) {
	retriever := func(_ string, _ int) []string { return nil }
	result := EvaluateRetrieval(nil, retriever, defaultKValues)

	assert.Equal(t, 0, result.NumQueries)
	assert.Empty(t, result.QueryBreakdown)
}

func TestEmptyKValues(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1"}},
	}
	retriever := func(_ string, _ int) []string { return []string{"doc1"} }

	result := EvaluateRetrieval(cases, retriever, nil)
	assert.Equal(t, 0, result.NumQueries)
}

func TestSingleKValue(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1", "doc2"}},
	}
	retriever := func(_ string, _ int) []string { return []string{"doc1", "doc3"} }

	result := EvaluateRetrieval(cases, retriever, []int{5})

	assert.InDelta(t, 0.5, result.MeanRecallAtK[5], 1e-9)
	assert.InDelta(t, 0.2, result.MeanPrecisionAtK[5], 1e-9)
	assert.InDelta(t, 1.0, result.MeanMRR, 1e-9)
	assert.InDelta(t, 1.0, result.MeanHitRateAtK[5], 1e-9)
}

func TestRetrieverReturnsFewerThanK(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1", "doc2", "doc3", "doc4", "doc5"}},
	}

	retriever := func(_ string, _ int) []string {
		return []string{"doc1", "doc3"}
	}

	result := EvaluateRetrieval(cases, retriever, []int{10})

	assert.InDelta(t, 2.0/5.0, result.MeanRecallAtK[10], 1e-9)
	assert.InDelta(t, 2.0/10.0, result.MeanPrecisionAtK[10], 1e-9)
}

func TestDuplicateRelevantDocIDs(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1", "doc1", "doc2"}},
	}

	retriever := func(_ string, _ int) []string {
		return []string{"doc1", "doc2"}
	}

	result := EvaluateRetrieval(cases, retriever, []int{5})

	assert.InDelta(t, 1.0, result.MeanRecallAtK[5], 1e-9)
}

func TestDuplicateRetrievedDocIDs(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1", "doc2"}},
	}

	retriever := func(_ string, _ int) []string {
		return []string{"doc1", "doc1", "doc2"}
	}

	result := EvaluateRetrieval(cases, retriever, []int{3})

	assert.InDelta(t, 1.0, result.MeanRecallAtK[3], 1e-4)
	assert.InDelta(t, 2.0/3.0, result.MeanPrecisionAtK[3], 1e-4)
}

func TestPrintSummary(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "golang", RelevantDocIDs: []string{"doc1", "doc2"}},
		{Query: "python", RelevantDocIDs: []string{"doc3"}},
	}

	retriever := func(query string, _ int) []string {
		switch query {
		case "golang":
			return []string{"doc1", "doc2"}
		case "python":
			return []string{"doc3"}
		default:
			return nil
		}
	}

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	var buf bytes.Buffer
	PrintSummary(result, &buf)

	output := buf.String()
	assert.Contains(t, output, "RETRIEVAL EVALUATION SUMMARY")
	assert.Contains(t, output, "Queries:")
	assert.Contains(t, output, "PER-QUERY BREAKDOWN")
	assert.Contains(t, output, "golang")
	assert.Contains(t, output, "python")
}

func TestWriteJSON(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "golang", RelevantDocIDs: []string{"doc1"}},
	}

	retriever := func(_ string, _ int) []string { return []string{"doc1"} }
	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	dir := t.TempDir()
	path := filepath.Join(dir, "result.json")

	err := WriteJSON(result, path)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var decoded RetrievalResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, 1, decoded.NumQueries)
	assert.InDelta(t, 1.0, decoded.MeanMRR, 1e-9)
}

func TestWriteJSONInvalidPath(t *testing.T) {
	result := RetrievalResult{
		NumQueries:    0,
		KValues:       defaultKValues,
		MeanRecallAtK: make(map[int]float64),
	}

	err := WriteJSON(result, "/nonexistent/deep/nested/dir/result.json")
	assert.Error(t, err)
}

func TestMRRNoRelevantInRetrieved(t *testing.T) {
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1"}},
	}

	retriever := func(_ string, _ int) []string { return []string{"doc2", "doc3", "doc4"} }

	result := EvaluateRetrieval(cases, retriever, defaultKValues)

	assert.InDelta(t, 0.0, result.MeanMRR, 1e-9)
	assert.Equal(t, 0, result.QueryBreakdown[0].FirstHitRank)
}

func TestNDCGBinaryRelevance(t *testing.T) {
	// With binary relevance, nDCG should equal 1.0 when all relevant docs
	// appear before any irrelevant docs in the top-K positions.
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"doc1", "doc2"}},
	}

	retriever := func(_ string, _ int) []string { return []string{"doc1", "doc2", "doc3"} }

	result := EvaluateRetrieval(cases, retriever, []int{3})

	assert.InDelta(t, 1.0, result.MeanNDCGAtK[3], 1e-9)
}

func TestRecallAtKWithMoreRelevantThanK(t *testing.T) {
	// 5 relevant docs but K=3, only 2 found in top-3
	cases := []RetrievalTestCase{
		{Query: "q1", RelevantDocIDs: []string{"d1", "d2", "d3", "d4", "d5"}},
	}

	retriever := func(_ string, _ int) []string { return []string{"d1", "d6", "d2"} }

	result := EvaluateRetrieval(cases, retriever, []int{3})

	assert.InDelta(t, 2.0/5.0, result.MeanRecallAtK[3], 1e-9)
	assert.InDelta(t, 2.0/3.0, result.MeanPrecisionAtK[3], 1e-4)
}
