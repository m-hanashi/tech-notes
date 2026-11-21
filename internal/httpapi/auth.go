package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"path"
)

// Authenticator は、リクエストが認証されているかを判断する部品に求めること。
// 認証の方式をあとで差し替えられるように、決まった形だけを決めておく（仕様書 4.5）。
type Authenticator interface {
	Authenticate(r *http.Request) bool
}

// APIKeyAuthenticator は、ヘッダー X-API-Key のキーを照らし合わせる。
type APIKeyAuthenticator struct {
	keyHash [sha256.Size]byte
}

// NewAPIKeyAuthenticator は、key を正しいキーとする APIKeyAuthenticator を返す。
// key が空なら、エラーを返す。空のキーのハッシュは「ヘッダーなし」のハッシュと同じになり、
// キーのないリクエストがすべて通ってしまうため。
func NewAPIKeyAuthenticator(key string) (*APIKeyAuthenticator, error) {
	if key == "" {
		return nil, errors.New("API キーが空です")
	}
	return &APIKeyAuthenticator{keyHash: sha256.Sum256([]byte(key))}, nil
}

// Authenticate は、X-API-Key のキーが正しいかを、一定時間比較で確かめる。
// 先に両方をハッシュにして長さをそろえる。長さが違うとすぐに結果が出て、
// 時間からキーの長さが推測できてしまうため。
func (a *APIKeyAuthenticator) Authenticate(r *http.Request) bool {
	got := sha256.Sum256([]byte(r.Header.Get("X-API-Key")))
	return subtle.ConstantTimeCompare(got[:], a.keyHash[:]) == 1
}

// requireAuth は、認証されていないリクエストを 401 で断り、next に渡さない。
// /health だけは認証しない（ALB のヘルスチェックはキーを送れないため）。
// ルーターより前で断るので、存在しないパスでも 401 になり、どんなパスがあるかを知られない。
func requireAuth(auth Authenticator, next http.Handler) http.Handler {
	if auth == nil {
		// 渡し忘れたまま起動すると、リクエストのたびに落ちるので、起動の時点で止める
		panic("httpapi: 認証の部品が nil です")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// //health のような書き方も、ルーターと同じように整えてから比べる
		if path.Clean(r.URL.Path) == "/health" || auth.Authenticate(r) {
			next.ServeHTTP(w, r)
			return
		}
		// RFC 9110 では、401 には WWW-Authenticate を必ず付ける
		w.Header().Set("WWW-Authenticate", `ApiKey header="X-API-Key"`)
		writeProblem(w, http.StatusUnauthorized, "ヘッダー X-API-Key に、正しい API キーを入れてください")
	})
}
