package headers

import (
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v4"
	"github.com/roadrunner-server/endure/v2"
	"github.com/roadrunner-server/headers/v4"
	httpPlugin "github.com/roadrunner-server/http/v4"
	"github.com/roadrunner-server/logger/v4"
	"github.com/roadrunner-server/server/v4"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slog"
)

func TestHeadersInit(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
		Path:    "configs/.rr-headers-init.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&headers.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	stopCh <- struct{}{}
	wg.Wait()
}

func TestRequestHeaders(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
		Path:    "configs/.rr-req-headers.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&headers.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("RequestHeaders", reqHeaders)

	stopCh <- struct{}{}
	wg.Wait()
}

func reqHeaders(t *testing.T) {
	req, err := http.NewRequest("GET", "http://127.0.0.1:22655?hello=value", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)

	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "CUSTOM-HEADER", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestResponseHeaders(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
		Path:    "configs/.rr-res-headers.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&headers.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("ResponseHeaders", resHeaders)

	stopCh <- struct{}{}
	wg.Wait()
}

func resHeaders(t *testing.T) {
	req, err := http.NewRequest("GET", "http://127.0.0.1:22455?hello=value", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	assert.Equal(t, "output-header", r.Header.Get("output"))

	b, err := io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)
	assert.Equal(t, "CUSTOM-HEADER", string(b))

	err = r.Body.Close()
	assert.NoError(t, err)
}

func TestCORSHeaders(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
		Path:    "configs/.rr-cors-headers.yaml",
		Prefix:  "rr",
	}

	err := cont.RegisterAll(
		cfg,
		&logger.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&headers.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	stopCh := make(chan struct{}, 1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	t.Run("CORSHeaders", corsHeaders)
	t.Run("CORSHeadersPass", corsHeadersPass)

	stopCh <- struct{}{}
	wg.Wait()
}

func corsHeadersPass(t *testing.T) {
	req, err := http.NewRequest("GET", "http://127.0.0.1:22855", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	assert.Equal(t, "true", r.Header.Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "*", r.Header.Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "*", r.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", r.Header.Get("Access-Control-Allow-Credentials"))

	_, err = io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}

func corsHeaders(t *testing.T) {
	req, err := http.NewRequest("OPTIONS", "http://127.0.0.1:22855", nil)
	assert.NoError(t, err)

	r, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)

	assert.Equal(t, "true", r.Header.Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "*", r.Header.Get("Access-Control-Allow-Headers"))
	assert.Equal(t, "GET,POST,PUT,DELETE", r.Header.Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "*", r.Header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "600", r.Header.Get("Access-Control-Max-Age"))
	assert.Equal(t, "true", r.Header.Get("Access-Control-Allow-Credentials"))

	_, err = io.ReadAll(r.Body)
	assert.NoError(t, err)
	assert.Equal(t, 200, r.StatusCode)

	err = r.Body.Close()
	assert.NoError(t, err)
}
