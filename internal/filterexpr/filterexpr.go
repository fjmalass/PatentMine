package filterexpr

import (
	"fmt"
	"strconv"
	"strings"

	"patentmine/internal/domain"
)

type Field string

const (
	FieldTag        Field = "tag"
	FieldState      Field = "state"
	FieldFetchState Field = "fetch_state"
	FieldClass      Field = "class"
	FieldSearch     Field = "search"
	FieldInventor   Field = "inventor"
	FieldAssignee   Field = "assignee"
	FieldCountry    Field = "country"
)

const LegacySyntaxHint = "legacy filter syntax is no longer supported; use field:value expressions like :filter state:active"

type fieldSpec struct {
	name    Field
	aliases []string
	parse   func(string) (TermExpr, error)
}

var fieldSpecs = []fieldSpec{
	{name: FieldSearch, aliases: []string{"find"}, parse: parseSearchTerm},
	{name: FieldTag, parse: parseTagTerm},
	{name: FieldState, aliases: []string{"status", "review_state"}, parse: parseStateTerm},
	{name: FieldFetchState, aliases: []string{"fetch", "fetched"}, parse: parseFetchStateTerm},
	{name: FieldClass, aliases: []string{"classification", "cpc", "ipc"}, parse: parseClassTerm},
	{name: FieldInventor, aliases: []string{"inv"}, parse: parseInventorTerm},
	{name: FieldAssignee, aliases: []string{"owner"}, parse: parseAssigneeTerm},
	{name: FieldCountry, parse: parseCountryTerm},
}

var fieldByName = func() map[string]fieldSpec {
	lookup := make(map[string]fieldSpec, len(fieldSpecs))
	for _, spec := range fieldSpecs {
		lookup[string(spec.name)] = spec
		for _, alias := range spec.aliases {
			lookup[alias] = spec
		}
	}
	return lookup
}()

type Expr interface{ exprNode() }

type AndExpr struct{ Left, Right Expr }
type OrExpr struct{ Left, Right Expr }
type NotExpr struct{ Child Expr }

type TermExpr struct {
	Field      Field
	Value      string
	Class      ClassificationPattern
	Inventor   ValuePattern
	Assignee   ValuePattern
	Country    ValuePattern
	State      domain.ReviewState
	FetchState domain.FetchState
}

type ClassificationPattern struct {
	Raw      string
	Wildcard bool
}

type ValuePattern struct {
	Raw      string
	Wildcard bool
}

func (AndExpr) exprNode()  {}
func (OrExpr) exprNode()   {}
func (NotExpr) exprNode()  {}
func (TermExpr) exprNode() {}

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenWord
	tokenLParen
	tokenRParen
	tokenAnd
	tokenOr
	tokenNot
)

type token struct {
	kind tokenKind
	text string
}

func Parse(input string) (Expr, error) {
	tokens := tokenize(input)
	p := parser{tokens: tokens}
	expr, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokenEOF {
		return nil, fmt.Errorf("unexpected token %q", tok.text)
	}
	return expr, nil
}

func CanonicalString(expr Expr) string {
	return renderExpr(expr, 0)
}

func ValidateProjectScope(expr Expr, projectActive bool) error {
	if projectActive {
		return nil
	}
	if UsesField(expr, FieldState) {
		return fmt.Errorf("state filters require an active project")
	}
	if UsesField(expr, FieldTag) {
		return fmt.Errorf("tag filters require an active project")
	}
	return nil
}

func UsesField(expr Expr, field Field) bool {
	switch e := expr.(type) {
	case AndExpr:
		return UsesField(e.Left, field) || UsesField(e.Right, field)
	case OrExpr:
		return UsesField(e.Left, field) || UsesField(e.Right, field)
	case NotExpr:
		return UsesField(e.Child, field)
	case TermExpr:
		return e.Field == field
	default:
		return false
	}
}

func RemoveField(expr Expr, field Field) Expr {
	switch e := expr.(type) {
	case nil:
		return nil
	case AndExpr:
		return join(false, RemoveField(e.Left, field), RemoveField(e.Right, field), true)
	case OrExpr:
		return join(false, RemoveField(e.Left, field), RemoveField(e.Right, field), false)
	case NotExpr:
		child := RemoveField(e.Child, field)
		if child == nil {
			return nil
		}
		return NotExpr{Child: child}
	case TermExpr:
		if e.Field == field {
			return nil
		}
		return e
	default:
		return nil
	}
}

func JoinAnd(left, right Expr) Expr {
	return join(false, left, right, true)
}

func ParseClassTerm(value string) (TermExpr, error) { return parseClassTerm(value) }

