package plugin

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kardianos/osext"
	"github.com/rockbears/log"
	"github.com/spf13/afero"

	"github.com/ovh/cds/engine/worker/pkg/workerruntime"
	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/grpcplugin"
)

func createGRPCPluginSocket(ctx context.Context, pluginType string, pluginName string, w workerruntime.Runtime, env map[string]string) (*clientSocket, *sdk.GRPCPlugin, error) {
	log.Info(ctx, "create socket for plugin %q", pluginName)
	currentOS := strings.ToLower(sdk.GOOS)
	currentARCH := strings.ToLower(sdk.GOARCH)

	var pluginBinaryInfos *sdk.GRPCPluginBinary
	var currentPlugin *sdk.GRPCPlugin
	switch pluginType {
	case TypeAction, TypeStream:
		currentPlugin = w.GetActionPlugin(pluginName)
		if currentPlugin == nil {
			var err error
			currentPlugin, err = w.PluginGet(pluginName)
			if err != nil {
				return nil, nil, sdk.NewErrorFrom(sdk.ErrNotFound, "plugin:%s Unable to get plugin ... Aborting", pluginName)
			}
			w.SetActionPlugin(currentPlugin)
		}
	case TypeIntegration:
		currentPlugin = w.GetIntegrationPlugin(pluginName)
		if currentPlugin == nil {
			return nil, nil, sdk.NewErrorFrom(sdk.ErrNotFound, "plugin:%s Unable to get plugin ... Aborting", pluginName)
		}
	}

	pluginBinaryInfos = currentPlugin.GetBinary(currentOS, currentARCH)
	if pluginBinaryInfos == nil {
		return nil, nil, sdk.NewErrorFrom(sdk.ErrNotFound, "unable to find plugin %s for %s/%s", pluginName, currentOS, currentARCH)
	}

	// Try to download the plugin
	if _, err := w.BaseDir().Stat(pluginBinaryInfos.Name); os.IsNotExist(err) {
		log.Debug(ctx, "Downloading the plugin %s", pluginBinaryInfos.PluginName)
		var retry int
		for {
			//If the file doesn't exist. Download it.
			fi, err := w.BaseDir().OpenFile(pluginBinaryInfos.Name, os.O_CREATE|os.O_RDWR, os.FileMode(pluginBinaryInfos.Perm))
			if err != nil {
				return nil, nil, sdk.WrapError(err, "unable to create the file %s", pluginBinaryInfos.Name)
			}

			log.Debug(ctx, "Get the binary plugin %s", pluginBinaryInfos.PluginName)
			if err := w.PluginGetBinary(pluginBinaryInfos.PluginName, currentOS, currentARCH, fi); err != nil {
				err = sdk.WrapError(err, "unable to get the binary plugin the file %s", pluginBinaryInfos.PluginName)
				if retry >= 20 {
					_ = fi.Close()
					return nil, nil, err
				}
				log.Debug(ctx, "%v", err)
				retry++
				time.Sleep(3 * time.Second)
				continue
			}

			_ = fi.Close()
			break
		}
	} else {
		log.Debug(ctx, "plugin binary is in cache %s", pluginBinaryInfos)
	}

	log.Info(ctx, "Starting GRPC Plugin %s", pluginBinaryInfos.Name)
	fileContent, err := afero.ReadFile(w.BaseDir(), pluginBinaryInfos.GetName())
	if err != nil {
		return nil, nil, sdk.WrapError(err, "plugin:%s unable to get plugin binary file... Aborting", pluginName)
	}

	switch {
	case sdk.IsTar(fileContent):
		if err := sdk.Untar(w.BaseDir(), "", bytes.NewReader(fileContent)); err != nil {
			return nil, nil, sdk.WrapError(err, "plugin:%s unable to untar binary file", pluginName)
		}
	case sdk.IsGz(fileContent):
		if err := sdk.UntarGz(w.BaseDir(), "", bytes.NewReader(fileContent)); err != nil {
			return nil, nil, sdk.WrapError(err, "plugin:%s unable to untarGz binary file", pluginName)
		}
	}

	var basedir string
	if x, ok := w.BaseDir().(*afero.BasePathFs); ok {
		basedir, _ = x.RealPath(".")
	} else {
		basedir = w.BaseDir().Name()
	}

	cmd := pluginBinaryInfos.Cmd
	if _, err := sdk.LookPath(w.BaseDir(), cmd); err != nil {
		return nil, nil, sdk.WrapError(err, "plugin:%s unable to find GRPC plugin, binary command not found.", pluginName)
	}
	cmd = path.Join(basedir, cmd)

	for i := range pluginBinaryInfos.Entrypoints {
		pluginBinaryInfos.Entrypoints[i] = path.Join(basedir, pluginBinaryInfos.Entrypoints[i])
	}
	args := append(pluginBinaryInfos.Entrypoints, pluginBinaryInfos.Args...)
	var errstart error

	workdir, err := workerruntime.WorkingDirectory(ctx)
	if err != nil {
		return nil, nil, err
	}
	var dir string
	if x, ok := w.BaseDir().(*afero.BasePathFs); ok {
		dir, _ = x.RealPath(workdir.Name())
	} else {
		dir = workdir.Name()
	}

	// Retrieve worker environment variables
	workerEnvs := w.Environ()
	mWorkerEnvs := make(map[string]string, len(workerEnvs))
	for _, e := range workerEnvs {
		splitted := strings.SplitN(e, "=", 2)
		if len(splitted) != 2 {
			continue
		}
		mWorkerEnvs[splitted[0]] = splitted[1]
	}

	// Add env variable from execution context
	for k, v := range env {
		// Set all env ( do not ovveride existing var )
		if _, ok := mWorkerEnvs[k]; !ok && k != "PATH" {
			mWorkerEnvs[k] = v
			continue
		}
	}

	// Manage PATH
	if v, has := env["PATH"]; has {
		existingPath := mWorkerEnvs["PATH"]
		existingPathList := filepath.SplitList(existingPath)
		newPath := v
		newPathList := filepath.SplitList(newPath)
		newPathList = append(newPathList, existingPathList...)
		newPathList = sdk.Unique(newPathList)
		mWorkerEnvs["PATH"] = strings.Join(newPathList, string(filepath.ListSeparator))
	}

	var envs []string
	for k, v := range mWorkerEnvs {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	c := clientSocket{}

	workerpath, err := osext.Executable()
	if err != nil {
		return nil, nil, sdk.WrapError(err, "unable to get current executable path")
	}

	log.Debug(ctx, "runScriptAction> Worker binary path: %s", path.Dir(workerpath))
	for i := range envs {
		if strings.HasPrefix(envs[i], "PATH") {
			envs[i] = fmt.Sprintf("%s:%s", envs[i], path.Dir(workerpath))
			break
		}
	}

	if c.StdPipe, c.Socket, errstart = grpcplugin.StartPlugin(ctx, pluginName, dir, cmd, args, envs); errstart != nil {
		return nil, nil, sdk.WrapError(errstart, "plugin:%s unable to start GRPC plugin... Aborting", pluginName)
	}
	return &c, currentPlugin, nil
}
