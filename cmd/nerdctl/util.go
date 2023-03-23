package main

import (
  "context"
  "errors"
  "fmt"
  "os"
  "path/filepath"
  "strings"
  "time"

  "github.com/containerd/containerd"
  "github.com/containerd/containerd/cmd/ctr/commands"
  "github.com/containerd/containerd/namespaces"
  "github.com/containerd/nerdctl/pkg/api/types"
  "github.com/containerd/nerdctl/pkg/clientutil"
  "github.com/containerd/nerdctl/pkg/cmd/container"
  spb "google.golang.org/protobuf/types/known/structpb"
)

type ReqParameter = string
type ParamValue = struct {
  key string
  value interface{}
}

// -------------------------------------------------------------- //
// hasParameter
// ---------------------------------------------------------------//
func hasParameters(ctx context.Context, keys ...string) bool {
  for _, key := range keys {
    if ctx.Value(ReqParameter(key)) == nil {
      return false
    }
  }
  return true
}

// -------------------------------------------------------------- //
// getParameter
// ---------------------------------------------------------------//
func getParameter(ctx context.Context, key string) (value *spb.Value) {
  if ivalue := ctx.Value(ReqParameter(key)); ivalue != nil {
    value, _ = spb.NewValue(ivalue)
  }
  return value
}

func reportDetachedModeError(ctx context.Context, label string, err error) {

  errLogDir := getParameter(ctx, "ErrLogDir").GetStringValue()

  if errLogDir == "" {
    panic("errLogDir is not defined")
  }
  
  logfile := fmt.Sprintf("%s/%s-%s.log", errLogDir, label, time.Now().Format("150405.000000"))
  
  os.WriteFile(logfile, []byte(err.Error()), 0644)
}

func waitForStop(statusC <-chan containerd.ExitStatus, rmFlag bool, removal func()) error {
  if rmFlag {
    fmt.Fprintf(os.Stdout, "!!!!!!!! removal exec is deferred\n")
    defer removal()
  }
  status := <-statusC
  code, _, err := status.Result()
  if err != nil {
    return err
  }
  if code != 0 {
    return ExitCodeError{
      exitCode: int(code),
    }
  }
  return nil
}

//----------------------------------------------------------------//
// WithNs
//----------------------------------------------------------------//
func WithNs(ctx context.Context, globalOpt types.GlobalCommandOptions, params ...ParamValue) (context.Context, error) {
  if ctx == nil {
    ctx = context.Background()
  }

  dataStore, err := clientutil.DataStore(globalOpt.DataRoot, globalOpt.Address)
  if err != nil {
    return ctx, err
  }

  if globalOpt.Namespace == "" {
    return ctx, errors.New("namespace is required")
  }
  if strings.Contains(globalOpt.Namespace, "/") {
    return ctx, errors.New("namespace with '/' is unsupported")
  }
  errLogDir :=  filepath.Join(dataStore, "containers", globalOpt.Namespace, "errlog")

  if err := os.MkdirAll(errLogDir, 0700); err != nil {
    return ctx, err
  }

  key := ReqParameter("ErrLogDir")
  ctx = context.WithValue(ctx, key, errLogDir)

  for _, param := range params {
    key := ReqParameter(param.key)
    ctx = context.WithValue(ctx, key, param.value)
  }

  return namespaces.WithNamespace(ctx, globalOpt.Namespace), nil
}

//----------------------------------------------------------------//
// getNamespaceErrLogDirPath
//----------------------------------------------------------------//
func getNamespaceErrLogDirPath(globalOpt types.GlobalCommandOptions) (string, error) {
  dataStore, err := clientutil.DataStore(globalOpt.DataRoot, globalOpt.Address)
  if err != nil {
    return "", err
  }

  if globalOpt.Namespace == "" {
    return "", errors.New("namespace is required")
  }
  if strings.Contains(globalOpt.Namespace, "/") {
    return "", errors.New("namespace with '/' is unsupported")
  }
  return filepath.Join(dataStore, "containers", globalOpt.Namespace, "errlog"), nil
}

//----------------------------------------------------------------//
// runWaitAsync
//----------------------------------------------------------------//
func runWaitAsync(ctx context.Context, cntr containerd.Container, globalOpt types.GlobalCommandOptions) error {

  task, err := cntr.Task(ctx, nil)
  if err != nil {
    return fmt.Errorf("container task retrieval failed : %v", err)
  }

  deadline, cancel := context.WithCancel(ctx)
  defer cancel()

  statusCh, err := task.Wait(deadline)
  if err != nil {
    return fmt.Errorf("status channel creation failed : %v", err)
  }

  err = task.Start(deadline)
  if err != nil {
    return fmt.Errorf("task failed to start : %v", err)
  }
  
  sigc := commands.ForwardAllSignals(deadline, task)
  defer commands.StopCatch(sigc)

  _, err = waitAsyncCntrStop(deadline, statusCh)

  if err != nil {
    return err
  }

  autoRemove := getParameter(ctx, "AutoRemove").GetBoolValue()

  if autoRemove {
    if err := container.RemoveContainer(ctx, cntr, globalOpt, true, true); err != nil {
      return err
    }
  }
  
  return err
}

// -------------------------------------------------------------- //
// waitAsyncCntrStop
// -------------------------------------------------------------- //
func waitAsyncCntrStop(deadline context.Context, statusCh <-chan containerd.ExitStatus) (int, error) {
  for {
    select {
    case <-deadline.Done():
      if err := deadline.Err(); err != nil {
        return 408, fmt.Errorf("deadline exceeded")
      }
    case status := <-statusCh:
      code, _, err := status.Result()
      return int(code), err
    }
  }
}