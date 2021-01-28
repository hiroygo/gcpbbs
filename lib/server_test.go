package lib_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hiroygo/gcpbbs/lib"
)

type testDB struct {
	posts     []lib.Post
	createdAt time.Time
	err       error
}

func (d *testDB) GetAll() ([]lib.Post, error) {
	return d.posts, d.err
}

func (d *testDB) Insert(p lib.Post) (lib.Post, error) {
	return lib.Post{Name: p.Name, Body: p.Body, ImageURL: p.ImageURL, CreatedAt: d.createdAt}, d.err
}

func (d *testDB) Close() error {
	return d.err
}

type testLocalBucket struct {
	dir string
}

func (b *testLocalBucket) Upload(objName string, obj io.Reader) (string, error) {
	path := filepath.Join(b.dir, objName)
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("Create error, %v", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, obj); err != nil {
		return "", fmt.Errorf("Copy error, %v", err)
	}

	return path, nil
}

func TestGetHandler(t *testing.T) {
	expected := []lib.Post{lib.Post{Name: "gopher", Body: "hello world!", ImageURL: "img.jpg", CreatedAt: time.Now()}}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	sv := lib.NewServer(&testLocalBucket{}, &testDB{posts: expected})
	lib.ExportServerGetHandler(sv, w, r)
	rw := w.Result()
	defer rw.Body.Close()

	if rw.StatusCode != http.StatusOK {
		t.Fatalf("Status code error, %v", rw.StatusCode)
	}

	var actual []lib.Post
	if err := json.NewDecoder(rw.Body).Decode(&actual); err != nil {
		t.Fatalf("Decode error, %v", err)
	}
	if len(actual) != len(expected) {
		t.Fatalf("len error, len(expected): %v != len(actual): %v", len(expected), len(actual))
	}
	for i := range actual {
		a := actual[i]
		e := expected[i]
		if a.Name != e.Name || a.Body != e.Body ||
			a.ImageURL != e.ImageURL || !a.CreatedAt.Equal(e.CreatedAt) {
			t.Errorf("want getHandler() = %v, got %v", expected, actual)
		}
	}
}

func newPostRequest(name, body string, img []byte) (*http.Request, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// json
	jsonHeader := make(textproto.MIMEHeader)
	jsonHeader.Set("Content-Type", `application/json; charset=utf-8`)
	jsonHeader.Set("Content-Disposition", `form-data; name="json"`)
	jsonPart, err := writer.CreatePart(jsonHeader)
	if err != nil {
		return nil, fmt.Errorf("CreatePart error, %v", err)
	}
	if _, err := fmt.Fprintf(jsonPart, `{"name":"%s", "body":"%s"}`, name, body); err != nil {
		return nil, fmt.Errorf("Fprintf error, %v", err)
	}

	// image
	if img != nil {
		imgHeader := make(textproto.MIMEHeader)
		imgHeader.Set("Content-Disposition", `form-data; name="attachment-file"; filename="dummy"`)
		imgPart, err := writer.CreatePart(imgHeader)
		if err != nil {
			return nil, fmt.Errorf("CreatePart error, %v", err)
		}
		if _, err := imgPart.Write(img); err != nil {
			return nil, fmt.Errorf("Write error, %v", err)
		}
	}

	// 最後に閉じる
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("Close error, %v", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r, nil
}

func TestPostHandler(t *testing.T) {
	tempDir := t.TempDir()
	createPng := func(name string, minFilesize int) (path string, rerr error) {
		p := filepath.Join(tempDir, name)
		f, err := os.Create(p)
		if err != nil {
			return "", fmt.Errorf("Create error, %v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				rerr = fmt.Errorf("Close error, %v", err)
			}
		}()

		e := png.Encoder{CompressionLevel: png.NoCompression}
		// png filesize = Width*Height*RGBA + other data
		// Width*Height*RGBA = minFilesize
		img := image.NewNRGBA(image.Rect(0, 0, minFilesize/4, 1))
		if err := e.Encode(f, img); err != nil {
			return "", fmt.Errorf("Encode error, %v", err)
		}
		return p, nil
	}
	loadImg := func(path string) ([]byte, error) {
		if path == "" {
			return nil, nil
		}
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ReadFile error, %v", err)
		}
		return b, nil
	}

	// create test data
	largePng, err := createPng("largePng", lib.ExportMaxFilesize)
	if err != nil {
		t.Fatalf("createPng error, %v", err)
	}

	cases := []struct {
		name              string
		expectedName      string
		expectedBody      string
		expectedCreatedAt time.Time
		imgPath           string
		wantErr           bool
	}{
		{name: "jpeg", expectedName: "Sophia", expectedBody: "bowwow", expectedCreatedAt: time.Now(), imgPath: "../testdata/go.jpg", wantErr: false},
		{name: "png", expectedName: "gopher", expectedBody: "hello", expectedCreatedAt: time.Now(), imgPath: "../testdata/go.png", wantErr: false},
		{name: "noimage", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), imgPath: "", wantErr: false},
		{name: "maxfilesize err", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), imgPath: largePng, wantErr: true},
		{name: "not an image err", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), imgPath: "../testdata/go.txt", wantErr: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			expectedImg, err := loadImg(c.imgPath)
			if err != nil {
				t.Fatalf("loadImg error, %v", err)
			}
			r, err := newPostRequest(c.expectedName, c.expectedBody, expectedImg)
			if err != nil {
				t.Fatalf("newPostRequest error, %v", err)
			}
			w := httptest.NewRecorder()
			sv := lib.NewServer(&testLocalBucket{dir: tempDir}, &testDB{createdAt: c.expectedCreatedAt})
			lib.ExportServerPostHandler(sv, w, r)
			rw := w.Result()
			defer rw.Body.Close()

			if c.wantErr && rw.StatusCode != http.StatusOK {
				return
			}
			if rw.StatusCode != http.StatusOK {
				t.Fatalf("Status code error, %v", rw.StatusCode)
			}
			var actual lib.Post
			if err := json.NewDecoder(rw.Body).Decode(&actual); err != nil {
				t.Fatalf("Decode error, %v", err)
			}
			if actual.Name != c.expectedName || actual.Body != c.expectedBody || !actual.CreatedAt.Equal(c.expectedCreatedAt) {
				t.Fatalf("want postHandler() = (%v, %v, %v), got (%v, %v, %v)",
					c.expectedName, c.expectedBody, c.expectedCreatedAt, actual.Name, actual.Body, actual.CreatedAt)
			}
			actualImg, err := loadImg(actual.ImageURL)
			if err != nil {
				t.Fatalf("loadImg error, %v", err)
			}
			if !bytes.Equal(actualImg, expectedImg) {
				t.Fatal("Image does not match error")
			}
		})
	}
}
