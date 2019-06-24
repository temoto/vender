package helpers

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/temoto/errors"
)

func FoldErrors(errs []error) error {
	// common fast path
	if len(errs) == 0 {
		return nil
	}

	ss := make([]string, 0, 1+len(errs))
	for _, e := range errs {
		if e != nil {
			// ss = append(ss, e.Error())
			ss = append(ss, errors.ErrorStack(e))
			// ss = append(ss, errors.Details(e))
		}
	}
	switch len(ss) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf(ss[0])
	default:
		ss = append(ss, "")
		copy(ss[1:], ss[0:])
		ss[0] = "multiple errors:"
		return fmt.Errorf(strings.Join(ss, "\n- "))
	}
}

func HexSpecialBytes(input []byte) string {
	const hexAlpha = "0123456789abcdef"
	rb := make([]byte, 0, len(input)*4)
	for _, b := range input {
		if unicode.In(rune(b), unicode.Digit, unicode.Letter, unicode.Punct, unicode.Space) {
			rb = append(rb, b)
		} else {
			rb = append(rb, '{', hexAlpha[b>>4], hexAlpha[b&0xf], '}')
		}
	}
	return string(rb)
}
func HexSpecialString(input string) string {
	result := ""
	for _, r := range input {
		if unicode.In(r, unicode.Digit, unicode.Letter, unicode.Punct, unicode.Space) {
			result += string(r)
		} else {
			result += fmt.Sprintf("{%02x}", r)
		}
	}
	return result
}
