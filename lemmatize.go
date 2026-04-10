package collatinus

import (
	"regexp"
	"strings"
	"unicode"
)

// reWord matches a single Latin/Unicode word token.
var reWord = regexp.MustCompile(`[a-zA-ZÀ-ÿ\x{0100}-\x{024F}\x{0300}-\x{036F}]+`)

// baseRuneIndex returns the rune index in grq of the n-th base (non-combining)
// character (0-indexed). Combining marks (e.g. U+0306 from Communes) are
// skipped when counting. Returns len(grq) if n is out of range.
func baseRuneIndex(grq []rune, n int) int {
	base := 0
	for i, r := range grq {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if base == n {
			return i
		}
		base++
	}
	return len(grq)
}

// enclitics are suffixes to strip when a form cannot be lemmatized.
// Mirrors the suffixes map in LemCore constructor: ne, que, ue, ve, st.
var enclitics = []string{"ne", "que", "ue", "ve", "st"}

// assim applies the atone assimilation table to a, preferring the longest
// matching prefix (e.g. "adst" → "ast" wins over "ads" → "ass" for an
// input starting with "adst"). The longest-first iteration order is
// built once at load time.
//
// Diverges from Lemmat::assim, which iterates Qt's QMap in ascending
// key order: there "ads" is visited first and replaces "adsp"/"adsc"/
// "adst" inputs with "ass..." before the longer keys ever get a chance.
// Picking the longest match is deterministic and semantically correct.
func (l *Lemmatizer) assim(a string) string {
	for _, e := range l.assimsByKey {
		if strings.HasPrefix(a, e.key) {
			return e.val + a[len(e.key):]
		}
	}
	return a
}

// desassim applies the reverse atone assimilation table to a, preferring
// the longest matching (assimilated) prefix. See assim for notes on the
// divergence from Lemmat::desassim's QMap iteration order.
func (l *Lemmatizer) desassim(a string) string {
	for _, e := range l.assimsByVal {
		if strings.HasPrefix(a, e.val) {
			return e.key + a[len(e.val):]
		}
	}
	return a
}

// assimq is the quantity-marked counterpart of assim. Used to rewrite the
// marked result form after a desassim fallback so the returned grq matches
// the caller's input. Mirrors LemCore::assimq, with the same longest-first
// divergence described on assim.
func (l *Lemmatizer) assimq(a string) string {
	for _, e := range l.assimsqByKey {
		if strings.HasPrefix(a, e.key) {
			return e.val + a[len(e.key):]
		}
	}
	return a
}

// desassimq is the quantity-marked counterpart of desassim. Used to
// rewrite the marked result form after an assim fallback. Mirrors
// LemCore::desassimq, with the same longest-first divergence.
func (l *Lemmatizer) desassimq(a string) string {
	for _, e := range l.assimsqByVal {
		if strings.HasPrefix(a, e.val) {
			return e.key + a[len(e.val):]
		}
	}
	return a
}

// decontracte tries to expand a contracted form.
// Mirrors Lemmat::decontracte.
func (l *Lemmatizer) decontracte(d string) string {
	for suffix, expanded := range l.contractions {
		if strings.HasSuffix(d, suffix) {
			return d[:len(d)-len(suffix)] + expanded
		}
	}
	return d
}

