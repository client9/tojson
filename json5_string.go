package tojson

import "unicode/utf8"

func appendRecodeString(dst []byte, src []byte) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(src); {
		if b := src[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}
			dst = append(dst, src[start:i]...)

			if b == '\\' && i+1 < len(src) {
				switch src[i+1] {
				case 'u':
					// \uXXXX passes through untouched, but only when the
					// four hex digits are actually there. Otherwise the
					// backslash is written literally, as a bad \xNN is.
					if i+5 < len(src) &&
						hexVal(src[i+2]) >= 0 && hexVal(src[i+3]) >= 0 &&
						hexVal(src[i+4]) >= 0 && hexVal(src[i+5]) >= 0 {
						dst = append(dst, '\\')
						i++
						start = i
						continue
					}
				case 'n':
					b = '\n'
					i++
				case 't':
					b = '\t'
					i++
				case 'b':
					b = '\b'
					i++
				case 'f':
					b = '\f'
					i++
				case '"':
					b = '"'
					i++
				case 'r':
					b = '\r'
					i++
				case '\\':
					b = '\\'
					i++
				case '/':
					b = '/'
					i++
				case 'a':
					b = '\a'
					i++
				case 'v':
					b = '\v'
					i++
				case 'x':
					if i+3 < len(src) {
						if v1, v2 := hexVal(src[i+2]), hexVal(src[i+3]); v1 >= 0 && v2 >= 0 {
							b = byte(v1<<4 | v2)
							i += 3
						}
					}
				case '\n':
					b = '\n'
					i++
				case '\'':
					b = '\''
					i++
				}
			}

			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				if i+1 < len(src) && src[i+1] == '\n' {
					break
				}
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				if b < utf8.RuneSelf && safeSet[b] {
					dst = append(dst, b)
				} else {
					dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
				}
			}
			i++
			start = i
			continue
		}
		n := min(len(src)-i, utf8.UTFMax)
		c, size := utf8.DecodeRune(src[i : i+n])
		if c == utf8.RuneError && size == 1 {
			dst = append(dst, src[start:i]...)
			dst = append(dst, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if c == '\u2028' || c == '\u2029' {
			dst = append(dst, src[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, src[start:]...)
	dst = append(dst, '"')
	return dst
}
