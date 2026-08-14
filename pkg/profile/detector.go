package profile

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"pmux/pkg/config"
)

func DetectProfiles() []config.Profile {
	var profiles []config.Profile

	// 1. CMD
	cmdPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if _, err := os.Stat(cmdPath); err == nil {
		profiles = append(profiles, config.Profile{
			ID:       "cmd",
			Name:     "Command Prompt (CMD)",
			Command:  cmdPath,
			Args:     []string{},
			WorkDir:  "",
			IsPreset: true,
		})
	}

	// 2. PowerShell / pwsh
	pwshPath, err := exec.LookPath("pwsh.exe")
	if err == nil {
		profiles = append(profiles, config.Profile{
			ID:       "pwsh",
			Name:     "PowerShell Core (pwsh)",
			Command:  pwshPath,
			Args:     []string{},
			WorkDir:  "",
			IsPreset: true,
		})
	}

	psPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(psPath); err == nil {
		profiles = append(profiles, config.Profile{
			ID:       "powershell",
			Name:     "Windows PowerShell",
			Command:  psPath,
			Args:     []string{},
			WorkDir:  "",
			IsPreset: true,
		})
	}

	// 3. MSYS2 Environments
	msysCandidates := []string{
		`C:\msys64\msys2_shell.cmd`,
		`C:\msys2\msys2_shell.cmd`,
		`D:\msys64\msys2_shell.cmd`,
		`D:\msys2\msys2_shell.cmd`,
	}

	var foundMsys string
	for _, cand := range msysCandidates {
		if _, err := os.Stat(cand); err == nil {
			foundMsys = cand
			break
		}
	}

	if foundMsys != "" {
		profiles = append(profiles,
			config.Profile{
				ID:       "msys2-mingw64",
				Name:     "MSYS2 MINGW64",
				Command:  foundMsys,
				Args:     []string{"-defterm", "-here", "-no-start", "-mingw64", "-use-full-path"},
				WorkDir:  "",
				IsPreset: true,
			},
			config.Profile{
				ID:       "msys2-clang64",
				Name:     "MSYS2 CLANG64",
				Command:  foundMsys,
				Args:     []string{"-defterm", "-here", "-no-start", "-clang64", "-use-full-path"},
				WorkDir:  "",
				IsPreset: true,
			},
			config.Profile{
				ID:       "msys2-ucrt64",
				Name:     "MSYS2 UCRT64",
				Command:  foundMsys,
				Args:     []string{"-defterm", "-here", "-no-start", "-ucrt64", "-use-full-path"},
				WorkDir:  "",
				IsPreset: true,
			},
			config.Profile{
				ID:       "msys2-mingw32",
				Name:     "MSYS2 MINGW32",
				Command:  foundMsys,
				Args:     []string{"-defterm", "-here", "-no-start", "-mingw32", "-use-full-path"},
				WorkDir:  "",
				IsPreset: true,
			},
		)
	}

	// 4. Git Bash
	gitBashCandidates := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	}
	for _, gb := range gitBashCandidates {
		if _, err := os.Stat(gb); err == nil {
			profiles = append(profiles, config.Profile{
				ID:       "git-bash",
				Name:     "Git Bash",
				Command:  gb,
				Args:     []string{"--login", "-i"},
				WorkDir:  "",
				IsPreset: true,
			})
			break
		}
	}

	// 5. Visual Studio Developer Command Prompt & Developer PowerShell
	vsProfiles := detectVisualStudioProfiles()
	profiles = append(profiles, vsProfiles...)

	return profiles
}

func detectVisualStudioProfiles() []config.Profile {
	var profiles []config.Profile

	cmdPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	psPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")

	vsPathSet := make(map[string]bool)
	var vsPaths []string

	// 1. Try vswhere.exe if installed
	vswhereExe := `C:\Program Files (x86)\Microsoft Visual Studio\Installer\vswhere.exe`
	if _, err := os.Stat(vswhereExe); err == nil {
		cmd := exec.Command(vswhereExe, "-prerelease", "-property", "installationPath")
		prepareCommand(cmd)
		output, err := cmd.Output()
		if err == nil {
			lines := strings.Split(string(output), "\r\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed != "" && !vsPathSet[trimmed] {
					if _, errStat := os.Stat(trimmed); errStat == nil {
						vsPathSet[trimmed] = true
						vsPaths = append(vsPaths, trimmed)
					}
				}
			}
		}
	}

	// 2. Static candidate fallbacks (VS 2022 & VS 2019 across drives)
	years := []string{"2022", "2019"}
	editions := []string{"Community", "Professional", "Enterprise", "BuildTools"}
	baseDrives := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft Visual Studio"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft Visual Studio"),
		`D:\Program Files\Microsoft Visual Studio`,
		`D:\Program Files (x86)\Microsoft Visual Studio`,
	}

	for _, drive := range baseDrives {
		if drive == "" {
			continue
		}
		for _, year := range years {
			for _, ed := range editions {
				vsDir := filepath.Join(drive, year, ed)
				if !vsPathSet[vsDir] {
					if _, err := os.Stat(vsDir); err == nil {
						vsPathSet[vsDir] = true
						vsPaths = append(vsPaths, vsDir)
					}
				}
			}
		}
	}

	// 3. For each found VS path, generate Developer Command Prompt and Developer PowerShell
	for _, vsDir := range vsPaths {
		edName := filepath.Base(vsDir)
		vsName := edName
		if strings.Contains(vsDir, "2022") {
			vsName = "2022 (" + edName + ")"
		} else if strings.Contains(vsDir, "2019") {
			vsName = "2019 (" + edName + ")"
		}

		vsDevCmd := filepath.Join(vsDir, "Common7", "Tools", "VsDevCmd.bat")
		if _, err := os.Stat(vsDevCmd); err == nil {
			profiles = append(profiles, config.Profile{
				ID:       "vs-cmd-" + edName,
				Name:     "Developer Command Prompt for VS " + vsName,
				Command:  cmdPath,
				Args:     []string{"/k", vsDevCmd},
				WorkDir:  "",
				IsPreset: true,
			})
		}

		devShellDll := filepath.Join(vsDir, "Common7", "Tools", "Microsoft.VisualStudio.DevShell.dll")
		if _, err := os.Stat(devShellDll); err == nil {
			psArgs := []string{"-NoExit", "-Command", fmt.Sprintf("& {Import-Module '%s'; Enter-VsDevShell -VsInstallFolder '%s' -SkipAutomaticLocation}", devShellDll, vsDir)}
			profiles = append(profiles, config.Profile{
				ID:       "vs-ps-" + edName,
				Name:     "Developer PowerShell for VS " + vsName,
				Command:  psPath,
				Args:     psArgs,
				WorkDir:  "",
				IsPreset: true,
			})
		}
	}

	return profiles
}
