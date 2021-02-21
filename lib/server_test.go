package lib

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/api/option"
)

func openTestMySQL(t *testing.T) (*sql.DB, func(t *testing.T)) {
	t.Helper()

	conn, err := envMySQLDSNToServer()
	if err != nil {
		t.Fatal(err)
	}
	newDB, err := sql.Open("mysql", conn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := newDB.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	// create database
	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	dbname := "test_" + strings.ReplaceAll(id.String(), "-", "")
	_, err = newDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbname))
	if err != nil {
		t.Fatal(err)
	}

	// create table
	db, err := sql.Open("mysql", getMySQLDSNToDB(conn, dbname))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE posts (
        id int(10) unsigned NOT NULL AUTO_INCREMENT,
        name varchar(50) NOT NULL,
        body text NOT NULL,
        imageurl varchar(512) DEFAULT NULL,
        created_at datetime NOT NULL,
        PRIMARY KEY (id)
    )`)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func(t *testing.T) {
		t.Helper()

		db, err := sql.Open("mysql", conn)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}()

		_, err = db.Exec(fmt.Sprintf("DROP DATABASE %s", dbname))
		if err != nil {
			t.Fatal(err)
		}
	}

	return db, cleanup
}

func insertIntoDB(t *testing.T, db *sql.DB, ps []Post) {
	t.Helper()

	stmt, err := db.Prepare("INSERT INTO posts VALUES(NULL, ?, ?, ?, NOW())")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	for _, p := range ps {
		_, err := stmt.Exec(p.Name, p.Body, p.ImageURL)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetHandler(t *testing.T) {
	db, cleanup := openTestMySQL(t)
	defer cleanup(t)

	expecteds := []Post{
		Post{
			Name:     "gopher",
			Body:     "hello world!",
			ImageURL: "",
		},
		Post{
			Name:     "いぬ",
			Body:     "わんわん",
			ImageURL: "dog.jpg",
		},
	}
	insertIntoDB(t, db, expecteds)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	sv := NewServer(nil, &mySQL{db})
	sv.getHandler(w, r)
	wr := w.Result()
	defer wr.Body.Close()

	if wr.StatusCode != http.StatusOK {
		t.Errorf("got error StatusCode, %v", wr.StatusCode)
	}

	var actuals []Post
	if err := json.NewDecoder(wr.Body).Decode(&actuals); err != nil {
		t.Fatal(err)
	}
	if len(expecteds) != len(actuals) {
		t.Fatalf("len does not match, len(expecteds) = %v != len(actuals) = %v", len(expecteds), len(actuals))
	}
	for i := range actuals {
		a := actuals[i]
		e := expecteds[i]
		if a.Name != e.Name || a.Body != e.Body || a.ImageURL != e.ImageURL {
			t.Errorf("want getHandler() = (%v %v, %v), got (%v, %v, %v)", e.Name, e.Body, e.ImageURL, a.Name, a.Body, a.ImageURL)
		}
	}
}

func newPostRequest(t *testing.T, name, body string, img []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	// json
	jsonHeader := make(textproto.MIMEHeader)
	jsonHeader.Set("Content-Type", `application/json; charset=utf-8`)
	jsonHeader.Set("Content-Disposition", `form-data; name="json"`)
	jsonPart, err := writer.CreatePart(jsonHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(jsonPart, `{"name":"%s", "body":"%s"}`, name, body); err != nil {
		t.Fatal(err)
	}
	// image
	if img != nil {
		imgHeader := make(textproto.MIMEHeader)
		imgHeader.Set("Content-Disposition", `form-data; name="attachment-file"; filename="dummy"`)
		imgPart, err := writer.CreatePart(imgHeader)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := imgPart.Write(img); err != nil {
			t.Fatal(err)
		}
	}

	// 最後に閉じる
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}

func createTestPng(t *testing.T, dir, name string, minFilesize int) string {
	t.Helper()

	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	e := png.Encoder{CompressionLevel: png.NoCompression}
	// png filesize = Width*Height*RGBA + other data
	// Width*Height*RGBA = minFilesize
	img := image.NewNRGBA(image.Rect(0, 0, minFilesize/4, 1))
	if err := e.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func loadFile(t *testing.T, path string) []byte {
	t.Helper()

	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func deleteGCSObject(t *testing.T, client *storage.Client, bucketName, o string) {
	t.Helper()

	if err := client.Bucket(bucketName).Object(o).Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func downloadFile(t *testing.T, url string) []byte {
	t.Helper()

	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	if r.StatusCode != http.StatusOK {
		t.Fatalf("got error StatusCode, %v", r.StatusCode)
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

func newTestGCSClient(t *testing.T) *storage.Client {
	t.Helper()

	v := os.Getenv("GCS_CREDSFILE")
	if v != "" {
		c, err := storage.NewClient(context.Background(), option.WithCredentialsFile(v))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c, err := storage.NewClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPostHandler(t *testing.T) {
	db, cleanup := openTestMySQL(t)
	// t.Parallel() で t.Run() が並行に動くときは defer でなく t.Cleanup() を使う
	t.Cleanup(func() { cleanup(t) })

	gcsClient := newTestGCSClient(t)
	t.Cleanup(func() {
		if err := gcsClient.Close(); err != nil {
			t.Fatal(err)
		}
	})

	bucketName, err := EnvGCSBucket()
	if err != nil {
		t.Fatal(err)
	}

	// create large png
	largePng := createTestPng(t, t.TempDir(), "largePng", maxFilesize)
	cases := []struct {
		name         string
		expectedName string
		expectedBody string
		expectedImg  []byte
		wantErr      bool
	}{
		{name: "jpeg", expectedName: "Sophia", expectedBody: "bowwow", expectedImg: loadFile(t, "../testdata/go.jpg"), wantErr: false},
		{name: "png", expectedName: "gopher", expectedBody: "hello", expectedImg: loadFile(t, "../testdata/go.png"), wantErr: false},
		{name: "noimage", expectedName: "koro", expectedBody: "wanwan", expectedImg: nil, wantErr: false},
		{name: "maxfilesize err", expectedName: "koro", expectedBody: "wanwan", expectedImg: loadFile(t, largePng), wantErr: true},
		{name: "not an image err", expectedName: "koro", expectedBody: "wanwan", expectedImg: loadFile(t, "../testdata/go.txt"), wantErr: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			sv := NewServer(OpenGCSBucket(gcsClient, bucketName), &mySQL{db})
			r := newPostRequest(t, c.expectedName, c.expectedBody, c.expectedImg)
			w := httptest.NewRecorder()
			sv.postHandler(w, r)
			wr := w.Result()
			defer wr.Body.Close()

			if c.wantErr && wr.StatusCode != http.StatusOK {
				return
			}
			if wr.StatusCode != http.StatusOK {
				t.Errorf("got error StatusCode, %v", wr.StatusCode)
			}
			var actual Post
			if err := json.NewDecoder(wr.Body).Decode(&actual); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if actual.ImageURL != "" {
					deleteGCSObject(t, gcsClient, bucketName, path.Base(actual.ImageURL))
				}
			}()

			// CreatedAt はデータベースで NOW() するのでチェックしない
			if actual.Name != c.expectedName || actual.Body != c.expectedBody {
				t.Fatalf("want postHandler() = (%v, %v), got (%v, %v)",
					c.expectedName, c.expectedBody, actual.Name, actual.Body)
			}
			if c.expectedImg != nil && actual.ImageURL == "" {
				t.Fatal("ImageURL is empty")
			}

			if c.expectedImg != nil {
				actualImg := downloadFile(t, actual.ImageURL)
				if !bytes.Equal(actualImg, c.expectedImg) {
					t.Error("image does not match")
				}
			}
		})
	}
}
