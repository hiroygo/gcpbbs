package lib

import (
	"encoding/json"
	"fmt"
	"image"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"

	_ "image/jpeg"
	_ "image/png"
)

type Server struct {
	bucket CloudStorageBucket
	db     DB
}

func NewServer(b CloudStorageBucket, d DB) *Server {
	return &Server{bucket: b, db: d}
}

func (sv *Server) postHandler(w http.ResponseWriter, r *http.Request) {
	var p Post

	// Content-Disposition のフィールドによって値がどこにセットされるかが変わる
	// フィールドに filename と name が存在するときは FormFile にセットされる
	// フィールドが name だけのときは FormValue にセットされる
	// e.g. Content-Disposition: form-data; name="json"; filename="json"
	data := r.FormValue("json")
	if data == "" {
		log.Println("Empty json error")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		log.Printf("Unmarshal error, %v", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// 画像をストレージに登録する
	img, imgh, err := r.FormFile("attachment-file")
	switch err {
	case nil:
		const maxfilesize = 1024 * 1024 * 2
		if imgh.Size > maxfilesize {
			log.Println(http.StatusText(http.StatusRequestEntityTooLarge))
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}

		// 画像か確認する
		_, format, err := image.DecodeConfig(img)
		if err != nil {
			log.Printf("DecodeConfig error, %v", err)
			http.Error(w, http.StatusText(http.StatusUnsupportedMediaType), http.StatusUnsupportedMediaType)
			return
		}
		// DecodeConfig で移動した offset を先頭に戻す
		// DecodeConfig はファイルヘッダなどを読み、ファイル全体は読み込まない
		if _, err := img.Seek(0, io.SeekStart); err != nil {
			log.Printf("Seek error, %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		filename, err := randomFilename("." + format)
		if err != nil {
			log.Printf("randomFilename error, %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
		url, err := sv.bucket.Upload(filename, img)
		if err != nil {
			log.Printf("Upload error, %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		p.ImageURL = url
	case http.ErrMissingFile:
		p.ImageURL = ""
	default:
		log.Printf("FormFile error, %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// 投稿内容をデータベースに登録する
	ret, err := sv.db.Insert(p)
	if err != nil {
		log.Printf("Insert error, %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// 登録結果をクライアントに返す
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(ret); err != nil {
		log.Printf("Encode error, %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (sv *Server) getHandler(w http.ResponseWriter, r *http.Request) {
	ps, err := sv.db.GetAll()
	if err != nil {
		log.Printf("GetAll error, %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(ps); err != nil {
		log.Printf("Encode error, %v", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}

func (sv *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func (sv *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		sv.indexHandler(w, r)
		return
	}

	if r.URL.Path == "/posts" && r.Method == http.MethodPost {
		sv.postHandler(w, r)
		return
	}

	if r.URL.Path == "/posts" && r.Method == http.MethodGet {
		sv.getHandler(w, r)
		return
	}

	http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
}

func randomFilename(ext string) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("NewRandom error, %v", err)
	}
	return fmt.Sprintf("%s%s", id, ext), nil
}
