package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/charle-z/pixelgrama/internal/core"
	"github.com/charle-z/pixelgrama/internal/ratelimit"
	"github.com/charle-z/pixelgrama/internal/store"
)

const (
	DefaultWallLimit = 24
	MaxWallLimit     = 64
	MaxWallPage      = 1000
	defaultBodyLimit = 4096
)

type Config struct {
	Store             *store.Store
	Limiter           *ratelimit.Limiter
	Commit            string
	RepoURL           string
	PRURL             string
	Now               func() time.Time
	BodyLimit         int64
	TrustedProxyCIDRs []netip.Prefix
	RateLimitWindow   time.Duration
}

type server struct {
	store             *store.Store
	limiter           *ratelimit.Limiter
	commit            string
	repoURL           string
	prURL             string
	page              []byte
	csp               string
	now               func() time.Time
	bodyLimit         int64
	trustedProxyCIDRs []netip.Prefix
	rateLimitWindow   time.Duration
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type wallResponse struct {
	Postcards []store.Postcard `json:"postcards"`
	Page      int              `json:"page"`
	Limit     int              `json:"limit"`
}

func New(config Config) (http.Handler, error) {
	if config.Store == nil {
		return nil, errors.New("store is required")
	}
	if config.Limiter == nil {
		return nil, errors.New("limiter is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.BodyLimit <= 0 {
		config.BodyLimit = defaultBodyLimit
	}
	if config.RateLimitWindow <= 0 {
		config.RateLimitWindow = time.Minute
	}
	page, csp, err := buildPage(config.Commit, config.RepoURL, config.PRURL)
	if err != nil {
		return nil, err
	}
	return &server{
		store:             config.Store,
		limiter:           config.Limiter,
		commit:            config.Commit,
		repoURL:           config.RepoURL,
		prURL:             config.PRURL,
		page:              page,
		csp:               csp,
		now:               config.Now,
		bodyLimit:         config.BodyLimit,
		trustedProxyCIDRs: append([]netip.Prefix(nil), config.TrustedProxyCIDRs...),
		rateLimitWindow:   config.RateLimitWindow,
	}, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w, s.csp)
	switch r.URL.Path {
	case "/":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		http.Redirect(w, r, "/wall", http.StatusPermanentRedirect)
	case "/postcard":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
			return
		}
		s.handlePostcard(w, r)
	case "/wall":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		s.handleWall(w, r)
	case "/readyz":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		if err := s.store.Ready(r.Context()); err != nil {
			s.writeError(w, http.StatusServiceUnavailable, "not_ready", "storage is not ready")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	case "/healthz":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case "/version":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]string{
			"commit":       s.commit,
			"repository":   s.repoURL,
			"pull_request": s.prURL,
		})
	default:
		s.writeError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (s *server) handlePostcard(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		s.writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	body := http.MaxBytesReader(w, r.Body, s.bodyLimit)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	var payload struct {
		Pixels json.RawMessage `json:"pixels"`
		Alias  *string         `json:"alias,omitempty"`
	}
	if err := decoder.Decode(&payload); err != nil {
		s.writeDecodeError(w, err)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			s.writeDecodeError(w, err)
		} else {
			s.writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON object")
		}
		return
	}
	if len(payload.Pixels) == 0 || string(payload.Pixels) == "null" {
		s.writeError(w, http.StatusBadRequest, "missing_pixels", "pixels is required and must be an array")
		return
	}
	trimmedPixels := bytes.TrimSpace(payload.Pixels)
	if len(trimmedPixels) == 0 || trimmedPixels[0] != '[' {
		s.writeError(w, http.StatusBadRequest, "invalid_pixels_type", "pixels must be an array of exactly 256 integers")
		return
	}

	pixels, err := decodePixels(payload.Pixels)
	if err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "invalid_pixels", err.Error())
		return
	}
	if err := core.ValidateAlias(payload.Alias); err != nil {
		s.writeError(w, http.StatusUnprocessableEntity, "invalid_alias", err.Error())
		return
	}
	if !s.limiter.Allow(s.clientIP(r)) {
		retrySeconds := int((s.rateLimitWindow + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
		s.writeError(w, http.StatusTooManyRequests, "rate_limited", "postcard rate limit exceeded")
		return
	}

	item, err := s.store.Insert(r.Context(), pixels, payload.Alias, s.commit, s.now())
	if errors.Is(err, store.ErrDuplicate) {
		s.writeError(w, http.StatusConflict, "duplicate_postcard", "postcard is identical to the latest postcard")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "storage_error", "postcard could not be stored")
		return
	}
	s.writeJSON(w, http.StatusCreated, item)
}

