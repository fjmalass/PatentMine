package render

// AssignKey greedily assigns a unique shortcut key for a label.
// It prioritizes characters present in the label, falling back to 
// the first free letter, and then the first free digit.
// If prioritizeDigits is true, digits are tried before letters (perfect for claims: Claim 1, Claim 2, etc.).
func AssignKey(label string, used map[rune]bool, prioritizeDigits bool) rune {
	// 1. Try characters from the label in order
	for _, r := range label {
		switch {
		case prioritizeDigits && r >= '0' && r <= '9':
			if !used[r] {
				return r
			}
		case r >= 'A' && r <= 'Z':
			r = r - 'A' + 'a'
			fallthrough
		case r >= 'a' && r <= 'z':
			if !used[r] {
				return r
			}
		case !prioritizeDigits && r >= '0' && r <= '9':
			if !used[r] {
				return r
			}
		}
	}

	// 2. Fallbacks if no characters from the label are free
	if prioritizeDigits {
		for r := '0'; r <= '9'; r++ {
			if !used[r] {
				return r
			}
		}
	}
	for r := 'a'; r <= 'z'; r++ {
		if !used[r] {
			return r
		}
	}
	if !prioritizeDigits {
		for r := '0'; r <= '9'; r++ {
			if !used[r] {
				return r
			}
		}
	}
	return '?'
}
