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
		return "", fmt.Errorf("Create error, %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, obj); err != nil {
		return "", fmt.Errorf("Copy error, %w", err)
	}

	return path, nil
}

func TestGetHandler(t *testing.T) {
	expected := []lib.Post{lib.Post{Name: "gopher", Body: "hello world!", ImageURL: "img.jpg", CreatedAt: time.Now()}}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	sv := lib.NewServer(&testLocalBucket{}, &testDB{posts: expected})
	lib.ExportServerGetHandler(sv, w, r)
	wr := w.Result()
	defer wr.Body.Close()

	if wr.StatusCode != http.StatusOK {
		t.Errorf("Status code error, %v", wr.StatusCode)
	}

	var actual []lib.Post
	if err := json.NewDecoder(wr.Body).Decode(&actual); err != nil {
		t.Fatalf("Decode error, %v", err)
	}
	if len(expected) != len(actual) {
		t.Fatalf("len error, len(expected) = %v, got len(actual) = %v", len(expected), len(actual))
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
		return nil, fmt.Errorf("CreatePart error, %w", err)
	}
	if _, err := fmt.Fprintf(jsonPart, `{"name":"%s", "body":"%s"}`, name, body); err != nil {
		return nil, fmt.Errorf("Fprintf error, %w", err)
	}

	// image
	if img != nil {
		imgHeader := make(textproto.MIMEHeader)
		imgHeader.Set("Content-Disposition", `form-data; name="attachment-file"; filename="dummy"`)
		imgPart, err := writer.CreatePart(imgHeader)
		if err != nil {
			return nil, fmt.Errorf("CreatePart error, %w", err)
		}
		if _, err := imgPart.Write(img); err != nil {
			return nil, fmt.Errorf("Write error, %w", err)
		}
	}

	// 最後に閉じる
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("Close error, %w", err)
	}

	r := httptest.NewRequest(http.MethodPost, "/", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r, nil
}

func createTestPng(dir, name string, minFilesize int) (path string, rerr error) {
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		return "", fmt.Errorf("Create error, %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			rerr = fmt.Errorf("Close error, %w", err)
		}
	}()

	e := png.Encoder{CompressionLevel: png.NoCompression}
	// png filesize = Width*Height*RGBA + other data
	// Width*Height*RGBA = minFilesize
	img := image.NewNRGBA(image.Rect(0, 0, minFilesize/4, 1))
	if err := e.Encode(f, img); err != nil {
		return "", fmt.Errorf("Encode error, %w", err)
	}
	return p, nil
}

func loadTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error, %v", err)
	}
	return b
}

func TestPostHandler(t *testing.T) {
	tempDir := t.TempDir()

	// create large test image
	largePng, err := createTestPng(tempDir, "largePng", lib.ExportMaxFilesize)
	if err != nil {
		t.Fatalf("createPng error, %v", err)
	}
	cases := []struct {
		name              string
		expectedName      string
		expectedBody      string
		expectedCreatedAt time.Time
		expectedImg       []byte
		wantErr           bool
	}{
		{name: "jpeg", expectedName: "Sophia", expectedBody: "bowwow", expectedCreatedAt: time.Now(), expectedImg: loadTestFile(t, "../testdata/go.jpg"), wantErr: false},
		{name: "png", expectedName: "gopher", expectedBody: "hello", expectedCreatedAt: time.Now(), expectedImg: loadTestFile(t, "../testdata/go.png"), wantErr: false},
		{name: "noimage", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), expectedImg: nil, wantErr: false},
		{name: "maxfilesize err", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), expectedImg: loadTestFile(t, largePng), wantErr: true},
		{name: "not an image err", expectedName: "koro", expectedBody: "wanwan", expectedCreatedAt: time.Now(), expectedImg: loadTestFile(t, "../testdata/go.txt"), wantErr: true},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			r, err := newPostRequest(c.expectedName, c.expectedBody, c.expectedImg)
			if err != nil {
				t.Fatalf("newPostRequest error, %v", err)
			}
			w := httptest.NewRecorder()
			sv := lib.NewServer(&testLocalBucket{dir: tempDir}, &testDB{createdAt: c.expectedCreatedAt})
			lib.ExportServerPostHandler(sv, w, r)
			wr := w.Result()
			defer wr.Body.Close()

			if c.wantErr && wr.StatusCode != http.StatusOK {
				return
			}
			if wr.StatusCode != http.StatusOK {
				t.Errorf("Status code error, %v", wr.StatusCode)
			}
			var actual lib.Post
			if err := json.NewDecoder(wr.Body).Decode(&actual); err != nil {
				t.Fatalf("Decode error, %v", err)
			}
			if actual.Name != c.expectedName || actual.Body != c.expectedBody || !actual.CreatedAt.Equal(c.expectedCreatedAt) {
				t.Fatalf("want postHandler() = (%v, %v, %v), got (%v, %v, %v)",
					c.expectedName, c.expectedBody, c.expectedCreatedAt, actual.Name, actual.Body, actual.CreatedAt)
			}
			if c.expectedImg == nil && actual.ImageURL != "" {
				t.Fatal("Image does not match error")
			}
			if c.expectedImg != nil {
				actualImg := loadTestFile(t, actual.ImageURL)
				if !bytes.Equal(actualImg, c.expectedImg) {
					t.Fatal("Image does not match error")
				}
			}
		})
	}
}
