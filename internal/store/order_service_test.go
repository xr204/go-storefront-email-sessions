package store

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOrderLifecycleDecision(t *testing.T) {
	calls := 0
	client := &InfraiClient{APIKey: "test-key", HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		data := `{}`
		if r.URL.Path == "/v1/auth/user/create" {
			data = `{"id":"user-7"}`
		}
		return response(200, `{"ok":true,"data":`+data+`}`), nil
	})}}
	svc := NewService(client)
	svc.now = func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) }

	if err := svc.Signup(context.Background(), "buyer@example.com", "correct horse battery", "Buyer", "captcha-token", "127.0.0.1", "signup-7"); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Login(context.Background(), "buyer@example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	order, err := svc.Checkout(session, "sku-coffee-1", 2)
	if err != nil {
		t.Fatal(err)
	}
	fulfilled, err := svc.Fulfill(order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fulfilled.Status != "fulfilled" || !strings.HasPrefix(fulfilled.Receipt, "receipt-") || len(fulfilled.Updates) != 3 {
		t.Fatalf("unexpected fulfilled order: %+v", fulfilled)
	}
	if calls != 3 {
		t.Fatalf("expected captcha, user and session calls; got %d", calls)
	}
}

func TestFulfillmentDecisionTable(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		wantError bool
	}{
		{name: "placed order is fulfilled", status: "placed"},
		{name: "fulfilled order stays terminal", status: "fulfilled", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(nil)
			svc.now = func() time.Time { return time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC) }
			svc.orders["order-1"] = Order{ID: "order-1", UserID: "user-1", Status: tt.status}
			got, err := svc.Fulfill("order-1")
			if tt.wantError {
				if !errors.Is(err, ErrOrderState) {
					t.Fatalf("expected order-state error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != "fulfilled" || got.Receipt != "receipt-order-1-20260825" {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestEnvelopeBeforeHTTPStatus(t *testing.T) {
	client := &InfraiClient{APIKey: "test-key", HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(422, `{"ok":false,"error":{"code":"CAPTCHA_REJECTED","message":"verification rejected"}}`), nil
	})}}
	err := client.VerifyCaptcha(context.Background(), "token", "127.0.0.1")
	apiErr, ok := err.(*InfraiError)
	if !ok || apiErr.HTTPStatus != 422 || apiErr.Code != "CAPTCHA_REJECTED" {
		t.Fatalf("expected decoded business rejection, got %#v", err)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
