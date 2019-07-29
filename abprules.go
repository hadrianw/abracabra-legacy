package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func SplitSelectorRule(line string, ruleType string) (domains []string, selector string) {
	idx := strings.Index(line, ruleType)
	if idx != -1 {
		return strings.Split(line[:idx], ","), line[idx+len(ruleType):]
	}
	return
}

func AppendDomains(domains []string, notDomains []string, appendix []string) ([]string, []string) {
AppendixLoop:
	for _, a := range appendix {
		ds := &domains
		if strings.HasPrefix(a, "~") {
			a = a[1:]
			ds = &notDomains
		}
		for _, d := range *ds {
			if a == d {
				continue AppendixLoop
			}
		}
		*ds = append(*ds, a)
	}
	return domains, notDomains
}

type PatternOptions struct {
	Options    []string
	Domains    []string
	NotDomains []string
}

func main() {
	r := bufio.NewReader(os.Stdin)

	f, err := os.Create("unaccounted.txt")
	if err != nil {
		panic(err)
	}
	w := bufio.NewWriter(f)
	defer func() {
		w.Flush()
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}()

	b, err := r.Peek(1)
	if err != nil {
		if err == io.EOF {
			return
		}
		panic(err)
	}
	if b[0] == '[' {
		_, err := r.ReadBytes('\n')
		if err != nil {
			panic(err)
		}
	}

	total := 0
	unaccounted := 0

	selectorCount := 0
	bareSelectorCount := 0
	selectorExceptionCount := 0
	selectors := make(map[string]struct {
		Domains    []string
		NotDomains []string
	})
	extendedSelectorCount := 0
	type RegexRule struct {
		Pattern string
		Options PatternOptions
	}
	var patterns, notPatterns struct {
		Regexes []RegexRule
	}
	domainInOptionsCount := 0
	domainInPatternCount := 0
	simplePatternCount := 0
	backPatternCount := 0

	// TODO: ||example.com*whatever - domain wildcards
	var domainBlacklist []string
	pts := &patterns

	for {
		b, err := r.Peek(1)
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		// comment
		if b[0] == '!' {
			_, err := r.ReadBytes('\n')
			if err != nil {
				panic(err)
			}
			continue
		}

		line, err := r.ReadString('\n')
		if err != nil {
			panic(err)
		}
		line = line[:len(line)-1]

		total++

		// selector rule
		if domains, selector := SplitSelectorRule(line, "##"); len(selector) > 0 {
			sel := selectors[selector]
			sel.Domains, sel.NotDomains = AppendDomains(sel.Domains, sel.NotDomains, domains)
			selectors[selector] = sel
			selectorCount++
			if len(domains) == 0 || (len(domains) == 1 && domains[0] == "") {
				bareSelectorCount++
			}
			continue
		}

		// extended selector rule - ignore
		if idx := strings.Index(line, "#?#"); idx != -1 {
			extendedSelectorCount++
			continue
		}

		// selector exception rule
		if notDomains, selector := SplitSelectorRule(line, "#@#"); len(selector) > 0 {
			sel := selectors[selector]
			sel.NotDomains, sel.Domains = AppendDomains(sel.NotDomains, sel.Domains, notDomains)
			selectors[selector] = sel
			selectorExceptionCount++
			continue
		}

		// URL pattern rule

		pts = &patterns

		var pattern string
		var options string

		idx := strings.LastIndex(line, "$")
		if idx != -1 {
			pattern = line[:idx]
			options = line[idx+1:len(line)-1]
		} else {
			pattern = line[:len(line)-1]
		}
		if strings.HasPrefix(pattern, "@@") {
			pts = &notPatterns
			pattern = pattern[2:]
		}

		// parse options, search for domains option
		var opts PatternOptions
		opts.Options = strings.Split(options, ",")
		for _, o := range opts.Options {
			if strings.HasPrefix(o, "domain=") {
				rawds := strings.Split(o[len("domain="):], "|")
				for _, rd := range rawds {
					ds := &opts.Domains
					if strings.HasPrefix(rd, "~") {
						ds = &opts.NotDomains
						rd = rd[1:]
					}
					*ds = append(*ds, rd)
				}
				break
			}
		}

		// domain in options, it serves ads
		if pts == &patterns {
			for _, d := range opts.Domains {
				domainBlacklist = append(domainBlacklist, d)
			}
			if len(opts.Domains) > 0 {
				domainInOptionsCount++
				continue
			}
		}
		if pts == &notPatterns {
			for _, d := range opts.NotDomains {
				domainBlacklist = append(domainBlacklist, d)
			}
			if len(opts.NotDomains) > 0 {
				domainInOptionsCount++
				continue
			}
		}

		// regex rule
		if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
			regex := pattern[1 : len(pattern)-1]
			pts.Regexes = append(pts.Regexes, RegexRule{regex, opts})
			continue
		}

		if strings.HasPrefix(pattern, "||") {
			// FIXME: domain can be in pattern and in options at the same time
			pattern := pattern[2:]
			if idx := strings.IndexAny(pattern, "/^"); idx != -1 {
				domainBlacklist = append(domainBlacklist, pattern[:idx])
				domainInPatternCount++
				continue
			}
		} else if pts == &patterns && !strings.HasPrefix(pattern, "|") {
			count := &simplePatternCount
			position := 0
			length := len(pattern)
			if strings.HasPrefix(pattern, "*") {
				position++
				length--
			}
			if strings.HasSuffix(pattern, "|") {
				count = &backPatternCount
				length--
			} else if strings.HasSuffix(pattern, "*") {
				length--
			}
			pattern = pattern[position:length]
			if idx := strings.IndexAny(pattern, "*^"); idx == -1 {
				(*count)++
				continue
			}
		}

		unaccounted++
		if _, err := w.WriteString(line); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}

	fmt.Printf(`selector: %d (bare: %d)
selector exception: %d
extended selector: %d
patterns:
	domain in options: %d
	domain in pattern: %d
	regex: %d
	simple: %d
	back: %d
not patterns:
	regex: %d
total: %d
unaccounted: %d
`, selectorCount, bareSelectorCount, selectorExceptionCount, extendedSelectorCount,
domainInOptionsCount, domainInPatternCount, len(patterns.Regexes), simplePatternCount, backPatternCount,
len(notPatterns.Regexes),
total, unaccounted)
}
