package maintenance

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HouzuoGuo/laitos/misc"
	"github.com/HouzuoGuo/laitos/platform"
)

const (
	SwapFilePath = "/laitos-swap-file"
)

// SynchroniseSystemClock uses three different tools to immediately synchronise system clock via NTP servers.
func (daemon *Daemon) SynchroniseSystemClock(out *bytes.Buffer) {
	if misc.HostIsWindows() {
		daemon.logPrintStage(out, "skipped on windows: synchronise clock")
		daemon.CorrectStartupTime(out)
		return
	}
	daemon.logPrintStage(out, "synchronise clock")
	// Use three tools to immediately synchronise system clock
	result, err := platform.InvokeProgram([]string{"PATH=" + platform.CommonPATH}, 60, "ntpdate", "-4", "0.pool.ntp.org", "us.pool.ntp.org", "de.pool.ntp.org", "nz.pool.ntp.org")
	daemon.logPrintStageStep(out, "ntpdate: %v - %s", err, strings.TrimSpace(result))
	result, err = platform.InvokeProgram([]string{"PATH=" + platform.CommonPATH}, 60, "chronyd", "-q", "pool pool.ntp.org iburst")
	daemon.logPrintStageStep(out, "chronyd: %v - %s", err, strings.TrimSpace(result))
	result, err = platform.InvokeProgram([]string{"PATH=" + platform.CommonPATH}, 60, "busybox", "ntpd", "-n", "-q", "-p", "ie.pool.ntp.org", "ca.pool.ntp.org", "au.pool.ntp.org")
	daemon.logPrintStageStep(out, "busybox ntpd: %v - %s", err, strings.TrimSpace(result))

	daemon.CorrectStartupTime(out)
}

/*
CorrectStartTime corrects program start time in case system clock is skewed.
The program startup time is used to detect outdated commands (such as in telegram bot), in rare case if system clock
was severely skewed, causing program startup time to be in the future, the detection mechanisms will misbehave.
*/
func (daemon *Daemon) CorrectStartupTime(out *bytes.Buffer) {
	if misc.StartupTime.After(time.Now()) {
		daemon.logPrintStage(out, "clock was severely skewed, reset program startup time.")
		// The earliest possible opportunity for system maintenance to run is now minus initial delay
		misc.StartupTime = time.Now().Add(-InitialDelaySec * time.Second)
	}
}

// MaintainServices manipulate service state according to configuration.
func (daemon *Daemon) MaintainServices(out *bytes.Buffer) {
	if daemon.DisableStopServices == nil && daemon.EnableStartServices == nil {
		return
	}
	daemon.logPrintStage(out, "maintain service state")

	if daemon.DisableStopServices != nil {
		sort.Strings(daemon.DisableStopServices)
		for _, name := range daemon.DisableStopServices {
			daemon.logPrintStageStep(out, "disable&stop %s: success? %v", name, misc.DisableStopDaemon(name))
		}
	}
	if daemon.EnableStartServices != nil {
		sort.Strings(daemon.EnableStartServices)
		for _, name := range daemon.EnableStartServices {
			daemon.logPrintStageStep(out, "enable&start %s: success? %v", name, misc.EnableStartDaemon(name))
		}
	}
}

// BlockUnusedLogin will block/disable system login from users not listed in the exception list.
func (daemon *Daemon) BlockUnusedLogin(out *bytes.Buffer) {
	if daemon.BlockSystemLoginExcept == nil || len(daemon.BlockSystemLoginExcept) == 0 {
		return
	}
	daemon.logPrintStage(out, "block unused system login")
	// Exception name list is case insensitive
	exceptionMap := make(map[string]bool)
	for _, name := range daemon.BlockSystemLoginExcept {
		exceptionMap[strings.ToLower(name)] = true
	}
	for userName := range misc.GetLocalUserNames() {
		if exceptionMap[strings.ToLower(userName)] {
			daemon.logPrintStageStep(out, "not going to touch excluded user %s", userName)
			continue
		}
		if ok, blockOut := misc.BlockUserLogin(userName); ok {
			daemon.logPrintStageStep(out, "blocked user %s", userName)
		} else {
			daemon.logPrintStageStep(out, "failed to block user %s - %v", userName, blockOut)
		}
	}
}

