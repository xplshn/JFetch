// TODO: Use syscalls for everything
// efetch is a clone of https://raw.githubusercontent.com/eepykate/fet.sh/master/fet.sh
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v4/mem"
)

type SystemInfo struct {
	WM       string
	OS       string
	Terminal string
	Memory   string
	CPU      string
	Uptime   string
	Kernel   string
	Model    string
	Packages int
	Host     string
}

func getMemory(info *unix.Sysinfo_t) string {
	if memStats, err := mem.VirtualMemory(); err == nil {
		return strconv.Itoa(int(memStats.Used/1048576)) + " / " + strconv.Itoa(int(memStats.Total/1048576)) + " MiB"
	}
	return ""
}

func getUptime(info *unix.Sysinfo_t) string {
	uptime := int(info.Uptime)
	days := uptime / (24 * 3600)
	hours := (uptime % (24 * 3600)) / 3600
	minutes := (uptime % 3600) / 60
	return fmt.Sprintf("%dd %02d:%02d", days, hours, minutes)
}

func getKernel(utsname *unix.Utsname) string {
	n := 0
	for ; n < len(utsname.Release); n++ {
		if utsname.Release[n] == 0 {
			break
		}
	}
	return string(utsname.Release[:n])
}

func main() {
	printInfo := func(label, value string) string {
		if value != "" {
			return fmt.Sprintf("\x1b[34m%6s\x1b[0m ~ %s", label, value)
		}
		return ""
	}

	getHost := func() string {
		hostname, err := os.Hostname()
		if err != nil {
			return "Unknown Host"
		}
		u, err := user.Current()
		if err != nil {
			return "Unknown User"
		}
		username := u.Username
		data := fmt.Sprintf("%6s@%s", username, hostname)
		return data
	}

	getWM := func() string {
		if wm := os.Getenv("XDG_CURRENT_DESKTOP"); wm != "" {
			return wm
		}
		if wm := os.Getenv("DESKTOP_SESSION"); wm != "" {
			return wm
		}
		pattern := regexp.MustCompile(`(awesome|xmonad.*|qtile|sway|i3|[bfo]*box|.*wm)`)
		files, err := os.ReadDir("/proc")
		if err != nil {
			return ""
		}
		for _, file := range files {
			if _, err := strconv.Atoi(file.Name()); err != nil {
				continue
			}
			commPath := filepath.Join("/proc", file.Name(), "comm")
			f, err := os.Open(commPath)
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				comm := scanner.Text()
				if pattern.MatchString(comm) {
					f.Close()
					return comm
				}
			}
			f.Close()
		}
		return ""
	}

	getModel := func() string {
		f, err := os.Open("/sys/devices/virtual/dmi/id/product_name")
		if err != nil {
			return "Unknown Model"
		}
		scanner := bufio.NewScanner(f)
		if scanner.Scan() {
			model := scanner.Text()
			f.Close()
			return model
		}
		f.Close()
		return "Unknown Model"
	}

	getOS := func() string {
		files := []string{"/etc/os-release", "/usr/lib/os-release"}
		for _, file := range files {
			f, err := os.Open(file)
			if err != nil {
				continue
			}
			defer f.Close() // Ensure file is closed properly

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					return strings.Trim(line[12:], "\"") // Trim quotes if present
				}
			}
			if err := scanner.Err(); err != nil {
				continue
			}
		}

		return "Unknown OS"
	}

	getTerminal := func() string {
		// Use tput to get the terminal type
		if _, err := exec.Command("tput", "-T", "try", "setaf", "0").Output(); err == nil {
			// Use `exec.Command` directly in the return statement to avoid allocation
			output, err := exec.Command("tput", "term").Output()
			if err == nil {
				return string(output)
			}
		}
		// Fallback to $TERM environment variable
		return os.Getenv("TERM")
	}

	getCPU := func() string {
		f, err := os.Open("/proc/cpuinfo")
		if err != nil {
			return "Unknown CPU"
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "model name") {
				cpu := strings.SplitN(line, ":", 2)[1]
				f.Close()
				return strings.TrimSpace(cpu)
			}
		}
		f.Close()
		return "Unknown CPU"
	}

	getPackages := func(osName string) int {
		var wg sync.WaitGroup
		var mu sync.Mutex
		totalPackages := 0

		runCommand := func(cmd string, args ...string) (string, error) {
			command := exec.Command(cmd, args...)
			var out bytes.Buffer
			command.Stdout = &out
			err := command.Run()
			return out.String(), err
		}

		countPackages := func(cmd string, args ...string) (int, error) {
			if _, err := exec.LookPath(cmd); err != nil {
				return 0, nil
			}
			output, err := runCommand(cmd, args...)
			if err != nil {
				return 0, err
			}
			lines := strings.Split(output, "\n")
			return len(lines) - 1, nil
		}

		countFilesInDir := func(dir string) (int, error) {
			count := 0
			err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					count++
				}
				return nil
			})
			if err != nil {
				return 0, err
			}
			return count, nil
		}

		countPackagesInDir := func(dirs ...string) (int, error) {
			total := 0
			for _, dir := range dirs {
				matches, err := filepath.Glob(dir)
				if err != nil {
					return 0, err
				}
				for _, match := range matches {
					count, err := countFilesInDir(match)
					if err != nil {
						return 0, err
					}
					total += count
				}
			}
			return total, nil
		}

		packageManagerPatterns := map[*regexp.Regexp][]string{
			regexp.MustCompile(`(?i)^debian`):   {"dpkg-query", "-f", ".", "-W"},
			regexp.MustCompile(`(?i)^ubuntu`):   {"dpkg-query", "-f", ".", "-W"},
			regexp.MustCompile(`(?i)^arch`):     {"pacman", "-Q"},
			regexp.MustCompile(`(?i)^fedora`):   {"rpm", "-qa"},
			regexp.MustCompile(`(?i)^alpine`):   {"apk", "info"},
			regexp.MustCompile(`(?i)^gentoo`):   {"equery", "list"},
			regexp.MustCompile(`(?i)^opensuse`): {"zypper", "se", "-i"},
		}

		dirPatterns := map[*regexp.Regexp][]string{
			regexp.MustCompile(`(?i)^kiss`):     {"/var/db/kiss/installed/*/"},
			regexp.MustCompile(`(?i)^cpt-list`): {"/var/db/cpt/installed/*/"},
			regexp.MustCompile(`(?i)^homebrew`): {"/usr/local/Cellar/*/", "/usr/local/Caskroom/*/"},
			regexp.MustCompile(`(?i)^portage`):  {"/var/db/pkg/*/*/"},
			regexp.MustCompile(`(?i)^pkgtool`):  {"/var/log/packages/*"},
			regexp.MustCompile(`(?i)^eopkg`):    {"/var/lib/eopkg/package/*"},
		}

		excludePkgm := os.Getenv("EF_EXCLUDE_PKGM")
		excludePkgmList := strings.Split(excludePkgm, ",")

		for pattern, args := range packageManagerPatterns {
			if pattern.MatchString(osName) && !contains(excludePkgmList, args[0]) {
				wg.Add(1)
				go func(cmd string, args []string) {
					defer wg.Done()
					count, err := countPackages(cmd, args...)
					if err == nil {
						mu.Lock()
						totalPackages += count
						mu.Unlock()
					}
				}(args[0], args[1:])
			}
		}

		for pattern, dirs := range dirPatterns {
			if pattern.MatchString(osName) && !contains(excludePkgmList, pattern.String()) {
				wg.Add(1)
				go func(dirs []string) {
					defer wg.Done()
					count, err := countPackagesInDir(dirs...)
					if err == nil {
						mu.Lock()
						totalPackages += count
						mu.Unlock()
					}
				}(dirs)
			}
		}

		wg.Wait()
		return totalPackages
	}

	var info unix.Sysinfo_t
	unix.Sysinfo(&info)

	var utsname unix.Utsname
	unix.Uname(&utsname)

	osName := os.Getenv("EF_OSNAME")
	if osName == "" {
		osName = getOS()
	}

	sysInfo := SystemInfo{
		WM:       getWM(),
		OS:       osName,
		Terminal: getTerminal(),
		Memory:   getMemory(&info),
		CPU:      getCPU(),
		Uptime:   getUptime(&info),
		Kernel:   getKernel(&utsname),
		Model:    getModel(),
		Packages: getPackages(osName),
		Host:     getHost(),
	}

	// Retrieve the logo
	logo := GetLogo(osName)

	// Make strings for injection
	injectStrings := []string{
		fmt.Sprintf("\x1b[34m%6s\x1b[0m", sysInfo.Host),
		printInfo("os", sysInfo.OS),
		printInfo("kern", sysInfo.Kernel),
		printInfo("up", sysInfo.Uptime),
		printInfo("wm", sysInfo.WM),
		printInfo("term", sysInfo.Terminal),
		printInfo("cpu", sysInfo.CPU),
		printInfo("mem", sysInfo.Memory),
		printInfo("host", sysInfo.Model),
	}
	if pkgs := sysInfo.Packages; pkgs != 0 {
		injectStrings = append(injectStrings, printInfo("pkgs", strconv.Itoa(pkgs)))
	}

	// Ensure logo and injectStrings are of equal length
	for len(logo) < len(injectStrings) {
		logo = append(logo, "")
	}
	for len(injectStrings) < len(logo) {
		injectStrings = append(injectStrings, "")
	}

	// Print the logo and system info side by side
	for i := 0; i < len(logo); i++ {
		fmt.Printf("%-15s\t%s\n", logo[i], injectStrings[i])
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
