package favro

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// recordSeq orders captured exchanges within a process.
var recordSeq uint64

// recordExchange writes the request/response pair to FAVRO_RECORD_DIR when set.
// This captures real Favro API shapes so they can be anonymized and used as
// test fixtures. No-op when the env var is unset (normal operation).
func recordExchange(method, url string, reqBody []byte, status int, respBody []byte) {
	dir := os.Getenv("FAVRO_RECORD_DIR")
	if dir == "" {
		return
	}
	seq := atomic.AddUint64(&recordSeq, 1)
	rec := map[string]any{
		"request":  map[string]any{"method": method, "url": url, "body": bodyOrText(reqBody)},
		"response": map[string]any{"status": status, "body": bodyOrText(respBody)},
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d_%s.json", seq, method)), b, 0o644)
}

// bodyOrText parses JSON bodies (so fixtures are diffable), returns nil for
// empty, and guards against recording large binary uploads verbatim.
func bodyOrText(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	if len(b) > 64<<10 {
		return fmt.Sprintf("<binary %d bytes>", len(b))
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		return v
	}
	return string(b)
}
