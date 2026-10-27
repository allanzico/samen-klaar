package workflow

import (
	"math"
	"testing"
)

func TestPlatformFeeUsesAgreedTotalAndCentRounding(t *testing.T) {
	for _, tc := range []struct {
		total int64
		rate  int
		want  int64
	}{{10500, 500, 525}, {11000, 500, 550}, {199, 500, 10}, {10, 500, 1}, {9, 500, 0}, {10500, 0, 0}, {0, 500, 0}, {math.MaxInt64, 10000, math.MaxInt64}} {
		fee, err := PlatformFeeAmount(tc.total, tc.rate)
		if err != nil || fee != tc.want {
			t.Fatalf("fee(%d,%d)=%d,%v want %d", tc.total, tc.rate, fee, err, tc.want)
		}
	}
	for _, tc := range []struct {
		total int64
		rate  int
	}{{-1, 500}, {10500, -1}, {10500, 10001}} {
		if _, err := PlatformFeeAmount(tc.total, tc.rate); err == nil {
			t.Fatal("invalid terms accepted")
		}
	}
}
