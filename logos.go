package main

import (
	"fmt"
	"regexp"
	"strings"
)

var SystemLogos map[string][]string
var DefaultLogo []string
var ColorPalette []string

func init() {
	SystemLogos = make(map[string][]string)
	ColorPalette = []string{
		"${c0}", "\u001b[0m\u001b[38;5;248m",
		"${c1}", "\u001b[0m\u001b[31;1m",
		"${c2}", "\u001b[0m\u001b[32;1m",
		"${c3}", "\u001b[0m\u001b[33;1m",
		"${c4}", "\u001b[0m\u001b[34;1m",
		"${c5}", "\u001b[0m\u001b[35;1m",
		"${c6}", "\u001b[0m\u001b[36;1m",
		"${c7}", "\u001b[0m\u001b[37;1m",
	}
	DefaultLogo = []string{
		"  ${c4}     ___     ",
		"  ${c4}    (${c7}.. ${c4}|",
		"  ${c4}    (${c5}<> ${c4}|",
		"  ${c4}   / ${c7}__  ${c4}\\",
		"  ${c4}  ( ${c7}/  \\ ${c4}/|",
		"  ${c5} _${c4}/\\ ${c7}__)${c4}/${c5}_${c4})",
		`  ${c5} \/${c4}-____${c5}\/`,
	}

	parseLogos(TheLogosThemselves)
}

func splitIntoBlocks(input string) []string {
	return strings.Split(input, ";;")
}

func extractPatternAndLogo(lines []string) (string, []string) {
	var pattern string
	var logo []string

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "(") {
			pattern = strings.TrimPrefix(strings.TrimSuffix(strings.TrimSpace(line), "*"), "[")
			for j := i + 1; j < len(lines); j++ {
				if strings.Contains(lines[j], "${c") {
					logoLine := strings.ReplaceAll(strings.TrimLeft(lines[j], " "), "\t", "")
					logo = append(logo, logoLine)
				}
			}
			break
		}
	}
	return pattern, logo
}

func parseLogos(input string) {
	blocks := splitIntoBlocks(input)
	for _, block := range blocks {
		lines := strings.Split(block, "\n")
		pattern, logo := extractPatternAndLogo(lines)
		if _, err := regexp.CompilePOSIX(pattern); err == nil {
			SystemLogos[pattern] = logo
		} else {
			fmt.Println("Invalid pattern:", pattern, "Error:", err)
		}
	}
}

func ApplyColorPalette(logo []string) []string {
	for line := range logo {
		logo[line] = strings.NewReplacer(ColorPalette...).Replace(logo[line])
	}
	return append(logo, "\r")
}

func GetLogo(name string) []string {
	var bestMatch []string
	longestMatch := 0

	for pattern, logo := range SystemLogos {
		regex := regexp.MustCompilePOSIX(pattern)
		matches := regex.FindAllString(name, -1)
		if len(matches) > 0 && len(pattern) > longestMatch {
			bestMatch = logo
			longestMatch = len(pattern)
		}
	}

	if bestMatch != nil {
		return ApplyColorPalette(bestMatch)
	}
	return ApplyColorPalette(DefaultLogo)
}
