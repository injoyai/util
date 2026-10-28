package monthly

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

var (
	kindPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	monthPattern = regexp.MustCompile(`^\d{4}-\d{2}\.db$`)
)

type Spec struct {
	Schema     string
	Table      string
	TimeColumn string
}

type Store struct {
	root string

	opMu     sync.RWMutex
	mu       sync.Mutex
	dbs      map[string]*cachedDB
	clock    uint64
	spec     map[string]Spec
	closed   bool
	closeErr error
}

const maxIdleShards = 8

type cachedDB struct {
	db    *sql.DB
	users int
	used  uint64
}

type Shard struct {
	Month string
	Start time.Time
	DB    *sql.DB
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("monthly SQLite root is empty")
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create monthly SQLite root: %w", err)
	}
	return &Store{root: root, dbs: map[string]*cachedDB{}, spec: map[string]Spec{}}, nil
}

func (s *Store) Register(kind string, spec Spec) error {
	if !kindPattern.MatchString(kind) {
		return fmt.Errorf("invalid monthly SQLite kind %q", kind)
	}
	if strings.TrimSpace(spec.Schema) == "" || !namePattern.MatchString(spec.Table) || !namePattern.MatchString(spec.TimeColumn) {
		return fmt.Errorf("invalid monthly SQLite specification for %q", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("monthly SQLite store is closed")
	}
	if previous, ok := s.spec[kind]; ok && previous != spec {
		return fmt.Errorf("monthly SQLite kind %q already registered", kind)
	}
	s.spec[kind] = spec
	return nil
}

func (s *Store) WithDB(ctx context.Context, kind string, at time.Time, fn func(*sql.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if at.IsZero() {
		return errors.New("monthly SQLite time is zero")
	}
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	db, err := s.open(kind, at.UTC().Format("2006-01"), true)
	if err != nil {
		return err
	}
	defer s.release(kind, at.UTC().Format("2006-01"))
	return fn(db)
}

func (s *Store) WithShard(ctx context.Context, kind, month string, fn func(*sql.DB) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	db, err := s.open(kind, month, false)
	if err != nil {
		return err
	}
	defer s.release(kind, month)
	return fn(db)
}

func (s *Store) Walk(ctx context.Context, kind string, desc bool, fn func(Shard) error) error {
	return s.WalkFiltered(ctx, kind, desc, nil, fn)
}

// WalkFiltered 在打开分库之前按 UTC 月首时间筛选；include 为 nil 时遍历全部。
// 筛选不读取数据库，不相关分库的损坏不会阻断本次遍历。
func (s *Store) WalkFiltered(ctx context.Context, kind string, desc bool, include func(time.Time) bool, fn func(Shard) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	months, err := s.months(kind, desc)
	if err != nil {
		return err
	}
	for _, month := range months {
		if err := ctx.Err(); err != nil {
			return err
		}
		start, _ := time.Parse("2006-01", month)
		if include != nil && !include(start) {
			continue
		}
		db, err := s.open(kind, month, false)
		if err != nil {
			return err
		}
		err = func() error {
			defer s.release(kind, month)
			return fn(Shard{Month: month, Start: start, DB: db})
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeleteBefore(ctx context.Context, kind string, before time.Time) error {
	if before.IsZero() {
		return errors.New("monthly SQLite delete time is zero")
	}
	if !time.Unix(0, before.UnixNano()).Equal(before) {
		return errors.New("monthly SQLite delete time is outside the Unix nanosecond range")
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	spec, err := s.getSpec(kind)
	if err != nil {
		return err
	}
	months, err := s.months(kind, false)
	if err != nil {
		return err
	}
	cutoff := before.UTC()
	cutoffMonth := cutoff.Format("2006-01")
	for _, month := range months {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch strings.Compare(month, cutoffMonth) {
		case -1:
			if err := s.remove(kind, month); err != nil {
				return err
			}
		case 0:
			db, err := s.open(kind, month, false)
			if err != nil {
				return err
			}
			query := fmt.Sprintf("DELETE FROM %s WHERE %s < ?", spec.Table, spec.TimeColumn)
			_, err = db.ExecContext(ctx, query, cutoff.UnixNano())
			s.release(kind, month)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Close() error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	for key, db := range s.dbs {
		s.closeErr = errors.Join(s.closeErr, db.db.Close())
		delete(s.dbs, key)
	}
	return s.closeErr
}

func (s *Store) getSpec(kind string) (Spec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Spec{}, errors.New("monthly SQLite store is closed")
	}
	spec, ok := s.spec[kind]
	if !ok {
		return Spec{}, fmt.Errorf("monthly SQLite kind %q is not registered", kind)
	}
	return spec, nil
}

func (s *Store) open(kind, month string, create bool) (*sql.DB, error) {
	spec, err := s.getSpec(kind)
	if err != nil {
		return nil, err
	}
	if !monthPattern.MatchString(month + ".db") {
		return nil, fmt.Errorf("invalid monthly SQLite month %q", month)
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, fmt.Errorf("invalid monthly SQLite month %q", month)
	}
	key := kind + "/" + month
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.dbs[key]; entry != nil {
		entry.users++
		return entry.db, nil
	}
	dir := filepath.Join(s.root, kind)
	filename := filepath.Join(dir, month+".db")
	if !create {
		if _, err := os.Stat(filename); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	// Connection-local settings must also apply after an idle connection is reopened.
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath}
	params := url.Values{"_pragma": {"busy_timeout(5000)", "journal_mode(WAL)", "synchronous(NORMAL)"}}
	dsn.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(time.Minute)
	if _, err := db.Exec(spec.Schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize %s/%s: %w", kind, month, err)
	}
	s.dbs[key] = &cachedDB{db: db, users: 1}
	return db, nil
}

// A shard remains pinned throughout its callback, including active transactions.
func (s *Store) release(kind, month string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock++
	entry := s.dbs[kind+"/"+month]
	entry.users--
	entry.used = s.clock
	idle := 0
	var oldest string
	for key, candidate := range s.dbs {
		if candidate.users != 0 {
			continue
		}
		idle++
		if oldest == "" || candidate.used < s.dbs[oldest].used {
			oldest = key
		}
	}
	if idle > maxIdleShards {
		s.closeErr = errors.Join(s.closeErr, s.dbs[oldest].db.Close())
		delete(s.dbs, oldest)
	}
}

func (s *Store) months(kind string, desc bool) ([]string, error) {
	if _, err := s.getSpec(kind); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, kind)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	months := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && monthPattern.MatchString(entry.Name()) {
			month := strings.TrimSuffix(entry.Name(), ".db")
			if _, err := time.Parse("2006-01", month); err == nil {
				months = append(months, month)
			}
		}
	}
	sort.Strings(months)
	if desc {
		for left, right := 0, len(months)-1; left < right; left, right = left+1, right-1 {
			months[left], months[right] = months[right], months[left]
		}
	}
	return months, nil
}

func (s *Store) remove(kind, month string) error {
	key := kind + "/" + month
	s.mu.Lock()
	if db := s.dbs[key]; db != nil {
		if err := db.db.Close(); err != nil {
			s.mu.Unlock()
			return err
		}
		delete(s.dbs, key)
	}
	s.mu.Unlock()
	filename := filepath.Join(s.root, kind, month+".db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(filename + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

type Scanner interface {
	Scan(...any) error
}

type PageQuery struct {
	Columns  string
	Where    string
	Args     []any
	PageNum  int
	PageSize int
}

func Page[T any](ctx context.Context, store *Store, kind string, query PageQuery, scan func(Scanner) (T, error)) ([]T, int64, error) {
	if store == nil {
		return nil, 0, errors.New("monthly SQLite store is not registered")
	}
	if query.PageNum < 0 || query.PageSize < 0 {
		return nil, 0, errors.New("monthly SQLite page values cannot be negative")
	}
	if query.PageSize > 0 && int64(query.PageNum) > math.MaxInt64/int64(query.PageSize) {
		return nil, 0, errors.New("monthly SQLite page offset overflows int64")
	}
	spec, err := store.getSpec(kind)
	if err != nil {
		return nil, 0, err
	}
	store.opMu.RLock()
	defer store.opMu.RUnlock()
	where := ""
	if strings.TrimSpace(query.Where) != "" {
		where = " WHERE " + query.Where
	}
	var total int64
	type source struct {
		month string
		count int64
	}
	sources := []source{}
	months, err := store.months(kind, true)
	if err != nil {
		return nil, 0, err
	}
	for _, month := range months {
		db, err := store.open(kind, month, false)
		if err != nil {
			return nil, 0, err
		}
		var count int64
		statement := fmt.Sprintf("SELECT COUNT(*) FROM %s%s", spec.Table, where)
		err = db.QueryRowContext(ctx, statement, query.Args...).Scan(&count)
		store.release(kind, month)
		if err != nil {
			return nil, 0, err
		}
		total += count
		sources = append(sources, source{month: month, count: count})
	}
	offset := int64(query.PageNum) * int64(query.PageSize)
	if query.PageSize == 0 {
		offset = 0
	}
	result := []T{}
	remaining := query.PageSize
	for _, source := range sources {
		if offset >= source.count {
			offset -= source.count
			continue
		}
		statement := fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s DESC, id DESC", query.Columns, spec.Table, where, spec.TimeColumn)
		args := append([]any(nil), query.Args...)
		if query.PageSize > 0 {
			statement += " LIMIT ? OFFSET ?"
			args = append(args, remaining, offset)
		}
		db, err := store.open(kind, source.month, false)
		if err != nil {
			return nil, 0, err
		}
		err = func() error {
			defer store.release(kind, source.month)
			rows, err := db.QueryContext(ctx, statement, args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				item, err := scan(rows)
				if err != nil {
					_ = rows.Close()
					return err
				}
				result = append(result, item)
			}
			if err := rows.Close(); err != nil {
				return err
			}
			if err := rows.Err(); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return nil, 0, err
		}
		if query.PageSize > 0 {
			remaining = query.PageSize - len(result)
			if remaining == 0 {
				break
			}
		}
		offset = 0
	}
	return result, total, nil
}
