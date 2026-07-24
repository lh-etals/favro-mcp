package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// WriteResult enumerates the outcome of an idempotent config write.
type WriteResult string

const (
	writeOK                WriteResult = "ok"
	writeNoop              WriteResult = "noop"
	writeSkippedUnparseable WriteResult = "skipped-unparseable"
)

func entryObject(e ServerTarget) map[string]any {
	o := map[string]any{"command": e.Command}
	if len(e.Args) > 0 {
		o["args"] = e.Args
	}
	if len(e.Env) > 0 {
		o["env"] = e.Env
	}
	return o
}

// envFlagArgs turns the target's env into repeated "-e KEY=VALUE" flags (keys
// sorted for deterministic output) for CLIs that take env via flags.
func envFlagArgs(e ServerTarget) []string {
	if len(e.Env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(e.Env))
	for k := range e.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		out = append(out, "-e", k+"="+e.Env[k])
	}
	return out
}

// opencodeEntry builds the OpenCode mcp entry shape:
//
//	{type:"local", command:[...], enabled:true, environment:{...}}
func opencodeEntry(e ServerTarget) map[string]any {
	cmd := append([]string{e.Command}, e.Args...)
	m := map[string]any{"type": "local", "command": cmd, "enabled": true}
	if len(e.Env) > 0 {
		m["environment"] = e.Env
	}
	return m
}

func entryEquals(a, b any) bool {
	// Normalize both sides so a nil slice/map and a missing key compare equal
	// (e.g. an existing "args": [] vs our omitted args). Otherwise every install
	// would be a non-idempotent rewrite.
	return bytes.Equal(jsonBytes(normalizeForCompare(a)), jsonBytes(normalizeForCompare(b)))
}

func jsonBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// normalizeForCompare drops nil values and empty collections so two semantically
// equal entries compare equal despite nil-vs-empty differences (e.g. an
// existing "args": [] vs our omitted args, or a typed-nil []string).
func normalizeForCompare(v any) any {
	if isNil(v) {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			n := normalizeForCompare(val)
			if n == nil {
				continue
			}
			if m, ok := n.(map[string]any); ok && len(m) == 0 {
				continue
			}
			out[k] = n
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		if len(x) == 0 {
			return nil
		}
		out := make([]any, 0, len(x))
		for _, val := range x {
			if n := normalizeForCompare(val); n != nil {
				out = append(out, n)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return v
	}
}

// isNil reports whether v is an untyped nil or a typed nil (nil slice/map/ptr).
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func:
		return rv.IsNil()
	}
	return false
}

// ---- JSON (mcpServers / context_servers / ...) ---------------------------

func readJSONTolerant(file string) (fresh bool, data map[string]any, err error) {
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return true, map[string]any{}, nil
		}
		return false, nil, readErr
	}
	data = map[string]any{}
	// Strip an optional UTF-8 BOM (some Windows editors add one); Go's
	// encoding/json rejects it, but the host client reads the file fine.
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if strings.TrimSpace(string(raw)) == "" {
		return false, data, nil
	}
	if jerr := json.Unmarshal(raw, &data); jerr != nil {
		return false, nil, nil // unparseable
	}
	return false, data, nil
}

