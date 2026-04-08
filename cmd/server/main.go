// Command server exposes the Collatinus lemmatizer as a JSON REST API.
//
// Endpoints:
//
//	GET  /api/lemmatize?form=<word>[&sentence_start=true][&lang=fr]
//	POST /api/lemmatize/text   body: {"text":"...", "lang":"fr"}
//	GET  /api/inflection?lemma=<key>[&lang=fr]
//	GET  /api/languages
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"

	collatinus "github.com/cours-de-latin/collatinus"
)

// ---- JSON response types ------------------------------------------------

type lemmaJSON struct {
	Key         string `json:"key"`
	Form        string `json:"form"`
	POS         string `json:"pos"`
	MorphoInfo  string `json:"morpho_info"`
	HomonymNum  int    `json:"homonym_num,omitempty"`
	Translation string `json:"translation,omitempty"`
}

type formJSON struct {
	FormWithMarks     string `json:"form_with_marks"`
	MorphoDescription string `json:"morpho_description"`
	MorphoIndex       int    `json:"morpho_index"`
}

type analysisJSON struct {
	Lemma  lemmaJSON  `json:"lemma"`
	Forms  []formJSON `json:"forms"`
}

type lemmatizeWordResponse struct {
	Form     string         `json:"form"`
	Analyses []analysisJSON `json:"analyses"`
}

type tokenResultJSON struct {
	Token    string         `json:"token"`
	Analyses []analysisJSON `json:"analyses"`
}

type lemmatizeTextResponse struct {
	Results []tokenResultJSON `json:"results"`
}

type inflectionResponse struct {
	Lemma  *lemmaJSON          `json:"lemma"`
	Cells  map[string][]string `json:"cells"`
	Labels map[string]string   `json:"labels"`
}

type languagesResponse struct {
	Languages map[string]string `json:"languages"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// ---- helpers ------------------------------------------------------------

func posName(p collatinus.PartOfSpeech) string {
	switch p {
	case collatinus.POSNoun:
		return "noun"
	case collatinus.POSVerb:
		return "verb"
	case collatinus.POSAdjective:
		return "adjective"
	case collatinus.POSPronoun:
		return "pronoun"
	case collatinus.POSAdverb:
		return "adverb"
	case collatinus.POSConjunction:
		return "conjunction"
	case collatinus.POSExclamation:
		return "exclamation"
	case collatinus.POSInterjection:
		return "interjection"
	case collatinus.POSNumeral:
		return "numeral"
	case collatinus.POSPreposition:
		return "preposition"
	default:
		return "unknown"
	}
}

func parseLang(r *http.Request) string {
	if lang := r.URL.Query().Get("lang"); lang != "" {
		return lang
	}
	return "fr"
}

func toLemmaJSON(l *collatinus.Lemma, lang string) lemmaJSON {
	return lemmaJSON{
		Key:         l.Key,
		Form:        l.Gr,
		POS:         posName(l.POS),
		MorphoInfo:  l.IndMorphLang(lang),
		HomonymNum:  l.HomonymNum,
		Translation: l.Translation(lang),
	}
}

func toAnalysesJSON(lem *collatinus.Lemmatizer, analyses map[*collatinus.Lemma][]collatinus.Analysis, lang string) []analysisJSON {
	out := make([]analysisJSON, 0, len(analyses))
	for lemma, forms := range analyses {
		fj := make([]formJSON, 0, len(forms))
		for _, f := range forms {
			fj = append(fj, formJSON{
				FormWithMarks:     f.FormWithMarks,
				MorphoDescription: lem.MorphoLang(f.MorphoIndex, lang),
				MorphoIndex:       f.MorphoIndex,
			})
		}
		// sort forms by morpho index for deterministic output
		sort.Slice(fj, func(i, j int) bool {
			return fj[i].MorphoIndex < fj[j].MorphoIndex
		})
		lj := toLemmaJSON(lemma, lang)
		out = append(out, analysisJSON{Lemma: lj, Forms: fj})
	}
	// sort by lemma key for deterministic output
	sort.Slice(out, func(i, j int) bool {
		return out[i].Lemma.Key < out[j].Lemma.Key
	})
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// ---- handlers -----------------------------------------------------------

func handleLemmatizeWord(lem *collatinus.Lemmatizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		form := r.URL.Query().Get("form")
		if form == "" {
			writeError(w, http.StatusBadRequest, "missing 'form' query parameter")
			return
		}
		sentenceStart, _ := strconv.ParseBool(r.URL.Query().Get("sentence_start"))
		extended, _ := strconv.ParseBool(r.URL.Query().Get("extended"))
		lang := parseLang(r)

		analyses := lem.LemmatizeWord(form, sentenceStart)
		if !extended {
			analyses = collatinus.FilterExtended(analyses)
		}
		status := http.StatusOK
		if len(analyses) == 0 {
			status = http.StatusNotFound
		}
		writeJSON(w, status, lemmatizeWordResponse{
			Form:     form,
			Analyses: toAnalysesJSON(lem, analyses, lang),
		})
	}
}

func handleLemmatizeText(lem *collatinus.Lemmatizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST required")
			return
		}
		var body struct {
			Text     string `json:"text"`
			Lang     string `json:"lang"`
			Extended bool   `json:"extended"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			writeError(w, http.StatusBadRequest, "body must be JSON with a non-empty 'text' field")
			return
		}
		lang := parseLang(r)
		if lang == "fr" && body.Lang != "" {
			lang = body.Lang
		}

		results := lem.LemmatizeText(body.Text)
		out := make([]tokenResultJSON, 0, len(results))
		for _, res := range results {
			analyses := res.Analyses
			if !body.Extended {
				analyses = collatinus.FilterExtended(analyses)
			}
			out = append(out, tokenResultJSON{
				Token:    res.Token,
				Analyses: toAnalysesJSON(lem, analyses, lang),
			})
		}
		writeJSON(w, http.StatusOK, lemmatizeTextResponse{Results: out})
	}
}

func handleInflection(lem *collatinus.Lemmatizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		key := r.URL.Query().Get("lemma")
		if key == "" {
			writeError(w, http.StatusBadRequest, "missing 'lemma' query parameter")
			return
		}
		lemma := lem.Lemma(key)
		if lemma == nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("lemma %q not found", key))
			return
		}
		lang := parseLang(r)
		table := lem.InflectionTable(lemma)

		cells := make(map[string][]string, len(table.Cells))
		labels := make(map[string]string, len(table.Cells))
		for idx, forms := range table.Cells {
			key := strconv.Itoa(idx)
			cells[key] = forms
			labels[key] = lem.MorphoLang(idx, lang)
		}
		lj := toLemmaJSON(lemma, lang)
		writeJSON(w, http.StatusOK, inflectionResponse{Lemma: &lj, Cells: cells, Labels: labels})
	}
}

func handleLanguages(lem *collatinus.Lemmatizer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "GET required")
			return
		}
		writeJSON(w, http.StatusOK, languagesResponse{Languages: lem.Languages()})
	}
}

// ---- main ---------------------------------------------------------------

func main() {
	dataDir := flag.String("data", "data", "path to Collatinus data directory")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	log.Printf("loading data from %s …", *dataDir)
	lem, err := collatinus.New(*dataDir)
	if err != nil {
		log.Fatalf("failed to load data: %v", err)
	}
	log.Println("data loaded")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/lemmatize/text", handleLemmatizeText(lem))
	mux.HandleFunc("/api/lemmatize", handleLemmatizeWord(lem))
	mux.HandleFunc("/api/inflection", handleInflection(lem))
	mux.HandleFunc("/api/languages", handleLanguages(lem))

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
