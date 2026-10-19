package vorbis

import (
	"math"
	"sync"
)

// imdct 计算 Vorbis 的 IMDCT：n/2 个频谱系数 → n 个时域样本（未乘窗）。
//
//	x[i] = sum_{k=0}^{M-1} X[k] · cos(pi/M · (i + 1/2 + M/2)(k + 1/2)),  M = n/2
//
// 令 c[m] = DCT-IV(X)[m] = sum_k X[k] cos(pi/M (m+1/2)(k+1/2))，则 x[i] = c[i + M/2]。
// c 的延拓规则：关于 -1/2 偶对称 c[-m-1]=c[m]；对 2M 反周期 c[m+2M]=-c[m]；
// 因而 m∈[M,2M) 时 c[m] = -c[2M-1-m]。
func imdct(x []float64, n int) []float64 {
	M := n / 2
	if M <= 0 || len(x) < M {
		return make([]float64, n)
	}
	c := dct4(x[:M], M)

	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := i + M/2
		// 先按反周期 2M 归约，并记录符号。
		q := t / (2 * M)
		t -= q * 2 * M
		sign := 1.0
		if q%2 != 0 {
			sign = -1
		}
		if t >= M {
			t = 2*M - 1 - t
			sign = -sign
		}
		out[i] = sign * c[t]
	}
	return out
}

// naiveIMDCT 是 IMDCT 的 O(n*M) 直译，仅用于单测对拍。
func naiveIMDCT(x []float64, n int) []float64 {
	M := n / 2
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		var sum float64
		for k := 0; k < M && k < len(x); k++ {
			ang := math.Pi / float64(M) * (float64(i) + 0.5 + float64(M)/2) * (float64(k) + 0.5)
			sum += x[k] * math.Cos(ang)
		}
		out[i] = sum
	}
	return out
}

// dct4 计算长度 N 的 DCT-IV（N 必须是 2 的幂），O(N log N)。
//
//	c[m] = sum_{k=0}^{N-1} x[k] cos(pi/N (k+1/2)(m+1/2))
//
// 展开相位：pi/N (k+1/2)(m+1/2) = 2pi·km/(2N) + pi·k/(2N) + pi·m/(2N) + pi/(4N)。
// 令 z[k] = x[k]·e^{i·pi·k/(2N)}（k<N，其余补零），Z[m] = sum_k z[k] e^{i·2pi·km/(2N)}，则
//
//	c[m] = Re{ Z[m] · e^{i·(pi/(4N) + pi·m/(2N))} }
func dct4(x []float64, N int) []float64 {
	size := 2 * N
	re := make([]float64, size)
	im := make([]float64, size)

	for k := 0; k < N; k++ {
		ang := math.Pi * float64(k) / float64(2*N)
		s, c := math.Sincos(ang)
		re[k] = x[k] * c
		im[k] = x[k] * s
	}

	// fft(inverse) 给出 (1/size)·Σ z e^{+i2pi km/size}，乘回 size 得到 Z。
	fft(re, im, true)
	scale := float64(size)

	out := make([]float64, N)
	for m := 0; m < N; m++ {
		ang := math.Pi/float64(4*N) + math.Pi*float64(m)/float64(2*N)
		s, c := math.Sincos(ang)
		zr := re[m] * scale
		zi := im[m] * scale
		out[m] = zr*c - zi*s
	}
	return out
}

// twiddle 缓存：size → 前半周期的 e^{-2*pi*i*k/size}。
var (
	twMu    sync.Mutex
	twCache = map[int][]complex128{}
)

func twiddles(size int) []complex128 {
	twMu.Lock()
	defer twMu.Unlock()
	if t, ok := twCache[size]; ok {
		return t
	}
	half := size / 2
	t := make([]complex128, half)
	for k := 0; k < half; k++ {
		ang := -2 * math.Pi * float64(k) / float64(size)
		s, c := math.Sincos(ang)
		t[k] = complex(c, s)
	}
	twCache[size] = t
	return t
}

// fft 原地基-2 FFT。inverse 为 true 时做逆变换（含 1/n 归一化）。
func fft(re, im []float64, inverse bool) {
	n := len(re)
	if n <= 1 || n&(n-1) != 0 {
		return
	}

	// 位反转置换
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}

	tw := twiddles(n)

	for length := 2; length <= n; length <<= 1 {
		step := n / length
		half := length / 2
		for i := 0; i < n; i += length {
			for k := 0; k < half; k++ {
				w := tw[k*step]
				wr, wi := real(w), imag(w)
				if inverse {
					wi = -wi
				}
				a := i + k
				b := a + half
				tr := re[b]*wr - im[b]*wi
				ti := re[b]*wi + im[b]*wr
				re[b] = re[a] - tr
				im[b] = im[a] - ti
				re[a] += tr
				im[a] += ti
			}
		}
	}

	if inverse {
		inv := 1 / float64(n)
		for i := 0; i < n; i++ {
			re[i] *= inv
			im[i] *= inv
		}
	}
}

// window 生成 Vorbis 窗函数（§4.3.1），长度 n。
//
// 斜率函数：y = sin(pi/2 · sin²((x+0.5)/n · pi/2))，其中 n 为半窗长度；
// 等价于以整窗长 N 写作 (x+0.5)/N · pi。
func window(n, bs0 int, long, prevFlag, nextFlag bool) []float64 {
	w := make([]float64, n)
	center := n / 2

	leftStart, leftEnd, leftN := 0, center, n/2
	if long && !prevFlag {
		leftStart = n/4 - bs0/4
		leftEnd = n/4 + bs0/4
		leftN = bs0 / 2
	}
	rightStart, rightEnd, rightN := center, n, n/2
	if long && !nextFlag {
		rightStart = n*3/4 - bs0/4
		rightEnd = n*3/4 + bs0/4
		rightN = bs0 / 2
	}

	// [0, leftStart) 为 0（make 已清零）。
	for i := leftStart; i < leftEnd && i < n; i++ {
		pos := (float64(i-leftStart) + 0.5) / float64(leftN)
		s := math.Sin(pos * math.Pi / 2)
		w[i] = math.Sin(math.Pi / 2 * s * s)
	}
	for i := leftEnd; i < rightStart && i < n; i++ {
		w[i] = 1
	}
	for i := rightStart; i < rightEnd && i < n; i++ {
		pos := (float64(i-rightStart) + 0.5) / float64(rightN)
		s := math.Sin(pos*math.Pi/2 + math.Pi/2)
		w[i] = math.Sin(math.Pi / 2 * s * s)
	}
	// [rightEnd, n) 为 0。
	return w
}
