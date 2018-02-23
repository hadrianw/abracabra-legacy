package main

import (
	"bufio"
	//"fmt"
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
		Options []string
		Domains []string
		NotDomains []string
}

func main() {
	r := bufio.NewReader(os.Stdin)

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

	selectors := make(map[string]struct {
		Domains []string
		NotDomains []string
	})
	type RegexRule struct {Pattern string; Options PatternOptions}
	var patterns, notPatterns struct {
		Regexes []RegexRule
	}
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

		// selector rule
		if domains, selector := SplitSelectorRule(line, "##"); len(selector) > 0 {
			sel := selectors[selector]
			sel.Domains, sel.NotDomains = AppendDomains(sel.Domains, sel.NotDomains, domains)
			continue
		}

		// selector exception rule
		if notDomains, selector := SplitSelectorRule(line, "#@#"); len(selector) > 0 {
			sel := selectors[selector]
			sel.NotDomains, sel.Domains = AppendDomains(sel.NotDomains, sel.Domains, notDomains)
			continue
		}

		// URL pattern rule

		pts = &patterns

		var pattern string
		var options string

		idx := strings.LastIndex(line, "$")
		if idx != -1 {
			pattern = line[:idx]
			options = line[idx+1:]
		} else {
			pattern = line
		}
		if strings.HasPrefix(pattern, "@@") {
			pts = &notPatterns
			pattern = pattern[2:]
		}

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

		// regex rule
		if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
			regex := pattern[1:len(pattern)-1]
			pts.Regexes = append(pts.Regexes, RegexRule{regex, opts})
			continue
		}
	}
}
