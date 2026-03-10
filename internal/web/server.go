package web

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	indexRoutePathConstant               = "/"
	assetsRoutePathConstant              = "/assets"
	apiRepositoriesRoutePathConstant     = "/api/repos"
	apiRepositoryBranchesRouteConstant   = "/api/repos/:id/branches"
	apiBranchesRoutePathConstant         = "/api/branches"
	apiCommandsRoutePathConstant         = "/api/commands"
	apiFoldersRoutePathConstant          = "/api/folders"
	apiAuditInspectRoutePathConstant     = "/api/audit/inspect"
	apiAuditApplyRoutePathConstant       = "/api/audit/apply"
	apiRunsRoutePathConstant             = "/api/runs"
	apiRunRoutePathTemplateConstant      = "/api/runs/:id"
	indexDocumentFilePathConstant        = "ui/index.html"
	htmlContentTypeConstant              = "text/html; charset=utf-8"
	serverShutdownTimeoutConstant        = 5 * time.Second
	missingServerAddressErrorConstant    = "missing server address"
	missingCommandExecutorErrorConstant  = "missing command executor"
	missingBranchLoaderErrorConstant     = "missing branch loader"
	missingDirectoryBrowserErrorConstant = "missing directory browser"
	missingAuditInspectorErrorConstant   = "missing audit inspector"
	missingAuditChangeExecutorConstant   = "missing audit change executor"
	missingRepositoryIDErrorConstant     = "missing repository identifier"
	missingFolderPathErrorConstant       = "missing folder path"
	repositoryNotFoundTemplateConstant   = "repository %q was not found"
)

//go:embed ui
var embeddedUIFiles embed.FS

type serverRuntimeOptions struct {
	address      string
	repositories RepositoryCatalog
	catalog      CommandCatalog
	loadBranches BranchCatalogLoader
	browseDirs   DirectoryBrowser
	execute      CommandExecutor
	inspectAudit AuditInspector
	applyAudit   AuditChangeExecutor
}

// Server hosts the embedded gix browser interface and JSON API.
type Server struct {
	engine          *gin.Engine
	httpServer      *http.Server
	runStore        *runStore
	options         serverRuntimeOptions
	indexHTML       []byte
	repositoryIndex map[string]RepositoryDescriptor
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

	indexHTML, indexHTMLError := embeddedFile(indexDocumentFilePathConstant)
	if indexHTMLError != nil {
		return nil, indexHTMLError
	}
	assetsFileSystem, assetsFileSystemError := embeddedFileSystem("ui/assets")
	if assetsFileSystemError != nil {
		return nil, assetsFileSystemError
	}

	server := &Server{
		engine:          engine,
		runStore:        newRunStore(),
		options:         runtimeOptions,
		indexHTML:       indexHTML,
		repositoryIndex: buildRepositoryIndex(runtimeOptions.repositories),
	}
	server.httpServer = &http.Server{
		Addr:    runtimeOptions.address,
		Handler: engine,
	}
	server.registerRoutes(assetsFileSystem)

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
	if options.LoadBranches == nil {
		return serverRuntimeOptions{}, errors.New(missingBranchLoaderErrorConstant)
	}
	if options.BrowseDirectories == nil {
		return serverRuntimeOptions{}, errors.New(missingDirectoryBrowserErrorConstant)
	}
	if options.InspectAudit == nil {
		return serverRuntimeOptions{}, errors.New(missingAuditInspectorErrorConstant)
	}
	if options.ApplyAuditChanges == nil {
		return serverRuntimeOptions{}, errors.New(missingAuditChangeExecutorConstant)
	}

	return serverRuntimeOptions{
		address:      trimmedAddress,
		repositories: options.Repositories,
		catalog:      options.Catalog,
		loadBranches: options.LoadBranches,
		browseDirs:   options.BrowseDirectories,
		execute:      options.Execute,
		inspectAudit: options.InspectAudit,
		applyAudit:   options.ApplyAuditChanges,
	}, nil
}

func embeddedFileSystem(subdirectory string) (http.FileSystem, error) {
	subtree, subtreeError := fs.Sub(embeddedUIFiles, subdirectory)
	if subtreeError != nil {
		return nil, subtreeError
	}
	return http.FS(subtree), nil
}

func embeddedFile(path string) ([]byte, error) {
	return embeddedUIFiles.ReadFile(path)
}

