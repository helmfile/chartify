package chartify

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

func RunCommand(cmd string, args []string, stdout, stderr io.Writer, env map[string]string) error {
	command := exec.Command(cmd, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = mergeEnv(os.Environ(), env)
	return command.Run()
}

func mergeEnv(orig []string, new map[string]string) []string {
	wanted := env2map(orig)
	for k, v := range new {
		wanted[k] = v
	}
	return map2env(wanted)
}

func map2env(wanted map[string]string) []string {
	result := []string{}
	for k, v := range wanted {
		result = append(result, k+"="+v)
	}
	return result
}

func env2map(env []string) map[string]string {
	wanted := map[string]string{}
	for _, cur := range env {
		pair := strings.SplitN(cur, "=", 2)
		wanted[pair[0]] = pair[1]
	}
	return wanted
}
