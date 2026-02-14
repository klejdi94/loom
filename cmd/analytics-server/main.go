// Command analytics-server exposes the analytics store over HTTP (POST /record, GET /aggregates, GET /health).
package main

import (
	"database/sql"
	"flag"
	"log"
	"os"

	"github.com/klejdi94/loom/analytics"
	"github.com/redis/go-redis/v9"
	_ "github.com/lib/pq"
)

func main() {
	addr := flag.String("addr", ":8080", "Listen address")
	storeKind := flag.String("store", "memory", "Store: memory, postgres, redis")
	maxRecords := flag.Int("max", 100000, "Max in-memory records when store=memory (0 = unbounded)")
	dsn := flag.String("dsn", "", "PostgreSQL DSN when store=postgres (or ANALYTICS_DSN env)")
	redisAddr := flag.String("redis", "", "Redis address when store=redis (e.g. localhost:6379, or ANALYTICS_REDIS env)")
	redisKey := flag.String("redis-key", "", "Redis key for analytics (default: loom:analytics:runs)")
	pgTable := flag.String("table", "prompt_runs", "Postgres table name when store=postgres")
	flag.Parse()

	if v := os.Getenv("ANALYTICS_DSN"); v != "" && *dsn == "" {
		*dsn = v
	}
	if v := os.Getenv("ANALYTICS_REDIS"); v != "" && *redisAddr == "" {
		*redisAddr = v
	}

	var store analytics.Store
	switch *storeKind {
	case "memory":
		store = analytics.NewMemoryStore(*maxRecords)
	case "postgres":
		if *dsn == "" {
			log.Fatal("postgres store requires -dsn or ANALYTICS_DSN")
		}
		db, err := openPostgres(*dsn)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer db.Close()
		pg, err := analytics.NewPostgresStore(db, *pgTable)
		if err != nil {
			log.Fatalf("postgres store: %v", err)
		}
		store = pg
	case "redis":
		if *redisAddr == "" {
			log.Fatal("redis store requires -redis or ANALYTICS_REDIS")
		}
		rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
		store = analytics.NewRedisStore(rdb, *redisKey)
	default:
		log.Fatalf("unknown store: %s", *storeKind)
	}

	srv := analytics.NewServer(store, *addr)
	log.Printf("analytics server listening on %s (store=%s)", *addr, *storeKind)
	log.Fatal(srv.ListenAndServe())
}

func openPostgres(dsn string) (*sql.DB, error) {
	return sql.Open("postgres", dsn)
}
