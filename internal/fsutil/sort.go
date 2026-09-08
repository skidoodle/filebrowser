package fsutil

import "strings"

// NaturalCompare compares two strings the way humans expect: embedded
// digit runs are compared numerically, so "file2" sorts before "file10".
// Comparison is case-insensitive.
func NaturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			nextA, nextB := i, j
			for nextA < len(a) && isDigit(a[nextA]) {
				nextA++
			}
			for nextB < len(b) && isDigit(b[nextB]) {
				nextB++
			}
			if c := compareDigitRuns(a[i:nextA], b[j:nextB]); c != 0 {
				return c
			}
			i, j = nextA, nextB
			continue
		}
		if c := compareRunes(ca, cb); c != 0 {
			return c
		}
		i++
		j++
	}
	return compareRemainder(a[i:], b[j:])
}

// compareRunes orders two bytes case-insensitively, breaking ties by
// byte value so the order is stable.
func compareRunes(a, b byte) int {
	la, lb := toLower(a), toLower(b)
	if la != lb {
		if la < lb {
			return -1
		}
		return 1
	}
	if a != b {
		if a < b {
			return -1
		}
		return 1
	}
	return 0
}

func compareRemainder(ra, rb string) int {
	switch {
	case len(ra) < len(rb):
		return -1
	case len(ra) > len(rb):
		return 1
	default:
		return 0
	}
}

// compareDigitRuns compares two decimal runs numerically.
func compareDigitRuns(ra, rb string) int {
	na := strings.TrimLeft(ra, "0")
	nb := strings.TrimLeft(rb, "0")
	if len(na) != len(nb) {
		if len(na) < len(nb) {
			return -1
		}
		return 1
	}
	return strings.Compare(na, nb)
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func toLower(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
