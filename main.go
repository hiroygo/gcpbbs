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
	// Cloud Storage
	gcsClient, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer gcsClient.Close()
	bucketName, err := lib.EnvGCSBucket()
	if err != nil {
		log.Fatal(err)
	}
	bucket := lib.OpenGCSBucket(gcsClient, bucketName)

	// Cloud SQL
	dsn, err := lib.EnvDSNToDB()
	if err != nil {
		log.Fatal(err)
	}
	db, err := lib.OpenMySQL(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = "80"
	}
	log.Println("Listening on port: " + port)
	sv := lib.NewServer(bucket, db)
	if err := http.ListenAndServe(":"+port, sv); err != nil {
		log.Fatal(err)
	}
}
