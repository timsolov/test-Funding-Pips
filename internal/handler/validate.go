package handler

import (
	"math"
	"regexp"
)

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validUUID(s string) bool {
	return uuidRe.MatchString(s)
}

func validAmount(amount float64) bool {
	return amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

func validCurrency(currency string) bool {
	if currency == "" {
		return true
	}
	if len(currency) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		c := currency[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}
