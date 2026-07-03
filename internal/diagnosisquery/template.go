// Package diagnosisquery contains the shared conservative query-template rules
// used by diagnosis tool catalogs and evidence collection.
package diagnosisquery

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const maxPlaceholderValueBytes = 200

var (
	placeholderPattern        = regexp.MustCompile(`\{\{\s*(label|annotation)\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
	labelMatcherPrefixPattern = regexp.MustCompile(`(?:^|[,{])\s*[A-Za-z_][A-Za-z0-9_]*\s*(=|!=|=~|!~)\s*$`)
)

// Values provides alert-derived values for safe query-template expansion.
type Values struct {
	Labels      map[string]string
	Annotations map[string]string
}

type compiledTemplate struct {
	re           *regexp.Regexp
	placeholders []string
}

// ValidateTemplate rejects unsupported placeholder syntax. Ordinary PromQL
// without OpenClarion placeholders is accepted as-is.
func ValidateTemplate(queryTemplate string) error {
	if !containsTemplateDelimiter(queryTemplate) {
		return nil
	}
	if _, ok := compileTemplate(queryTemplate); !ok {
		return fmt.Errorf("unsupported diagnosis query placeholder")
	}
	return nil
}

// ExpandTemplate replaces supported placeholders with alert-derived values.
// It returns false when a required value is missing or unsafe for a PromQL
// quoted label value.
func ExpandTemplate(queryTemplate string, values Values) (string, bool) {
	queryTemplate = strings.TrimSpace(queryTemplate)
	if !containsTemplateDelimiter(queryTemplate) {
		return queryTemplate, true
	}
	if _, ok := compileTemplate(queryTemplate); !ok {
		return "", false
	}
	matches := placeholderPattern.FindAllStringSubmatchIndex(queryTemplate, -1)
	if len(matches) == 0 {
		return "", false
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		out.WriteString(queryTemplate[last:match[0]])
		kind := queryTemplate[match[2]:match[3]]
		key := queryTemplate[match[4]:match[5]]
		value, ok := lookupValue(values, kind, key)
		if !ok || !safePlaceholderValue(value) {
			return "", false
		}
		out.WriteString(value)
		last = match[1]
	}
	out.WriteString(queryTemplate[last:])
	return out.String(), true
}

// MatchesTemplate reports whether query is permitted by queryTemplate.
func MatchesTemplate(queryTemplate string, query string) bool {
	queryTemplate = strings.TrimSpace(queryTemplate)
	query = strings.TrimSpace(query)
	if !containsTemplateDelimiter(queryTemplate) {
		return query == queryTemplate
	}
	compiled, ok := compileTemplate(queryTemplate)
	if !ok {
		return false
	}
	matches := compiled.re.FindStringSubmatch(query)
	if matches == nil {
		return false
	}
	seen := make(map[string]string, len(compiled.placeholders))
	for i, placeholder := range compiled.placeholders {
		value := matches[i+1]
		if existing, ok := seen[placeholder]; ok && existing != value {
			return false
		}
		seen[placeholder] = value
	}
	return true
}

// ResolveExecutableQuery returns the concrete query that may be executed for a
// request. Parameterized templates require a concrete requested query.
func ResolveExecutableQuery(queryTemplate string, requestedQuery string) (string, bool) {
	queryTemplate = strings.TrimSpace(queryTemplate)
	requestedQuery = strings.TrimSpace(requestedQuery)
	if requestedQuery == "" {
		if containsTemplateDelimiter(queryTemplate) {
			return "", false
		}
		return queryTemplate, queryTemplate != ""
	}
	if !MatchesTemplate(queryTemplate, requestedQuery) {
		return "", false
	}
	return requestedQuery, true
}

func compileTemplate(queryTemplate string) (compiledTemplate, bool) {
	queryTemplate = strings.TrimSpace(queryTemplate)
	matches := placeholderPattern.FindAllStringSubmatchIndex(queryTemplate, -1)
	if len(matches) == 0 {
		return compiledTemplate{}, false
	}
	var pattern strings.Builder
	placeholders := make([]string, 0, len(matches))
	pattern.WriteString("^")
	last := 0
	for _, match := range matches {
		if containsTemplateDelimiter(queryTemplate[last:match[0]]) {
			return compiledTemplate{}, false
		}
		if !placeholderIsQuotedValue(queryTemplate, match[0], match[1]) {
			return compiledTemplate{}, false
		}
		kind := queryTemplate[match[2]:match[3]]
		key := queryTemplate[match[4]:match[5]]
		placeholders = append(placeholders, kind+"."+key)
		pattern.WriteString(regexp.QuoteMeta(queryTemplate[last:match[0]]))
		pattern.WriteString(`([^"\\\r\n]*)`)
		last = match[1]
	}
	if containsTemplateDelimiter(queryTemplate[last:]) {
		return compiledTemplate{}, false
	}
	pattern.WriteString(regexp.QuoteMeta(queryTemplate[last:]))
	pattern.WriteString("$")
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return compiledTemplate{}, false
	}
	return compiledTemplate{re: re, placeholders: placeholders}, true
}

func placeholderIsQuotedValue(queryTemplate string, start int, end int) bool {
	if start <= 0 || end >= len(queryTemplate) {
		return false
	}
	if queryTemplate[start-1] != '"' || queryTemplate[end] != '"' {
		return false
	}
	matches := labelMatcherPrefixPattern.FindStringSubmatch(queryTemplate[:start-1])
	if matches == nil {
		return false
	}
	return matches[1] == "=" || matches[1] == "!="
}

func containsTemplateDelimiter(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "}}")
}

func lookupValue(values Values, kind string, key string) (string, bool) {
	switch kind {
	case "label":
		value, ok := values.Labels[key]
		return value, ok
	case "annotation":
		value, ok := values.Annotations[key]
		return value, ok
	default:
		return "", false
	}
}

func safePlaceholderValue(value string) bool {
	if len([]byte(value)) > maxPlaceholderValueBytes {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}
