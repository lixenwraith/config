package config

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/lixenwraith/toml"
)

type fileSettings struct{ security SecurityOptions }
type fileSnapshot struct {
	data   []byte
	info   os.FileInfo
	digest [32]byte
}

func (c *Config) fileSettingsLocked() fileSettings {
	var s fileSettings
	if c.securityOpts != nil {
		s.security = *c.securityOpts
	}
	return s
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

// Read a regular file and validate the opened descriptor, not just the pathname.
func readConfigFile(ctx context.Context, path string, opts fileSettings) (*fileSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.security.PreventPathTraversal {
		clean := filepath.Clean(path)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, wrapError(ErrFileAccess, fmt.Errorf("path traversal in %q", path))
		}
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".json" || ext == ".yaml" || ext == ".yml" {
		return nil, ErrFileFormat
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrConfigNotFound
		}
		return nil, wrapError(ErrFileAccess, err)
	}
	if !info.Mode().IsRegular() {
		return nil, wrapError(ErrFileAccess, fmt.Errorf("config must be a regular file"))
	}
	file, err := openConfig(path)
	if err != nil {
		return nil, wrapError(ErrFileAccess, err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, wrapError(ErrFileAccess, fmt.Errorf("config must be a regular file"))
	}
	if opts.security.EnforceFileOwnership {
		if err := checkFileOwner(info); err != nil {
			return nil, err
		}
	}
	limit := opts.security.MaxFileSize
	if limit > 0 && info.Size() > limit {
		return nil, wrapError(ErrFileAccess, fmt.Errorf("config exceeds maximum size %d", limit))
	}
	var reader io.Reader = contextReader{ctx, file}
	if limit > 0 && limit < int64(^uint64(0)>>1) {
		reader = io.LimitReader(reader, limit+1)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if limit > 0 && int64(len(data)) > limit {
		return nil, wrapError(ErrFileAccess, fmt.Errorf("config exceeds maximum size %d", limit))
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("config changed during read")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &fileSnapshot{data, after, sha256.Sum256(data)}, nil
}

// openConfig opens without blocking: a FIFO swapped in after the Stat cannot
// hang open(2), and a regular-looking file whose reads block (/proc/kmsg)
// fails its read instead
func openConfig(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|openNonBlock, 0)
}

func parseFile(snapshot *fileSnapshot) (map[string]any, []string, error) {
	root, err := toml.NewParser(snapshot.data).Parse()
	if err != nil {
		return nil, nil, wrapError(ErrDecode, fmt.Errorf("failed to parse TOML: %w", err))
	}
	var comments []string
	lexer := toml.NewLexer(snapshot.data)
	for {
		token := lexer.NextToken()
		if token.Type == toml.TokenEOF {
			break
		}
		if token.Type == toml.TokenError {
			return nil, nil, wrapError(ErrDecode, fmt.Errorf("%s", token.Literal))
		}
		if token.Type == toml.TokenComment {
			comments = append(comments, token.Literal)
		}
	}
	return root, comments, nil
}

func (c *Config) loadFile(path string) error {
	c.mutex.RLock()
	opts := c.fileSettingsLocked()
	c.mutex.RUnlock()
	snapshot, err := readConfigFile(context.Background(), path, opts)
	if err != nil {
		return err
	}
	root, comments, err := parseFile(snapshot)
	if err != nil {
		return err
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if err := c.applyFileLocked(root, comments, path); err != nil {
		return err
	}
	if c.watcher != nil && c.watcher.filePath != path {
		c.watcher.stop()
		c.watcher = nil
	}
	return nil
}

func (c *Config) applyFileLocked(root map[string]any, comments []string, path string, guard ...func() error) error {
	values := make(map[string]any)
	for registered := range c.items {
		var current any = root
		exists := true
		for _, key := range strings.Split(registered, ".") {
			m, ok := current.(map[string]any)
			if !ok {
				return wrapError(ErrTypeMismatch, fmt.Errorf("scalar parent of registered path %q", registered))
			}
			current, exists = m[key]
			if !exists {
				break
			}
		}
		if exists {
			values[registered] = current
		}
	}
	if err := c.replaceSourceLocked(SourceFile, values, guard...); err != nil {
		return err
	}
	c.configFilePath, c.fileComments = path, comments
	c.fileGeneration++
	return nil
}

// Save writes registered effective values as TOML. Comments from the loaded file
// are retained as a preamble; their original positions and whitespace are not.
func (c *Config) Save(path string) error { return c.save(path, "") }
func (c *Config) SaveSource(path string, source Source) error {
	if !validSource(source) {
		return wrapError(ErrTypeMismatch, fmt.Errorf("invalid source %q", source))
	}
	return c.save(path, source)
}
func (c *Config) save(path string, source Source) error {
	c.mutex.RLock()
	nested := c.nestedLocked(source)
	comments := append([]string(nil), c.fileComments...)
	c.mutex.RUnlock()
	data, err := marshalConfig(nested)
	if err != nil {
		return err
	}
	var preamble strings.Builder
	for _, comment := range comments {
		preamble.WriteByte('#')
		preamble.WriteString(comment)
		preamble.WriteByte('\n')
	}
	return atomicWriteFile(path, append([]byte(preamble.String()), data...))
}

func marshalConfig(value any) ([]byte, error) {
	normalized, err := fileValue(reflect.ValueOf(value), 0)
	if err != nil {
		return nil, wrapError(ErrFileFormat, err)
	}
	data, err := toml.Marshal(normalized)
	if err != nil {
		return nil, wrapError(ErrFileFormat, err)
	}
	return data, nil
}

func fileValue(v reflect.Value, depth int) (any, error) {
	if depth > maxValueDepth {
		return nil, fmt.Errorf("file value nesting exceeds %d", maxValueDepth)
	}
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
		depth++
		if depth > maxValueDepth {
			return nil, fmt.Errorf("cyclic pointer")
		}
	}
	if !v.IsValid() {
		return nil, nil
	}
	switch v.Type() {
	case durationType:
		return time.Duration(v.Int()).String(), nil
	case timeType:
		return v.Interface().(time.Time).Format(time.RFC3339Nano), nil
	case ipType:
		ip := v.Interface().(net.IP)
		if len(ip) == 0 {
			return nil, nil
		}
		return ip.String(), nil
	case ipNetType:
		network := v.Interface().(net.IPNet)
		return network.String(), nil
	case urlType:
		u := v.Interface().(url.URL)
		return u.String(), nil
	}
	switch v.Kind() {
	case reflect.Struct:
		out := make(map[string]any)
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() || fieldKey(f) == "-" {
				continue
			}
			value, err := fileValue(v.Field(i), depth+1)
			if err != nil {
				return nil, err
			}
			key := fieldKey(f)
			if _, exists := out[key]; exists {
				return nil, fmt.Errorf("duplicate TOML field %q", key)
			}
			out[key] = value
		}
		return out, nil
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map keys must be strings")
		}
		out := make(map[string]any, v.Len())
		it := v.MapRange()
		for it.Next() {
			value, err := fileValue(it.Value(), depth+1)
			if err != nil {
				return nil, err
			}
			out[it.Key().String()] = value
		}
		return out, nil
	case reflect.Array, reflect.Slice:
		out := make([]any, v.Len())
		for i := range out {
			value, err := fileValue(v.Index(i), depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	default:
		return v.Interface(), nil
	}
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DirPermissions); err != nil {
		return wrapError(ErrFileAccess, err)
	}
	mode := os.FileMode(0600)
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return wrapError(ErrFileAccess, fmt.Errorf("destination must be a regular file"))
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return wrapError(ErrFileAccess, err)
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return wrapError(ErrFileAccess, err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(mode)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return wrapError(ErrFileAccess, err)
	}
	return nil
}
