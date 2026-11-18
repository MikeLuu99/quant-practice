package games

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Bounded, locale-independent rational parsing shared by timed arithmetic and
// assessments. Scientific notation is explicit, bounded to exponents +/-60.
var exactNumberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]{1,2})?$|^[+-]?[0-9]+/[+-]?[0-9]+$`)

func parseExactNumber(raw string) (*big.Rat, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 128 || !exactNumberPattern.MatchString(raw) {
		return nil, false
	}
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		n, err := strconv.Atoi(raw[i+1:])
		if err != nil || n < -60 || n > 60 {
			return nil, false
		}
	}
	value, ok := new(big.Rat).SetString(raw)
	return value, ok
}
