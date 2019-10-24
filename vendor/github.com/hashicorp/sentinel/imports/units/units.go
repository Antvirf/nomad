package units

// Byte sizes (base 2 form)
const (
	B int64 = 1 << (10 * iota)
	KB
	MB
	GB
	TB
	PB
)

// unitMap is the lookup table for the units
var unitMap = map[string]interface{}{
	// Byte sizes
	"byte":     B,
	"kilobyte": KB,
	"megabyte": MB,
	"gigabyte": GB,
	"terabyte": TB,
	"petabyte": PB,
}
