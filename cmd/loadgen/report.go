package main

import (
	"fmt"
	"io"
	"sort"
	"time"
)

type Summary struct {
	Requests   int
	Planned    int
	OK         int
	Elapsed    time.Duration
	Throughput float64
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
	Max        time.Duration
	Causes     []Cause
}

type Cause struct {
	Label string
	Count int
}

func (s Summary) SuccessRate() float64 {
	if s.Requests == 0 {
		return 0
	}
	return float64(s.OK) / float64(s.Requests) * 100
}

func summarize(phase Phase) Summary {
	summary := Summary{
		Requests: len(phase.Results),
		Planned:  phase.Planned,
		Elapsed:  phase.Ended.Sub(phase.Started),
	}

	latencies := make([]time.Duration, 0, len(phase.Results))
	counted := map[string]int{}
	for _, result := range phase.Results {
		latencies = append(latencies, result.Latency)
		if result.OK() {
			summary.OK++
			continue
		}
		counted[cause(result)]++
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	summary.P50 = percentile(latencies, 50)
	summary.P95 = percentile(latencies, 95)
	summary.P99 = percentile(latencies, 99)
	summary.Max = percentile(latencies, 100)

	if summary.Elapsed > 0 {
		summary.Throughput = float64(summary.Requests) / summary.Elapsed.Seconds()
	}

	for label, count := range counted {
		summary.Causes = append(summary.Causes, Cause{Label: label, Count: count})
	}
	sort.Slice(summary.Causes, func(i, j int) bool {
		if summary.Causes[i].Count != summary.Causes[j].Count {
			return summary.Causes[i].Count > summary.Causes[j].Count
		}
		return summary.Causes[i].Label < summary.Causes[j].Label
	})
	return summary
}

func cause(result Result) string {
	switch {
	case result.Failure != "":
		return result.Failure
	case result.Partner != "":
		return fmt.Sprintf("HTTP %d from %s", result.Status, result.Partner)
	default:
		return fmt.Sprintf("HTTP %d", result.Status)
	}
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

type Report struct {
	Config   Config
	Baseline *Phase
	Load     Phase
}

func (r Report) Write(w io.Writer) {
	fmt.Fprintf(w, "\nloadgen · %s · tenant %s · %d distinct quotes\n",
		r.Config.Endpoint(), r.Config.Tenant, r.Config.Distinct)

	var baseline Summary
	if r.Baseline != nil {
		baseline = summarize(*r.Baseline)
		writePhase(w, *r.Baseline, baseline)
	}
	load := summarize(r.Load)
	writePhase(w, r.Load, load)

	if r.Baseline != nil && baseline.Requests > 0 && load.Requests > 0 {
		writeComparison(w, baseline, load)
	}
	writeEvidence(w, r.Load)
}

func writePhase(w io.Writer, phase Phase, summary Summary) {
	fmt.Fprintf(w, "\n%s — %d requests, %d in flight, %s to %s\n", phase.Name, summary.Planned,
		phase.Concurrency, clock(phase.Started), clock(phase.Ended))

	if summary.Requests == 0 {
		fmt.Fprintf(w, "  %-14s no request completed\n", "answered")
		return
	}
	if summary.Requests < summary.Planned {
		fmt.Fprintf(w, "  %-14s %d of %d (the run hit the -duration limit)\n", "answered",
			summary.Requests, summary.Planned)
	}

	fmt.Fprintf(w, "  %-14s %d of %d (%.0f%%)\n", "success", summary.OK, summary.Requests, summary.SuccessRate())
	fmt.Fprintf(w, "  %-14s p50 %s   p95 %s   p99 %s   max %s\n", "latency",
		short(summary.P50), short(summary.P95), short(summary.P99), short(summary.Max))
	fmt.Fprintf(w, "  %-14s %.1f req/s in %s\n", "throughput", summary.Throughput, short(summary.Elapsed))

	for i, cause := range summary.Causes {
		label := ""
		if i == 0 {
			label = "failures"
		}
		fmt.Fprintf(w, "  %-14s %s: %d\n", label, cause.Label, cause.Count)
	}
}

func writeComparison(w io.Writer, baseline, load Summary) {
	fmt.Fprintf(w, "\nbaseline → load\n")
	fmt.Fprintf(w, "  %-14s %s → %s%s\n", "p95 latency", short(baseline.P95), short(load.P95), times(baseline.P95, load.P95))
	fmt.Fprintf(w, "  %-14s %s → %s%s\n", "max latency", short(baseline.Max), short(load.Max), times(baseline.Max, load.Max))
	fmt.Fprintf(w, "  %-14s %.0f%% → %.0f%%\n", "success", baseline.SuccessRate(), load.SuccessRate())
	fmt.Fprintf(w, "  %-14s %.1f → %.1f req/s\n", "throughput", baseline.Throughput, load.Throughput)
}

func writeEvidence(w io.Writer, load Phase) {
	fmt.Fprintf(w, "\nwhere to look\n")
	fmt.Fprintf(w, "  %-14s http://localhost:16686 · service quotation-api · operation POST /quotes\n", "Jaeger")
	fmt.Fprintf(w, "  %-14s %s to %s (sort by longest first)\n", "load window", clock(load.Started), clock(load.Ended))
	fmt.Fprintf(w, "  %-14s docs/roteiro-cenario-de-falha.md\n\n", "step by step")
}

func times(before, after time.Duration) string {
	if before <= 0 {
		return ""
	}
	return fmt.Sprintf("   %.1fx", float64(after)/float64(before))
}

func clock(t time.Time) string { return t.Format("15:04:05 MST") }

func short(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func unreachable(phases ...*Phase) (string, bool) {
	counted := map[string]int{}
	for _, phase := range phases {
		if phase == nil {
			continue
		}
		for _, result := range phase.Results {
			if result.Status != 0 {
				return "", false
			}
			counted[cause(result)]++
		}
	}

	reason, most := "", 0
	for label, count := range counted {
		if count > most || (count == most && label < reason) {
			reason, most = label, count
		}
	}
	return reason, true
}
