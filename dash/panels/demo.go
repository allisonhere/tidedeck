package panels

import "math"

// Synthesis helpers shared by the panels' Demo methods, so the dashboard has
// believable sample data before anything is configured. They live here rather
// than in one panel's file because more than one panel demos with them.

// wave is a smooth repeating signal, so demo data drifts instead of jumping.
func wave(t, period, phase float64) float64 {
	return math.Sin(2*math.Pi*(t/period) + phase)
}

// clamp bounds a value.
func clamp(value, low, high float64) float64 {
	return math.Max(low, math.Min(high, value))
}

// series samples a smooth signal into n buckets ending at t, normalised to 0..1.
func series(t float64, n int, period, phase, base, amp float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := t - float64(n-i)
		out[i] = clamp(base+amp*wave(x, period, phase+float64(i)*0.35), 0, 1)
	}
	return out
}
