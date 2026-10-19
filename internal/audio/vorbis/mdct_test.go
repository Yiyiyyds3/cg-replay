package vorbis

import (
	"math"
	"testing"
)

// TestIMDCTMatchesNaive 校验 O(N log N) 的 IMDCT 与直译公式一致。
func TestIMDCTMatchesNaive(t *testing.T) {
	for _, n := range []int{64, 128, 256, 512, 1024, 2048, 4096, 8192} {
		M := n / 2
		x := make([]float64, M)
		for i := range x {
			x[i] = math.Sin(float64(i)*0.37) * math.Cos(float64(i)*0.011)
		}

		got := imdct(x, n)
		want := naiveIMDCT(x, n)

		var maxErr float64
		for i := range got {
			d := math.Abs(got[i] - want[i])
			if d > maxErr {
				maxErr = d
			}
		}
		if maxErr > 1e-7 {
			t.Errorf("n=%d 最大误差 %.3e", n, maxErr)
		}
	}
}

// TestWindowShape 校验窗函数的关键性质：值域、对称、TDAC 重建条件。
func TestWindowShape(t *testing.T) {
	for _, n := range []int{512, 1024, 2048} {
		w := window(n, n/4, false, true, true)
		if len(w) != n {
			t.Fatalf("n=%d 长度 %d", n, len(w))
		}
		// 短窗形状对称（两端都接近 0，中心为 1）。
		if math.Abs(w[0]-w[n-1]) > 1e-12 {
			t.Errorf("n=%d 窗不对称：%v vs %v", n, w[0], w[n-1])
		}
		if w[n/2-1] < 0.9999 {
			t.Errorf("n=%d 中心应为 1，实得 %v", n, w[n/2-1])
		}
		for i, v := range w {
			if v < 0 || v > 1+1e-12 {
				t.Fatalf("n=%d i=%d 窗值越界 %v", n, i, v)
			}
		}
		// TDAC 完美重建：相邻两帧相差 n/2 个样本，w[i]² + w[i+n/2]² == 1。
		for i := 0; i < n/2; i++ {
			s := w[i]*w[i] + w[i+n/2]*w[i+n/2]
			if math.Abs(s-1) > 1e-9 {
				t.Fatalf("n=%d i=%d TDAC 条件不满足 %.9f", n, i, s)
			}
		}
	}
}
