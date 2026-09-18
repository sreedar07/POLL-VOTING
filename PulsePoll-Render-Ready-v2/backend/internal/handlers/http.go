package handlers

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"pulsepoll/internal/auth"
	"pulsepoll/internal/models"
	"pulsepoll/internal/realtime"
	"pulsepoll/internal/store"
)

type Handler struct {
	Store *store.Store
	Hub   *realtime.Hub
	Auth  *auth.Service
}

func New(s *store.Store, h *realtime.Hub, a *auth.Service) *Handler {
	return &Handler{Store: s, Hub: h, Auth: a}
}

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.POST("/auth/signup", h.signup)
	r.POST("/auth/login", h.login)
	r.GET("/polls/:slug", h.getPoll)
	r.POST("/polls", h.auth, h.createPoll)
	r.POST("/polls/:slug/vote", h.vote)
	r.POST("/polls/:slug/reaction", h.reaction)
	admin := r.Group("/admin", h.auth)
	admin.GET("/polls", h.adminPolls)
	admin.GET("/polls/:slug/export", h.exportPoll)
	admin.POST("/polls/:slug/close", h.closePoll)
	admin.DELETE("/polls/:slug", h.deletePoll)
}

func (h *Handler) auth(c *gin.Context) {
	cl, err := h.Auth.Parse(c.GetHeader("Authorization"))
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"error": "authentication required"})
		return
	}
	c.Set("claims", cl)
	c.Next()
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) signup(c *gin.Context) {
	var in credentials
	if c.ShouldBindJSON(&in) != nil || len(in.Password) < 8 {
		c.JSON(400, gin.H{"error": "email and password (8+ chars) required"})
		return
	}
	u, err := h.Store.CreateUser(c, in.Email, in.Password)
	if err != nil {
		c.JSON(409, gin.H{"error": "account may already exist"})
		return
	}
	token, _ := h.Auth.Issue(u.ID, u.Email)
	c.JSON(201, gin.H{"token": token, "email": u.Email})
}
func (h *Handler) login(c *gin.Context) {
	var in credentials
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	u, err := h.Store.Authenticate(c, in.Email, in.Password)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}
	token, _ := h.Auth.Issue(u.ID, u.Email)
	c.JSON(200, gin.H{"token": token, "email": u.Email})
}

type createInput struct {
	Question           string   `json:"question"`
	Options            []string `json:"options"`
	DurationSeconds    int64    `json:"durationSeconds"`
	Multiple           bool     `json:"multiple"`
	RestrictDuplicates bool     `json:"restrictDuplicates"`
	Pulse              bool     `json:"pulse"`
	MaxVotes           int64    `json:"maxVotes"`
}

func (h *Handler) createPoll(c *gin.Context) {
	var in createInput
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, gin.H{"error": "invalid JSON"})
		return
	}
	if len([]rune(strings.TrimSpace(in.Question))) < 5 || len([]rune(in.Question)) > 240 {
		c.JSON(400, gin.H{"error": "question must be 5-240 characters"})
		return
	}
	if len(in.Options) < 2 || len(in.Options) > 8 {
		c.JSON(400, gin.H{"error": "2-8 options required"})
		return
	}
	if in.DurationSeconds != 0 && (in.DurationSeconds < 30 || in.DurationSeconds > 7*24*3600) {
		c.JSON(400, gin.H{"error": "duration must be 30 seconds to 7 days"})
		return
	}
	if in.MaxVotes < 0 || in.MaxVotes > 1000000 {
		c.JSON(400, gin.H{"error": "maxVotes must be 0 (unlimited) or between 1 and 1000000"})
		return
	}
	seen := map[string]bool{}
	opts := make([]models.Option, 0, len(in.Options))
	for _, text := range in.Options {
		text = strings.TrimSpace(text)
		if len([]rune(text)) < 1 || len([]rune(text)) > 120 || seen[strings.ToLower(text)] {
			c.JSON(400, gin.H{"error": "options must be unique and 1-120 characters"})
			return
		}
		seen[strings.ToLower(text)] = true
		opts = append(opts, models.Option{ID: uuid.NewString(), Text: text})
	}
	cl := c.MustGet("claims").(*auth.Claims)
	slug := store.NewSlug(in.Question)
	p := &models.Poll{ID: uuid.NewString(), Slug: slug, OwnerID: cl.UserID, Question: strings.TrimSpace(in.Question), Options: opts, DurationSeconds: in.DurationSeconds, Multiple: in.Multiple, RestrictDuplicates: in.RestrictDuplicates, Pulse: in.Pulse, MaxVotes: in.MaxVotes, Status: "open", CreatedAt: time.Now()}
	if err := h.Store.CreatePoll(c, p); err != nil {
		c.JSON(500, gin.H{"error": "could not create poll"})
		return
	}
	h.Store.SeedCounters(c, p)
	c.JSON(201, gin.H{"poll": p})
}

