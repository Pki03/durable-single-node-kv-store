package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/prateekkhurmi/kvprjt/store"
)

type BenchResult struct {
	Name       string
	Ops        int
	Duration   time.Duration
	OpsPerSec  float64
	P50Latency time.Duration
	P99Latency time.Duration
	RecoveryMs float64
	WALSizeMB  float64
}

func RunBenchmarks(storeDir string) ([]BenchResult, error) {
	defer os.RemoveAll(storeDir)

	var results []BenchResult

	r, err := benchWrite(storeDir)
	if err != nil {
		return nil, err
	}
	results = append(results, r)

	r, err = benchRead(storeDir)
	if err != nil {
		return nil, err
	}
	results = append(results, r)

	r, err = benchRecovery(storeDir)
	if err != nil {
		return nil, err
	}
	results = append(results, r)

	return results, nil
}

func benchWrite(dir string) (BenchResult, error) {
	s, err := store.New(dir)
	if err != nil {
		return BenchResult{}, err
	}
	defer s.Close()

	n := 10000
	start := time.Now()
	latencies := make([]time.Duration, 0, n)

	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key_%06d", i)
		val := fmt.Sprintf("value_%06d", i)
		t0 := time.Now()
		if err := s.Put(key, []byte(val)); err != nil {
			return BenchResult{}, err
		}
		latencies = append(latencies, time.Since(t0))
	}

	duration := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	walSize, _ := dirSize(dir)

	return BenchResult{
		Name:       "Write",
		Ops:        n,
		Duration:   duration,
		OpsPerSec:  float64(n) / duration.Seconds(),
		P50Latency: latencies[n/2],
		P99Latency: latencies[int(float64(n)*0.99)],
		WALSizeMB:  float64(walSize) / (1024 * 1024),
	}, nil
}

func benchRead(dir string) (BenchResult, error) {
	s, err := store.New(dir)
	if err != nil {
		return BenchResult{}, err
	}
	defer s.Close()

	n := 10000
	start := time.Now()
	latencies := make([]time.Duration, 0, n)

	for i := 0; i < n; i++ {
		key := fmt.Sprintf("key_%06d", i)
		t0 := time.Now()
		s.Get(key)
		latencies = append(latencies, time.Since(t0))
	}

	duration := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	return BenchResult{
		Name:       "Read",
		Ops:        n,
		Duration:   duration,
		OpsPerSec:  float64(n) / duration.Seconds(),
		P50Latency: latencies[n/2],
		P99Latency: latencies[int(float64(n)*0.99)],
	}, nil
}

func benchRecovery(dir string) (BenchResult, error) {
	s, err := store.New(dir)
	if err != nil {
		return BenchResult{}, err
	}
	keyCount := s.Size()
	s.Close()

	walSize, _ := dirSize(dir)
	recoveryStart := time.Now()
	_, err = store.New(dir)
	if err != nil {
		return BenchResult{}, err
	}
	recoveryMs := float64(time.Since(recoveryStart).Milliseconds())

	return BenchResult{
		Name:       "Recovery",
		Ops:        keyCount,
		RecoveryMs: recoveryMs,
		WALSizeMB:  float64(walSize) / (1024 * 1024),
	}, nil
}

func dirSize(dir string) (int64, error) {
	var size int64
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, _ := d.Info()
			size += info.Size()
		}
		return nil
	})
	return size, nil
}

func PrintResults(results []BenchResult) {
	for _, r := range results {
		fmt.Printf("=== %s ===\n", r.Name)
		fmt.Printf("  Ops: %d\n", r.Ops)
		fmt.Printf("  Duration: %v\n", r.Duration)
		fmt.Printf("  Throughput: %.0f ops/sec\n", r.OpsPerSec)
		if r.P50Latency > 0 {
			fmt.Printf("  p50 latency: %v\n", r.P50Latency)
			fmt.Printf("  p99 latency: %v\n", r.P99Latency)
		}
		if r.RecoveryMs > 0 {
			fmt.Printf("  Recovery time: %.2fms\n", r.RecoveryMs)
		}
		if r.WALSizeMB > 0 {
			fmt.Printf("  WAL size: %.2f MB\n", r.WALSizeMB)
		}
		fmt.Println()
	}
}
