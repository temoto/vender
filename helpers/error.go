package helpers

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/juju/errors"
)

func FoldErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	ss := make([]string, 0, len(errs))
	for _, e := range errs {
		if e != nil {
			ss = append(ss, e.Error())
		}
	}
	if len(ss) == 0 {
		return nil
	}
	return errors.New(strings.Join(ss, "\n"))
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
