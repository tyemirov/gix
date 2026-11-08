// Command local manages the repository's local Gix web application.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tyemirov/gix/cmd/cli"
)

const (
	controlResource = "/runtime"
	startupTimeout  = 15 * time.Second
	requestTimeout  = 20 * time.Second
	loopbackHost    = "127.0.0.1"
)

type specification struct {
	Root   string `json:"root"`
	State  string `json:"state"`
	Config string `json:"config"`
	Port   int    `json:"port"`
}

type startupResult struct {
	Error string `json:"error"`
}

func (spec specification) url() string {
	return "http://" + net.JoinHostPort(loopbackHost, strconv.Itoa(spec.Port))
}

func (spec specification) client() *http.Client {
	return &http.Client{Timeout: requestTimeout, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(spec.State, "control.sock"))
		},
	}}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return errors.New("local application requires up or down")
	}
	action := os.Args[1]
	if action != "up" && action != "down" && action != "serve" {
		return fmt.Errorf("unknown local application operation %q", action)
	}
	flags := flag.NewFlagSet("local "+action, flag.ContinueOnError)
	var spec specification
	flags.StringVar(&spec.Root, "root", ".", "local repository root")
	flags.StringVar(&spec.State, "state-dir", ".git/gix-local", "owned local runtime directory")
	flags.StringVar(&spec.Config, "config", "", "Gix configuration file")
	flags.IntVar(&spec.Port, "port", 8080, "loopback application port")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || spec.Port < 1 || spec.Port > 65535 {
		return errors.New("local application requires named options and a port between 1 and 65535")
	}
	for _, path := range []*string{&spec.Root, &spec.State, &spec.Config} {
		if *path != "" {
			absolute, err := filepath.Abs(*path)
			if err != nil {
				return err
			}
			*path = absolute
		}
	}
	switch action {
	case "up":
		return start(spec)
	case "down":
		return stop(spec)
	case "serve":
		ready := os.NewFile(3, "startup")
		defer ready.Close()
		return serve(spec, ready)
	}
	panic("validated local application operation")
}

func start(spec specification) error {
	client := spec.client()
	defer client.CloseIdleConnections()
	response, err := client.Get("http://local" + controlResource)
	if err == nil {
		defer response.Body.Close()
		var running specification
		if err := json.NewDecoder(response.Body).Decode(&running); err != nil {
			return fmt.Errorf("read local application: %w", err)
		}
		if response.StatusCode != http.StatusOK || running != spec {
			return errors.New("local application is already running with different settings; use make down first")
		}
		fmt.Println("Local Gix is running at", spec.url())
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read local application control socket: %w", err)
	}
	if err := os.MkdirAll(spec.State, 0o755); err != nil {
		return fmt.Errorf("create local runtime directory: %w", err)
	}
	log, err := os.OpenFile(filepath.Join(spec.State, "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	if err != nil {
		return fmt.Errorf("open local application log: %w", err)
	}
	defer log.Close()
	readiness, announcement, err := os.Pipe()
	if err != nil {
		return err
	}
	defer readiness.Close()
	defer announcement.Close()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, append([]string{"serve"}, os.Args[2:]...)...)
	command.Stdout, command.Stderr = log, log
	command.ExtraFiles = []*os.File{announcement}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start local application: %w", err)
	}
	announcement.Close()
	var result startupResult
	if err := json.NewDecoder(readiness).Decode(&result); err != nil {
		_ = command.Wait()
		return fmt.Errorf("read local application startup result (%s): %w", filepath.Join(spec.State, "server.log"), err)
	}
	if result.Error != "" {
		_ = command.Wait()
		return errors.New(result.Error)
	}
	if err := command.Process.Release(); err != nil {
		return err
	}
	fmt.Println("Local Gix is running at", spec.url())
	return nil
}

func stop(spec specification) error {
	client := spec.client()
	defer client.CloseIdleConnections()
	request, err := http.NewRequest(http.MethodDelete, "http://local"+controlResource, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stop local application: %w", err)
	}
	if response != nil {
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return fmt.Errorf("stop local application: HTTP %d", response.StatusCode)
		}
	}
	fmt.Println("Local Gix is stopped.")
	return nil
}

func serve(spec specification, ready *os.File) (result error) {
	announced := false
	defer func() {
		if !announced {
			_ = json.NewEncoder(ready).Encode(startupResult{fmt.Sprint(result)})
		}
		ready.Close()
	}()
	listener, err := net.Listen("unix", filepath.Join(spec.State, "control.sock"))
	if err != nil {
		return fmt.Errorf("create local application control socket: %w", err)
	}
	defer listener.Close()
	port, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(spec.Port)))
	if err != nil {
		return fmt.Errorf("start local application listener: %w", err)
	}
	port.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	done := make(chan struct{})
	var applicationError error
	go func() {
		defer close(done)
		arguments := []string{"--web", "--bind", loopbackHost, "--port", strconv.Itoa(spec.Port), "--roots", spec.Root}
		if spec.Config != "" {
			arguments = append(arguments, "--config", spec.Config)
		}
		applicationError = cli.NewApplication().ExecuteWithOptions(cli.ExecutionOptions{
			Arguments: arguments, Context: ctx, StandardInput: os.Stdin, StandardOutput: os.Stdout, StandardError: os.Stderr,
		})
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+controlResource, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(spec)
	})
	mux.HandleFunc("DELETE "+controlResource, func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		<-done
		w.WriteHeader(http.StatusNoContent)
	})
	control := &http.Server{Handler: mux, ReadHeaderTimeout: requestTimeout}
	defer control.Close()
	if err := waitForApplication(spec, done, &applicationError); err != nil {
		cancel()
		<-done
		return err
	}
	controlErrors := make(chan error, 1)
	go func() { controlErrors <- control.Serve(listener) }()
	if err := json.NewEncoder(ready).Encode(startupResult{}); err != nil {
		cancel()
		<-done
		return err
	}
	announced = true
	ready.Close()
	select {
	case <-done:
	case err := <-controlErrors:
		cancel()
		<-done
		return fmt.Errorf("serve local application control socket: %w", err)
	}
	shutdown, finish := context.WithTimeout(context.Background(), requestTimeout)
	defer finish()
	if err := control.Shutdown(shutdown); err != nil {
		return err
	}
	if ctx.Err() != nil && errors.Is(applicationError, context.Canceled) {
		return nil
	}
	return applicationError
}

func waitForApplication(spec specification, done <-chan struct{}, applicationError *error) error {
	deadline := time.NewTimer(startupTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for {
		select {
		case <-done:
			return fmt.Errorf("start local Gix: %w", *applicationError)
		case <-deadline.C:
			return fmt.Errorf("start local Gix: listener did not become ready at %s", spec.url())
		case <-poll.C:
			response, err := client.Get(spec.url() + "/api/repos")
			if err == nil {
				_, readError := io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if readError != nil {
					return fmt.Errorf("read local application readiness: %w", readError)
				}
				if response.StatusCode != http.StatusOK {
					return fmt.Errorf("verify local application readiness: HTTP %d", response.StatusCode)
				}
				return nil
			}
		}
	}
}