func (h *Handler) getPoll(c *gin.Context) {
	p, err := h.Store.GetPoll(c, c.Param("slug"))
	if err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	_, _ = h.Store.ExpireIfNeeded(c, p)
	snap, err := h.Store.Snapshot(c, p)
	if err != nil {
		c.JSON(500, gin.H{"error": "could not load results"})
		return
	}
	c.JSON(200, gin.H{"poll": p, "snapshot": snap})
}

func (h *Handler) fingerprint(c *gin.Context) string {
	cookie, err := c.Cookie("pp_voter")
	if err != nil {
		cookie = uuid.NewString()
		http.SetCookie(c, &http.Cookie{Name: "pp_voter", Value: cookie, MaxAge: 8 * 24 * 3600, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	raw := c.ClientIP() + "|" + cookie
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

type voteInput struct {
	OptionIDs []string `json:"optionIds"`
}

func (h *Handler) vote(c *gin.Context) {
	p, err := h.Store.GetPoll(c, c.Param("slug"))
	if err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	var in voteInput
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, gin.H{"error": "invalid vote"})
		return
	}
	snap, err := h.Store.Vote(c, p, in.OptionIDs, h.fingerprint(c))
	if err == store.ErrDuplicate {
		c.JSON(409, gin.H{"error": "you have already voted"})
		return
	}
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	h.Hub.Publish(c, p.Slug, snap, len(in.OptionIDs))
	c.JSON(200, gin.H{"snapshot": snap})
}

type reactionInput struct {
	OptionID string `json:"optionId"`
	Emoji    string `json:"emoji"`
}

func (h *Handler) reaction(c *gin.Context) {
	var in reactionInput
	if c.ShouldBindJSON(&in) != nil {
		c.JSON(400, gin.H{"error": "invalid reaction"})
		return
	}
	if err := h.Store.Reaction(c, c.Param("slug"), in.OptionID, in.Emoji); err != nil {
		c.JSON(400, gin.H{"error": "invalid reaction"})
		return
	}
	h.Hub.PublishReaction(c, c.Param("slug"), in.OptionID, in.Emoji)
	c.Status(204)
}

func (h *Handler) adminPolls(c *gin.Context) {
	cl := c.MustGet("claims").(*auth.Claims)
	ps, err := h.Store.OwnerPolls(c, cl.UserID)
	if err != nil {
		c.JSON(500, gin.H{"error": "load failed"})
		return
	}
	c.JSON(200, gin.H{"polls": ps})
}
func (h *Handler) closePoll(c *gin.Context) {
	cl := c.MustGet("claims").(*auth.Claims)
	slug := c.Param("slug")
	if err := h.Store.ClosePoll(c, cl.UserID, slug); err != nil {
		c.JSON(404, gin.H{"error": "poll not found or already closed"})
		return
	}
	p, err := h.Store.GetPoll(c, slug)
	if err == nil {
		snap, _ := h.Store.Snapshot(c, p)
		h.Hub.PublishClosed(c, slug, snap)
	}
	c.JSON(200, gin.H{"status": "closed"})
}
func (h *Handler) deletePoll(c *gin.Context) {
	cl := c.MustGet("claims").(*auth.Claims)
	if err := h.Store.DeletePoll(c, cl.UserID, c.Param("slug")); err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	c.Status(204)
}
func (h *Handler) exportPoll(c *gin.Context) {
	cl := c.MustGet("claims").(*auth.Claims)
	p, snap, err := h.Store.Export(c, cl.UserID, c.Param("slug"))
	if err != nil {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if c.Query("format") == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename="+p.Slug+".csv")
		w := csv.NewWriter(c.Writer)
		_ = w.Write([]string{"option", "votes", "percent"})
		for _, o := range snap.Options {
			_ = w.Write([]string{o.Text, strconv.FormatInt(o.Votes, 10), fmt.Sprintf("%.2f", o.Percent)})
		}
		w.Flush()
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+p.Slug+".json")
	c.JSON(200, gin.H{"poll": p, "results": snap})
}

func Logger() gin.HandlerFunc { return func(c *gin.Context) { c.Next() } }
func CORS(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin != "" && origin != "same-origin" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
func RateLimit(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "rl:" + c.ClientIP() + ":" + c.Request.URL.Path
		n, err := rdb.Incr(c, key).Result()
		if err == nil && n == 1 {
			rdb.Expire(c, key, time.Minute)
		}
		if err == nil && n > 120 {
			c.AbortWithStatusJSON(429, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}

var _ = json.Marshal
