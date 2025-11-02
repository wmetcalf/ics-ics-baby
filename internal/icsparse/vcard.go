package icsparse

import (
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"net/url"
	"strings"

	htmlcharset "golang.org/x/net/html/charset"
)

// VCard represents a vCard/vCalendar-like component captured from the source file.
type VCard struct {
	Kind       string          `json:"kind"`
	Properties []VCardProperty `json:"properties"`
	Children   []VCard         `json:"children,omitempty"`
}

// VCardProperty retains a single vCard property along with its parameters.
type VCardProperty struct {
	Group     *string             `json:"group,omitempty"`
	Name      string              `json:"name"`
	Params    map[string][]string `json:"params,omitempty"`
	Value     *string             `json:"value,omitempty"`
	Values    []string            `json:"values,omitempty"`
	Fields    []string            `json:"fields,omitempty"`
	Binary    *string             `json:"binary,omitempty"`
	MediaType *string             `json:"media_type,omitempty"`
	Encoding  *string             `json:"encoding,omitempty"`
}

// parseVCardBlock consumes lines starting at "BEGIN:<name>" and returns the parsed component
// alongside the index for the matching END line (inclusive).
func parseVCardBlock(lines []string, start int) (VCard, int) {
	line := lines[start]
	kind := componentName(line)
	card := VCard{Kind: kind}

	for i := start + 1; i < len(lines); i++ {
		cur := lines[i]
		upper := strings.ToUpper(cur)
		if strings.HasPrefix(upper, "BEGIN:") {
			child, endIdx := parseVCardBlock(lines, i)
			card.Children = append(card.Children, child)
			i = endIdx
			continue
		}
		if isEndFor(cur, kind) {
			return card, i
		}
		if prop, ok := parseVCardProperty(cur); ok {
			card.Properties = append(card.Properties, prop)
		}
	}
	return card, len(lines) - 1
}

func componentName(line string) string {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return strings.ToUpper(strings.TrimSpace(line))
	}
	return strings.ToUpper(strings.TrimSpace(line[idx+1:]))
}

func isEndFor(line, kind string) bool {
	if !strings.HasPrefix(strings.ToUpper(line), "END:") {
		return false
	}
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(line[idx+1:]), kind)
}

func parseVCardProperty(line string) (VCardProperty, bool) {
	var empty VCardProperty
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return empty, false
	}
	head := line[:idx]
	rawValue := line[idx+1:]
	headParts := strings.Split(head, ";")
	token := headParts[0]
	var group *string
	name := token
	if dot := strings.IndexByte(token, '.'); dot >= 0 {
		grp := token[:dot]
		name = token[dot+1:]
		if grp != "" {
			group = ptr(grp)
		}
	}
	upName := strings.ToUpper(strings.TrimSpace(name))

	params := map[string][]string{}
	encoding := ""
	charset := ""
	mediaType := ""
	hints := paramsValue(headParts[1:], params)
	if val, ok := params["ENCODING"]; ok && len(val) > 0 {
		encoding = strings.ToLower(val[0])
	}
	if val, ok := params["CHARSET"]; ok && len(val) > 0 {
		charset = val[0]
	}
	if val, ok := params["MEDIATYPE"]; ok && len(val) > 0 {
		mediaType = val[0]
	}
	if encoding == "" && hints.isBinary {
		encoding = "b"
	}

	prop := VCardProperty{Group: group, Name: upName}

	if len(params) > 0 {
		prop.Params = params
	}

	if mediaType != "" {
		prop.MediaType = ptr(mediaType)
	}

	trimmedValue := strings.TrimSpace(rawValue)
	if strings.HasPrefix(strings.ToLower(trimmedValue), "data:") {
		if mt, bin, ok := parseDataURI(trimmedValue); ok {
			encoded := base64.StdEncoding.EncodeToString(bin)
			prop.Binary = &encoded
			if prop.MediaType == nil && mt != "" {
				prop.MediaType = ptr(mt)
			}
			encLabel := "b"
			prop.Encoding = &encLabel
			return prop, true
		}
	}

	textValue, binary := decodeVCardValue(rawValue, encoding)
	if binary != nil {
		encoded := base64.StdEncoding.EncodeToString(binary)
		prop.Binary = &encoded
		if encoding != "" {
			encLabel := strings.ToLower(encoding)
			prop.Encoding = &encLabel
		} else {
			encLabel := "b"
			prop.Encoding = &encLabel
		}
		if prop.MediaType == nil && hints.mediaType != "" {
			prop.MediaType = ptr(hints.mediaType)
		}
		return prop, true
	}

	if charset != "" {
		textValue = convertCharset(textValue, charset)
	}

	clean := unescapeVCardLiteral(textValue)
	prop.Value = ptr(clean)

	if strings.Contains(rawValue, ";") || strings.HasSuffix(rawValue, ";") {
		fields := splitEscaped(rawValue, ';', true)
		decodedFields := make([]string, 0, len(fields))
		for _, f := range fields {
			decodedFields = append(decodedFields, unescapeVCardLiteral(f))
		}
		prop.Fields = decodedFields
	}

	if strings.Contains(rawValue, ",") {
		parts := splitEscaped(rawValue, ',', false)
		decodedParts := make([]string, 0, len(parts))
		for _, p := range parts {
			decodedParts = append(decodedParts, unescapeVCardLiteral(p))
		}
		prop.Values = decodedParts
	}

	if encoding != "" {
		encLabel := strings.ToLower(encoding)
		prop.Encoding = &encLabel
	}

	if prop.MediaType == nil && hints.mediaType != "" {
		prop.MediaType = ptr(hints.mediaType)
	}

	return prop, true
}

