// Package humanize formats machine numbers for people to read.
package humanize

import "fmt"

// Bytes phrases a byte count for someone reading a page or a terminal line.
//
// Powers of 1024 with the IEC names, because these are files on a disk and
// that is what an operator's `du -h` will say about the same directory. It is
// for reading, never for parsing: the exact figure stays in the API's JSON,
// which is where a caller that needs to compute on it should look.
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
