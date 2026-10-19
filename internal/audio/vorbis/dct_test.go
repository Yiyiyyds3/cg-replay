package vorbis

import (
	"math"
	"testing"
)

func naiveDCT4(x []float64, N int) []float64 {
	out := make([]float64, N)
	for m := 0; m < N; m++ {
		var s float64
		for k := 0; k < N; k++ {
			s += x[k] * math.Cos(math.Pi/float64(N)*(float64(k)+0.5)*(float64(m)+0.5))
		}
		out[m] = s
	}
	return out
}

func TestDCT4AgainstNaive(t *testing.T) {
	for _, N := range []int{8, 16, 32, 64, 256, 1024} {
		x := make([]float64, N)
		for i := range x {
			x[i] = math.Sin(float64(i)*0.7) + 0.3*math.Cos(float64(i)*0.13)
		}
		got := dct4(x, N)
		want := naiveDCT4(x, N)
		var maxErr float64
		var at int
		for i := range got {
			if d := math.Abs(got[i] - want[i]); d > maxErr {
				maxErr, at = d, i
			}
		}
		if maxErr > 1e-9 {
			t.Errorf("N=%d 最大误差 %.3e @%d (got=%v want=%v)", N, maxErr, at, got[at], want[at])
		}
	}
}

func TestFFTRoundTrip(t *testing.T) {
	N := 64
	re := make([]float64, N)
	im := make([]float64, N)
	origRe := make([]float64, N)
	origIm := make([]float64, N)
	for i := 0; i < N; i++ {
		re[i] = math.Sin(float64(i) * 0.31)
		im[i] = math.Cos(float64(i) * 0.17)
		origRe[i], origIm[i] = re[i], im[i]
	}
	fft(re, im, false)
	fft(re, im, true)
	var maxErr float64
	for i := 0; i < N; i++ {
		if d := math.Abs(re[i] - origRe[i]); d > maxErr {
			maxErr = d
		}
		if d := math.Abs(im[i] - origIm[i]); d > maxErr {
			maxErr = d
		}
	}
	if maxErr > 1e-12 {
		t.Errorf("FFT 往返误差 %.3e", maxErr)
	}
}