func (s *server) handleWall(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") != "json" && !strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(s.page)
		return
	}

	page, err := positiveQueryInt(r, "page", 1)
	if err != nil || page > MaxWallPage {
		s.writeError(w, http.StatusBadRequest, "invalid_page", fmt.Sprintf("page must be between 1 and %d", MaxWallPage))
		return
	}
	limit, err := positiveQueryInt(r, "limit", DefaultWallLimit)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
		return
	}
	if limit > MaxWallLimit {
		limit = MaxWallLimit
	}
	offset := (page - 1) * limit
	items, err := s.store.List(r.Context(), limit, offset)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "storage_error", "wall could not be loaded")
		return
	}
	s.writeJSON(w, http.StatusOK, wallResponse{Postcards: items, Page: page, Limit: limit})
}

func decodePixels(raw json.RawMessage) (core.Pixels, error) {
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return core.Pixels{}, errors.New("pixels must be an array of exactly 256 integers")
	}
	if len(elements) != core.PixelCount {
		return core.Pixels{}, fmt.Errorf("pixels must contain exactly %d integers", core.PixelCount)
	}
	values := make([]int, core.PixelCount)
	for i, element := range elements {
		text := string(element)
		if text == "" || !isIntegerLexeme(text) {
			return core.Pixels{}, fmt.Errorf("pixel %d must be an integer between 0 and 15", i)
		}
		value, err := strconv.Atoi(text)
		if err != nil || value < 0 || value >= core.PaletteSize {
			return core.Pixels{}, fmt.Errorf("pixel %d must be an integer between 0 and 15", i)
		}
		values[i] = value
	}
	return core.FromInts(values)
}

func isIntegerLexeme(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func positiveQueryInt(r *http.Request, name string, fallback int) (int, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, errors.New("value must be positive")
	}
	return parsed, nil
}

func (s *server) clientIP(r *http.Request) string {
	remote := remoteAddress(r.RemoteAddr)
	if !s.isTrustedProxy(remote) {
		return remote.String()
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(forwarded) - 1; i >= 0; i-- {
		address, err := netip.ParseAddr(strings.TrimSpace(forwarded[i]))
		if err != nil {
			return remote.String()
		}
		address = address.Unmap()
		if !s.isTrustedProxy(address) {
			return address.String()
		}
	}
	return remote.String()
}

func remoteAddress(value string) netip.Addr {
	host, _, err := net.SplitHostPort(value)
	if err == nil {
		if address, parseErr := netip.ParseAddr(host); parseErr == nil {
			return address.Unmap()
		}
	}
	address, _ := netip.ParseAddr(value)
	return address.Unmap()
}

func (s *server) isTrustedProxy(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	for _, prefix := range s.trustedProxyCIDRs {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func setSecurityHeaders(w http.ResponseWriter, csp string) {
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "accelerometer=(), camera=(), geolocation=(), gyroscope=(), microphone=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Cache-Control", "no-store")
}

func (s *server) writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		s.writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", fmt.Sprintf("request body exceeds %d bytes", s.bodyLimit))
		return
	}
	s.writeError(w, http.StatusBadRequest, "invalid_json", "request body must be one valid JSON object with no unknown fields")
}

func (s *server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeJSON(w, status, errorResponse{Error: code, Message: message})
}

func (s *server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
