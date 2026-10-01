package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"clocksync/internal/solver"
)

func post(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/solve", strings.NewReader(body))
	rec := httptest.NewRecorder()
	NewHandler("").ServeHTTP(rec, req)
	return rec
}

func TestSolveEndpointFeasible(t *testing.T) {
	body := `{
	  "services": [
	    {"id": "gateway", "minOffset": 0, "maxOffset": 0, "anchor": true},
	    {"id": "auth", "minOffset": -8, "maxOffset": 8},
	    {"id": "db", "minOffset": -8, "maxOffset": 8}
	  ],
	  "spans": [
	    {"id": "s1", "serviceId": "gateway", "parentId": "", "start": 0, "end": 60},
	    {"id": "s2", "serviceId": "auth", "parentId": "s1", "start": 5, "end": 30},
	    {"id": "s3", "serviceId": "db", "parentId": "s2", "start": 2, "end": 15},
	    {"id": "s4", "serviceId": "db", "parentId": "s1", "start": 40, "end": 55}
	  ]
	}`
	rec := post(t, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码=%d，响应=%s", rec.Code, rec.Body)
	}
	var res solver.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Feasible {
		t.Fatalf("应可行，得到矛盾环：%+v", res.Cycle)
	}
	// 字典序：auth < db < gateway → auth=-5, db=-2, gateway=0
	want := []solver.Offset{
		{ServiceID: "auth", Offset: -5},
		{ServiceID: "db", Offset: -2},
		{ServiceID: "gateway", Offset: 0},
	}
	got, _ := json.Marshal(res.Offsets)
	wantJSON, _ := json.Marshal(want)
	if string(got) != string(wantJSON) {
		t.Fatalf("偏移=%s，期望 %s", got, wantJSON)
	}
	if len(res.CorrectedSpans) != 4 || len(res.Margins) != 3 {
		t.Fatalf("校正 span 数=%d，余量数=%d", len(res.CorrectedSpans), len(res.Margins))
	}
}

func TestSolveEndpointInfeasible(t *testing.T) {
	body := `{
	  "services": [
	    {"id": "a", "minOffset": 0, "maxOffset": 0, "anchor": true},
	    {"id": "b", "minOffset": -5, "maxOffset": 5}
	  ],
	  "spans": [
	    {"id": "p", "serviceId": "a", "parentId": "", "start": 0, "end": 10},
	    {"id": "c", "serviceId": "b", "parentId": "p", "start": 5, "end": 20}
	  ]
	}`
	rec := post(t, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码=%d，响应=%s", rec.Code, rec.Body)
	}
	var res solver.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Feasible {
		t.Fatal("应不可行")
	}
	if res.Cycle == nil || len(res.Cycle.Edges) == 0 || res.Cycle.TotalWeight >= 0 {
		t.Fatalf("矛盾环非法：%+v", res.Cycle)
	}
	if res.Offsets != nil || res.CorrectedSpans != nil || res.Margins != nil {
		t.Fatal("不可行时不得返回偏移/校正时间线/余量")
	}
}

func TestSolveEndpointBadRequests(t *testing.T) {
	cases := map[string]string{
		"非 JSON":      `{`,
		"未知字段":        `{"services":[],"spans":[],"extra":1}`,
		"服务太少":        `{"services":[{"id":"a","minOffset":0,"maxOffset":0,"anchor":true}],"spans":[]}`,
		"缺少锚定":        `{"services":[{"id":"a","minOffset":-1,"maxOffset":1},{"id":"b","minOffset":-1,"maxOffset":1}],"spans":[]}`,
		"span 引用未知服务": `{"services":[{"id":"a","minOffset":0,"maxOffset":0,"anchor":true},{"id":"b","minOffset":-1,"maxOffset":1}],"spans":[{"id":"s","serviceId":"zz","parentId":"","start":0,"end":1}]}`,
	}
	for name, body := range cases {
		rec := post(t, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s：状态码=%d，期望 400", name, rec.Code)
		}
		var e map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
			t.Fatalf("%s：错误响应应含 error 字段：%s", name, rec.Body)
		}
	}
}
