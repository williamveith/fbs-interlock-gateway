package process

import (
	"fmt"
	"os/exec"
	"runtime"
)

func KillPort(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}

	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		script := fmt.Sprintf(`Get-NetTCPConnection -LocalPort %d -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique | ForEach-Object { Stop-Process -Id $_ -Force }`, port)
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	} else {
		script := fmt.Sprintf(`pids=$(lsof -ti tcp:%d -sTCP:LISTEN 2>/dev/null || true); if [ -n "$pids" ]; then kill -9 $pids; fi`, port)
		cmd = exec.Command("bash", "-c", script)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to kill listener on port %d: %w: %s", port, err, string(output))
	}

	return nil
}