// MaintainWindowsIntegrity uses DISM and FSC utilities to maintain Windows system integrity.
func (daemon *Daemon) MaintainWindowsIntegrity(out *bytes.Buffer) {
	if !misc.HostIsWindows() {
		return
	}
	daemon.logPrintStage(out, "maintain windows system integrity")
	// These tools seriously spend a lot of time
	progOut, err := platform.InvokeProgram(nil, 3*3600, `C:\Windows\system32\Dism.exe`, "/Online", "/Cleanup-Image", "/StartComponentCleanup", "/ResetBase")
	daemon.logPrintStageStep(out, "dism StartComponentCleanup: %v - %s", err, progOut)
	progOut, err = platform.InvokeProgram(nil, 3*3600, `C:\Windows\system32\Dism.exe`, "/Online", "/Cleanup-Image", "/SPSuperseded")
	daemon.logPrintStageStep(out, "dism SPSuperseded: %v - %s", err, progOut)
	progOut, err = platform.InvokeProgram(nil, 3*3600, `C:\Windows\system32\Dism.exe`, "/Online", "/Cleanup-Image", "/Restorehealth")
	daemon.logPrintStageStep(out, "dism Restorehealth: %v - %s", err, progOut)
	progOut, err = platform.InvokeProgram(nil, 3*3600, `C:\Windows\system32\sfc.exe`, "/ScanNow")
	daemon.logPrintStageStep(out, "sfc ScanNow: %v - %s", err, progOut)
	// Installation of windows update is kicked off in background by the usoclient command, this way it will not run in parallel to DISM actions above.
	daemon.logPrintStage(out, "install windows updates in background")
	progOut, err = platform.InvokeProgram(nil, 10*60, `C:\Windows\system32\usoclient.exe`, "StartInstallWait")
	daemon.logPrintStageStep(out, "usoclient: %v - %s", err, progOut)
}

// MaintainSwapFile creates and activates a swap file for Linux system, or turns swap off depending on configuration input.
func (daemon *Daemon) MaintainSwapFile(out *bytes.Buffer) {
	if misc.HostIsWindows() {
		daemon.logPrintStage(out, "skipped on windows: maintain swap file")
		return
	}
	if daemon.SwapFileSizeMB == 0 {
		return
	}
	daemon.logPrintStage(out, "create/turn on swap file "+SwapFilePath)
	if daemon.SwapFileSizeMB < 0 {
		daemon.logPrintStageStep(out, "turn off swap")
		if err := misc.SwapOff(); err != nil {
			daemon.logPrintStageStep(out, "failed to turn off swap: %v", err)
		}
		return
	} else if daemon.SwapFileSizeMB > 0 {
		_, swapFileStatus := os.Stat(SwapFilePath)
		// Create the swap file if it does not yet exist
		if os.IsNotExist(swapFileStatus) {
			buf := make([]byte, 1048576)
			fh, err := os.Create(SwapFilePath)
			if err != nil {
				daemon.logPrintStageStep(out, "failed to create swap file - %v", err)
				return
			}
			for i := 0; i < daemon.SwapFileSizeMB; i++ {
				if _, err := fh.Write(buf); err != nil {
					daemon.logPrintStageStep(out, "failed to create swap file - %v", err)
					return
				}
			}
			if err := fh.Sync(); err != nil {
				daemon.logPrintStageStep(out, "failed to create swap file - %v", err)
				return
			}
			if err := fh.Close(); err != nil {
				daemon.logPrintStageStep(out, "failed to create swap file - %v", err)
				return
			}
			// If the file already exists, it will not be grown or recreated.
		} else if swapFileStatus != nil {
			daemon.logPrintStageStep(out, "failed to determine swap file status - %v", swapFileStatus)
			return
		} else {
			daemon.logPrintStage(out, "the swap file appears to already exist")
		}
		// Correct the swap file permission and ownership
		if err := os.Chmod(SwapFilePath, 0600); err != nil {
			daemon.logPrintStageStep(out, "failed to correct swap file permission - %v", err)
			return
		}
		if err := os.Chown(SwapFilePath, 0, 0); err != nil {
			daemon.logPrintStageStep(out, "failed to correct swap file owner - %v", err)
			return
		}
		// Format the swap file
		if progOut, err := platform.InvokeProgram(nil, misc.CommonOSCmdTimeoutSec, "mkswap", SwapFilePath); err != nil {
			daemon.logPrintStageStep(out, "failed to format swap file - %v - %s", err, progOut)
		}
		// Turn on the swap file
		progOut, err := platform.InvokeProgram(nil, misc.CommonOSCmdTimeoutSec, "swapon", SwapFilePath)
		if err != nil {
			daemon.logPrintStageStep(out, "failed to turn on swap file - %v - %s", err, progOut)
		}
	}
}