// lemmatizeRaw is the core lemmatization function.
// It applies deramise to the form, then tries:
// 1. irregular forms
// 2. radical+desinence combinations
// Mirrors Lemmat::lemmatise.
func (l *Lemmatizer) lemmatizeRaw(form string) map[*Lemma][]Analysis {
	frMorphos := l.morphos["fr"] // for bounds checking
	// Compute vowel counts from original form (before deramise)
	lower := strings.ToLower(form)
	cntV := strings.Count(lower, "v")
	cntAe := strings.Count(lower, "\u00e6") // æ
	cntOe := strings.Count(lower, "\u0153") // œ
	// subtract trailing æ (matches C++ behaviour)
	if strings.HasSuffix(lower, "\u00e6") {
		cntAe--
	}

	form = Deramise(form)
	result := make(map[*Lemma][]Analysis)

	// 1. Check irregular forms
	if irregs, ok := l.irregs[form]; ok {
		for _, irr := range irregs {
			for _, mn := range irr.Morphos {
				an := Analysis{
					FormWithMarks:     irr.Grq,
					MorphoDescription: l.Morpho(mn),
					MorphoIndex:       mn,
				}
				result[irr.Lemma] = append(result[irr.Lemma], an)
			}
		}
	}

	// 2. Split at each rune boundary: form[:i] = stem, form[i:] = ending
	runes := []rune(form)
	for i := 0; i <= len(runes); i++ {
		r := string(runes[:i])
		d := string(runes[i:])

		rads, hasRad := l.radicals[r]
		if !hasRad {
			continue
		}

		// ii/ī ambiguity: try inserting an extra 'i'
		// Cases:
		// 1. d empty and r ends with 'i'
		// 2. d starts with 'i' but not "ii", and r does not end with 'i'
		// 3. r ends with 'i' but not "ii", and d does not start with 'i'
		rEndsI := len(r) > 0 && r[len(r)-1] == 'i'
		rEndsII := strings.HasSuffix(r, "ii")
		dStartsI := len(d) > 0 && d[0] == 'i'
		dStartsII := strings.HasPrefix(d, "ii")

		needDoubleI := (len(d) == 0 && rEndsI) ||
			(dStartsI && !dStartsII && !rEndsI) ||
			(rEndsI && !rEndsII && !dStartsI)

		if needDoubleI {
			nf := r + "i" + d
			nm := l.lemmatizeRaw(nf)
			// Remove the extra 'i' we inserted from each returned grq.
			// The inserted 'i' is the (rLen)-th base character (0-indexed)
			// in the marked form. We must skip combining marks (e.g. the
			// combining breve U+0306 added by Communes) when counting.
			rLen := len([]rune(r))
			for nl, lsl := range nm {
				for k := range lsl {
					grq := []rune(lsl[k].FormWithMarks)
					idx := baseRuneIndex(grq, rLen)
					if idx < len(grq) {
						lsl[k].FormWithMarks = string(grq[:idx]) + string(grq[idx+1:])
					}
				}
				result[nl] = append(result[nl], lsl...)
			}
		}

		des, hasDes := l.desinences[d]
		if !hasDes {
			continue
		}

		for _, rad := range rads {
			lemma := rad.Lemma
			for _, de := range des {
				if de.Model != lemma.model {
					continue
				}
				if de.RadNum != rad.Num {
					continue
				}
				if lemma.isExclusiveIrreg(de.MorphoNum) {
					continue
				}
				if de.MorphoNum < 1 || de.MorphoNum >= len(frMorphos) {
					continue
				}

				// Vowel-count consistency check (mirrors C++ lemmatise())
				radGrqLower := strings.ToLower(rad.Grq)
				desGrqLower := strings.ToLower(de.Grq)
				cOK := (cntV == 0) || (cntV == strings.Count(radGrqLower, "v")+strings.Count(desGrqLower, "v"))
				cOK = cOK && ((cntOe == 0) || (cntOe == strings.Count(radGrqLower, "\u014de")))                                         // ōe
				cOK = cOK && ((cntAe == 0) || (cntAe == strings.Count(radGrqLower, "\u0101e")+strings.Count(radGrqLower, "pr\u0103e"))) // āe + prăe
				if !cOK {
					continue
				}

				an := Analysis{
					FormWithMarks:     rad.Grq + de.Grq,
					MorphoDescription: l.Morpho(de.MorphoNum),
					MorphoIndex:       de.MorphoNum,
				}
				result[lemma] = append(result[lemma], an)
			}
		}
	}

	return result
}

// lemmatizeM implements the full lemmatization with all fallbacks.
// Mirrors LemCore::lemmatiseM using recursive etapes logic.
// etape=0 is the entry point; higher etapes are more basic.
func (l *Lemmatizer) lemmatizeM(form string, sentenceStart bool) map[*Lemma][]Analysis {
	return l.lemmatizeMEtape(form, sentenceStart, 0)
}

