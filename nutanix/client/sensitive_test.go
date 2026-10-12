package client

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSensitiveClientRejectsHTTPFailuresAndProtectsSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/private/users" {
			t.Errorf("incorrect path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	base, err := NewClient(&Credentials{URL: strings.TrimPrefix(server.URL, "http://"), Username: "user", Password: "auth-secret"}, "test", "original", true)
	if err != nil {
		t.Fatal(err)
	}
	sensitive, err := base.SensitiveAPIClient("private")
	if err != nil {
		t.Fatal(err)
	}
	if base.AbsolutePath != "original" {
		t.Fatal("sensitive client mutated shared client")
	}
	t.Setenv("TF_LOG", "DEBUG")
	var output bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(oldWriter)
	req, err := sensitive.NewRequest(context.Background(), http.MethodPost, "/users", map[string]string{"password": "body-secret"})
	if err != nil {
		t.Fatal(err)
	}
	err = sensitive.DoSensitive(context.Background(), req, nil)
	var status *SensitiveHTTPError
	if !errors.As(err, &status) || status.StatusCode != 500 {
		t.Fatal("empty HTTP 500 was accepted", err)
	}
	if strings.Contains(output.String(), "body-secret") || strings.Contains(output.String(), "auth-secret") {
		t.Fatal("secret logged by sensitive client")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = sensitive.DoSensitive(ctx, req, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("request ignored cancellation", err)
	}
}
