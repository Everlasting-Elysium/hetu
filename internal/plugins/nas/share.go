package nas

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/httpjson"
)

// tokenBytes is the number of random bytes used for share tokens (32 bytes =
// 64 hex chars). This produces cryptographically unpredictable tokens.
const tokenBytes = 32

// createShareReq is the JSON body for POST /api/nas/shares.
type createShareReq struct {
	TargetType string `json:"target_type"` // "file" or "folder"
	TargetPath string `json:"target_path"` // storage-relative path
	ExpiresIn  int64  `json:"expires_in"`  // seconds from now; 0 = never
	Password   string `json:"password"`    // optional plaintext; stored as bcrypt hash
	Permission string `json:"permission"`  // "read" (default)
}

// createShareResp is the JSON response for a successful share creation.
type createShareResp struct {
	ID        string  `json:"id"`
	Token     string  `json:"token"`
	URL       string  `json:"url"`       // relative path: /s/<token>
	ExpiresAt *string `json:"expires_at"` // RFC3339 or null
}

// createShare handles POST /api/nas/shares.
func (p *Plugin) createShare(w http.ResponseWriter, r *http.Request) {
	var req createShareReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("decode body: %w", err))
		return
	}
	if err := validateShareReq(req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err)
		return
	}

	token, err := generateToken()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("generate token: %w", err))
		return
	}

	var passwordHash string
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("hash password: %w", err))
			return
		}
		passwordHash = string(hash)
	}

	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().UTC().Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	perm := req.Permission
	if perm == "" {
		perm = "read"
	}

	id, err := uuid.NewV7()
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("generate id: %w", err))
		return
	}
	sid, _ := domain.NewShareID(id.String())

	share := domain.Share{
		ID:           sid,
		Owner:        p.owner,
		TargetType:   req.TargetType,
		TargetID:     req.TargetPath,
		Token:        token,
		ExpiresAt:    expiresAt,
		PasswordHash: passwordHash,
		Permission:   perm,
		CreatedAt:    time.Now().UTC(),
	}
	if err := p.k.Store.CreateShare(r.Context(), share); err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, fmt.Errorf("create share: %w", err))
		return
	}

	resp := createShareResp{
		ID:    id.String(),
		Token: token,
		URL:   "/s/" + token,
	}
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		resp.ExpiresAt = &s
	}
	httpjson.WriteJSON(w, http.StatusCreated, resp)
}

// accessShare handles GET /s/{token}. It validates the share (expiry,
// password) and streams the file or lists the directory.
func (p *Plugin) accessShare(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		httpjson.WriteError(w, http.StatusBadRequest, fmt.Errorf("missing token"))
		return
	}

	share, err := p.k.Store.GetShareByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpjson.WriteError(w, http.StatusNotFound, fmt.Errorf("share not found"))
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err)
		return
	}

	if share.ExpiresAt != nil && time.Now().UTC().After(*share.ExpiresAt) {
		httpjson.WriteError(w, http.StatusGone, fmt.Errorf("share link has expired"))
		return
	}

	if share.PasswordHash != "" {
		password := r.URL.Query().Get("password")
		if password == "" {
			httpjson.WriteError(w, http.StatusUnauthorized, fmt.Errorf("password required"))
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(share.PasswordHash), []byte(password)); err != nil {
			httpjson.WriteError(w, http.StatusForbidden, fmt.Errorf("incorrect password"))
			return
		}
	}

	path := share.TargetID
	switch share.TargetType {
	case "file":
		p.serveFile(w, r, path)
	case "folder":
		// Allow browsing into subdirectories via ?path= relative to the shared root.
		// Clean the sub-path to prevent traversal outside the shared folder.
		sub := r.URL.Query().Get("path")
		if sub != "" {
			clean := filepath.Join("/", sub)
			joined := filepath.Join(path, clean)
			if !strings.HasPrefix(joined, path) {
				httpjson.WriteError(w, http.StatusForbidden,
					fmt.Errorf("path escapes shared folder"))
				return
			}
			path = joined
		}
		p.serveDir(w, r, path)
	default:
		httpjson.WriteError(w, http.StatusBadRequest,
			fmt.Errorf("unsupported share target type %q", share.TargetType))
	}
}

func validateShareReq(req createShareReq) error {
	if req.TargetType != "file" && req.TargetType != "folder" {
		return fmt.Errorf("target_type must be \"file\" or \"folder\"")
	}
	if req.TargetPath == "" {
		return fmt.Errorf("target_path must not be empty")
	}
	if req.Permission != "" && req.Permission != "read" {
		return fmt.Errorf("permission must be \"read\"")
	}
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}
