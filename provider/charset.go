package provider

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Feeds still declare legacy charsets, and encoding/xml refuses a document
// whose declaration it cannot honour — note it refuses on the declaration
// alone, even when every byte is plain ASCII, which is how a feed like
// Slashdot fails today. Decoding latin-1 is a byte-to-rune widening and
// windows-1252 only adds a 32-entry table, so both are handled here rather
// than by taking on golang.org/x/text; the provider package stays
// standard-library only.

// windows1252High maps the 0x80-0x9F range, the only part where windows-1252
// differs from ISO-8859-1.
var windows1252High = [32]rune{
	'€', '�', '‚', 'ƒ', '„', '…', '†', '‡',
	'ˆ', '‰', 'Š', '‹', 'Œ', '�', 'Ž', '�',
	'�', '‘', '’', '“', '”', '•', '–', '—',
	'˜', '™', 'š', '›', 'œ', '�', 'ž', 'Ÿ',
}

// charsetReader converts a declared charset to UTF-8 for encoding/xml. An
// unknown charset is reported rather than guessed: a wrong guess produces
// mojibake that looks like real content, which is worse than a named failure.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch normalizeCharset(charset) {
	case "utf8", "usascii", "":
		return input, nil
	case "iso88591", "latin1", "iso885915", "cp1252", "windows1252":
		body, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(decodeLatin1(body, strings.Contains(normalizeCharset(charset), "1252"))), nil
	default:
		return nil, fmt.Errorf("provider: unsupported charset %q", charset)
	}
}

// normalizeCharset lowercases a charset name and drops the punctuation that
// spellings differ in, so "ISO-8859-1" and "iso_8859_1" compare equal.
func normalizeCharset(charset string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(charset) {
		if r == '-' || r == '_' || r == ' ' {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// decodeLatin1 widens single-byte text to UTF-8, optionally honouring the
// windows-1252 additions in 0x80-0x9F.
func decodeLatin1(body []byte, windows bool) []byte {
	out := make([]byte, 0, len(body))
	buf := make([]byte, utf8.UTFMax)
	for _, b := range body {
		r := rune(b)
		if windows && b >= 0x80 && b <= 0x9F {
			r = windows1252High[b-0x80]
		}
		out = append(out, buf[:utf8.EncodeRune(buf, r)]...)
	}
	return out
}
