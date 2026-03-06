package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	runStatusRunningConstant          = "running"
	runStatusSucceededConstant        = "succeeded"
	runStatusFailedConstant           = "failed"
	missingRunArgumentsErrorConstant  = "at least one command argument is required"
	missingRunIdentifierErrorConstant = "missing run identifier"
	runNotFoundTemplateConstant       = "run %q was not found"
	runIdentifierTemplateConstant     = "run-%06d"
	panicErrorTemplateConstant        = "panic: %v"
)

type errorResponse struct {
	Error string `json:"error"`
}

type rawRunRequest struct {
	Arguments []string `json:"arguments"`
	Input     string   `json:"stdin"`
}

type runCommandRequest struct {
	arguments []string
	input     string
}

// RunSnapshot describes one background CLI execution.
type RunSnapshot struct {
	ID             string     `json:"id"`
	Arguments      []string   `json:"arguments"`
	Status         string     `json:"status"`
	StandardOutput string     `json:"stdout"`
	StandardError  string     `json:"stderr"`
	Error          string     `json:"error,omitempty"`
	ExitCode       int        `json:"exit_code"`
	StartedAt      time.Time  `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

type runStore struct {
	mutex    sync.RWMutex
	sequence uint64
	runs     map[string]*runState
}

type runState struct {
	mutex         sync.RWMutex
	snapshot      RunSnapshot
	standardOut   *synchronizedBuffer
	standardError *synchronizedBuffer
}

type synchronizedBuffer struct {
	mutex   sync.RWMutex
	builder strings.Builder
}

func newRunStore() *runStore {
	return &runStore{runs: map[string]*runState{}}
}

func newRunCommandRequest(rawRequest rawRunRequest) (runCommandRequest, error) {
	sanitizedArguments := make([]string, 0, len(rawRequest.Arguments))
	for _, argument := range rawRequest.Arguments {
		trimmedArgument := strings.TrimSpace(argument)
		if len(trimmedArgument) == 0 {
			continue
		}
		sanitizedArguments = append(sanitizedArguments, trimmedArgument)
	}
	if len(sanitizedArguments) == 0 {
		return runCommandRequest{}, errors.New(missingRunArgumentsErrorConstant)
	}

	return runCommandRequest{
		arguments: sanitizedArguments,
		input:     rawRequest.Input,
	}, nil
}

func (server *Server) handleCreateRun(requestContext *gin.Context) {
	var rawRequest rawRunRequest
	if bindError := requestContext.ShouldBindJSON(&rawRequest); bindError != nil {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: bindError.Error()})
		return
	}

	runRequest, requestError := newRunCommandRequest(rawRequest)
	if requestError != nil {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: requestError.Error()})
		return
	}

	runState := server.runStore.create(runRequest)
	go server.executeRun(runState, runRequest)

	requestContext.JSON(http.StatusAccepted, runState.snapshotView())
}

func (server *Server) handleGetRun(requestContext *gin.Context) {
	runIdentifier := strings.TrimSpace(requestContext.Param("id"))
	if len(runIdentifier) == 0 {
		requestContext.JSON(http.StatusBadRequest, errorResponse{Error: missingRunIdentifierErrorConstant})
		return
	}

	runState, exists := server.runStore.lookup(runIdentifier)
	if !exists {
		requestContext.JSON(http.StatusNotFound, errorResponse{Error: fmt.Sprintf(runNotFoundTemplateConstant, runIdentifier)})
		return
	}

	requestContext.JSON(http.StatusOK, runState.snapshotView())
}

func (server *Server) executeRun(state *runState, request runCommandRequest) {
	executionError := error(nil)
	defer func() {
		if recoveredValue := recover(); recoveredValue != nil {
			executionError = fmt.Errorf(panicErrorTemplateConstant, recoveredValue)
		}
		state.complete(executionError)
	}()

	executionError = server.options.execute(
		context.Background(),
		append([]string(nil), request.arguments...),
		strings.NewReader(request.input),
		state.standardOut,
		state.standardError,
	)
}

func (store *runStore) create(request runCommandRequest) *runState {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.sequence++
	runIdentifier := fmt.Sprintf(runIdentifierTemplateConstant, store.sequence)
	state := newRunState(runIdentifier, request.arguments)
	store.runs[runIdentifier] = state
	return state
}

func (store *runStore) lookup(runIdentifier string) (*runState, bool) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	state, exists := store.runs[runIdentifier]
	return state, exists
}

func newRunState(runIdentifier string, arguments []string) *runState {
	return &runState{
		snapshot: RunSnapshot{
			ID:        runIdentifier,
			Arguments: append([]string(nil), arguments...),
			Status:    runStatusRunningConstant,
			StartedAt: time.Now().UTC(),
		},
		standardOut:   &synchronizedBuffer{},
		standardError: &synchronizedBuffer{},
	}
}

func (state *runState) snapshotView() RunSnapshot {
	state.mutex.RLock()
	snapshot := state.snapshot
	state.mutex.RUnlock()

	snapshot.Arguments = append([]string(nil), snapshot.Arguments...)
	snapshot.StandardOutput = state.standardOut.String()
	snapshot.StandardError = state.standardError.String()
	return snapshot
}

func (state *runState) complete(executionError error) {
	completedAt := time.Now().UTC()

	state.mutex.Lock()
	defer state.mutex.Unlock()

	state.snapshot.CompletedAt = &completedAt
	state.snapshot.StandardOutput = state.standardOut.String()
	state.snapshot.StandardError = state.standardError.String()
	if executionError != nil {
		state.snapshot.Status = runStatusFailedConstant
		state.snapshot.Error = executionError.Error()
		state.snapshot.ExitCode = 1
		return
	}

	state.snapshot.Status = runStatusSucceededConstant
	state.snapshot.ExitCode = 0
}

func (buffer *synchronizedBuffer) Write(payload []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.builder.Write(payload)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.mutex.RLock()
	defer buffer.mutex.RUnlock()
	return buffer.builder.String()
}
