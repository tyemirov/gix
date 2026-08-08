package web

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/githubauth"
)

func TestServeContextPropagatesLaunchContextToRequests(testInstance *testing.T) {
	contextValue := "configured-token"
	executionContext, cancelExecution := context.WithCancel(
		githubauth.WithCredential(context.Background(), contextValue),
	)
	defer cancelExecution()

	requestContextValues := make(chan string, 1)
	server, serverError := NewServer(ServerOptions{
		Address: "127.0.0.1:0",
		BrowseDirectories: func(requestContext context.Context, folderPath string) DirectoryListing {
			resolvedToken, _ := githubauth.ResolveToken(requestContext, nil)
			requestContextValues <- resolvedToken
			return DirectoryListing{Path: folderPath}
		},
		InspectAudit: func(context.Context, AuditInspectionRequest) AuditInspectionResponse {
			return AuditInspectionResponse{}
		},
		ApplyAuditChanges: func(context.Context, AuditChangeApplyRequest) AuditChangeApplyResponse {
			return AuditChangeApplyResponse{}
		},
	})
	require.NoError(testInstance, serverError)

	listener, listenError := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(testInstance, listenError)

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.serveContext(executionContext, listener)
	}()

	requestURL := "http://" + listener.Addr().String() + apiFoldersRoutePathConstant + "?path=" + url.QueryEscape("/tmp")
	response, requestError := http.Get(requestURL)
	require.NoError(testInstance, requestError)
	_, readError := io.Copy(io.Discard, response.Body)
	require.NoError(testInstance, readError)
	require.NoError(testInstance, response.Body.Close())
	require.Equal(testInstance, http.StatusOK, response.StatusCode)
	require.Equal(testInstance, contextValue, <-requestContextValues)

	cancelExecution()
	require.NoError(testInstance, <-serverErrors)
}
