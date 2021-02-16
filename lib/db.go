package lib

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type DB interface {
	GetAll() ([]Post, error)
	Insert(Post) (Post, error)
	Close() error
}

type mySQL struct {
	db *sql.DB
}

func OpenMySQL(dsn string) (DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("Open error, %w", err)
	}
	return &mySQL{db}, nil
}

func (s *mySQL) Close() error {
	if s.db == nil {
		return nil
	}

	err := s.db.Close()
	if err != nil {
		return fmt.Errorf("Close error, %w", err)
	}

	return nil
}

func (s *mySQL) Insert(p Post) (rp Post, rerr error) {
	stmt, err := s.db.Prepare("INSERT INTO posts VALUES(NULL, ?, ?, ?, NOW())")
	if err != nil {
		return Post{}, fmt.Errorf("Prepare error, %w", err)
	}
	defer func() {
		err := stmt.Close()
		if err == nil {
			return
		}
		rerr = fmt.Errorf("Close error, %v and %v", err, rerr)
	}()

	ret, err := stmt.Exec(p.Name, p.Body, p.ImageURL)
	if err != nil {
		return Post{}, fmt.Errorf("Exec error, %w", err)
	}
	id, err := ret.LastInsertId()
	if err != nil {
		return Post{}, fmt.Errorf("LastInsertId error, %w", err)
	}

	if err := s.db.QueryRow("SELECT name, body, imageurl, created_at FROM posts WHERE id = ?", id).Scan(&rp.Name, &rp.Body, &rp.ImageURL, &rp.CreatedAt); err != nil {
		return Post{}, fmt.Errorf("Scan error, %w", err)
	}

	return
}

func (s *mySQL) GetAll() (rps []Post, rerr error) {
	rows, err := s.db.Query("SELECT name, body, imageurl, created_at FROM posts")
	if err != nil {
		return nil, fmt.Errorf("Query error, %w", err)
	}
	defer func() {
		err := rows.Close()
		if err == nil {
			return
		}
		rerr = fmt.Errorf("Close error, %v and %v", err, rerr)
	}()

	// json にエンコードしたときに nil にならないよう、サイズ 0 で初期化しておく
	ps := make([]Post, 0)
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.Name, &p.Body, &p.ImageURL, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("Scan error, %w", err)
		}
		ps = append(ps, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Err error, %w", err)
	}

	return ps, nil
}
