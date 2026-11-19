//go:build race

package clientview

// The race detector slows the sanitizer about tenfold.
const raceSlowdown = 10
