package rag

import (
	"strings"
	"unicode"
)

const (
	// ChunkTarget is the soft maximum chunk size in characters (runes).
	ChunkTarget = 800
	// ChunkOverlap is the approximate number of characters repeated at the start of the next chunk.
	ChunkOverlap = 120
)

// abbreviations (folded) after which a period does not end a sentence.
var abbreviations = toSet("dr", "prof", "doc", "doç", "no", "sn", "sk", "mah", "cad", "vb", "vs", "vd", "ör", "or", "bkz", "yy", "ltd", "sti", "şti",
	"st", "mr", "mrs", "ms", "etc", "fig", "ca", "approx", "jan", "feb", "mar", "apr", "jun", "jul", "aug", "sep", "oct", "nov", "dec", "tel", "km", "m", "ha", "kg")

// SplitSentences splits text into sentences. Paragraph breaks always split. A period/!/?/… followed by space
// and an upper-case letter, digit or quote ends a sentence unless the previous word is an abbreviation, a single
// letter, or a number ("3.5", "12.10.2026").
func SplitSentences(text string) []string {
	var out []string
	for _, para := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		out = append(out, splitParagraph(para)...)
	}
	return out
}

func splitParagraph(p string) []string {
	rs := []rune(p)
	var out []string
	start := 0
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r != '.' && r != '!' && r != '?' && r != '…' {
			continue
		}
		// swallow repeated terminators and closing quotes/brackets
		j := i + 1
		for j < len(rs) && (rs[j] == '.' || rs[j] == '!' || rs[j] == '?' || rs[j] == '"' || rs[j] == '\'' || rs[j] == ')' || rs[j] == '”' || rs[j] == '’') {
			j++
		}
		if j >= len(rs) {
			break
		}
		if !unicode.IsSpace(rs[j]) {
			i = j - 1
			continue
		}
		k := j
		for k < len(rs) && unicode.IsSpace(rs[k]) {
			k++
		}
		if k >= len(rs) {
			break
		}
		next := rs[k]
		if !(unicode.IsUpper(next) || unicode.IsDigit(next) || next == '"' || next == '“' || next == '\'') {
			i = j - 1
			continue
		}
		if r == '.' && endsWithAbbreviation(rs[start:i]) {
			i = j - 1
			continue
		}
		out = append(out, strings.TrimSpace(string(rs[start:j])))
		start = k
		i = k - 1
	}
	if start < len(rs) {
		if s := strings.TrimSpace(string(rs[start:])); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func endsWithAbbreviation(prefix []rune) bool {
	end := len(prefix)
	st := end
	for st > 0 && !unicode.IsSpace(prefix[st-1]) {
		st--
	}
	w := string(prefix[st:end])
	if w == "" {
		return false
	}
	if len([]rune(w)) == 1 {
		return true // initials ("A. Yılmaz") and enumerations
	}
	hasLetter, hasDigit := false, false
	for _, r := range w {
		if unicode.IsDigit(r) {
			hasDigit = true
		} else if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	if hasDigit && !hasLetter {
		return true // numbers and dates: "3.5", "12.10.2026", "No. 5"
	}
	return abbreviations[foldTR(strings.Trim(w, ".,;:()\"'"))]
}

// Chunk splits text into overlapping, sentence-aware chunks of about ChunkTarget characters.
// It is deterministic: same input, same output. Empty/blank text yields no chunks.
func Chunk(text string) []string {
	return ChunkSized(text, ChunkTarget, ChunkOverlap)
}

// ChunkSized is Chunk with explicit sizes (tests).
func ChunkSized(text string, target, overlap int) []string {
	sents := expandLong(SplitSentences(text), target)
	if len(sents) == 0 {
		return nil
	}
	var chunks []string
	var cur []string
	curLen := 0
	flush := func() {
		if len(cur) > 0 {
			chunks = append(chunks, strings.Join(cur, " "))
		}
	}
	for _, s := range sents {
		sl := len([]rune(s))
		if curLen > 0 && curLen+1+sl > target {
			flush()
			// carry trailing sentences totalling <= overlap into the next chunk
			var carry []string
			carryLen := 0
			for i := len(cur) - 1; i >= 0; i-- {
				l := len([]rune(cur[i]))
				if carryLen+l > overlap {
					break
				}
				carry = append([]string{cur[i]}, carry...)
				carryLen += l + 1
			}
			cur, curLen = carry, carryLen
			if len(carry) == len(sentsOf(chunks[len(chunks)-1])) { // nothing new would fit; avoid infinite repetition
				cur, curLen = nil, 0
			}
		}
		cur = append(cur, s)
		curLen += sl + 1
	}
	if len(cur) > 0 {
		last := strings.Join(cur, " ")
		// do not emit a tail that is entirely contained in the previous chunk
		if len(chunks) == 0 || !strings.Contains(chunks[len(chunks)-1], last) {
			chunks = append(chunks, last)
		}
	}
	return chunks
}

func sentsOf(chunk string) []string { return SplitSentences(chunk) }

// expandLong breaks sentences longer than target at word boundaries.
func expandLong(sents []string, target int) []string {
	var out []string
	for _, s := range sents {
		rs := []rune(s)
		for len(rs) > target {
			cut := target
			for cut > target/2 && !unicode.IsSpace(rs[cut]) {
				cut--
			}
			if cut <= target/2 {
				cut = target
			}
			out = append(out, strings.TrimSpace(string(rs[:cut])))
			rs = []rune(strings.TrimSpace(string(rs[cut:])))
		}
		if len(rs) > 0 {
			out = append(out, string(rs))
		}
	}
	return out
}

// TokenEstimate approximates the token count (about 4 characters per token for Turkish/English text).
func TokenEstimate(s string) int { return (len([]rune(s)) + 3) / 4 }
