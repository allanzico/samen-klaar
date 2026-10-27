package workflow

import "errors"

// New agreements retain this rate even when platform pricing changes later.
const PlatformFeeBasisPoints = 500

// PlatformFeeAmount rounds half a minor unit up, without floating point or
// overflowing the multiplication for large totals. A zero rate preserves
// agreements created before platform fees were introduced.
func PlatformFeeAmount(total int64, basisPoints int) (int64, error) {
	if total < 0 || basisPoints < 0 || basisPoints > 10000 {
		return 0, errors.New("invalid platform fee terms")
	}
	rate := int64(basisPoints)
	return total/10000*rate + (total%10000*rate+5000)/10000, nil
}
