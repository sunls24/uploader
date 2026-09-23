package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Dir          string        `env:"STORAGE_DIR" envDefault:"./fs"`
	Password     string        `env:"ADMIN_PASSWORD"`
	Digits       int           `env:"SHARE_CODE_LENGTH" envDefault:"8"`
	TTL          time.Duration `env:"SHARE_TTL" envDefault:"30m"`
	MaxBytes     int64         `env:"MAX_FILE_BYTES" envDefault:"1073741824"`
	MaxShares    int           `env:"MAX_SHARES" envDefault:"8"`
	CookieSecure bool          `env:"COOKIE_SECURE" envDefault:"false"`
}
type task struct {
	mu                                       sync.Mutex
	name                                     string
	size, written                            int64
	expires                                  time.Time
	file                                     *os.File
	owner, receiver                          string
	uploading, done, failed, active, expired bool
	changed                                  chan struct{}
}

func (t *task) signal() { close(t.changed); t.changed = make(chan struct{}) }

type Server struct {
	cfg      Config
	root     *os.Root
	temp     string
	lock     *os.File
	hash     []byte
	mu       sync.Mutex
	sessions map[string]time.Time
	tasks    map[string]*task
	stop     chan struct{}
	wg       sync.WaitGroup
	uploads  chan struct{}
}

