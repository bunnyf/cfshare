package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	tests := []struct {
		length int
	}{
		{8},
		{12},
		{16},
		{32},
	}

	for _, tt := range tests {
		pwd := GeneratePassword(tt.length)
		if len(pwd) != tt.length {
			t.Errorf("GeneratePassword(%d) = %q, length = %d, want %d",
				tt.length, pwd, len(pwd), tt.length)
		}
	}
}

func TestGeneratePasswordUniqueness(t *testing.T) {
	passwords := make(map[string]bool)
	for i := 0; i < 100; i++ {
		pwd := GeneratePassword(16)
		if passwords[pwd] {
			t.Errorf("duplicate password generated: %s", pwd)
		}
		passwords[pwd] = true
	}
}

func TestGeneratePasswordCharset(t *testing.T) {
	charset := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charMap := make(map[rune]bool)
	for _, c := range charset {
		charMap[c] = true
	}

	pwd := GeneratePassword(100) // 长密码更容易覆盖字符集
	for _, c := range pwd {
		if !charMap[c] {
			t.Errorf("invalid character in password: %c", c)
		}
	}
}

func TestBasicAuthMiddleware_Success(t *testing.T) {
	username := "testuser"
	password := "testpass"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	protected := BasicAuthMiddleware(username, password, handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "success" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestBasicAuthMiddleware_NoAuth(t *testing.T) {
	username := "testuser"
	password := "testpass"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := BasicAuthMiddleware(username, password, handler)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBasicAuthMiddleware_WrongPassword(t *testing.T) {
	username := "testuser"
	password := "testpass"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := BasicAuthMiddleware(username, password, handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth(username, "wrongpass")
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestBasicAuthMiddleware_WrongUsername(t *testing.T) {
	username := "testuser"
	password := "testpass"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	protected := BasicAuthMiddleware(username, password, handler)

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("wronguser", password)
	w := httptest.NewRecorder()

	protected.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