func tokenize(input string) []token {
	var tokens []token
	for i := 0; i < len(input); {
		switch input[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case '(':
			tokens = append(tokens, token{kind: tokenLParen, text: "("})
			i++
		case ')':
			tokens = append(tokens, token{kind: tokenRParen, text: ")"})
			i++
		default:
			start := i
			inQuote := false
			for i < len(input) {
				switch input[i] {
				case '"':
					inQuote = !inQuote
					i++
				case '(', ')':
					if !inQuote {
						goto tokenDone
					}
					i++
				default:
					if !inQuote && isSpace(input[i]) {
						goto tokenDone
					}
					i++
				}
			}
		tokenDone:
			text := input[start:i]
			switch strings.ToLower(text) {
			case "and":
				tokens = append(tokens, token{kind: tokenAnd, text: text})
			case "or":
				tokens = append(tokens, token{kind: tokenOr, text: text})
			case "not":
				tokens = append(tokens, token{kind: tokenNot, text: text})
			default:
				tokens = append(tokens, token{kind: tokenWord, text: text})
			}
		}
	}
	tokens = append(tokens, token{kind: tokenEOF})
	return tokens
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) parseExpr() (Expr, error) { return p.parseOr() }

func (p *parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = OrExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenAnd {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = AndExpr{Left: left, Right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (Expr, error) {
	if p.peek().kind == tokenNot {
		p.next()
		child, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return NotExpr{Child: child}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (Expr, error) {
	switch tok := p.peek(); tok.kind {
	case tokenLParen:
		p.next()
		expr, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokenRParen {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.next()
		return expr, nil
	case tokenWord:
		p.next()
		return parseTerm(tok.text)
	case tokenEOF:
		return nil, fmt.Errorf("missing filter expression")
	default:
		return nil, fmt.Errorf("unexpected token %q", tok.text)
	}
}

func (p *parser) peek() token { return p.tokens[p.pos] }

func (p *parser) next() token {
	tok := p.tokens[p.pos]
	p.pos++
	return tok
}

func parseTerm(raw string) (Expr, error) {
	fieldName, value, ok := strings.Cut(raw, ":")
	if !ok {
		return nil, fmt.Errorf(LegacySyntaxHint)
	}
	if value == "" {
		return nil, fmt.Errorf("missing value after %q", fieldName+":")
	}
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return nil, fmt.Errorf("invalid quoted value %q", value)
		}
		value = unquoted
	}
	spec, ok := fieldByName[strings.ToLower(fieldName)]
	if !ok {
		return nil, fmt.Errorf("unknown filter field %q", fieldName)
	}
	term, err := spec.parse(value)
	if err != nil {
		return nil, err
	}
	term.Field = spec.name
	return term, nil
}

func parseTagTerm(value string) (TermExpr, error) {
	if err := domain.ValidateTagName(value); err != nil {
		return TermExpr{}, err
	}
	return TermExpr{Field: FieldTag, Value: value}, nil
}

func parseSearchTerm(value string) (TermExpr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TermExpr{}, fmt.Errorf("search filter requires a value")
	}
	return TermExpr{Field: FieldSearch, Value: value}, nil
}

func parseStateTerm(value string) (TermExpr, error) {
	state, err := domain.ParseReviewState(strings.ToLower(value))
	if err != nil {
		return TermExpr{}, err
	}
	if state == domain.ReviewStateNone {
		return TermExpr{}, fmt.Errorf("state filter requires a non-empty review state")
	}
	return TermExpr{Field: FieldState, Value: string(state), State: state}, nil
}

// fetchStateAliases maps human-friendly terms to their canonical FetchState value.
var fetchStateAliases = map[string]domain.FetchState{
	"stub":     domain.FetchStub,
	"cached":   domain.FetchCached,
	"full":     domain.FetchCached,
	"complete": domain.FetchCached,
	"fetched":  domain.FetchCached,
}

func parseFetchStateTerm(value string) (TermExpr, error) {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		return TermExpr{}, fmt.Errorf("fetch_state filter requires a value")
	}
	fs, ok := fetchStateAliases[key]
	if !ok {
		return TermExpr{}, fmt.Errorf("unknown fetch state %q: use stub, cached (or full/complete/fetched)", value)
	}
	return TermExpr{Field: FieldFetchState, Value: string(fs), FetchState: fs}, nil
}

func parseClassTerm(value string) (TermExpr, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return TermExpr{}, fmt.Errorf("classification filter requires a value")
	}
	if strings.HasPrefix(normalized, "/") && strings.HasSuffix(normalized, "/") {
		return TermExpr{}, unsupportedClassPatternError()
	}
	if strings.ContainsAny(normalized, "[]?\\+{}|^$") {
		return TermExpr{}, unsupportedClassPatternError()
	}
	return TermExpr{Field: FieldClass, Value: normalized, Class: ClassificationPattern{Raw: normalized, Wildcard: strings.Contains(normalized, "*")}}, nil
}

func parseInventorTerm(value string) (TermExpr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TermExpr{}, fmt.Errorf("inventor filter requires a value")
	}
	return TermExpr{Field: FieldInventor, Value: value, Inventor: ValuePattern{Raw: value, Wildcard: strings.Contains(value, "*")}}, nil
}

