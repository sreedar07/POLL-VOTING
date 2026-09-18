package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"golang.org/x/crypto/bcrypt"

	"pulsepoll/internal/models"
)

var ErrNotFound = errors.New("not found")
var ErrDuplicate = errors.New("duplicate")

type Store struct {
	DB    *mongo.Database
	Redis *redis.Client
}

func New(db *mongo.Database, r *redis.Client) *Store { return &Store{DB: db, Redis: r} }

func (s *Store) CreateUser(ctx context.Context, email, password string) (*models.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(password) < 8 {
		return nil, errors.New("invalid credentials")
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	u := &models.User{ID: uuid.NewString(), Email: email, PasswordHash: string(hash), CreatedAt: time.Now()}
	_, err := s.DB.Collection("users").InsertOne(ctx, u)
	return u, err
}

func (s *Store) Authenticate(ctx context.Context, email, password string) (*models.User, error) {
	var u models.User
	err := s.DB.Collection("users").FindOne(ctx, bson.M{"email": strings.ToLower(strings.TrimSpace(email))}).Decode(&u)
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, errors.New("invalid credentials")
	}
	return &u, nil
}

func (s *Store) CreatePoll(ctx context.Context, p *models.Poll) error {
	_, err := s.DB.Collection("polls").InsertOne(ctx, p)
	return err
}

