package bulk_domains

import (
	"bytes"
	"encoding/csv"
)

// newCSVReader returns a CSV reader for data, ignoring a leading UTF-8 BOM
// (as produced by Excel's "CSV UTF-8" export).
func newCSVReader(data []byte) *csv.Reader {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return csv.NewReader(bytes.NewReader(data))
}