func upsertJSONServer(file, topKey, name string, e ServerTarget, dryRun bool, entryFn func(ServerTarget) map[string]any) (WriteResult, error) {
	fresh, data, err := readJSONTolerant(file)
	if err != nil {
		return "", err
	}
	if data == nil {
		return writeSkippedUnparseable, nil
	}
	servers, _ := data[topKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if entryFn == nil {
		entryFn = entryObject
	}
	obj := entryFn(e)
	if !fresh && entryEquals(servers[name], obj) {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	data[topKey] = mergeMap(servers, map[string]any{name: obj})
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return "", err
	}
	return writeOK, writeJSON(file, data)
}

func removeJSONServer(file, topKey, name string, dryRun bool) (WriteResult, error) {
	fresh, data, err := readJSONTolerant(file)
	if err != nil {
		return "", err
	}
	if data == nil {
		return writeSkippedUnparseable, nil
	}
	if fresh {
		return writeNoop, nil
	}
	servers, _ := data[topKey].(map[string]any)
	if servers == nil {
		return writeNoop, nil
	}
	if _, ok := servers[name]; !ok {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	delete(servers, name)
	if len(servers) > 0 {
		data[topKey] = servers
	} else {
		delete(data, topKey)
	}
	return writeOK, writeJSON(file, data)
}

func writeJSON(file string, data map[string]any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(file, b, 0o600)
}

func mergeMap(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// ---- TOML (Codex ~/.codex/config.toml -> [mcp_servers.<id>]) --------------

func tomlStr(s string) string { return fmt.Sprintf("%q", s) }

func renderTomlBlock(name string, e ServerTarget) string {
	var b strings.Builder
	// Quote the server name so a name containing '.' (e.g. "my.server") is a
	// single key, not nested tables.
	fmt.Fprintf(&b, "\n[mcp_servers.%s]\ncommand = ", tomlStr(name))
	b.WriteString(tomlStr(e.Command))
	if len(e.Args) > 0 {
		quoted := make([]string, len(e.Args))
		for i, a := range e.Args {
			quoted[i] = tomlStr(a)
		}
		fmt.Fprintf(&b, "args = [%s]\n", strings.Join(quoted, ", "))
	}
	if len(e.Env) > 0 {
		var items []string
		for k, v := range e.Env {
			items = append(items, fmt.Sprintf("%s = %s", k, tomlStr(v)))
		}
		fmt.Fprintf(&b, "env = { %s }\n", strings.Join(items, ", "))
	}
	return b.String()
}

func upsertTomlServer(file, name string, e ServerTarget, dryRun bool) (WriteResult, error) {
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return "", readErr
		}
		if dryRun {
			return writeOK, nil
		}
		doc := map[string]any{"mcp_servers": map[string]any{name: entryObject(e)}}
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return "", err
		}
		return writeOK, writeToml(file, doc)
	}

	obj := entryObject(e)
	doc := map[string]any{}
	if strings.TrimSpace(string(raw)) != "" {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return writeSkippedUnparseable, nil
		}
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if entryEquals(servers[name], obj) {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	if _, present := servers[name]; !present {
		// Append at EOF to preserve the user's comments/formatting. Append to
		// the existing bytes (NOT a fresh slice) so the file is not truncated.
		block := []byte(renderTomlBlock(name, e))
		return writeOK, os.WriteFile(file, append(raw, block...), 0o600)
	}
	servers[name] = obj
	doc["mcp_servers"] = servers
	return writeOK, writeToml(file, doc)
}

func removeTomlServer(file, name string, dryRun bool) (WriteResult, error) {
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return writeNoop, nil
		}
		return "", readErr
	}
	doc := map[string]any{}
	if strings.TrimSpace(string(raw)) != "" {
		if err := toml.Unmarshal(raw, &doc); err != nil {
			return writeSkippedUnparseable, nil
		}
	}
	servers, _ := doc["mcp_servers"].(map[string]any)
	if servers == nil {
		return writeNoop, nil
	}
	if _, ok := servers[name]; !ok {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	delete(servers, name)
	if len(servers) > 0 {
		doc["mcp_servers"] = servers
	} else {
		delete(doc, "mcp_servers")
	}
	return writeOK, writeToml(file, doc)
}

func writeToml(file string, doc map[string]any) error {
	b, err := toml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(file, b, 0o600)
}

// ---- YAML list (Continue ~/.continue/config.yaml -> mcpServers: [...]) ----

func continueEntry(name string, e ServerTarget) map[string]any {
	return map[string]any{
		"name":    name,
		"type":    "stdio",
		"command": e.Command,
		"args":    e.Args,
		"env":     e.Env,
	}
}

func upsertYamlServerList(file, name string, e ServerTarget, dryRun bool) (WriteResult, error) {
	rootMap := map[string]any{}
	raw, readErr := os.ReadFile(file)
	fresh := false
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return "", readErr
		}
		fresh = true
	} else {
		if err := yaml.Unmarshal(raw, &rootMap); err != nil {
			return writeSkippedUnparseable, nil
		}
	}

	list, _ := rootMap["mcpServers"].([]any)
	if list == nil {
		list = []any{}
	}
	obj := continueEntry(name, e)
	idx := -1
	for i, it := range list {
		if m, ok := it.(map[string]any); ok {
			if n, _ := m["name"].(string); n == name {
				idx = i
				break
			}
		}
	}
	if idx >= 0 && entryEquals(list[idx], obj) {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	if idx >= 0 {
		list[idx] = obj
	} else {
		list = append(list, obj)
	}
	rootMap["mcpServers"] = list
	if fresh {
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return "", err
		}
	}
	out, err := yaml.Marshal(rootMap)
	if err != nil {
		return "", err
	}
	return writeOK, os.WriteFile(file, out, 0o600)
}

func removeYamlServerList(file, name string, dryRun bool) (WriteResult, error) {
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return writeNoop, nil
		}
		return "", readErr
	}
	rootMap := map[string]any{}
	if err := yaml.Unmarshal(raw, &rootMap); err != nil {
		return writeSkippedUnparseable, nil
	}
	list, _ := rootMap["mcpServers"].([]any)
	if list == nil {
		return writeNoop, nil
	}
	filtered := list[:0]
	removed := false
	for _, it := range list {
		if m, ok := it.(map[string]any); ok {
			if n, _ := m["name"].(string); n == name {
				removed = true
				continue
			}
		}
		filtered = append(filtered, it)
	}
	if !removed {
		return writeNoop, nil
	}
	if dryRun {
		return writeOK, nil
	}
	if len(filtered) > 0 {
		rootMap["mcpServers"] = filtered
	} else {
		delete(rootMap, "mcpServers")
	}
	out, err := yaml.Marshal(rootMap)
	if err != nil {
		return "", err
	}
	return writeOK, os.WriteFile(file, out, 0o600)
}