// MaintainFileSystem gets rid of unused temporary files on both Unix-like and Windows OSes.
func (daemon *Daemon) MaintainFileSystem(out *bytes.Buffer) {
	daemon.logPrintStage(out, "maintain file system")
	// Remove files from temporary locations that have not been modified for over a week
	daemon.logPrintStageStep(out, "clean up unused temporary files")
	sevenDaysAgo := time.Now().Add(-(7 * 24 * time.Hour))
	// Keep in mind that /var/tmp is supposed to hold "persistent temporary files" in Linux
	for _, location := range []string{`/tmp`, `C:\Temp`, `C:\Windows\Temp`} {
		filepath.Walk(location, func(thisPath string, info os.FileInfo, err error) error {
			if err == nil {
				if info.ModTime().Before(sevenDaysAgo) {
					if deleteErr := os.RemoveAll(thisPath); deleteErr == nil {
						daemon.logPrintStageStep(out, "deleted %s", thisPath)
					} else {
						daemon.logPrintStageStep(out, "failed to deleted %s - %v", thisPath, deleteErr)
					}
				}
			}
			return nil
		})
	}
}

// recursivelyChown changes owner and group of all files underneath the path, including the path itself.
func recursivelyChown(rootPath string, newUID, newGID int) (succeeded, failed int) {
	filepath.Walk(rootPath, func(thisPath string, info os.FileInfo, err error) error {
		if err := os.Lchown(thisPath, newUID, newGID); err == nil {
			succeeded++
		} else {
			failed++
		}
		return nil
	})
	return
}

