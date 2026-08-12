package main

// isFlow reports whether the checks JSON is a step-flow object ({"steps":[…]})
// rather than a flat array of static checks.
func isFlow(b []byte) bool {
	t := bytesTrimLeadingSpace(b)
	return len(t) > 0 && t[0] == '{'
}

func bytesTrimLeadingSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	return b[i:]
}
