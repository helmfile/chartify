package chartify

import (
	"fmt"
	"regexp"
	"strings"
)

const semVerRegex string = `v([0-9]+)(\.[0-9]+)?(\.[0-9]+)?` +
	`(-([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?` +
	`(\+([0-9A-Za-z\-]+(\.[0-9A-Za-z\-]+)*))?`

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

func FindSemVerInfo(version string) (string, error) {
	if version == "" {
		return "", fmt.Errorf("version cannot be empty")
	}
	processedVersion := strings.TrimSpace(version)
	if !strings.HasPrefix(processedVersion, "v") {
		processedVersion = fmt.Sprintf("v%s", processedVersion)
	}
	v := regexp.MustCompile(semVerRegex).FindString(processedVersion)

	if v == "" {
		return "", fmt.Errorf("unable to find semver info in %s", version)
	}
	return v, nil
}
