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

const maxFilesize = 1024 * 1024 * 2

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
		writeAndLogError(w, http.StatusBadRequest, fmt.Errorf("Empty JSON error"))
		return
	}
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		writeAndLogError(w, http.StatusBadRequest, fmt.Errorf("Unmarshal error, %v", err))
		return
	}

	// 画像をストレージに登録する
	img, imgh, err := r.FormFile("attachment-file")
	switch err {
	case nil:
		if imgh.Size > maxFilesize {
			writeAndLogError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("%s", http.StatusText(http.StatusRequestEntityTooLarge)))
			return
		}

		// 画像か確認する
		_, format, err := image.DecodeConfig(img)
		if err != nil {
			writeAndLogError(w, http.StatusUnsupportedMediaType, fmt.Errorf("DecodeConfig error, %v", err))
			return
		}
		// DecodeConfig で移動した offset を先頭に戻す
		// DecodeConfig はファイルヘッダなどを読み、ファイル全体は読み込まない
		if _, err := img.Seek(0, io.SeekStart); err != nil {
			writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("Seek error, %v", err))
			return
		}

		filename, err := randomFilename("." + format)
		if err != nil {
			writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("randomFilename error, %v", err))
		}
		url, err := sv.bucket.Upload(filename, img)
		if err != nil {
			writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("Upload error, %v", err))
			return
		}

		p.ImageURL = url
	case http.ErrMissingFile:
		p.ImageURL = ""
	default:
		writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("FormFile error, %v", err))
		return
	}

	// 投稿内容をデータベースに登録する
	ret, err := sv.db.Insert(p)
	if err != nil {
		writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("Insert error, %v", err))
		return
	}

	// 登録結果をクライアントに返す
	writeJSON(w, http.StatusOK, ret)
}

func (sv *Server) getHandler(w http.ResponseWriter, r *http.Request) {
	ps, err := sv.db.GetAll()
	if err != nil {
		writeAndLogError(w, http.StatusInternalServerError, fmt.Errorf("GetAll error, %v", err))
		return
	}
	writeJSON(w, http.StatusOK, ps)
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

	writeError(w, http.StatusForbidden, fmt.Errorf("%s", http.StatusText(http.StatusForbidden)))
}

func writeAndLogError(w http.ResponseWriter, status int, err error) {
	log.Println(err)
	writeError(w, status, err)
}

func writeError(w http.ResponseWriter, status int, err error) {
	var s = struct {
		Error string `json:"error"`
	}{Error: err.Error()}
	writeJSON(w, status, s)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println(fmt.Sprintf("Encode error, %v", err))
	}
}

func randomFilename(ext string) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("NewRandom error, %v", err)
	}
	return fmt.Sprintf("%s%s", id, ext), nil
}
