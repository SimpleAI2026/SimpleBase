// Package limit normalizes pagination limits.
package limit

// Normalize returns def when n is non-positive or exceeds max.
// A non-positive max disables the upper bound.
func Normalize(n, def, max int) int {
	if n <= 0 || (max > 0 && n > max) {
		return def
	}
	return n
}

// Clamp replaces a non-positive n with def and caps n at max.
// A non-positive max disables the upper bound.
func Clamp(n, def, max int) int {
	if n <= 0 {
		n = def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