type paramHints struct {
	isBinary  bool
	mediaType string
}

func paramsValue(parts []string, params map[string][]string) paramHints {
	hints := paramHints{}
	for _, raw := range parts {
		if raw == "" {
			continue
		}
		key, values := splitParam(raw)
		key = strings.ToUpper(strings.TrimSpace(key))
		if key == "" {
			key = "TYPE"
		}
		if len(values) == 0 {
			params[key] = append(params[key], "")
			continue
		}
		cleanVals := make([]string, 0, len(values))
		for _, v := range values {
			val := unescapeVCardLiteral(v)
			cleanVals = append(cleanVals, val)
		}
		params[key] = append(params[key], cleanVals...)
	}

	if vals, ok := params["VALUE"]; ok {
		for _, v := range vals {
			if strings.EqualFold(v, "BINARY") {
				hints.isBinary = true
			}
		}
	}
	if vals, ok := params["MEDIATYPE"]; ok && len(vals) > 0 {
		hints.mediaType = vals[0]
	}
	return hints
}

func splitParam(raw string) (string, []string) {
	if eq := strings.IndexByte(raw, '='); eq >= 0 {
		key := raw[:eq]
		val := raw[eq+1:]
		vals := splitEscaped(trimQuotes(val), ',', false)
		return key, vals
	}
	vals := splitEscaped(raw, ',', false)
	return "", vals
}

func splitEscaped(s string, delim byte, keepEmpty bool) []string {
	if s == "" {
		if keepEmpty {
			return []string{""}
		}
		return nil
	}
	var res []string
	start := 0
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == delim {
			part := s[start:i]
			if keepEmpty || part != "" {
				res = append(res, part)
			}
			start = i + 1
		}
	}
	part := s[start:]
	if keepEmpty || part != "" {
		res = append(res, part)
	}
	if keepEmpty && s[len(s)-1] == delim {
		res = append(res, "")
	}
	return res
}

func unescapeVCardLiteral(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			switch c {
			case 'n', 'N':
				b.WriteByte('\n')
			case 't', 'T':
				b.WriteByte('\t')
			case '\\', ';', ',', ':':
				b.WriteByte(c)
			default:
				b.WriteByte(c)
			}
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		b.WriteByte(c)
	}
	if escaped {
		b.WriteByte('\\')
	}
	return b.String()
}

func decodeVCardValue(val string, encoding string) (string, []byte) {
	enc := strings.ToLower(strings.TrimSpace(encoding))
	switch enc {
	case "b", "base64":
		cleaned := removeWhitespace(val)
		if data, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
			return "", data
		}
	case "quoted-printable":
		reader := quotedprintable.NewReader(strings.NewReader(val))
		decoded, err := io.ReadAll(reader)
		if err == nil {
			return string(decoded), nil
		}
	}
	return val, nil
}

func removeWhitespace(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func convertCharset(val, charset string) string {
	reader, err := htmlcharset.NewReaderLabel(charset, strings.NewReader(val))
	if err != nil {
		return val
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return val
	}
	return string(data)
}

func parseDataURI(val string) (string, []byte, bool) {
	lower := strings.ToLower(val)
	if !strings.HasPrefix(lower, "data:") {
		return "", nil, false
	}
	idx := strings.Index(val, ",")
	if idx < 0 {
		return "", nil, false
	}
	meta := val[5:idx]
	payload := val[idx+1:]
	parts := strings.Split(meta, ";")
	mediaType := ""
	base64Flag := false
	for _, p := range parts {
		if strings.EqualFold(p, "base64") {
			base64Flag = true
			continue
		}
		if mediaType == "" {
			mediaType = p
		}
	}
	if base64Flag {
		data, err := base64.StdEncoding.DecodeString(removeWhitespace(payload))
		if err != nil {
			return "", nil, false
		}
		return mediaType, data, true
	}
	if decoded, err := url.QueryUnescape(payload); err == nil {
		return mediaType, []byte(decoded), true
	}
	return "", nil, false
}