func (server *Server) registerRoutes(assetsFileSystem http.FileSystem) {
	server.engine.GET(indexRoutePathConstant, func(requestContext *gin.Context) {
		requestContext.Data(http.StatusOK, htmlContentTypeConstant, server.indexHTML)
	})
	server.engine.StaticFS(assetsRoutePathConstant, assetsFileSystem)
	server.engine.GET(apiRepositoriesRoutePathConstant, server.handleRepositories)
	server.engine.GET(apiRepositoryBranchesRouteConstant, server.handleRepositoryBranches)
	server.engine.GET(apiBranchesRoutePathConstant, server.handleBranches)
	server.engine.GET(apiCommandsRoutePathConstant, server.handleCommands)
	server.engine.GET(apiFoldersRoutePathConstant, server.handleBrowseDirectories)
	server.engine.POST(apiAuditInspectRoutePathConstant, server.handleInspectAudit)
	server.engine.POST(apiAuditApplyRoutePathConstant, server.handleApplyAuditChanges)
	server.engine.POST(apiRunsRoutePathConstant, server.handleCreateRun)
	server.engine.GET(apiRunRoutePathTemplateConstant, server.handleGetRun)
}

func buildRepositoryIndex(catalog RepositoryCatalog) map[string]RepositoryDescriptor {
	index := make(map[string]RepositoryDescriptor, len(catalog.Repositories))
	for _, repository := range catalog.Repositories {
		index[repository.ID] = repository
	}
	return index
}

func (server *Server) handleRepositories(requestContext *gin.Context) {
	requestContext.JSON(http.StatusOK, server.options.repositories)
}

func (server *Server) handleRepositoryBranches(requestContext *gin.Context) {
	repositoryID := strings.TrimSpace(requestContext.Param("id"))
	if len(repositoryID) == 0 {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: missingRepositoryIDErrorConstant})
		return
	}

	repository, exists := server.repositoryIndex[repositoryID]
	if !exists {
		requestContext.JSON(http.StatusNotFound, errorResponse{Error: fmt.Sprintf(repositoryNotFoundTemplateConstant, repositoryID)})
		return
	}

	requestContext.JSON(http.StatusOK, server.options.loadBranches(requestContext.Request.Context(), repository))
}

func (server *Server) handleBranches(requestContext *gin.Context) {
	selectedRepository := server.selectedRepository()
	if selectedRepository == nil {
		requestContext.JSON(http.StatusOK, BranchCatalog{Error: server.options.repositories.Error})
		return
	}
	requestContext.JSON(http.StatusOK, server.options.loadBranches(requestContext.Request.Context(), *selectedRepository))
}

func (server *Server) handleCommands(requestContext *gin.Context) {
	requestContext.JSON(http.StatusOK, server.options.catalog)
}

func (server *Server) handleBrowseDirectories(requestContext *gin.Context) {
	folderPath := strings.TrimSpace(requestContext.Query("path"))
	if len(folderPath) == 0 {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: missingFolderPathErrorConstant})
		return
	}

	requestContext.JSON(http.StatusOK, server.options.browseDirs(requestContext.Request.Context(), folderPath))
}

func (server *Server) handleInspectAudit(requestContext *gin.Context) {
	var request AuditInspectionRequest
	if bindError := requestContext.ShouldBindJSON(&request); bindError != nil {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: bindError.Error()})
		return
	}

	requestContext.JSON(http.StatusOK, server.options.inspectAudit(requestContext.Request.Context(), request))
}

func (server *Server) handleApplyAuditChanges(requestContext *gin.Context) {
	var request AuditChangeApplyRequest
	if bindError := requestContext.ShouldBindJSON(&request); bindError != nil {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: bindError.Error()})
		return
	}

	requestContext.JSON(http.StatusOK, server.options.applyAudit(requestContext.Request.Context(), request))
}

func (server *Server) selectedRepository() *RepositoryDescriptor {
	selectedRepositoryID := strings.TrimSpace(server.options.repositories.SelectedRepositoryID)
	if len(selectedRepositoryID) > 0 {
		if repository, exists := server.repositoryIndex[selectedRepositoryID]; exists {
			repositoryCopy := repository
			return &repositoryCopy
		}
	}

	if len(server.options.repositories.Repositories) == 0 {
		return nil
	}

	repository := server.options.repositories.Repositories[0]
	return &repository
}
