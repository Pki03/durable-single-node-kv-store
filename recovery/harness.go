package recovery

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Harness struct {
	binaryPath string
	iterations int
	successful int
	failed     int
	dataLoss   int
	mu         sync.Mutex
}

type Result struct {
	Iterations  int
	Successful  int
	Failed      int
	DataLoss    int
	SuccessRate float64
}

func New(binaryPath string, iterations int) *Harness {
	if iterations <= 0 {
		iterations = 50
	}
	return &Harness{
		binaryPath: binaryPath,
		iterations: iterations,
	}
}

func (h *Harness) Run() *Result {
	h.successful = 0
	h.failed = 0
	h.dataLoss = 0

	var wg sync.WaitGroup
	for i := 0; i < h.iterations; i++ {
		wg.Add(1)
		go func(iteration int) {
			defer wg.Done()
			ok := h.runOnce(iteration)
			h.mu.Lock()
			if ok {
				h.successful++
			} else {
				h.failed++
			}
			h.mu.Unlock()
		}(i)
	}
	wg.Wait()

	successRate := 0.0
	if h.iterations > 0 {
		successRate = float64(h.successful) / float64(h.iterations) * 100
	}
	return &Result{
		Iterations:  h.iterations,
		Successful:  h.successful,
		Failed:      h.failed,
		DataLoss:    h.dataLoss,
		SuccessRate: successRate,
	}
}

func (h *Harness) runOnce(iteration int) bool {
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("kvprjt_kill_%d_", iteration))
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	count := 50 + iteration%100
	killPoint := time.Duration(20+(iteration%80)) * time.Millisecond

	cmd := exec.Command(h.binaryPath, "-dir", tmpDir, "-count", strconv.Itoa(count))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	if err := cmd.Start(); err != nil {
		return false
	}

	okCount := 0
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "OK:") {
				okCount++
			}
		}
	}()

	time.Sleep(killPoint)
	cmd.Process.Kill()
	cmd.Wait()
	stdout.Close()

	time.Sleep(50 * time.Millisecond)

	recoveryCmd := exec.Command(h.binaryPath, "-dir", tmpDir, "-recover")
	if _, err := recoveryCmd.Output(); err != nil {
		return false
	}

	verifyCmd := exec.Command(h.binaryPath, "-dir", tmpDir, "-count", strconv.Itoa(count), "-verify")
	verifyOut, err := verifyCmd.Output()
	if err != nil {
		return false
	}

	var total, missing int
	fmt.Sscanf(string(verifyOut), "VERIFY:%d:%d", &total, &missing)

	if missing > 0 {
		h.mu.Lock()
		h.dataLoss += missing
		h.mu.Unlock()
		return false
	}

	return true
}
