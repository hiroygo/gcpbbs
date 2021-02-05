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
		msg := "Empty json error"
		log.Println(msg)
		writeErrorJSON(w, http.StatusBadRequest, msg)
		return
	}
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		msg := fmt.Sprintf("Unmarshal error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusBadRequest, msg)
		return
	}

	// 画像をストレージに登録する
	img, imgh, err := r.FormFile("attachment-file")
	switch err {
	case nil:
		if imgh.Size > maxFilesize {
			msg := http.StatusText(http.StatusRequestEntityTooLarge)
			log.Println(msg)
			writeErrorJSON(w, http.StatusRequestEntityTooLarge, msg)
			return
		}

		// 画像か確認する
		_, format, err := image.DecodeConfig(img)
		if err != nil {
			msg := fmt.Sprintf("DecodeConfig error, %v", err)
			log.Println(msg)
			writeErrorJSON(w, http.StatusUnsupportedMediaType, msg)
			return
		}
		// DecodeConfig で移動した offset を先頭に戻す
		// DecodeConfig はファイルヘッダなどを読み、ファイル全体は読み込まない
		if _, err := img.Seek(0, io.SeekStart); err != nil {
			msg := fmt.Sprintf("Seek error, %v", err)
			log.Println(msg)
			writeErrorJSON(w, http.StatusInternalServerError, msg)
			return
		}

		filename, err := randomFilename("." + format)
		if err != nil {
			msg := fmt.Sprintf("randomFilename error, %v", err)
			log.Println(msg)
			writeErrorJSON(w, http.StatusInternalServerError, msg)
		}
		url, err := sv.bucket.Upload(filename, img)
		if err != nil {
			msg := fmt.Sprintf("Upload error, %v", err)
			log.Println(msg)
			writeErrorJSON(w, http.StatusInternalServerError, msg)
			return
		}

		p.ImageURL = url
	case http.ErrMissingFile:
		p.ImageURL = ""
	default:
		msg := fmt.Sprintf("FormFile error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusInternalServerError, msg)
		return
	}

	// 投稿内容をデータベースに登録する
	ret, err := sv.db.Insert(p)
	if err != nil {
		msg := fmt.Sprintf("Insert error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusInternalServerError, msg)
		return
	}

	// 登録結果をクライアントに返す
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(ret); err != nil {
		msg := fmt.Sprintf("Encode error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusInternalServerError, msg)
	}
}

func (sv *Server) getHandler(w http.ResponseWriter, r *http.Request) {
	ps, err := sv.db.GetAll()
	if err != nil {
		msg := fmt.Sprintf("GetAll error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusInternalServerError, msg)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(ps); err != nil {
		msg := fmt.Sprintf("Encode error, %v", err)
		log.Println(msg)
		writeErrorJSON(w, http.StatusInternalServerError, msg)
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

	writeErrorJSON(w, http.StatusForbidden, http.StatusText(http.StatusForbidden))
}

func writeErrorJSON(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, message)))
}

func randomFilename(ext string) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("NewRandom error, %v", err)
	}
	return fmt.Sprintf("%s%s", id, ext), nil
}
