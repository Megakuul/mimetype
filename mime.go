package mimetype

import (
	"mime"

	"github.com/gabriel-vasile/mimetype/internal/charset"
	"github.com/gabriel-vasile/mimetype/internal/magic"
	"github.com/gabriel-vasile/mimetype/types"
)

// MIME struct holds information about a file format: the string representation
// of the MIME type, the extension and the parent file format.
type MIME struct {
	typ       types.TYPE
	aliases   []string
	params    map[string]string
	extension string
	// detector receives the raw input and a limit for the number of bytes it is
	// allowed to check. It returns whether the input matches a signature or not.
	detector magic.Detector
	children []*MIME
	parent   *MIME
}

// String returns the string representation of the MIME type including params, e.g., "text/html; charset=UTF-8".
func (m *MIME) String() string {
	if len(m.params) > 0 {
		return mime.FormatMediaType(string(m.typ), m.params)
	}
	return string(m.typ)
}

// Type returns the string representation of the MIME type excluding params, e.g., "application/zip".
func (m *MIME) Type() types.TYPE {
	return m.typ
}

// Extension returns the file extension associated with the MIME type.
// It includes the leading dot, as in ".html". When the file format does not
// have an extension, the empty string is returned.
func (m *MIME) Extension() string {
	return m.extension
}

// Parent returns the parent MIME type from the hierarchy.
// Each MIME type has a non-nil parent, except for the root MIME type.
//
// For example, the application/json and text/html MIME types have text/plain as
// their parent because they are text files who happen to contain JSON or HTML.
// Another example is the ZIP format, which is used as container
// for Microsoft Office files, EPUB files, JAR files, and others.
func (m *MIME) Parent() *MIME {
	return m.parent
}

// Is checks whether this MIME type, or any of its aliases, is equal to the
// expected MIME type. MIME type equality test is done on the "type/subtype"
// section, ignores any optional MIME parameters, ignores any leading and
// trailing whitespace, and is case insensitive.
func (m *MIME) Is(expectedMIME string) bool {
	// Parsing is needed because some detected MIME types contain parameters
	// that need to be stripped for the comparison.
	expectedMIME, _, _ = mime.ParseMediaType(expectedMIME)

	if expectedMIME == string(m.typ) {
		return true
	}

	for _, alias := range m.aliases {
		if alias == expectedMIME {
			return true
		}
	}

	return false
}

func newMIME(
	typ types.TYPE, extension string,
	detector magic.Detector,
	children ...*MIME) *MIME {
	m := &MIME{
		typ:       typ,
		extension: extension,
		params:    map[string]string{},
		detector:  detector,
		children:  children,
	}

	for _, c := range children {
		c.parent = m
	}

	return m
}

func (m *MIME) alias(aliases ...string) *MIME {
	m.aliases = aliases
	return m
}

// match does a depth-first search on the signature tree. It returns the deepest
// successful node for which all the children detection functions fail.
func (m *MIME) match(in []byte, readLimit uint32) *MIME {
	for _, c := range m.children {
		if c.detector(in, readLimit) {
			return c.match(in, readLimit)
		}
	}

	needsCharset := map[types.TYPE]func([]byte) string{
		types.TEXT: charset.FromPlain,
		types.HTML: charset.FromHTML,
		types.XML:  charset.FromXML,
	}
	// ps holds optional MIME parameters.
	ps := map[string]string{}
	if f, ok := needsCharset[m.typ]; ok {
		if cset := f(in); cset != "" {
			ps["charset"] = cset
		}
	}

	return m.cloneHierarchy(ps)
}

// flatten transforms an hierarchy of MIMEs into a slice of MIMEs.
func (m *MIME) flatten() []*MIME {
	out := []*MIME{m}
	for _, c := range m.children {
		out = append(out, c.flatten()...)
	}

	return out
}

// clone creates a new MIME with the provided optional MIME parameters.
func (m *MIME) clone(ps map[string]string) *MIME {
	clonedMIME := &MIME{
		typ:       m.typ,
		aliases:   m.aliases,
		params:    map[string]string{},
		extension: m.extension,
	}

	// apply params from parent
	for k, v := range m.params {
		clonedMIME.params[k] = v
	}

	// apply optional params
	for k, v := range ps {
		clonedMIME.params[k] = v
	}

	return clonedMIME
}

// cloneHierarchy creates a clone of m and all its ancestors. The optional MIME
// parameters are set on the last child of the hierarchy.
func (m *MIME) cloneHierarchy(ps map[string]string) *MIME {
	ret := m.clone(ps)
	lastChild := ret
	for p := m.Parent(); p != nil; p = p.Parent() {
		pClone := p.clone(nil)
		lastChild.parent = pClone
		lastChild = pClone
	}

	return ret
}

func (m *MIME) lookup(typ string) *MIME {
	for _, n := range append(m.aliases, string(m.typ)) {
		if n == typ {
			return m
		}
	}

	for _, c := range m.children {
		if m := c.lookup(typ); m != nil {
			return m
		}
	}
	return nil
}

// Extend adds detection for a sub-format. The detector is a function
// returning true when the raw input file satisfies a signature.
// The sub-format will be detected if all the detectors in the parent chain return true.
// The extension should include the leading dot, as in ".html".
func (m *MIME) Extend(detector func(raw []byte, limit uint32) bool, mimestr, extension string, aliases ...string) {

	typ, params, _ := mime.ParseMediaType(mimestr)

	c := &MIME{
		typ:       types.TYPE(typ),
		params:    params,
		extension: extension,
		detector:  detector,
		parent:    m,
		aliases:   aliases,
	}

	mu.Lock()
	m.children = append([]*MIME{c}, m.children...)
	mu.Unlock()
}
