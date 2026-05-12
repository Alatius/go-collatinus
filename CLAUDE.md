# go-collatinus

Pure Go library and REST server for Latin morphological analysis and lemmatization, ported from the C++/Qt [Collatinus](https://github.com/biblissima/collatinus) project. Given a Latin word, it identifies possible dictionary headwords (lemmas) and their morphological descriptions (case, number, tense, mood, etc.). It can also generate full inflection tables.

The original C++ source is at /Users/johan/Code/collatinus and should be used as reference when investigating bugs or porting missing features.

## Build and run

```bash
# Build and run tests
go build ./... && go test ./...

# Run the server
go build -o bin/collatinus ./cmd/server && ./bin/collatinus -data ./data -addr :8080
```

No external dependencies (pure Go, module `github.com/cours-de-latin/collatinus`).

## API endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/lemmatize?form=amis&lang=en` | GET | Lemmatize a single word |
| `/api/lemmatize/text` | POST | Lemmatize a full text (JSON body: `{"text":"...", "lang":"fr"}`) |
| `/api/inflection?lemma=amo&lang=en` | GET | Full inflection table |
| `/api/languages` | GET | List available translation languages |

All endpoints accept `lang=` parameter (default: `fr`). Available languages: fr, de, en, es, it, pt, ca, gl.

Lemmatization endpoints filter out extended-lexicon (`lem_ext.la`) results when main-lexicon results exist, matching the C++ Collatinus behavior. Pass `extended=true` (query param for GET, JSON field for POST) to include all results.

## Code structure

| File | Purpose |
|---|---|
| `collatinus.go` | Public API: `Lemmatizer` struct, `LemmatizeWord()`, `LemmatizeText()`, `InflectionTable()` |
| `lemmatize.go` | Core lemmatization algorithm (radical+desinence matching) |
| `loader.go` | Parses all Collatinus data files (models, lexicons, irregulars, variables) |
| `model.go` | `Model`, `Desinence`, `PartOfSpeech` types |
| `lemma.go` | `Lemma`, `Radical`, `Irreg` types |
| `normalize.go` | Text normalization: `Deramise` (j/v->i/u), `Atone` (strip diacritics), `Communes` |
| `flexion.go` | Inflection table generation |
| `analysis.go` | Analysis result types |
| `cmd/server/main.go` | HTTP server with JSON API |

## Data files

All in `data/`, same format as the original C++ project:

| File | Content |
|---|---|
| `lemmes.la` | ~24,000 Latin headwords with radical rules and model references |
| `lem_ext.la` | Extended lexicon entries |
| `lemmes.fr/en/de/...` | Translations per language |
| `modeles.la` | ~141 inflection models (paradigms) with variable definitions (`$lupus`, `$uita`, etc.) |
| `morphos.fr/en/...` | 416 morphological descriptions (1-based indexed) |
| `irregs.la` | Irregular inflected forms |
| `assimilations.la` | Prefix-assimilation table (e.g., ad+fero -> affero) |
| `contractions.la` | Perfect-contraction expansions (e.g., amasti -> amavisti) |

## Key implementation notes

- `substituteVars` in `loader.go` expands `$variable` references in model definitions. When a variable has a prefix (e.g., `ānd$lupus`), the prefix is propagated to every semicolon-separated element, mirroring the C++ regex logic in `Modele::Modele`.
- Morphological indices are 1-based and defined in `morphos.*` files. Models map index ranges to radical numbers and desinence strings.
- Lemmatization works by trying all known desinences against the input form, checking if the remainder matches a known radical for the corresponding model.
- Enclitics (-que, -ne, -ve, -st) are stripped and tried separately.
- Assimilated prefixes (e.g., aff- -> adf-) and contracted perfects (e.g., amasti -> amavisti) are expanded before lookup.