// lemmatizeMEtape implements the etapes-based lemmatization.
// etape ranges from 0 (most transformations) to 4+ (terminal/raw).
func (l *Lemmatizer) lemmatizeMEtape(form string, sentenceStart bool, etape int) map[*Lemma][]Analysis {
	if form == "" {
		return nil
	}

	// Terminal condition: etape > 3 → raw lemmatize + sentence-start fallback
	if etape > 3 {
		mm := l.lemmatizeRaw(form)
		if sentenceStart && len(form) > 0 && unicode.IsUpper([]rune(form)[0]) {
			nf := strings.ToLower(form)
			for nl, lsl := range l.lemmatizeMEtape(nf, false, 4) {
				if mm == nil {
					mm = make(map[*Lemma][]Analysis)
				}
				mm[nl] = append(mm[nl], lsl...)
			}
		}
		return mm
	}

	// First try deeper (more basic) steps
	mm := l.lemmatizeMEtape(form, sentenceStart, etape+1)

	switch etape {
	case 3:
		// Contraction expansion (always tried, merged with base results)
		fd := l.decontracte(form)
		if fd != form {
			for nl, lsl := range l.lemmatizeMEtape(fd, sentenceStart, 4) {
				if mm == nil {
					mm = make(map[*Lemma][]Analysis)
				}
				mm[nl] = append(mm[nl], lsl...)
			}
		}

	case 2:
		// Assimilation and deassimilation. The input form is probed against
		// the atone assims table; if a transformation applies, the recursive
		// result's marked form is rewritten with the inverse *q operation so
		// the returned grq reflects the caller's input. Mirrors LemCore::lemmatiseM
		// case 2 (which uses mm.insert to replace rather than append).
		fa := l.assim(form)
		if fa != form {
			nmm := l.lemmatizeMEtape(fa, sentenceStart, 3)
			for nl, lsl := range nmm {
				for k := range lsl {
					lsl[k].FormWithMarks = l.desassimq(lsl[k].FormWithMarks)
				}
				if mm == nil {
					mm = make(map[*Lemma][]Analysis)
				}
				mm[nl] = lsl
			}
			return mm
		}
		fd := l.desassim(form)
		if fd != form {
			nmm := l.lemmatizeMEtape(fd, sentenceStart, 3)
			for nl, lsl := range nmm {
				for k := range lsl {
					lsl[k].FormWithMarks = l.assimq(lsl[k].FormWithMarks)
				}
				if mm == nil {
					mm = make(map[*Lemma][]Analysis)
				}
				mm[nl] = lsl
			}
			return mm
		}

	case 1:
		// Suffixes/enclitics (only when no results yet)
		if len(mm) == 0 {
			for _, suf := range enclitics {
				if len(mm) > 0 {
					break
				}
				if strings.HasSuffix(form, suf) {
					sf := form[:len(form)-len(suf)]
					// special case: "st" suffix → try also with trailing "s"
					if suf == "st" {
						mm = l.lemmatizeMEtape(sf+"s", sentenceStart, 1)
					} else {
						mm = l.lemmatizeMEtape(sf, sentenceStart, 1)
					}
				}
			}
		}

	case 0:
		// Capitalize first letter for proper-noun fallback (only when no results)
		if len(mm) == 0 && len(form) > 0 && unicode.IsLower([]rune(form)[0]) {
			runes := []rune(form)
			runes[0] = unicode.ToUpper(runes[0])
			return l.lemmatizeMEtape(string(runes), false, 1)
		}
	}

	return mm
}

// lemmatizeText tokenizes text and lemmatizes each word token.
func (l *Lemmatizer) lemmatizeText(text string) []LemmatizationResult {
	// Find all word tokens using a simple Unicode letter scanner
	var results []LemmatizationResult
	rePunct := regexp.MustCompile(`[.!?;:]`)
	tokens := reWord.FindAllString(text, -1)
	// Track sentence-start position by checking punctuation before each token
	positions := reWord.FindAllStringIndex(text, -1)

	for ti, token := range tokens {
		debPhr := ti == 0
		if !debPhr && positions[ti][0] > 0 {
			before := text[:positions[ti][0]]
			debPhr = rePunct.MatchString(before[max(0, len(before)-5):])
		}
		analyses := l.lemmatizeM(token, debPhr)
		results = append(results, LemmatizationResult{
			Token:    token,
			Analyses: analyses,
		})
	}
	return results
}
