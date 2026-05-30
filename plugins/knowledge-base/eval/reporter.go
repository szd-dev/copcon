package kbeval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func PrintSummary(result RetrievalResult, w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "RETRIEVAL EVALUATION SUMMARY")
	fmt.Fprintf(tw, "Queries:\t%d\n", result.NumQueries)
	fmt.Fprintln(tw, "")

	fmt.Fprintln(tw, "Metric\tK=3\tK=5\tK=10")
	for _, k := range result.KValues {
		fmt.Fprintf(tw, "Recall@%d\t%.4f\t\n", k, result.MeanRecallAtK[k])
	}
	for _, k := range result.KValues {
		fmt.Fprintf(tw, "Precision@%d\t%.4f\t\n", k, result.MeanPrecisionAtK[k])
	}
	fmt.Fprintf(tw, "MRR\t%.4f\t\n", result.MeanMRR)
	for _, k := range result.KValues {
		fmt.Fprintf(tw, "nDCG@%d\t%.4f\t\n", k, result.MeanNDCGAtK[k])
	}
	for _, k := range result.KValues {
		fmt.Fprintf(tw, "HitRate@%d\t%.4f\t\n", k, result.MeanHitRateAtK[k])
	}

	tw.Flush()

	if len(result.QueryBreakdown) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "PER-QUERY BREAKDOWN")
		fmt.Fprintln(w, "")

		qtw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(qtw, "Query\tRecall@5\tPrecision@5\tMRR\tnDCG@5\tFirstHit")
		for _, qs := range result.QueryBreakdown {
			r5 := qs.RecallAtK[5]
			p5 := qs.PrecisionAtK[5]
			n5 := qs.NDCGAtK[5]
			fmt.Fprintf(qtw, "%s\t%.4f\t%.4f\t%.4f\t%.4f\t%d\n",
				qs.Query, r5, p5, qs.MRR, n5, qs.FirstHitRank)
		}
		qtw.Flush()
	}
}

func WriteJSON(result RetrievalResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal retrieval result: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write json file: %w", err)
	}
	return nil
}