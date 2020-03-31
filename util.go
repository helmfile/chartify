package chartify

import (
	"fmt"
	"strings"
)

func createFlagChain(flag string, input []string) string {
	chain := ""
	dashes := "--"
	if len(flag) == 1 {
		dashes = "-"
	}

	for _, i := range input {
		if i != "" {
			i = " " + i
		}
		chain = fmt.Sprintf("%s %s%s%s", chain, dashes, flag, i)
	}

	return chain
}

// indents a block of text with an indent string
func indent(text, indent string) string {
	var b strings.Builder

	b.Grow(len(text) * 2)

	lines := strings.Split(text, "\n")

	last := len(lines) - 1

	for i, j := range lines {
		if i > 0 && i < last && j != "" {
			b.WriteString("\n")
		}

		if j != "" {
			b.WriteString(indent + j)
		}
	}

	return b.String()
}
