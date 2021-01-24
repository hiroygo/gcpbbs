package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"github.com/hiroygo/gcpbbs/lib"
)

func main() {
	gcsClient, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer gcsClient.Close()
	bucket := lib.NewGCSBucket(gcsClient, bName)

	dsn, ok := os.LookupEnv("MYSQL_DSN")
	if !ok {
		log.Fatal("環境変数 MYSQL_DSN が見つかりません")
	}
	db, err := lib.NewMySQL(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Listening on port: " + port)

	sv := lib.NewServer(bucket, db)
	if err := http.ListenAndServe(":"+port, sv); err != nil {
		log.Fatal(err)
	}
}
