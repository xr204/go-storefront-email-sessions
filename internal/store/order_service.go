package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("authentication required")
	ErrCredentials  = errors.New("invalid credentials")
	ErrOrderState   = errors.New("order cannot make that transition")
)

type User struct {
	ID, Email, Name, Salt, PasswordHash string
}

type Order struct {
	ID, UserID, SKU, Status, Receipt string
	Quantity                         int
	Updates                          []string
}

type Service struct {
	mu       sync.Mutex
	infrai   *InfraiClient
	users    map[string]User
	sessions map[string]string
	orders   map[string]Order
	now      func() time.Time
}

func NewService(client *InfraiClient) *Service {
	return &Service{infrai: client, users: map[string]User{}, sessions: map[string]string{}, orders: map[string]Order{}, now: time.Now}
}

func (s *Service) Signup(ctx context.Context, email, password, name, captchaToken, ip, requestID string) error {
	if err := s.infrai.VerifyCaptcha(ctx, captchaToken, ip); err != nil {
		return err
	}
	userID, err := s.infrai.CreateUser(ctx, email, password, name, requestID)
	if err != nil {
		return err
	}
	salt := randomID()
	s.mu.Lock()
	s.users[email] = User{ID: userID, Email: email, Name: name, Salt: salt, PasswordHash: passwordDigest(salt, password)}
	s.mu.Unlock()
	return nil
}

func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	s.mu.Lock()
	user, ok := s.users[email]
	s.mu.Unlock()
	if !ok || subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(passwordDigest(user.Salt, password))) != 1 {
		return "", ErrCredentials
	}
	if err := s.infrai.CreateSession(ctx, user.ID); err != nil {
		return "", err
	}
	token := randomID()
	s.mu.Lock()
	s.sessions[token] = user.ID
	s.mu.Unlock()
	return token, nil
}

func (s *Service) Checkout(session, sku string, quantity int) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.sessions[session]
	if !ok {
		return Order{}, ErrUnauthorized
	}
	order := Order{ID: randomID(), UserID: userID, SKU: sku, Quantity: quantity, Status: "placed", Updates: []string{"Order placed"}}
	s.orders[order.ID] = order
	return order, nil
}

func (s *Service) Fulfill(orderID string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok || order.Status != "placed" {
		return Order{}, ErrOrderState
	}
	order.Status = "fulfilled"
	order.Receipt = fmt.Sprintf("receipt-%s-%s", order.ID, s.now().UTC().Format("20060102"))
	order.Updates = append(order.Updates, "Order fulfilled", "Receipt ready: "+order.Receipt)
	s.orders[orderID] = order
	return order, nil
}

func (s *Service) Order(session, orderID string) (Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.sessions[session]
	order, exists := s.orders[orderID]
	if !ok || !exists || order.UserID != userID {
		return Order{}, ErrUnauthorized
	}
	return order, nil
}

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func passwordDigest(salt, password string) string {
	sum := sha256.Sum256([]byte(salt + "\x00" + password))
	return hex.EncodeToString(sum[:])
}
