package ls

import (
	"strconv"
)

// HumanSize formats a byte count like GNU ls -h: plain bytes below 1024,
// otherwise powers of 1024 with a K, M, G, T, P, or E suffix, rounded up,
// with one decimal place when the value is below 10 (1.1K, 15K, 1.0M).
func HumanSize(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10)
	}
	// Exact integer arithmetic: float64 cannot represent every int64.
	u, d, unit := uint64(n), uint64(1024), 0
	for unit < 5 && u/d >= 1024 {
		d *= 1024
		unit++
	}
	q, r := u/d, u%d
	if q < 10 {
		tenths := q*10 + (r*10+d-1)/d // r*10 < 10*2^60 fits in uint64
		if tenths < 100 {
			return strconv.FormatUint(tenths/10, 10) + "." + strconv.FormatUint(tenths%10, 10) + string("KMGTPE"[unit])
		}
		return "10" + string("KMGTPE"[unit])
	}
	if r > 0 {
		q++
	}
	if q >= 1024 && unit < 5 {
		return "1.0" + string("KMGTPE"[unit+1])
	}
	return strconv.FormatUint(q, 10) + string("KMGTPE"[unit])
}