// EnhanceFileSecurity hardens ownership and permission of common locations in file system.
func (daemon *Daemon) EnhanceFileSecurity(out *bytes.Buffer) {
	if !daemon.DoEnhanceFileSecurity {
		return
	}
	if misc.HostIsWindows() {
		daemon.logPrintStage(out, "skipped on windows: enhance file security")
		return
	}
	daemon.logPrintStage(out, "enhance file security")

	myUser, err := user.Current()
	if err != nil {
		daemon.logPrintStageStep(out, "failed to get current user - %v", err)
		return
	}
	// Will run chown on the following paths later
	pathUID := make(map[string]int)
	pathGID := make(map[string]int)
	// Will run chmod on the following paths later
	path600 := make(map[string]struct{})
	path700 := make(map[string]struct{})

	// Discover all ordinary user home directories
	allHomeDirAbs := make(map[string]struct{})
	if myUser.HomeDir != "" {
		allHomeDirAbs[myUser.HomeDir] = struct{}{}
	}
	for _, homeDirParent := range []string{"/home", "/Users"} {
		subDirs, err := ioutil.ReadDir(homeDirParent)
		if err != nil {
			continue
		}
		for _, subDir := range subDirs {
			allHomeDirAbs[filepath.Join(homeDirParent, subDir.Name())] = struct{}{}
		}
	}

	// Reset owner and group of an ordinary user's home directory
	for homeDirAbs := range allHomeDirAbs {
		userName := filepath.Base(homeDirAbs)
		if userName != "" && userName != "." {
			u, err := user.Lookup(userName)
			if err == nil {
				// Chown the home directory
				if i, err := strconv.Atoi(u.Gid); err == nil {
					pathGID[homeDirAbs] = i
				}
				if i, err := strconv.Atoi(u.Uid); err == nil {
					pathUID[homeDirAbs] = i
				}
			}
		}
	}

	// Reset owner and group of root home directory
	for _, rootHomeAbs := range []string{"/root", "/private/var/root"} {
		if stat, err := os.Stat(rootHomeAbs); err == nil && stat.IsDir() {
			pathUID[rootHomeAbs] = 0
			pathGID[rootHomeAbs] = 0
			// Reset permission on the home directory and ~/.ssh later
			allHomeDirAbs[rootHomeAbs] = struct{}{}
		}
	}

	// Reset permission on home directory and ~/.ssh
	for homeDirAbs := range allHomeDirAbs {
		// chmod 700 ~
		path700[homeDirAbs] = struct{}{}
		// Chmod 700 ~/.ssh
		sshDirAbs := filepath.Join(homeDirAbs, ".ssh")
		if stat, err := os.Stat(sshDirAbs); err == nil && stat.IsDir() {
			path700[sshDirAbs] = struct{}{}
			// Chmod 600 ~/.ssh/*
			if sshContent, err := ioutil.ReadDir(sshDirAbs); err == nil {
				for _, entry := range sshContent {
					path600[filepath.Join(sshDirAbs, entry.Name())] = struct{}{}
				}
			}
		}
	}

	// Do it!
	for aPath, newUID := range pathUID {
		succeeded, failed := recursivelyChown(aPath, newUID, -1)
		daemon.logPrintStageStep(out, "recursively set owner to %d in path %s - %d succeeded, %d failed", newUID, aPath, succeeded, failed)
	}
	for aPath, newGID := range pathGID {
		succeeded, failed := recursivelyChown(aPath, -1, newGID)
		daemon.logPrintStageStep(out, "recursively set group to %d in path %s - %d succeeded, %d failed", newGID, aPath, succeeded, failed)
	}
	for aPath := range path600 {
		daemon.logPrintStageStep(out, "set permission to 600 to path %s - %v", aPath, os.Chmod(aPath, 0600))

	}
	for aPath := range path700 {
		daemon.logPrintStageStep(out, "set permission to 700 to path %s - %v", aPath, os.Chmod(aPath, 0700))
	}
}

// RunPreMaintenanceScript runs the pre-maintenance script using system default script interpreter. The script is given 10 minutes to run.
func (daemon *Daemon) RunPreMaintenanceScript(out *bytes.Buffer) {
	var scriptOut string
	var err error
	if daemon.PreScriptUnix != "" && !misc.HostIsWindows() {
		daemon.logPrintStage(out, "run pre-maintenance script (unix-like)")
		scriptOut, err = misc.InvokeShell(10*600, misc.GetDefaultShellInterpreter(), daemon.PreScriptUnix)
	}
	if daemon.PreScriptWindows != "" && misc.HostIsWindows() {
		daemon.logPrintStage(out, "run pre-maintenance script (windows)")
		scriptOut, err = misc.InvokeShell(10*600, misc.GetDefaultShellInterpreter(), daemon.PreScriptWindows)
	}
	daemon.logPrintStage(out, "script result: %s - %v", scriptOut, err)
}
