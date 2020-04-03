package syntax

func isSpace(ch byte) bool {
	switch ch {
	case '\r', '\n', '\t', '\f', '\v':
		return true
	default:
		return false
	}
}

func isAlphanumeric(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}
