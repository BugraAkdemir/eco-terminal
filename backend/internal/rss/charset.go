package rss

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// charsetReader converts the single-byte legacy encodings still seen on Turkish sites to UTF-8
// (stdlib only; no golang.org/x/text dependency). Unknown charsets are rejected.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-8", "utf8", "us-ascii", "ascii", "":
		return input, nil
	case "iso-8859-9", "iso8859-9", "latin5", "iso_8859-9":
		return single(input, iso88599), nil
	case "windows-1254", "cp1254", "x-cp1254":
		return single(input, cp1254), nil
	case "iso-8859-1", "latin1", "latin-1", "iso_8859-1":
		return single(input, func(b byte) rune { return rune(b) }), nil
	case "windows-1252", "cp1252":
		return single(input, cp1252), nil
	}
	return nil, fmt.Errorf("unsupported feed charset %q", label)
}

func single(in io.Reader, m func(byte) rune) io.Reader {
	data, err := io.ReadAll(in)
	if err != nil {
		return &errReader{err}
	}
	var buf bytes.Buffer
	buf.Grow(len(data) * 2)
	for _, b := range data {
		buf.WriteRune(m(b))
	}
	return &buf
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }

// iso88599 is Latin-5: Latin-1 with six Turkish letters replaced.
func iso88599(b byte) rune {
	switch b {
	case 0xD0:
		return 'Ğ'
	case 0xDD:
		return 'İ'
	case 0xDE:
		return 'Ş'
	case 0xF0:
		return 'ğ'
	case 0xFD:
		return 'ı'
	case 0xFE:
		return 'ş'
	}
	return rune(b)
}

var cp1252High = [32]rune{
	0x20AC, 0x81, 0x201A, 0x0192, 0x201E, 0x2026, 0x2020, 0x2021, 0x02C6, 0x2030, 0x0160, 0x2039, 0x0152, 0x8D, 0x017D, 0x8F,
	0x90, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014, 0x02DC, 0x2122, 0x0161, 0x203A, 0x0153, 0x9D, 0x017E, 0x0178,
}

func cp1252(b byte) rune {
	if b >= 0x80 && b < 0xA0 {
		return cp1252High[b-0x80]
	}
	return rune(b)
}

// cp1254 is Windows-1254 (Turkish): cp1252 with the same six Latin-5 substitutions, and 0x8E/0x9E/0x9F unmapped.
func cp1254(b byte) rune {
	switch b {
	case 0xD0, 0xDD, 0xDE, 0xF0, 0xFD, 0xFE:
		return iso88599(b)
	case 0x8E, 0x9E:
		return rune(b)
	case 0x9F:
		return 0x0178
	}
	return cp1252(b)
}

// cleanXMLBytes removes a UTF-8 BOM and characters illegal in XML 1.0 (some CMSs emit stray control bytes),
// and repairs invalid UTF-8 sequences when the document is not declared with another charset.
func cleanXMLBytes(data []byte) []byte {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.TrimLeft(data, " \t\r\n")
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			continue
		}
		out = append(out, b)
	}
	if !declaresLegacyCharset(out) && !utf8.Valid(out) {
		out = bytes.ToValidUTF8(out, []byte("�"))
	}
	return out
}

func declaresLegacyCharset(data []byte) bool {
	head := data
	if len(head) > 200 {
		head = head[:200]
	}
	l := bytes.ToLower(head)
	i := bytes.Index(l, []byte("encoding="))
	if i < 0 {
		return false
	}
	rest := l[i+len("encoding="):]
	return !(bytes.HasPrefix(rest, []byte(`"utf-8"`)) || bytes.HasPrefix(rest, []byte(`'utf-8'`)))
}
