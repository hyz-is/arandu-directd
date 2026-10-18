package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// pushChunk matches one self.__next_f.push([1,"..."]) call and captures its
// JSON string literal.
var pushChunk = regexp.MustCompile(`self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]\)`)

// rowID matches the start of a flight row: a hexadecimal id and a colon.
var rowID = regexp.MustCompile(`^([0-9a-f]+):`)

// Flight is a page's React Server Components payload, split into rows by id.
type Flight map[string]string

// ParseFlight extracts and splits the payload of a Next.js page.
func ParseFlight(html []byte) (Flight, error) {
	var sb strings.Builder
	for _, m := range pushChunk.FindAllSubmatch(html, -1) {
		var s string
		if err := json.Unmarshal(m[1], &s); err != nil {
			return nil, fmt.Errorf("catalog: decoding a flight chunk: %w", err)
		}
		sb.WriteString(s)
	}
	if sb.Len() == 0 {
		return nil, errors.New("catalog: the page carries no flight payload")
	}
	return splitRows(sb.String()), nil
}

// splitRows splits a flight payload into rows. A text row, id:T<hex length>,
// is length-prefixed and may hold newlines; every other row ends at one.
func splitRows(f string) Flight {
	out := Flight{}
	i := 0
	for i < len(f) {
		m := rowID.FindStringSubmatch(f[i:])
		if m == nil {
			j := strings.IndexByte(f[i:], '\n')
			if j < 0 {
				break
			}
			i += j + 1
			continue
		}
		id := m[1]
		i += len(m[0])
		if strings.HasPrefix(f[i:], "T") {
			comma := strings.IndexByte(f[i:], ',')
			if comma > 1 {
				if n, err := strconv.ParseInt(f[i+1:i+comma], 16, 64); err == nil {
					start := i + comma + 1
					end := min(start+int(n), len(f))
					out[id] = "T" + f[start:end]
					i = end
					continue
				}
			}
		}
		j := strings.IndexByte(f[i:], '\n')
		if j < 0 {
			out[id] = f[i:]
			break
		}
		out[id] = f[i : i+j]
		i += j + 1
	}
	return out
}

// refPattern is a flight reference to another row.
var refPattern = regexp.MustCompile(`^\$([0-9a-f]+)$`)

// Resolve replaces every string that refers to a text row ("$9a") with the
// row's text, and unescapes a literal dollar ("$$x" is "$x"). Other
// references are left as they are.
func (f Flight) Resolve(v any) any {
	switch v := v.(type) {
	case string:
		if strings.HasPrefix(v, "$$") {
			return v[1:]
		}
		if m := refPattern.FindStringSubmatch(v); m != nil {
			if row, ok := f[m[1]]; ok && strings.HasPrefix(row, "T") {
				return row[1:]
			}
		}
		return v
	case []any:
		for i := range v {
			v[i] = f.Resolve(v[i])
		}
		return v
	case map[string]any:
		for k := range v {
			v[k] = f.Resolve(v[k])
		}
		return v
	}
	return v
}

// Element returns the props object of the first row that is a React element
// (["$", type, key, props]) whose props hold every one of keys.
func (f Flight) Element(keys ...string) (map[string]any, error) {
	for _, row := range f {
		if !strings.HasPrefix(row, `["$"`) {
			continue
		}
		hit := true
		for _, k := range keys {
			if !strings.Contains(row, `"`+k+`":`) {
				hit = false
				break
			}
		}
		if !hit {
			continue
		}
		var arr []any
		if err := json.Unmarshal([]byte(row), &arr); err != nil || len(arr) < 4 {
			continue
		}
		props, ok := arr[3].(map[string]any)
		if !ok {
			continue
		}
		all := true
		for _, k := range keys {
			if _, ok := props[k]; !ok {
				all = false
				break
			}
		}
		if all {
			return f.Resolve(props).(map[string]any), nil
		}
	}
	return nil, fmt.Errorf("catalog: no element with %s", strings.Join(keys, ", "))
}
