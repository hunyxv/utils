package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"testing"
	"time"

	"go.uber.org/zap"
)

var addr string = "127.0.0.1:80"

func newHttpService() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/queryget", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		query := req.Form
		log.Printf("get request: %+v", query)
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/postform", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		query := req.PostForm
		log.Printf("PostForm request: %+v", query)
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/postjson", func(w http.ResponseWriter, req *http.Request) {
		type Entity struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		body, _ := ioutil.ReadAll(req.Body)
		var entity Entity
		if err := json.Unmarshal(body, &entity); err != nil {
			log.Fatal(err)
		}

		log.Printf("PostJson request: %+v", entity)
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/timeout", func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Second * 3)
		io.WriteString(w, "ok")
	})
	mux.HandleFunc("/retry", func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "retry query", http.StatusGatewayTimeout)
	})

	return &http.Server{
		Handler: mux,
		Addr:    addr,
	}
}

func TestGet(t *testing.T) {
	service := newHttpService()
	defer service.Shutdown(context.TODO())
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	time.Sleep(DefaultRetryDelay)

	query := make(url.Values)
	query.Set("a", "1")
	query.Set("b", "2")
	body, err := Get(fmt.Sprintf("http://%s%s", addr, "/queryget"), query, WithTTL(time.Millisecond*3))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}

func TestRetry(t *testing.T) {
	service := newHttpService()
	defer service.Shutdown(context.TODO())
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	time.Sleep(DefaultRetryTimes)
	logger := zap.NewExample()
	_, err := Get(fmt.Sprintf("http://%s%s", addr, "/retry"), nil, WithLogger(logger), WithRetryTimes(3), WithTTL(time.Millisecond*3))
	if err != nil {
		t.Logf("%+v", err)
	}
}

func TestPostForm(t *testing.T) {
	service := newHttpService()
	defer service.Shutdown(context.TODO())
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	time.Sleep(DefaultRetryTimes)
	form := make(url.Values)
	form.Set("field1", "abcd")
	form.Set("field2", "123")
	body, err := PostForm(fmt.Sprintf("http://%s%s", addr, "/postform"), form, WithTTL(time.Millisecond*3))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}

func TestPostJson(t *testing.T) {
	service := newHttpService()
	defer service.Shutdown(context.TODO())
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	body, err := PostJSON(fmt.Sprintf("http://%s%s", addr, "/postjson"), []byte(`{
		"name": "zhangsan",
		"age": 22
	}`), WithTTL(time.Millisecond*3))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(body))
}

func TestTimeout(t *testing.T) {
	service := newHttpService()
	defer service.Shutdown(context.TODO())
	go func() {
		if err := service.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	time.Sleep(DefaultRetryDelay)
	logger := zap.NewExample()
	_, err := Get(fmt.Sprintf("http://%s%s", addr, "/timeout"), nil, WithTTL(time.Millisecond*3), WithLogger(logger))
	if err != nil {
		t.Log(err)
	}
}