func parseAssigneeTerm(value string) (TermExpr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TermExpr{}, fmt.Errorf("assignee filter requires a value")
	}
	return TermExpr{Field: FieldAssignee, Value: value, Assignee: ValuePattern{Raw: value, Wildcard: strings.Contains(value, "*")}}, nil
}

// countryAliases maps common names and codes to their ISO-3166 patent-office code (uppercase).
var countryAliases = map[string]string{
	"us": "US", "usa": "US", "united states": "US", "united_states": "US",
	"ep": "EP", "epo": "EP", "europe": "EP", "european patent office": "EP",
	"wo": "WO", "pct": "WO", "wipo": "WO", "world": "WO",
	"cn": "CN", "china": "CN", "chinese": "CN",
	"jp": "JP", "japan": "JP", "japanese": "JP",
	"de": "DE", "germany": "DE", "german": "DE",
	"gb": "GB", "uk": "GB", "united kingdom": "GB", "great britain": "GB",
	"fr": "FR", "france": "FR", "french": "FR",
	"kr": "KR", "korea": "KR", "south korea": "KR", "korean": "KR",
	"ca": "CA", "canada": "CA", "canadian": "CA",
	"au": "AU", "australia": "AU", "australian": "AU",
	"in": "IN", "india": "IN", "indian": "IN",
	"br": "BR", "brazil": "BR", "brazilian": "BR",
	"ru": "RU", "russia": "RU", "russian": "RU",
	"tw": "TW", "taiwan": "TW", "taiwanese": "TW",
	"mx": "MX", "mexico": "MX", "mexican": "MX",
	"es": "ES", "spain": "ES", "spanish": "ES",
	"it": "IT", "italy": "IT", "italian": "IT",
	"nl": "NL", "netherlands": "NL", "dutch": "NL",
	"se": "SE", "sweden": "SE", "swedish": "SE",
	"fi": "FI", "finland": "FI", "finnish": "FI",
	"ch": "CH", "switzerland": "CH", "swiss": "CH",
	"il": "IL", "israel": "IL", "israeli": "IL",
	"sg": "SG", "singapore": "SG",
	"hk": "HK", "hong kong": "HK",
}

func normalizeCountry(value string) (string, bool) {
	if code, ok := countryAliases[strings.ToLower(value)]; ok {
		return code, true
	}
	// Accept raw uppercase 2-letter codes not in the alias table.
	upper := strings.ToUpper(value)
	if len(upper) == 2 {
		return upper, true
	}
	return "", false
}

func parseCountryTerm(value string) (TermExpr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return TermExpr{}, fmt.Errorf("country filter requires a value")
	}
	if strings.Contains(value, "*") {
		return TermExpr{Field: FieldCountry, Value: strings.ToUpper(value), Country: ValuePattern{Raw: strings.ToUpper(value), Wildcard: true}}, nil
	}
	code, ok := normalizeCountry(value)
	if !ok {
		return TermExpr{}, fmt.Errorf("unknown country %q: use an ISO code (e.g. US, EP, WO) or common name", value)
	}
	return TermExpr{Field: FieldCountry, Value: code, Country: ValuePattern{Raw: code, Wildcard: false}}, nil
}

func unsupportedClassPatternError() error {
	return fmt.Errorf("unsupported classification filter syntax: only literal values and '*' wildcard are supported right now (examples: class:S04, class:S04*)")
}

func renderExpr(expr Expr, parentPrec int) string {
	switch e := expr.(type) {
	case AndExpr:
		return renderWithParen(parentPrec, 2, renderExpr(e.Left, 2)+" and "+renderExpr(e.Right, 2))
	case OrExpr:
		return renderWithParen(parentPrec, 1, renderExpr(e.Left, 1)+" or "+renderExpr(e.Right, 1))
	case NotExpr:
		return renderWithParen(parentPrec, 3, "not "+renderExpr(e.Child, 3))
	case TermExpr:
		return string(e.Field) + ":" + renderValue(e.Value)
	default:
		return ""
	}
}

func renderValue(value string) string {
	if value == "" {
		return value
	}
	if strings.ContainsAny(value, " \t\n\r()\"") {
		return strconv.Quote(value)
	}
	return value
}

func renderWithParen(parentPrec, prec int, text string) string {
	if prec < parentPrec {
		return "(" + text + ")"
	}
	return text
}

func join(_ bool, left, right Expr, and bool) Expr {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	if and {
		return AndExpr{Left: left, Right: right}
	}
	return OrExpr{Left: left, Right: right}
}
