package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/storage"
	"github.com/hiroygo/gcpbbs/lib"
)

func mustGetenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s environment variable not set error\n", k)
	}
	return v
}

func main() {
	// Cloud Storage
	bName := mustGetenv("GCS_BUCKET")
	gcsClient, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer gcsClient.Close()
	bucket := lib.NewGCSBucket(gcsClient, bName)

	// Cloud SQL
	dbUser := mustGetenv("DB_USER")
	dbPwd := mustGetenv("DB_PASS")
	instanceConnectionName := mustGetenv("INSTANCE_CONNECTION_NAME")
	dbName := mustGetenv("DB_NAME")
	socketDir, isSet := os.LookupEnv("DB_SOCKET_DIR")
	if !isSet {
		socketDir = "/cloudsql"
	}
	dsn := fmt.Sprintf("%s:%s@unix(/%s/%s)/%s?parseTime=true&loc=Asia%2FTokyo", dbUser, dbPwd, socketDir, instanceConnectionName, dbName)
	db, err := lib.NewMySQL(dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Port
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