func (s *Store) GetPoll(ctx context.Context, slug string) (*models.Poll, error) {
	var p models.Poll
	err := s.DB.Collection("polls").FindOne(ctx, bson.M{"slug": slug}).Decode(&p)
	if err == mongo.ErrNoDocuments {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) OwnerPolls(ctx context.Context, owner string) ([]models.Poll, error) {
	cur, err := s.DB.Collection("polls").Find(ctx, bson.M{"ownerId": owner})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var out []models.Poll
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ExpireIfNeeded(ctx context.Context, p *models.Poll) (bool, error) {
	if p.Status != "open" {
		return false, nil
	}
	expired := p.DurationSeconds > 0 && time.Now().After(p.CreatedAt.Add(time.Duration(p.DurationSeconds)*time.Second))
	if !expired {
		return false, nil
	}
	now := time.Now()
	r := s.DB.Collection("polls").FindOneAndUpdate(ctx, bson.M{"slug": p.Slug, "status": "open"},
		bson.M{"$set": bson.M{"status": "closed", "closedAt": now}})
	if r.Err() == mongo.ErrNoDocuments {
		return false, nil
	}
	if r.Err() != nil {
		return false, r.Err()
	}
	p.Status = "closed"
	p.ClosedAt = &now
	return true, nil
}

func (s *Store) ClosePoll(ctx context.Context, owner, slug string) error {
	now := time.Now()
	r := s.DB.Collection("polls").FindOneAndUpdate(ctx, bson.M{"ownerId": owner, "slug": slug, "status": "open"},
		bson.M{"$set": bson.M{"status": "closed", "closedAt": now}})
	return r.Err()
}

func (s *Store) DeletePoll(ctx context.Context, owner, slug string) error {
	r, err := s.DB.Collection("polls").DeleteOne(ctx, bson.M{"ownerId": owner, "slug": slug})
	if err != nil || r.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddVoteMongo(ctx context.Context, slug string, optionIDs []string) error {
	inc := bson.M{"aggregateVotes": int64(1)}
	for _, id := range optionIDs {
		inc["aggregateCounts."+id] = int64(1)
	}
	_, err := s.DB.Collection("polls").UpdateOne(ctx, bson.M{"slug": slug}, bson.M{"$inc": inc})
	return err
}

func (s *Store) SeedCounters(ctx context.Context, p *models.Poll) {
	key := "poll:" + p.Slug + ":counts"
	pipe := s.Redis.TxPipeline()
	for _, o := range p.Options {
		value := int64(0)
		if p.AggregateCounts != nil {
			value = p.AggregateCounts[o.ID]
		}
		pipe.HSet(ctx, key, o.ID, value)
	}
	total := p.AggregateVotes
	pipe.HSet(ctx, key, "__total", total)
	_, _ = pipe.Exec(ctx)
}

func (s *Store) Snapshot(ctx context.Context, p *models.Poll) (models.Snapshot, error) {
	key := "poll:" + p.Slug + ":counts"
	counts, err := s.Redis.HGetAll(ctx, key).Result()
	if err != nil {
		return models.Snapshot{}, err
	}
	if len(counts) == 0 {
		fresh, dbErr := s.GetPoll(ctx, p.Slug)
		if dbErr == nil {
			p = fresh
			s.SeedCounters(ctx, p)
			counts = map[string]string{"__total": strconv.FormatInt(p.AggregateVotes, 10)}
			for _, o := range p.Options {
				counts[o.ID] = strconv.FormatInt(p.AggregateCounts[o.ID], 10)
			}
		}
	}
	var total int64
	if v, ok := counts["__total"]; ok {
		total, _ = strconv.ParseInt(v, 10, 64)
	}
	opts := make([]models.ResultOption, 0, len(p.Options))
	for _, o := range p.Options {
		v, _ := strconv.ParseInt(counts[o.ID], 10, 64)
		pct := float64(0)
		if total > 0 {
			pct = float64(v) * 100 / float64(total)
		}
		opts = append(opts, models.ResultOption{ID: o.ID, Text: o.Text, Emoji: o.Emoji, Votes: v, Percent: pct})
	}
	return models.Snapshot{Slug: p.Slug, TotalVotes: total, Options: opts, Status: p.Status}, nil
}

func (s *Store) Vote(ctx context.Context, p *models.Poll, optionIDs []string, fingerprint string) (models.Snapshot, error) {
	if p.Status != "open" {
		return models.Snapshot{}, errors.New("poll is closed")
	}
	if p.DurationSeconds > 0 && time.Since(p.CreatedAt) > time.Duration(p.DurationSeconds)*time.Second {
		_, _ = s.ExpireIfNeeded(ctx, p)
		return models.Snapshot{}, errors.New("poll expired")
	}
	if p.MaxVotes > 0 {
		totalRaw, _ := s.Redis.HGet(ctx, "poll:"+p.Slug+":counts", "__total").Result()
		current, _ := strconv.ParseInt(totalRaw, 10, 64)
		if current >= p.MaxVotes {
			_, _ = s.ExpireIfNeeded(ctx, p)
			if p.Status == "open" {
				now := time.Now()
				_, _ = s.DB.Collection("polls").UpdateOne(ctx, bson.M{"slug": p.Slug, "status": "open"}, bson.M{"$set": bson.M{"status": "closed", "closedAt": now}})
				p.Status = "closed"
				p.ClosedAt = &now
			}
			return models.Snapshot{}, errors.New("maximum vote count reached")
		}
	}
	valid := map[string]bool{}
	for _, o := range p.Options {
		valid[o.ID] = true
	}
	if len(optionIDs) < 1 || len(optionIDs) > len(p.Options) {
		return models.Snapshot{}, errors.New("invalid selection")
	}
	seen := map[string]bool{}
	for _, id := range optionIDs {
		if !valid[id] || seen[id] {
			return models.Snapshot{}, errors.New("invalid option")
		}
		seen[id] = true
	}
	if !p.Multiple && len(optionIDs) != 1 {
		return models.Snapshot{}, errors.New("single-choice poll")
	}
	if p.RestrictDuplicates {
		ok, err := s.Redis.SetNX(ctx, "poll:"+p.Slug+":voter:"+fingerprint, "1", 8*24*time.Hour).Result()
		if err != nil {
			return models.Snapshot{}, err
		}
		if !ok {
			return models.Snapshot{}, ErrDuplicate
		}
	}

	key := "poll:" + p.Slug + ":counts"
	max := p.MaxVotes
	args := make([]interface{}, 0, len(optionIDs)+1)
	args = append(args, max)
	for _, id := range optionIDs {
		args = append(args, id)
	}
	script := redis.NewScript(`
local total = tonumber(redis.call("HGET", KEYS[1], "__total") or "0")
local max = tonumber(ARGV[1] or "0")
if max > 0 and total >= max then return 0 end
for i = 2, #ARGV do
  redis.call("HINCRBY", KEYS[1], ARGV[i], 1)
end
redis.call("HINCRBY", KEYS[1], "__total", 1)
return 1
`)
	result, err := script.Run(ctx, s.Redis, []string{key}, args...).Int()
	if err != nil {
		return models.Snapshot{}, err
	}
	if result == 0 {
		now := time.Now()
		_, _ = s.DB.Collection("polls").UpdateOne(ctx, bson.M{"slug": p.Slug, "status": "open"}, bson.M{"$set": bson.M{"status": "closed", "closedAt": now}})
		p.Status = "closed"
		p.ClosedAt = &now
		return models.Snapshot{}, errors.New("maximum vote count reached")
	}
	if err := s.AddVoteMongo(ctx, p.Slug, optionIDs); err != nil {
		return models.Snapshot{}, err
	}
	if p.MaxVotes > 0 {
		totalRaw, _ := s.Redis.HGet(ctx, key, "__total").Result()
		total, _ := strconv.ParseInt(totalRaw, 10, 64)
		if total >= p.MaxVotes {
			now := time.Now()
			_, _ = s.DB.Collection("polls").UpdateOne(ctx, bson.M{"slug": p.Slug, "status": "open"}, bson.M{"$set": bson.M{"status": "closed", "closedAt": now}})
			p.Status = "closed"
			p.ClosedAt = &now
		}
	}
	return s.Snapshot(ctx, p)
}

func (s *Store) Reaction(ctx context.Context, slug, optionID, emoji string) error {
	if len([]rune(emoji)) > 4 {
		return errors.New("invalid emoji")
	}
	_, err := s.Redis.HIncrBy(ctx, "poll:"+slug+":reactions:"+optionID, emoji, 1).Result()
	return err
}

func (s *Store) ExpireOpenPolls(ctx context.Context) ([]string, error) {
	cur, err := s.DB.Collection("polls").Find(ctx, bson.M{"status": "open"})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var polls []models.Poll
	if err := cur.All(ctx, &polls); err != nil {
		return nil, err
	}
	var closed []string
	for i := range polls {
		changed, err := s.ExpireIfNeeded(ctx, &polls[i])
		if err != nil {
			return nil, err
		}
		if changed {
			closed = append(closed, polls[i].Slug)
		}
	}
	return closed, nil
}

func (s *Store) Export(ctx context.Context, owner, slug string) (models.Poll, models.Snapshot, error) {
	p, err := s.GetPoll(ctx, slug)
	if err != nil {
		return models.Poll{}, models.Snapshot{}, err
	}
	if p.OwnerID != owner {
		return models.Poll{}, models.Snapshot{}, errors.New("forbidden")
	}
	snap, err := s.Snapshot(ctx, p)
	return *p, snap, err
}

func SlugExists(ctx context.Context, db *mongo.Database, slug string) bool {
	return db.Collection("polls").FindOne(ctx, bson.M{"slug": slug}).Err() == nil
}

func ValidateSlug(slug string) bool {
	if len(slug) < 5 || len(slug) > 24 {
		return false
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func NewSlug(question string) string {
	base := strings.ToLower(question)
	var b strings.Builder
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 14 {
		s = s[:14]
	}
	return fmt.Sprintf("%s-%s", s, uuid.NewString()[:4])
}
