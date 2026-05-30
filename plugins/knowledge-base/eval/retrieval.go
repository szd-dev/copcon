package kbeval

type RetrievalTestCase struct {
	Query          string
	RelevantDocIDs []string
}

type Retriever func(query string, k int) []string

type QueryScore struct {
	Query        string
	RecallAtK    map[int]float64
	PrecisionAtK map[int]float64
	MRR          float64
	NDCGAtK      map[int]float64
	HitRateAtK   map[int]float64
	FirstHitRank int
}

type RetrievalResult struct {
	NumQueries       int
	KValues          []int
	MeanRecallAtK    map[int]float64
	MeanPrecisionAtK map[int]float64
	MeanMRR          float64
	MeanNDCGAtK      map[int]float64
	MeanHitRateAtK   map[int]float64
	QueryBreakdown   []QueryScore
}

func EvaluateRetrieval(testCases []RetrievalTestCase, retriever Retriever, kValues []int) RetrievalResult {
	if len(testCases) == 0 || len(kValues) == 0 {
		return RetrievalResult{
			KValues:          kValues,
			MeanRecallAtK:    make(map[int]float64),
			MeanPrecisionAtK: make(map[int]float64),
			MeanNDCGAtK:      make(map[int]float64),
			MeanHitRateAtK:   make(map[int]float64),
		}
	}

	maxK := kValues[0]
	for _, k := range kValues[1:] {
		if k > maxK {
			maxK = k
		}
	}

	sumRecallAtK := make(map[int]float64)
	sumPrecisionAtK := make(map[int]float64)
	sumNDCGAtK := make(map[int]float64)
	sumHitRateAtK := make(map[int]float64)
	var sumMRR float64

	breakdown := make([]QueryScore, 0, len(testCases))

	for _, tc := range testCases {
		relevantSet := make(map[string]bool, len(tc.RelevantDocIDs))
		for _, id := range tc.RelevantDocIDs {
			relevantSet[id] = true
		}

		retrieved := retriever(tc.Query, maxK)
		numRelevant := len(relevantSet)

		qs := QueryScore{
			Query:        tc.Query,
			RecallAtK:    make(map[int]float64, len(kValues)),
			PrecisionAtK: make(map[int]float64, len(kValues)),
			NDCGAtK:      make(map[int]float64, len(kValues)),
			HitRateAtK:   make(map[int]float64, len(kValues)),
			MRR:          mrr(retrieved, relevantSet),
			FirstHitRank: firstHitRank(retrieved, relevantSet),
		}

		for _, k := range kValues {
			r := recallAtK(retrieved, relevantSet, numRelevant, k)
			p := precisionAtK(retrieved, relevantSet, k)
			n := ndcgAtK(retrieved, relevantSet, numRelevant, k)
			h := hitRateAtK(retrieved, relevantSet, k)

			qs.RecallAtK[k] = r
			qs.PrecisionAtK[k] = p
			qs.NDCGAtK[k] = n
			qs.HitRateAtK[k] = h

			sumRecallAtK[k] += r
			sumPrecisionAtK[k] += p
			sumNDCGAtK[k] += n
			sumHitRateAtK[k] += h
		}

		sumMRR += qs.MRR
		breakdown = append(breakdown, qs)
	}

	n := float64(len(testCases))
	meanRecallAtK := make(map[int]float64, len(kValues))
	meanPrecisionAtK := make(map[int]float64, len(kValues))
	meanNDCGAtK := make(map[int]float64, len(kValues))
	meanHitRateAtK := make(map[int]float64, len(kValues))

	for _, k := range kValues {
		meanRecallAtK[k] = sumRecallAtK[k] / n
		meanPrecisionAtK[k] = sumPrecisionAtK[k] / n
		meanNDCGAtK[k] = sumNDCGAtK[k] / n
		meanHitRateAtK[k] = sumHitRateAtK[k] / n
	}

	return RetrievalResult{
		NumQueries:       len(testCases),
		KValues:          kValues,
		MeanRecallAtK:    meanRecallAtK,
		MeanPrecisionAtK: meanPrecisionAtK,
		MeanMRR:          sumMRR / n,
		MeanNDCGAtK:      meanNDCGAtK,
		MeanHitRateAtK:   meanHitRateAtK,
		QueryBreakdown:   breakdown,
	}
}
