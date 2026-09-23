package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
)

func setup(t *testing.T) (*Server, *echo.Echo) {
	t.Helper()
	s, err := newServer(Config{Dir: t.TempDir(), Password: "test", Digits: 6, TTL: time.Minute, MaxBytes: 1024, MaxShares: 8})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.sessions["test"] = time.Now().Add(time.Hour)
	e := echo.New()
	s.Register(e)
	return s, e
}
func call(e *echo.Echo, method, url, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if auth {
		r.AddCookie(&http.Cookie{Name: "writer", Value: "test"})
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	return w
}
func TestFiles(t *testing.T) {
	s, e := setup(t)
	if w := call(e, "PUT", "/api/files?path=a.txt", "hello", false); w.Code != 401 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(e, "PUT", "/api/files?path=a.txt", "hello", true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(e, "PUT", "/api/files?path=a.txt", "other", true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(e, "GET", "/api/download?path=a.txt", "", false); w.Code != 200 || w.Body.String() != "hello" {
		t.Fatal(w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/download?path=a.txt", nil)
	r.Header.Set("Range", "bytes=1-3")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != "ell" {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(e, "PUT", "/api/files?path=large", strings.Repeat("x", 1025), true); w.Code != 413 {
		t.Fatal("oversized upload accepted")
	}
	if _, err := s.root.Stat("large"); !os.IsNotExist(err) {
		t.Fatal("partial file published", err)
	}
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0600)
	if err := os.Symlink(outside, filepath.Join(s.cfg.Dir, "outside")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", "outside/secret", ".uploader.lock"} {
		if w := call(e, "GET", "/api/download?path="+path, "", false); w.Code < 400 {
			t.Fatal("escaped root", path)
		}
	}
	if w := call(e, "DELETE", "/api/files?path=a.txt", "", true); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestUploadCreatesDirectories(t *testing.T) {
	s, e := setup(t)
	if w := call(e, "PUT", "/api/files?path=sub/dir/a.txt", "hello", true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	i, err := s.root.Stat("sub/dir/a.txt")
	if err != nil || !i.Mode().IsRegular() {
		t.Fatal("nested upload missing", err)
	}
	if w := call(e, "PUT", "/api/files?path=a.txt", "x", true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(e, "PUT", "/api/files?path=a.txt/conflict", "x", true); w.Code != 400 {
		t.Fatal("upload through a file accepted", w.Code, w.Body.String())
	}
	if w := call(e, "PUT", "/api/files?path=a.txt/deep/x", "x", true); w.Code != 400 {
		t.Fatal("upload through nested file accepted", w.Code, w.Body.String())
	}
}

func TestAuth(t *testing.T) {
	s, e := setup(t)
	if w := call(e, "GET", "/api/auth", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", "/api/auth", strings.NewReader(`{"password":"test"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Result().Cookies()) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe cookie")
	}
	r = httptest.NewRequest("GET", "/api/auth", nil)
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	e.ServeHTTP(w, r)
	var session struct {
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &session) != nil || !session.ExpiresAt.After(time.Now()) {
		t.Fatal("session not restored", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("DELETE", "/api/files?path=a", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://attacker.invalid")
	w = httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-site write accepted", w.Code)
	}
	s.mu.Lock()
	s.sessions[cookie.Value] = time.Now().Add(-time.Second)
	s.mu.Unlock()
	for _, method := range []string{"GET", "DELETE"} {
		url := "/api/auth"
		if method == "DELETE" {
			url = "/api/files?path=a"
		}
		r = httptest.NewRequest(method, url, nil)
		r.AddCookie(cookie)
		w = httptest.NewRecorder()
		e.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("expired session accepted", method, w.Code)
		}
	}
}

type brokenBody struct{ err error }

func (b brokenBody) Read([]byte) (int, error) { return 0, b.err }
func (b brokenBody) Close() error             { return nil }
func TestUploadPreservesIOError(t *testing.T) {
	s, e := setup(t)
	cause := errors.New("injected I/O failure")
	var received error
	defaultHandler := echo.DefaultHTTPErrorHandler(false)
	e.HTTPErrorHandler = func(c *echo.Context, err error) { received = err; defaultHandler(c, err) }
	r := httptest.NewRequest("PUT", "/api/files?path=broken.txt", nil)
	r.Body = brokenBody{cause}
	r.AddCookie(&http.Cookie{Name: "writer", Value: "test"})
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != 500 || !errors.Is(received, cause) {
		t.Fatal("I/O cause lost", w.Code, received)
	}
	if strings.Contains(w.Body.String(), cause.Error()) {
		t.Fatal("internal error exposed to client")
	}
	if _, err := s.root.Stat("broken.txt"); !os.IsNotExist(err) {
		t.Fatal("failed upload published", err)
	}
	items, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".upload-") {
			t.Fatal("temporary upload leaked")
		}
	}
}
func createTask(t *testing.T, e *echo.Echo, size int) (string, string) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"name": "test.bin", "size": size})
	w := call(e, "POST", "/api/shares", string(b), true)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct{ Code, Owner string }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Code, result.Owner
}
func TestCodeLengths(t *testing.T) {
	s, e := setup(t)
	for _, digits := range []int{4, 6, 8} {
		s.cfg.Digits = digits
		code, _ := createTask(t, e, 1)
		if len(code) != digits || strings.Trim(code, "0123456789") != "" {
			t.Fatal(code)
		}
	}
}
func TestSingleReceiver(t *testing.T) {
	_, e := setup(t)
	code, _ := createTask(t, e, 1)
	var wg sync.WaitGroup
	status := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); status <- call(e, "POST", "/api/shares/"+code+"/claim", "", false).Code }()
	}
	wg.Wait()
	close(status)
	got := map[int]int{}
	for n := range status {
		got[n]++
	}
	if got[204] != 1 || got[409] != 1 {
		t.Fatal(got)
	}
}
func TestStreamingAndRetry(t *testing.T) {
	s, e := setup(t)
	code, owner := createTask(t, e, 6)
	server := httptest.NewServer(e)
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	claim := call(e, "POST", "/api/shares/"+code+"/claim", "", false)
	if claim.Code != 204 {
		t.Fatal(claim.Code)
	}
	receiver := claim.Result().Cookies()[0]
	reader, writer := io.Pipe()
	defer writer.Close()
	req, _ := http.NewRequest("PUT", server.URL+"/api/shares/"+code, reader)
	req.AddCookie(&http.Cookie{Name: "writer", Value: "test"})
	req.Header.Set("X-Share-Owner", owner)
	sent := make(chan error, 1)
	go func() {
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			if res.StatusCode != 201 {
				err = io.ErrUnexpectedEOF
			}
		}
		sent <- err
	}()
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	download, _ := http.NewRequest("GET", server.URL+"/api/shares/"+code+"/download", nil)
	download.AddCookie(receiver)
	res, err := client.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	first := make([]byte, 3)
	if _, err = io.ReadFull(res.Body, first); err != nil || string(first) != "abc" {
		t.Fatal(string(first), err)
	}
	s.mu.Lock()
	task := s.tasks[code]
	s.mu.Unlock()
	task.mu.Lock()
	done := task.done
	task.mu.Unlock()
	if done {
		t.Fatal("download did not stream during upload")
	}
	writer.Write([]byte("def"))
	writer.Close()
	rest, err := io.ReadAll(res.Body)
	if err != nil || string(rest) != "def" {
		t.Fatal(string(rest), err)
	}
	if err = <-sent; err != nil {
		t.Fatal(err)
	}
	res, err = client.Do(download)
	if err != nil {
		t.Fatal(err)
	}
	all, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !bytes.Equal(all, []byte("abcdef")) {
		t.Fatal(string(all), err)
	}
	s.clean(time.Now().Add(2 * time.Minute))
	if _, err = os.Stat(task.file.Name()); !os.IsNotExist(err) {
		t.Fatal("expired file remains", err)
	}
}
func TestFailedUpload(t *testing.T) {
	s, e := setup(t)
	code, owner := createTask(t, e, 6)
	r := httptest.NewRequest("PUT", "/api/shares/"+code, strings.NewReader("abc"))
	r.AddCookie(&http.Cookie{Name: "writer", Value: "test"})
	r.Header.Set("X-Share-Owner", owner)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	s.mu.Lock()
	task := s.tasks[code]
	s.mu.Unlock()
	task.mu.Lock()
	defer task.mu.Unlock()
	if !task.failed || task.done {
		t.Fatal("incomplete upload marked successful")
	}
}

func TestStorageLockAndRestartCleanup(t *testing.T) {
	c := Config{Dir: t.TempDir(), Password: "test", Digits: 6, TTL: time.Minute, MaxBytes: 1024, MaxShares: 8}
	s, err := newServer(c)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := newServer(c); err == nil {
		second.Close()
		s.Close()
		t.Fatal("second instance accepted")
	}
	s.Close()
	if err = os.MkdirAll(filepath.Join(c.Dir, ".uploader-shares"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(c.Dir, ".uploader-shares", "stale"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(c.Dir, "keep.txt"), []byte("permanent"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = newServer(c)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = os.Stat(filepath.Join(c.Dir, ".uploader-shares", "stale")); !os.IsNotExist(err) {
		t.Fatal("stale share not removed", err)
	}
	if b, err := os.ReadFile(filepath.Join(c.Dir, "keep.txt")); err != nil || string(b) != "permanent" {
		t.Fatal("permanent file affected", err)
	}
}
