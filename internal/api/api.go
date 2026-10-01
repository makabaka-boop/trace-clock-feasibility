// Package api 提供同步调用链时钟偏移校正的 HTTP 接口。
package api

import (
	"encoding/json"
	"net/http"

	"clocksync/internal/solver"
)

// NewHandler 返回 HTTP 路由。staticDir 非空时托管前端静态文件（如 web/dist）。
func NewHandler(staticDir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/solve", solve)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	if staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(staticDir)))
	}
	return mux
}

func solve(w http.ResponseWriter, r *http.Request) {
	var in solver.Input
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "请求体不是合法的 JSON："+err.Error())
		return
	}
	res, err := solver.Solve(&in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
