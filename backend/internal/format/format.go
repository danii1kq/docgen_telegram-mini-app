// Package format mirrors src/lib/format.ts: KZT money with the ru-RU
// non-breaking-space grouping and the Russian plural helper.
package format

import (
	"strconv"
	"strings"
)

// nbsp is U+00A0, the group separator Intl.NumberFormat('ru-RU') uses.
const nbsp = "\u00a0"

// GroupThousands renders 1234567 as "1 234 567" (non-breaking spaces).
func GroupThousands(value int) string {
	negative := value < 0
	if negative {
		value = -value
	}
	digits := strconv.Itoa(value)
	var out strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out.WriteString(nbsp)
		}
		out.WriteRune(digit)
	}
	if negative {
		return "-" + out.String()
	}
	return out.String()
}

// Money renders an integer KZT amount exactly like formatMoney(value): the
// group separator is U+00A0 ("1 000") and the currency sign is separated by a
// regular space, matching `${money.format(value)} ₸` in format.ts.
func Money(value int) string {
	return GroupThousands(value) + " ₸"
}

// Plural picks the Russian word form for n: 1 страница, 2 страницы, 5 страниц.
func Plural(n int, one, few, many string) string {
	mod10 := n % 10
	mod100 := n % 100
	if mod10 == 1 && mod100 != 11 {
		return one
	}
	if mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) {
		return few
	}
	return many
}

// Capitalize uppercases the first rune, like "страницы" -> "Страницы".
func Capitalize(value string) string {
	if value == "" {
		return value
	}
	runes := []rune(value)
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}
