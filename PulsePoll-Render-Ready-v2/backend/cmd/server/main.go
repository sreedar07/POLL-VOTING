package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"pulsepoll/internal/auth"
	"pulsepoll/internal/handlers"
	"pulsepoll/internal/realtime"
	"pulsepoll/internal/store"
)

func main() {
	port := getenv("PORT", "10000")
	mongoURI := getenv("MONGO_URI", "mongodb://localhost:27017")
	dbName := getenv("MONGO_DB", "pulsepoll")
	redisURL := os.Getenv("REDIS_URL")
	redisAddr := getenv("REDIS_ADDR", "localhost:6379")
	jwtSecret := getenv("JWT_SECRET", "dev-secret-change-me")
	corsOrigin := getenv("CORS_ORIGIN", "http://localhost:5173")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mc, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatal(err)
	}
	if err = mc.Ping(ctx, nil); err != nil {
		log.Fatal(err)
	}

	var rdb *redis.Client
	if redisURL != "" {
		opts, parseErr := redis.ParseURL(redisURL)
		if parseErr != nil {
			log.Fatal(parseErr)
		}
		rdb = redis.NewClient(opts)
	} else {
		rdb = redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD")})
	}
	if err = rdb.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}

	st := store.New(mc.Database(dbName), rdb)
	hub := realtime.NewHub(rdb, st)
	go hub.Run(context.Background())

	// Close duration-based polls even when nobody is voting at the exact expiry moment.
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			slugs, err := st.ExpireOpenPolls(ctx)
			cancel()
			if err != nil {
				continue
			}
			for _, slug := range slugs {
				// Reuse Redis Pub/Sub so every WebSocket client sees the final state.
				p, err := st.GetPoll(context.Background(), slug)
				if err != nil {
					continue
				}
				snap, err := st.Snapshot(context.Background(), p)
				if err != nil {
					continue
				}
				hub.PublishClosed(context.Background(), slug, snap)
			}
		}
	}()

	h := handlers.New(st, hub, auth.New(jwtSecret))

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), handlers.Logger(), handlers.CORS(corsOrigin), handlers.RateLimit(rdb))

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "pulsepoll"})
	})

	api := router.Group("/api")
	h.RegisterRoutes(api)

	router.GET("/ws", hub.ServeWSGin)

	// Serve the production React build from the same origin when present.
	router.Static("/assets", "./frontend/dist/assets")
	router.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			c.File("./frontend/dist/index.html")
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})

	log.Printf("PulsePoll API listening on :%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