func token() string { return rand.Text() }
func New() (*Server, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, err
	}
	return newServer(c)
}
func newServer(c Config) (*Server, error) {
	if c.Password == "" {
		return nil, errors.New("ADMIN_PASSWORD must be configured")
	}
	if c.Digits < 4 || c.Digits > 8 || c.TTL <= 0 || c.MaxBytes <= 0 || c.MaxShares < 1 || c.MaxShares > 1000 {
		return nil, errors.New("invalid storage/share limits")
	}
	if err := os.MkdirAll(c.Dir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(c.Dir)
	if err != nil {
		return nil, err
	}
	lock, err := root.OpenFile(".uploader.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		root.Close()
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		root.Close()
		return nil, errors.New("storage directory is already in use")
	}
	// Only this application's reserved temporary directory is removed on restart.
	if err = root.RemoveAll(".uploader-shares"); err == nil {
		err = root.Mkdir(".uploader-shares", 0700)
	}
	if err != nil {
		lock.Close()
		root.Close()
		return nil, err
	}
	temp := filepath.Join(root.Name(), ".uploader-shares")
	digest := sha256.Sum256([]byte(c.Password))
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(digest[:])), bcrypt.DefaultCost)
	if err != nil {
		lock.Close()
		root.Close()
		os.RemoveAll(temp)
		return nil, err
	}
	c.Password = ""
	s := &Server{cfg: c, root: root, temp: temp, lock: lock, hash: hash, sessions: map[string]time.Time{}, tasks: map[string]*task{}, stop: make(chan struct{}), uploads: make(chan struct{}, 3)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-s.stop:
				return
			case now := <-tick.C:
				s.clean(now)
			}
		}
	}()
	return s, nil
}
func (s *Server) Close() {
	close(s.stop)
	s.wg.Wait()
	s.clean(time.Now().Add(100 * 365 * 24 * time.Hour))
	if err := os.RemoveAll(s.temp); err != nil {
		log.Error().Err(err).Msg("clean temporary shares")
	}
	s.lock.Close()
	s.root.Close()
}
func (s *Server) clean(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.sessions {
		if !now.Before(v) {
			delete(s.sessions, k)
		}
	}
	for k, t := range s.tasks {
		t.mu.Lock()
		if !now.Before(t.expires) {
			t.expired = true
			t.signal()
			if err := t.file.Close(); err != nil {
				log.Error().Err(err).Msg("close expired share")
			}
			if err := os.Remove(t.file.Name()); err != nil && !os.IsNotExist(err) {
				log.Error().Err(err).Msg("remove expired share")
			}
			delete(s.tasks, k)
		}
		t.mu.Unlock()
	}
}
func fail(status int, msg string) error { return echo.NewHTTPError(status, msg) }
func (s *Server) Register(e *echo.Echo) {
	e.IPExtractor = echo.ExtractIPDirect()
	g := e.Group("/api")
	g.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("Cache-Control", "no-store")
			c.Response().Header().Set("X-Content-Type-Options", "nosniff")
			if c.Request().Method != "GET" && c.Request().Method != "HEAD" {
				if c.Request().Header.Get("Sec-Fetch-Site") == "cross-site" {
					return fail(403, "跨站请求被拒绝")
				}
				if origin := c.Request().Header.Get("Origin"); origin != "" {
					u, err := url.Parse(origin)
					if err != nil || u.Host != c.Request().Host {
						return fail(403, "跨站请求被拒绝")
					}
				}
			}
			return next(c)
		}
	})
	loginLimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{Rate: 1, Burst: 5, ExpiresIn: 3 * time.Minute}))
	shareLimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(2))
	downloadLimit := middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(2))
	g.POST("/auth", s.login, loginLimit, middleware.BodyLimit(4<<10))
	g.GET("/auth", func(c *echo.Context) error {
		return c.JSON(200, map[string]any{"expiresAt": c.Get("writerExpires")})
	}, s.auth)
	g.GET("/files", s.list)
	g.GET("/download", s.download)
	g.HEAD("/download", s.download)
	g.PUT("/files", s.upload, s.auth)
	g.DELETE("/files", s.remove, s.auth)
	g.POST("/shares", s.create, s.auth, middleware.BodyLimit(4<<10))
	g.GET("/shares/:code", s.info, shareLimit)
	g.POST("/shares/:code/claim", s.claim, shareLimit)
	g.PUT("/shares/:code", s.send, s.auth)
	g.GET("/shares/:code/download", s.receive, downloadLimit)
}
func (s *Server) login(c *echo.Context) error {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&body); err != nil {
		return fail(400, "无效请求")
	}
	d := sha256.Sum256([]byte(body.Password))
	if bcrypt.CompareHashAndPassword(s.hash, []byte(hex.EncodeToString(d[:]))) != nil {
		return fail(401, "密码错误")
	}
	key := token()
	exp := time.Now().Add(15 * time.Minute)
	s.mu.Lock()
	s.sessions[key] = exp
	s.mu.Unlock()
	c.SetCookie(&http.Cookie{Name: "writer", Value: key, Path: "/api", HttpOnly: true, Secure: s.cfg.CookieSecure || c.Request().TLS != nil, SameSite: http.SameSiteStrictMode, Expires: exp})
	return c.JSON(200, map[string]any{"expiresAt": exp})
}
func (s *Server) auth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cookie, err := c.Cookie("writer")
		if err != nil {
			return fail(401, "请输入管理密码")
		}
		s.mu.Lock()
		exp := s.sessions[cookie.Value]
		s.mu.Unlock()
		if !time.Now().Before(exp) {
			return fail(401, "请重新输入管理密码")
		}
		c.Set("writerExpires", exp)
		return next(c)
	}
}
func (s *Server) path(c *echo.Context) (string, error) {
	p := c.QueryParam("path")
	if p == "" {
		return ".", nil
	}
	if !filepath.IsLocal(p) || strings.Contains(p, "\\") {
		return "", fail(400, "无效路径")
	}
	parts := strings.Split(p, "/")
	for _, v := range parts {
		if strings.HasPrefix(v, ".") {
			return "", fail(400, "不允许隐藏路径")
		}
	}
	part := ""
	for _, name := range parts {
		part = filepath.Join(part, name)
		i, err := s.root.Lstat(part)
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			break
		}
		if err != nil {
			return "", err
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return "", fail(400, "不允许符号链接路径")
		}
	}
	return p, nil
}
func (s *Server) list(c *echo.Context) error {
	p, err := s.path(c)
	if err != nil {
		return err
	}
	f, err := s.root.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(404, "目录不存在")
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return fail(400, "不是目录")
		}
		return err
	}
	defer f.Close()
	items, err := f.ReadDir(-1)
	if err != nil {
		if errors.Is(err, syscall.ENOTDIR) {
			return fail(400, "不是目录")
		}
		return err
	}
	out := []map[string]any{}
	for _, v := range items {
		if strings.HasPrefix(v.Name(), ".") || v.Type()&os.ModeSymlink != 0 {
			continue
		}
		i, err := v.Info()
		if err != nil {
			return err
		}
		if !i.IsDir() && !i.Mode().IsRegular() {
			continue
		}
		out = append(out, map[string]any{"name": i.Name(), "directory": i.IsDir(), "size": i.Size()})
	}
	return c.JSON(200, out)
}
func (s *Server) download(c *echo.Context) error {
	p, err := s.path(c)
	if err != nil {
		return err
	}
	f, err := s.root.Open(p)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return fail(404, "文件不存在")
		}
		return err
	}
	defer f.Close()
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if !i.Mode().IsRegular() {
		return fail(400, "不是文件")
	}
	if err := http.NewResponseController(c.Response()).SetWriteDeadline(time.Now().Add(2 * time.Hour)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	attachment(c, i.Name())
	http.ServeContent(c.Response(), c.Request(), i.Name(), i.ModTime(), f)
	return nil
}
func attachment(c *echo.Context, name string) {
	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
}
func (s *Server) upload(c *echo.Context) error {
	if err := http.NewResponseController(c.Response()).SetReadDeadline(time.Now().Add(30 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	select {
	case s.uploads <- struct{}{}:
		defer func() { <-s.uploads }()
	default:
		return fail(429, "同时上传数量已达上限")
	}
	p, err := s.path(c)
	if err != nil {
		return err
	}
	if p == "." {
		return fail(400, "需要文件名")
	}
	// Create missing parent directories so uploads can target nested paths.
	if dir := filepath.Dir(p); dir != "." {
		part := ""
		for _, name := range strings.Split(dir, "/") {
			part = filepath.Join(part, name)
			if err = s.root.Mkdir(part, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				if errors.Is(err, syscall.ENOTDIR) {
					return fail(400, "不是目录")
				}
				return err
			}
		}
	}
	tmp := filepath.Join(filepath.Dir(p), ".upload-"+token())
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(404, "目录不存在")
		}
		if errors.Is(err, syscall.ENOTDIR) {
			return fail(400, "不是目录")
		}
		return err
	}
	defer s.root.Remove(tmp)
	_, err = io.Copy(f, http.MaxBytesReader(c.Response(), c.Request().Body, s.cfg.MaxBytes))
	closeErr := f.Close()
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return echo.NewHTTPError(413, "文件超过大小限制").Wrap(err)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, c.Request().Context().Err()) {
			return echo.NewHTTPError(400, "上传中断").Wrap(err)
		}
		return fmt.Errorf("upload %q: %w", p, err)
	}
	if closeErr != nil {
		return closeErr
	}
	// Link publishes the completed file without replacing a concurrent upload.
	if err = s.root.Link(tmp, p); errors.Is(err, os.ErrExist) {
		return fail(409, "同名文件已存在")
	}
	if err != nil {
		return err
	}
	return c.NoContent(201)
}
func (s *Server) remove(c *echo.Context) error {
	p, err := s.path(c)
	if err != nil {
		return err
	}
	i, err := s.root.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			return fail(404, "文件不存在")
		}
		return err
	}
	if !i.Mode().IsRegular() {
		return fail(400, "仅支持删除普通文件")
	}
	if err = s.root.Remove(p); err != nil {
		return err
	}
	return c.NoContent(204)
}
func (s *Server) create(c *echo.Context) error {
	var b struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if c.Bind(&b) != nil || b.Name == "" || filepath.Base(b.Name) != b.Name || b.Size < 0 || b.Size > s.cfg.MaxBytes {
		return fail(400, "无效文件或超过大小限制")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.tasks) >= s.cfg.MaxShares {
		return fail(429, "临时共享名额已满")
	}
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(s.cfg.Digits)), nil)
	var code string
	for {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return err
		}
		code = fmt.Sprintf("%0*d", s.cfg.Digits, n.Int64())
		if s.tasks[code] == nil {
			break
		}
	}
	f, err := os.CreateTemp(s.temp, "share-")
	if err != nil {
		return err
	}
	t := &task{name: b.Name, size: b.Size, file: f, owner: token(), expires: time.Now().Add(s.cfg.TTL), changed: make(chan struct{})}
	s.tasks[code] = t
	return c.JSON(201, map[string]any{"code": code, "owner": t.owner, "expiresAt": t.expires})
}
func (s *Server) get(c *echo.Context) (*task, error) {
	s.mu.Lock()
	t := s.tasks[c.Param("code")]
	s.mu.Unlock()
	if t == nil {
		return nil, fail(404, "共享不存在或已过期")
	}
	if !time.Now().Before(t.expires) {
		return nil, fail(410, "共享已过期")
	}
	return t, nil
}
func (s *Server) info(c *echo.Context) error {
	t, err := s.get(c)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.expired {
		return fail(410, "已过期")
	}
	return c.JSON(200, map[string]any{"name": t.name, "size": t.size, "written": t.written, "done": t.done, "failed": t.failed, "claimed": t.receiver != "", "expiresAt": t.expires})
}
func (s *Server) claim(c *echo.Context) error {
	t, err := s.get(c)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.expired || t.failed {
		return fail(410, "共享不可用")
	}
	cookie, _ := c.Cookie("receiver-" + c.Param("code"))
	if t.receiver != "" {
		if cookie == nil || cookie.Value != t.receiver {
			return fail(409, "已被其他接收者领取")
		}
	} else {
		t.receiver = token()
	}
	c.SetCookie(&http.Cookie{Name: "receiver-" + c.Param("code"), Value: t.receiver, Path: "/api/shares/" + c.Param("code"), HttpOnly: true, Secure: s.cfg.CookieSecure || c.Request().TLS != nil, SameSite: http.SameSiteStrictMode, Expires: t.expires})
	return c.NoContent(204)
}
func (s *Server) send(c *echo.Context) error {
	t, err := s.get(c)
	if err != nil {
		return err
	}
	t.mu.Lock()
	if subtle.ConstantTimeCompare([]byte(c.Request().Header.Get("X-Share-Owner")), []byte(t.owner)) != 1 {
		t.mu.Unlock()
		return fail(403, "无发送权限")
	}
	if t.uploading || t.done || t.failed || t.expired {
		t.mu.Unlock()
		return fail(409, "任务不可上传")
	}
	t.uploading = true
	t.mu.Unlock()
	ok := false
	defer func() {
		t.mu.Lock()
		t.uploading = false
		t.done = ok
		t.failed = !ok
		t.signal()
		t.mu.Unlock()
	}()
	controller := http.NewResponseController(c.Response())
	if err := controller.SetReadDeadline(t.expires); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	body := http.MaxBytesReader(c.Response(), c.Request().Body, t.size)
	buf := make([]byte, 64*1024)
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			t.mu.Lock()
			if t.expired {
				t.mu.Unlock()
				return fail(410, "已过期")
			}
			written, e := t.file.WriteAt(buf[:n], t.written)
			t.written += int64(written)
			t.signal()
			t.mu.Unlock()
			if e != nil {
				return e
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fail(400, "上传中断或大小不符")
		}
	}
	t.mu.Lock()
	ok = t.written == t.size && !t.expired
	t.mu.Unlock()
	if !ok {
		return fail(400, "文件大小不符")
	}
	return c.NoContent(201)
}
func (s *Server) receive(c *echo.Context) error {
	t, err := s.get(c)
	if err != nil {
		return err
	}
	t.mu.Lock()
	cookie, e := c.Cookie("receiver-" + c.Param("code"))
	if e != nil || t.receiver == "" || cookie.Value != t.receiver {
		t.mu.Unlock()
		return fail(403, "请先领取")
	}
	if t.active || t.failed || t.expired {
		t.mu.Unlock()
		return fail(409, "共享不可下载或已有下载连接")
	}
	t.active = true
	t.mu.Unlock()
	defer func() { t.mu.Lock(); t.active = false; t.mu.Unlock() }()
	if err := http.NewResponseController(c.Response()).SetWriteDeadline(t.expires); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	attachment(c, t.name)
	c.Response().Header().Set("Content-Length", fmt.Sprint(t.size))
	c.Response().Header().Set("X-Accel-Buffering", "no")
	offset := int64(0)
	buf := make([]byte, 64*1024)
	for {
		t.mu.Lock()
		available := t.written - offset
		done, failed, ch := t.done, t.failed || t.expired, t.changed
		// Hold the final byte until successful upload completion, so failure cannot look complete.
		if !done && offset+available == t.size && available > 0 {
			available--
		}
		n := 0
		var readErr error
		if !failed && available > 0 {
			if available > int64(len(buf)) {
				available = int64(len(buf))
			}
			n, readErr = t.file.ReadAt(buf[:available], offset)
		}
		t.mu.Unlock()
		if failed || readErr != nil {
			panic(http.ErrAbortHandler)
		}
		if n > 0 {
			if _, err = c.Response().Write(buf[:n]); err != nil {
				return err
			}
			offset += int64(n)
			if err = http.NewResponseController(c.Response()).Flush(); err != nil {
				return err
			}
			continue
		}
		if done && offset == t.size {
			return nil
		}
		select {
		case <-ch:
		case <-c.Request().Context().Done():
			return c.Request().Context().Err()
		}
	}
}
