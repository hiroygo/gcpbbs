package lib_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
		t.Fatalf("want getHandler() = %v, got %v", expected, actual)
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

func newPostRequest(name, body string, img io.ReadSeeker) (*http.Request, error) {
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
		_, format, err := image.DecodeConfig(img)
		if err != nil {
			return nil, fmt.Errorf("DecodeConfig error, %v", err)
		}
		if _, err := img.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("Seek error, %v", err)
		}
		imgHeader := make(textproto.MIMEHeader)
		imgHeader.Set("Content-Type", "image/"+format)
		imgHeader.Set("Content-Disposition", `form-data; name="attachment-file"; filename="dummy,jpg"`)
		imgPart, err := writer.CreatePart(imgHeader)
		if err != nil {
			return nil, fmt.Errorf("CreatePart error, %v", err)
		}
		if _, err := io.Copy(imgPart, img); err != nil {
			return nil, fmt.Errorf("Copy error, %v", err)
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
	cases := []struct {
		name         string
		expectedName string
		expectedBody string
		expectedTime time.Time
		imgPath      string
	}{
		{name: "jpeg", expectedName: "Sophia", expectedBody: "bowwow", expectedTime: time.Now(), imgPath: "../testdata/go.jpg"},
		{name: "png", expectedName: "gopher", expectedBody: "hello", expectedTime: time.Now(), imgPath: "../testdata/go.png"},
		{name: "noimage", expectedName: "koro", expectedBody: "wanwan", expectedTime: time.Now(), imgPath: ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var expectedImg []byte
			var expectedImgReader io.ReadSeeker
			if c.imgPath != "" {
				var err error
				expectedImg, err = ioutil.ReadFile(c.imgPath)
				if err != nil {
					t.Fatalf("ReadFile error, %v", err)
				}
				expectedImgReader = bytes.NewReader(expectedImg)
			}

			r, err := newPostRequest(c.expectedName, c.expectedBody, expectedImgReader)
			if err != nil {
				t.Fatalf("newPostRequest error, %v", err)
			}
			w := httptest.NewRecorder()
			sv := lib.NewServer(&testLocalBucket{dir: t.TempDir()}, &testDB{createdAt: c.expectedTime})
			lib.ExportServerPostHandler(sv, w, r)
			rw := w.Result()
			defer rw.Body.Close()

			if rw.StatusCode != http.StatusOK {
				t.Fatalf("Status code error, %v", rw.StatusCode)
			}
			var actual lib.Post
			if err := json.NewDecoder(rw.Body).Decode(&actual); err != nil {
				t.Fatalf("Decode error, %v", err)
			}
			if actual.Name != c.expectedName || actual.Body != c.expectedBody ||
				!actual.CreatedAt.Equal(c.expectedTime) {
				t.Errorf("want postHandler() = (%v, %v, %v), got (%v, %v, %v)",
					c.expectedName, c.expectedBody, c.expectedTime, actual.Name, actual.Body, actual.CreatedAt)
			}
			if expectedImg != nil {
				img, err := ioutil.ReadFile(actual.ImageURL)
				if err != nil {
					t.Fatalf("ReadFile error, %v", err)
				}
				if !bytes.Equal(expectedImg, img) {
					t.Fatal("Image does not match error")
				}
			}
		})
	}
}
