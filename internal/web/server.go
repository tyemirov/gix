package web

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	indexRoutePathConstant              = "/"
	assetsRoutePathConstant             = "/assets"
	apiCommandsRoutePathConstant        = "/api/commands"
	apiRunsRoutePathConstant            = "/api/runs"
	apiRunRoutePathTemplateConstant     = "/api/runs/:id"
	serverShutdownTimeoutConstant       = 5 * time.Second
	missingServerAddressErrorConstant   = "missing server address"
	missingCommandExecutorErrorConstant = "missing command executor"
)

//go:embed ui/index.html ui/assets/app.js ui/assets/styles.css
var embeddedUIFiles embed.FS

type serverRuntimeOptions struct {
	address string
	catalog CommandCatalog
	execute CommandExecutor
}

// Server hosts the embedded gix browser interface and JSON API.
type Server struct {
	engine     *gin.Engine
	httpServer *http.Server
	runStore   *runStore
	options    serverRuntimeOptions
}

// Run starts the configured server and blocks until the context is canceled or the server exits.
func Run(executionContext context.Context, options ServerOptions) error {
	server, serverError := NewServer(options)
	if serverError != nil {
		return serverError
	}

	return server.ListenAndServeContext(executionContext)
}

// NewServer assembles a reusable HTTP server instance.
func NewServer(options ServerOptions) (*Server, error) {
	runtimeOptions, optionsError := newServerRuntimeOptions(options)
	if optionsError != nil {
		return nil, optionsError
	}

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	uiFileSystem, uiFileSystemError := embeddedFileSystem("ui")
	if uiFileSystemError != nil {
		return nil, uiFileSystemError
	}
	assetsFileSystem, assetsFileSystemError := embeddedFileSystem("ui/assets")
	if assetsFileSystemError != nil {
		return nil, assetsFileSystemError
	}

	server := &Server{
		engine:   engine,
		runStore: newRunStore(),
		options:  runtimeOptions,
	}
	server.httpServer = &http.Server{
		Addr:    runtimeOptions.address,
		Handler: engine,
	}
	server.registerRoutes(uiFileSystem, assetsFileSystem)

	return server, nil
}

// Handler exposes the HTTP handler for tests and custom hosting.
func (server *Server) Handler() http.Handler {
	return server.engine
}

// Serve accepts an existing listener.
func (server *Server) Serve(listener net.Listener) error {
	return server.httpServer.Serve(listener)
}

// ListenAndServeContext starts the server on the configured address and shuts it down when the context is canceled.
func (server *Server) ListenAndServeContext(executionContext context.Context) error {
	if executionContext == nil {
		executionContext = context.Background()
	}

	listener, listenError := net.Listen("tcp", server.httpServer.Addr)
	if listenError != nil {
		return listenError
	}

	serverErrorChannel := make(chan error, 1)
	go func() {
		serveError := server.Serve(listener)
		if errors.Is(serveError, http.ErrServerClosed) {
			serverErrorChannel <- nil
			return
		}
		serverErrorChannel <- serveError
	}()

	select {
	case serveError := <-serverErrorChannel:
		return serveError
	case <-executionContext.Done():
		shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), serverShutdownTimeoutConstant)
		defer cancelShutdown()

		shutdownError := server.httpServer.Shutdown(shutdownContext)
		serveError := <-serverErrorChannel
		if shutdownError != nil {
			return shutdownError
		}
		return serveError
	}
}

func newServerRuntimeOptions(options ServerOptions) (serverRuntimeOptions, error) {
	trimmedAddress := strings.TrimSpace(options.Address)
	if len(trimmedAddress) == 0 {
		return serverRuntimeOptions{}, errors.New(missingServerAddressErrorConstant)
	}
	if options.Execute == nil {
		return serverRuntimeOptions{}, errors.New(missingCommandExecutorErrorConstant)
	}

	return serverRuntimeOptions{
		address: trimmedAddress,
		catalog: options.Catalog,
		execute: options.Execute,
	}, nil
}

func embeddedFileSystem(subdirectory string) (http.FileSystem, error) {
	subtree, subtreeError := fs.Sub(embeddedUIFiles, subdirectory)
	if subtreeError != nil {
		return nil, subtreeError
	}
	return http.FS(subtree), nil
}

func (server *Server) registerRoutes(uiFileSystem http.FileSystem, assetsFileSystem http.FileSystem) {
	server.engine.GET(indexRoutePathConstant, func(requestContext *gin.Context) {
		requestContext.FileFromFS("index.html", uiFileSystem)
	})
	server.engine.StaticFS(assetsRoutePathConstant, assetsFileSystem)
	server.engine.GET(apiCommandsRoutePathConstant, server.handleCommands)
	server.engine.POST(apiRunsRoutePathConstant, server.handleCreateRun)
	server.engine.GET(apiRunRoutePathTemplateConstant, server.handleGetRun)
}

func (server *Server) handleCommands(requestContext *gin.Context) {
	requestContext.JSON(http.StatusOK, server.options.catalog)
}
