# gcpbbs
* 掲示板 Web アプリケーション
* アプリケーションホスティングには Google App Engine を使う
* 画像の保存には Google Cloud Storage を使う
* 投稿内容の保存には Cloud SQL を使う

## MySQL でテーブル作成
```
CREATE TABLE posts (id INTEGER UNSIGNED NOT NULL AUTO_INCREMENT,name VARCHAR(50) NOT NULL, body TEXT NOT NULL, imageurl VARCHAR(512), created_at DATETIME NOT NULL, PRIMARY KEY (id));
```

## curl でのテスト
`curl -v -F 'json={"name":"sophia", "body":"sophia"};type=application/json' -F "attachment-file=@../go.jpg;" http://localhost:8080/posts`
