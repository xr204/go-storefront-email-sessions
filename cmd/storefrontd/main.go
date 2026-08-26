package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"example.com/infrai-storefront/internal/store"
)

type server struct{ service *store.Service }

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	s := &server{service: store.NewService(&store.InfraiClient{APIKey: key})}
	log.Println("storefrontd listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", s.routes()))
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", s.signup)
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /checkout", s.checkout)
	mux.HandleFunc("POST /orders/{id}/fulfill", s.fulfill)
	mux.HandleFunc("GET /orders/{id}", s.order)
	return mux
}

func (s *server) signup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, Name, CaptchaToken string }
	if !decode(w, r, &in) {
		return
	}
	err := s.service.Signup(r.Context(), in.Email, in.Password, in.Name, in.CaptchaToken, r.RemoteAddr, r.Header.Get("Idempotency-Key"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !decode(w, r, &in) {
		return
	}
	token, err := s.service.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		writeError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "store_session", Value: token, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (s *server) checkout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SKU      string
		Quantity int
	}
	if !decode(w, r, &in) {
		return
	}
	order, err := s.service.Checkout(sessionToken(r), in.SKU, in.Quantity)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *server) fulfill(w http.ResponseWriter, r *http.Request) {
	order, err := s.service.Fulfill(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *server) order(w http.ResponseWriter, r *http.Request) {
	order, err := s.service.Order(sessionToken(r), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func sessionToken(r *http.Request) string {
	c, err := r.Cookie("store_session")
	if err != nil {
		return ""
	}
	return c.Value
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var apiErr *store.InfraiError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		status = apiErr.HTTPStatus
	}
	if errors.Is(err, store.ErrCredentials) || errors.Is(err, store.ErrUnauthorized) {
		status = http.StatusUnauthorized
	}
	if errors.Is(err, store.ErrOrderState) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
