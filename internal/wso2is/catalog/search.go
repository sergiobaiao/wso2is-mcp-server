/*
 * Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package catalog

import (
	"sort"
	"strings"
	"unicode"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 50

	scoreExactID          = 20_000_000
	scoreExactOperationID = 10_000_000
	scoreAllStructural    = 2_000_000
	scoreAllMatched       = 1_000_000
	scorePartialToken     = 10_000
	scorePartialStruct    = 5_000

	scorePhraseID      = 2_000
	scorePhrasePath    = 1_500
	scorePhraseArea    = 1_200
	scorePhraseTags    = 600
	scorePhraseSummary = 400

	scoreIDToken       = 300
	scoreIDSubstring   = 150
	scoreAreaToken     = 250
	scoreAreaSubstring = 120
	scorePathToken     = 240
	scorePathSubstring = 110
	scoreTagsToken     = 180
	scoreTagsSubstring = 80
	scoreSummaryToken  = 120
	scoreSummarySubstr = 50
)

type matchLevel uint8

const (
	matchNone matchLevel = iota
	matchSubstring
	matchExactToken
)

type scoredOp struct {
	op    *Operation
	score int
}

// Search matches query tokens against catalog operations filtered by Area, Kind, and Organization.
// It returns up to Limit (default 20, clamped to 1..50) ranked matches and the total matched count.
func (c *Catalog) Search(query string, filter SearchFilter) (matches []*Operation, total int) {
	if c == nil || len(c.Operations) == 0 {
		return nil, 0
	}

	limit := clampLimit(filter.Limit)
	rawQuery := strings.ToLower(strings.TrimSpace(query))
	tokens := tokenizeQuery(rawQuery)
	hasPhrase := len(tokens) > 1 || (rawQuery != "" && strings.ContainsAny(rawQuery, " _./-"))

	var scored []scoredOp
	for _, op := range c.Operations {
		if !matchesFilter(op, filter) {
			continue
		}
		if len(tokens) == 0 {
			scored = append(scored, scoredOp{op: op, score: 0})
			continue
		}
		score := scoreOperation(op, rawQuery, tokens, hasPhrase)
		if score > 0 {
			scored = append(scored, scoredOp{op: op, score: score})
		}
	}

	total = len(scored)
	if total == 0 {
		return nil, 0
	}

	sort.Slice(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.op.Deprecated != b.op.Deprecated {
			return !a.op.Deprecated
		}
		if a.op.OrgScoped() != b.op.OrgScoped() {
			return !a.op.OrgScoped()
		}
		return a.op.ID < b.op.ID
	})

	if limit > total {
		limit = total
	}
	matches = make([]*Operation, limit)
	for i := 0; i < limit; i++ {
		matches[i] = scored[i].op
	}
	return matches, total
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultSearchLimit
	}
	if limit > maxSearchLimit {
		return maxSearchLimit
	}
	return limit
}

func matchesFilter(op *Operation, filter SearchFilter) bool {
	if filter.Area != "" && op.Area != filter.Area {
		return false
	}
	if filter.Kind != "" && op.Kind != filter.Kind {
		return false
	}
	if filter.Organization != nil && op.OrgScoped() != *filter.Organization {
		return false
	}
	return true
}

func scoreOperation(op *Operation, rawQuery string, tokens []string, hasPhrase bool) int {
	matchedTokens := 0
	structuralMatched := 0
	tokenScore := 0

	for _, tok := range tokens {
		idLvl := maxMatchLevel(
			matchField(op.ID, tok, true),
			matchField(op.OperationID, tok, true),
		)
		areaLvl := matchField(op.Area, tok, false)
		pathLvl := matchField(op.Path, tok, true)
		tagsLvl := matchTags(op.Tags, tok)
		summaryLvl := matchField(op.Summary, tok, true)

		tokBest := idLvl | areaLvl | pathLvl | tagsLvl | summaryLvl
		if tokBest == 0 {
			continue
		}
		matchedTokens++
		if idLvl > 0 || areaLvl > 0 || pathLvl > 0 {
			structuralMatched++
		}

		tokenScore += fieldScore(idLvl, scoreIDToken, scoreIDSubstring)
		tokenScore += fieldScore(areaLvl, scoreAreaToken, scoreAreaSubstring)
		tokenScore += fieldScore(pathLvl, scorePathToken, scorePathSubstring)
		tokenScore += fieldScore(tagsLvl, scoreTagsToken, scoreTagsSubstring)
		tokenScore += fieldScore(summaryLvl, scoreSummaryToken, scoreSummarySubstr)
	}

	if matchedTokens == 0 {
		return 0
	}

	score := tokenScore

	if strings.EqualFold(op.ID, rawQuery) {
		score += scoreExactID
	} else if strings.EqualFold(op.OperationID, rawQuery) {
		score += scoreExactOperationID
	}

	if matchedTokens == len(tokens) {
		score += scoreAllMatched
		if structuralMatched == len(tokens) {
			score += scoreAllStructural
		}
	} else {
		score += matchedTokens*scorePartialToken + structuralMatched*scorePartialStruct
	}

	if hasPhrase {
		if containsFold(op.ID, rawQuery) || containsFold(op.OperationID, rawQuery) {
			score += scorePhraseID
		}
		if containsFold(op.Path, rawQuery) {
			score += scorePhrasePath
		}
		if containsFold(op.Area, rawQuery) {
			score += scorePhraseArea
		}
		for _, tag := range op.Tags {
			if containsFold(tag, rawQuery) {
				score += scorePhraseTags
				break
			}
		}
		if containsFold(op.Summary, rawQuery) {
			score += scorePhraseSummary
		}
	}

	return score
}

func fieldScore(lvl matchLevel, exactPts, substrPts int) int {
	switch lvl {
	case matchExactToken:
		return exactPts
	case matchSubstring:
		return substrPts
	default:
		return 0
	}
}

func maxMatchLevel(a, b matchLevel) matchLevel {
	if a > b {
		return a
	}
	return b
}

func matchTags(tags []string, tokenLower string) matchLevel {
	best := matchNone
	for _, tag := range tags {
		lvl := matchField(tag, tokenLower, true)
		if lvl > best {
			best = lvl
			if best == matchExactToken {
				return best
			}
		}
	}
	return best
}

func matchField(s, tokenLower string, splitCamel bool) matchLevel {
	if s == "" || tokenLower == "" {
		return matchNone
	}
	if hasExactToken(s, tokenLower, splitCamel) {
		return matchExactToken
	}
	if containsFold(s, tokenLower) {
		return matchSubstring
	}
	return matchNone
}

func tokenizeQuery(rawLower string) []string {
	if rawLower == "" {
		return nil
	}
	var tokens []string
	var cur strings.Builder
	seen := make(map[string]bool)
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tok := cur.String()
		cur.Reset()
		if !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, tok)
		}
	}
	for _, r := range rawLower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func hasExactToken(s, tokenLower string, splitCamel bool) bool {
	if isASCII(s) && isASCII(tokenLower) {
		return hasExactTokenASCII(s, tokenLower, splitCamel)
	}
	return hasExactTokenUnicode(s, tokenLower, splitCamel)
}

func hasExactTokenASCII(s, tokenLower string, splitCamel bool) bool {
	n := len(s)
	tokLen := len(tokenLower)
	start := -1
	for i := 0; i < n; i++ {
		c := s[i]
		if !isASCIIAlnum(c) {
			if start >= 0 && i-start == tokLen && equalFoldASCII(s[start:i], tokenLower) {
				return true
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		if splitCamel {
			prev := s[i-1]
			if (isASCIILower(prev) && isASCIIUpper(c)) ||
				(isASCIIUpper(prev) && isASCIIUpper(c) && i+1 < n && isASCIILower(s[i+1])) {
				if i-start == tokLen && equalFoldASCII(s[start:i], tokenLower) {
					return true
				}
				start = i
			}
		}
	}
	if start >= 0 && n-start == tokLen && equalFoldASCII(s[start:n], tokenLower) {
		return true
	}
	return false
}

func hasExactTokenUnicode(s, tokenLower string, splitCamel bool) bool {
	runes := []rune(s)
	n := len(runes)
	start := -1
	for i := 0; i < n; i++ {
		r := runes[i]
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if start >= 0 && strings.EqualFold(string(runes[start:i]), tokenLower) {
				return true
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		if splitCamel {
			prev := runes[i-1]
			if (unicode.IsLower(prev) && unicode.IsUpper(r)) ||
				(unicode.IsUpper(prev) && unicode.IsUpper(r) && i+1 < n && unicode.IsLower(runes[i+1])) {
				if strings.EqualFold(string(runes[start:i]), tokenLower) {
					return true
				}
				start = i
			}
		}
	}
	if start >= 0 && strings.EqualFold(string(runes[start:n]), tokenLower) {
		return true
	}
	return false
}

func containsFold(s, substrLower string) bool {
	if substrLower == "" {
		return true
	}
	if len(s) < len(substrLower) {
		return false
	}
	if isASCII(s) && isASCII(substrLower) {
		n := len(substrLower)
		for i := 0; i <= len(s)-n; i++ {
			if equalFoldASCII(s[i:i+n], substrLower) {
				return true
			}
		}
		return false
	}
	return strings.Contains(strings.ToLower(s), substrLower)
}

func equalFoldASCII(s, lower string) bool {
	for i := 0; i < len(lower); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func isASCIIAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func isASCIILower(c byte) bool { return c >= 'a' && c <= 'z' }
func isASCIIUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
