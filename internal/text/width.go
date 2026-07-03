package text

import "strings"

// DisplayWidth returns the number of fixed-width cells s occupies in a monospace
// table, counting East-Asian wide runes (CJK) as 2 and others as 1. Padding by
// rune count would misalign rows containing Chinese names.
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// PadRight right-pads s with spaces to the given display width (no-op if s is
// already at least that wide).
func PadRight(s string, width int) string {
	if n := width - DisplayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// isWide reports whether r renders as a double-width (East-Asian) glyph. Covers
// the ranges that appear in atlas watchlist names/symbols; not a full Unicode
// East-Asian-Width table (avoids a third-party dependency).
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals, Kangxi, CJK symbols
		r >= 0x3041 && r <= 0x33FF, // Hiragana/Katakana/CJK compat
		r >= 0x3400 && r <= 0x4DBF, // CJK Ext A
		r >= 0x4E00 && r <= 0x9FFF, // CJK Unified
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compat ideographs
		r >= 0xFF00 && r <= 0xFF60, // Fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6, // Fullwidth signs
		r >= 0x20000 && r <= 0x3FFFD: // CJK Ext B+
		return true
	}
	return false
}
