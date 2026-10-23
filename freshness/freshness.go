// Package freshness holds age logic and thresholds. Pure: the clock is
// injected, never time.Now directly.
package freshness

import "time"

// AgeHours returns the age of t relative to now in hours.
func AgeHours(t, now time.Time) float64 {
	return now.Sub(t).Hours()
}

// Fresh reports whether t is within threshold of now. Boundary rule:
// exactly at the threshold counts as fresh (age <= threshold).
func Fresh(t time.Time, threshold time.Duration, now time.Time) bool {
	return now.Sub(t) <= threshold
}
