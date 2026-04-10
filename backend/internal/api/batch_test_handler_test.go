package api

import "testing"

func TestComputeBatchWorkers(t *testing.T) {
	tests := []struct {
		name        string
		globalLimit int
		want        int
	}{
		{"unlimited falls back to default", 0, batchWorkerDefaultMax},
		{"negative treated as unlimited", -1, batchWorkerDefaultMax},
		{"tiny global clamps to 1", 1, 1},
		{"4 clamps to 1", 4, 1},
		{"exactly 5 yields 1", 5, 1},
		{"9 yields 1", 9, 1},
		{"10 yields 2", 10, 2},
		{"25 yields 5", 25, 5},
		{"40 yields exactly default", 40, batchWorkerDefaultMax},
		{"large global clamps to default", 1000, batchWorkerDefaultMax},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeBatchWorkers(tc.globalLimit)
			if got != tc.want {
				t.Errorf("computeBatchWorkers(%d) = %d, want %d", tc.globalLimit, got, tc.want)
			}
		})
	}
}
