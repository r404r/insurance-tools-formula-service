package api

import "testing"

func TestComputeBatchWorkers(t *testing.T) {
	tests := []struct {
		name        string
		globalLimit int
		want        int
	}{
		{"unlimited falls back to default", 0, batchWorkerUnlimitedDefault},
		{"negative treated as unlimited", -1, batchWorkerUnlimitedDefault},
		{"tiny global floors to 1", 1, 1},
		{"4 floors to 1", 4, 1},
		{"exactly 5 yields 1", 5, 1},
		{"9 yields 1", 9, 1},
		{"10 yields 2", 10, 2},
		{"25 yields 5", 25, 5},
		// The upper cap of 8 was removed — workers now scale with globalLimit.
		{"40 yields 8", 40, 8},
		{"100 yields 20", 100, 20},
		{"1000 yields 200", 1000, 200},
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
