package auth

import (
    "strings"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type Service struct { secret []byte }

type Claims struct {
    UserID string `json:"sub"`
    Email string `json:"email"`
    jwt.RegisteredClaims
}

func New(secret string) *Service { return &Service{[]byte(secret)} }

func (s *Service) Issue(userID, email string) (string, error) {
    c := Claims{UserID:userID, Email:email, RegisteredClaims:jwt.RegisteredClaims{
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24*time.Hour)),
        IssuedAt: jwt.NewNumericDate(time.Now()),
    }}
    return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
}

func (s *Service) Parse(header string) (*Claims, error) {
    token := strings.TrimPrefix(header, "Bearer ")
    parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token)(any,error){
        return s.secret, nil
    })
    if err != nil { return nil, err }
    return parsed.Claims.(*Claims), nil
}
