package eval

import "math"

func recallAtK(retrieved []string, relevantSet map[string]bool, numRelevant, k int) float64 {
	if numRelevant == 0 || k == 0 {
		return 0
	}

	hits := countHits(retrieved, relevantSet, k)
	return float64(hits) / float64(numRelevant)
}

func precisionAtK(retrieved []string, relevantSet map[string]bool, k int) float64 {
	if k == 0 {
		return 0
	}

	hits := countHits(retrieved, relevantSet, k)
	return float64(hits) / float64(k)
}

func hitRateAtK(retrieved []string, relevantSet map[string]bool, k int) float64 {
	if countHits(retrieved, relevantSet, k) > 0 {
		return 1
	}
	return 0
}

func mrr(retrieved []string, relevantSet map[string]bool) float64 {
	for i, docID := range retrieved {
		if relevantSet[docID] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

func firstHitRank(retrieved []string, relevantSet map[string]bool) int {
	for i, docID := range retrieved {
		if relevantSet[docID] {
			return i + 1
		}
	}
	return 0
}

func ndcgAtK(retrieved []string, relevantSet map[string]bool, numRelevant, k int) float64 {
	if numRelevant == 0 || k == 0 {
		return 0
	}

	dcg := 0.0
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	for i := 0; i < limit; i++ {
		if relevantSet[retrieved[i]] {
			dcg += 1.0 / math.Log2(float64(i+2))
		}
	}

	idcg := 0.0
	idcgLimit := k
	if idcgLimit > numRelevant {
		idcgLimit = numRelevant
	}
	for i := 0; i < idcgLimit; i++ {
		idcg += 1.0 / math.Log2(float64(i+2))
	}

	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func countHits(retrieved []string, relevantSet map[string]bool, k int) int {
	limit := k
	if limit > len(retrieved) {
		limit = len(retrieved)
	}
	seen := make(map[string]bool, limit)
	hits := 0
	for i := 0; i < limit; i++ {
		if relevantSet[retrieved[i]] && !seen[retrieved[i]] {
			seen[retrieved[i]] = true
			hits++
		}
	}
	return hits
}
