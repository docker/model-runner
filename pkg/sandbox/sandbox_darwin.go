package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// ConfigurationLlamaCpp is the sandbox configuration for llama.cpp processes.
const ConfigurationLlamaCpp = `(version 1)

;;; Keep a default allow policy (because encoding things like DYLD support and
;;; device access is quite difficult), but deny critical exploitation targets
;;; (generally aligned with the App Sandbox entitlements that aren't on by
;;; default). In theory we'll be subject to the Docker.app sandbox as well
;;; (unless we're running standalone), but even Docker.app has a very privileged
;;; sandbox, so we need additional constraints.
;;;
;;; Note: The following are known to be required at some level for llama.cpp
;;; (though we could further experiment to deny certain sub-permissions):
;;;   - authorization
;;;   - darwin
;;;   - iokit
;;;   - mach
;;;   - socket
;;;   - syscall
;;;   - process
(allow default)

;;; Deny network access, except for our IPC sockets.
;;; NOTE: We use different socket nomenclature when running in Docker Desktop
;;; (inference-N.sock) vs. standalone (inference-runner-N.sock), so we use a
;;; wildcard to support both.
(deny network*)
(allow network-bind network-inbound
    (regex #"inference.*-[0-9]+\.sock$"))

;;; Deny access to the camera and microphone.
(deny device*)

;;; Deny access to NVRAM settings.
(deny nvram*)

;;; Deny access to system-level privileges.
(deny system*)

;;; Deny access to job creation.
(deny job-creation)

;;; Don't allow new executable code to be created in memory at runtime.
(deny dynamic-code-generation)

;;; Disable access to user preferences.
(deny user-preference*)

;;; Restrict file access.
;;; NOTE: For some reason, the (home-subpath "...") predicate used in system
;;; sandbox profiles doesn't work with sandbox-exec.
;;; NOTE: We have to allow access to the working directory for standalone mode.
;;; NOTE: We have to allow access to a regex-based Docker.app location to
;;; support Docker Desktop development as well as Docker.app installs that don't
;;; live inside /Applications.
;;; NOTE: For some reason (deny file-read*) really doesn't like to play nice
;;; with llama.cpp, so for that reason we'll avoid a blanket ban and just ban
;;; directories that might contain sensitive data.
(deny file-map-executable)
(deny file-write*)
(deny file-read*
    (subpath "/Applications")
    (subpath "/private/etc")
    (subpath "/Library")
    (subpath "/Users")
    (subpath "/Volumes"))
(allow file-read* file-map-executable
    (subpath "/usr")
    (subpath "/System")
    (regex #"Docker\.app/Contents/Resources/model-runner")
    (subpath "[UPDATEDBINPATH]")
    (subpath "[UPDATEDLIBPATH]"))
(allow file-write*
    (literal "/dev/null")
    (subpath "/private/var")
    (subpath "[HOMEDIR]/Library/Containers/com.docker.docker/Data")
    (subpath "[WORKDIR]"))
(allow file-read*
    (subpath "[HOMEDIR]/.docker/models")
    (subpath "[HOMEDIR]/Library/Containers/com.docker.docker/Data")
    (subpath "[WORKDIR]"))
`

// sandbox is the Darwin sandbox implementation.
type sandbox struct {
	// cancel cancels the context associated with the process.
	cancel context.CancelFunc
	// command is the sandboxed process handle.
	command *exec.Cmd
}

// Command implements Sandbox.Command.
func (s *sandbox) Command() *exec.Cmd {
	return s.command
}

// Command implements Sandbox.Close.
func (s *sandbox) Close() error {
	s.cancel()
	return nil
}

// Create creates a sandbox containing a single process that has been started.
// The ctx, name, and arg arguments correspond to their counterparts in
// os/exec.CommandContext. The configuration argument specifies the sandbox
// configuration, for which a pre-defined value should be used. The modifier
// function allows for an optional callback (which may be nil) to configure the
// command before it is started.
// If configuration is empty, the process is started without sandboxing.
func Create(ctx context.Context, configuration string, modifier func(*exec.Cmd), updatedBinPath, name string, arg ...string) (Sandbox, error) {
	ctx, cancel := context.WithCancel(ctx)

	var command *exec.Cmd

	if configuration == "" {
		command = exec.CommandContext(ctx, name, arg...)
	} else {
		currentUser, err := user.Current()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("unable to lookup user: %w", err)
		}

		currentDirectory, err := os.Getwd()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("unable to determine working directory: %w", err)
		}

		// TODO: switch to text/template if this gets any more complex
		profile := strings.ReplaceAll(configuration, "[HOMEDIR]", currentUser.HomeDir)
		profile = strings.ReplaceAll(profile, "[WORKDIR]", currentDirectory)
		profile = strings.ReplaceAll(profile, "[UPDATEDBINPATH]", updatedBinPath)
		profile = strings.ReplaceAll(profile, "[UPDATEDLIBPATH]", filepath.Join(filepath.Dir(updatedBinPath), "lib"))

		sandboxedArgs := make([]string, 0, len(arg)+3)
		sandboxedArgs = append(sandboxedArgs, "-p", profile, name)
		sandboxedArgs = append(sandboxedArgs, arg...)
		command = exec.CommandContext(ctx, "sandbox-exec", sandboxedArgs...)
	}

	if modifier != nil {
		modifier(command)
	}

	if err := command.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("unable to start sandboxed process: %w", err)
	}
	return &sandbox{
		cancel:  cancel,
		command: command,
	}, nil
}
