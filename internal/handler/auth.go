package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "readress_session"

func passwordDigest(password string) string {
	sum := sha256.Sum256([]byte("readress:" + password))
	return "sha256:" + base64.RawURLEncoding.EncodeToString(sum[:])
}

func (a *App) signSession(userID int64, expires int64) string {
	payload := fmt.Sprintf("%d:%d", userID, expires)
	mac := hmac.New(sha256.New, a.sessionKey)
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + sig
}

func (a *App) verifySession(value string) (int64, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return 0, false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	payload := string(payloadBytes)
	mac := hmac.New(sha256.New, a.sessionKey)
	mac.Write([]byte(payload))
	expected := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(got, expected) {
		return 0, false
	}
	fields := strings.Split(payload, ":")
	if len(fields) != 2 {
		return 0, false
	}
	userID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, false
	}
	expires, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return 0, false
	}
	return userID, true
}

func (a *App) setSession(w http.ResponseWriter, userID int64, remember bool) {
	maxAge := 86400
	if remember {
		maxAge = 86400 * 30
	}
	expires := time.Now().Add(time.Duration(maxAge) * time.Second)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    a.signSession(userID, expires.Unix()),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Expires:  expires,
	})
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (a *App) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	userID, ok := a.verifySession(cookie.Value)
	return ok && userID == a.userID
}

func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/login?next="+r.URL.RequestURI(), http.StatusSeeOther)
	})
}
