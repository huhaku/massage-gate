package adapter

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func capture(t *testing.T, fn func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *[]byte) {
	t.Helper()
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		body = buf[:n]
		fn(w, r)
	}))
	return srv, &body
}

func TestWeComText(t *testing.T) {
	srv, body := capture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	defer srv.Close()
	if err := Send(context.Background(), "wecom", map[string]any{"url": srv.URL}, &Message{Title: "t1", Body: "b1"}); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	json.Unmarshal(*body, &p)
	if p["msgtype"] != "text" {
		t.Fatalf("msgtype = %v", p["msgtype"])
	}
	text := p["text"].(map[string]any)
	if text["content"] != "t1\nb1" {
		t.Fatalf("content = %v", text["content"])
	}
}

func TestWeComMarkdown(t *testing.T) {
	srv, body := capture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	defer srv.Close()
	err := Send(context.Background(), "wecom", map[string]any{"url": srv.URL, "msg_type": "markdown"},
		&Message{Title: "t1", Body: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	json.Unmarshal(*body, &p)
	md := p["markdown"].(map[string]any)
	if p["msgtype"] != "markdown" || md["content"] != "**t1**\nb1" {
		t.Fatalf("payload = %v", p)
	}
}

func TestFeishuSign(t *testing.T) {
	srv, body := capture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	defer srv.Close()
	err := Send(context.Background(), "feishu", map[string]any{"url": srv.URL, "secret": "s3cret"},
		&Message{Title: "t1", Body: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.Unmarshal(*body, &got)
	tsStr, _ := got["timestamp"].(string)
	sign, _ := got["sign"].(string)
	if tsStr == "" || sign == "" {
		t.Fatalf("missing timestamp/sign: %v", got)
	}
	mac := hmac.New(sha256.New, []byte(fmt.Sprintf("%s\n%s", tsStr, "s3cret")))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if sign != want {
		t.Fatalf("sign mismatch: got %s want %s", sign, want)
	}
	content := got["content"].(map[string]any)
	if got["msg_type"] != "text" || content["text"] != "t1\nb1" {
		t.Fatalf("payload = %v", got)
	}
}

func TestDingTalkSign(t *testing.T) {
	var queryTS, querySign string
	var bodyCopy []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queryTS, querySign = q.Get("timestamp"), q.Get("sign")
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		bodyCopy = buf[:n]
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	err := Send(context.Background(), "dingtalk", map[string]any{"url": srv.URL + "/robot/send?access_token=x", "secret": "s3cret"},
		&Message{Title: "t1", Body: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	if queryTS == "" || querySign == "" {
		t.Fatalf("missing timestamp/sign in query")
	}
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(fmt.Sprintf("%s\n%s", queryTS, "s3cret")))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if querySign != want {
		t.Fatalf("sign mismatch: got %s want %s", querySign, want)
	}
	var p map[string]any
	json.Unmarshal(bodyCopy, &p)
	text := p["text"].(map[string]any)
	if p["msgtype"] != "text" || text["content"] != "t1\nb1" {
		t.Fatalf("payload = %v", p)
	}
}
