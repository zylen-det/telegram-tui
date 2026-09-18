package config

import "path/filepath"

const appDirectory = "telegram-tui"

type Paths struct {
	ConfigFile     string
	StateDir       string
	TDLibLog       string
	DataDir        string
	TDLibDatabase  string
	TDLibFiles     string
	AvatarCacheDir string
}

func ResolvePaths(getenv func(string) string, home string) Paths {
	configRoot := xdgRoot(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
	stateRoot := xdgRoot(getenv("XDG_STATE_HOME"), filepath.Join(home, ".local", "state"))
	dataRoot := xdgRoot(getenv("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	cacheRoot := xdgRoot(getenv("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))

	configDir := filepath.Join(configRoot, appDirectory)
	stateDir := filepath.Join(stateRoot, appDirectory)
	dataDir := filepath.Join(dataRoot, appDirectory)
	avatarDir := filepath.Join(cacheRoot, appDirectory, "avatars")

	return Paths{
		ConfigFile:     filepath.Join(configDir, "config.toml"),
		StateDir:       stateDir,
		TDLibLog:       filepath.Join(stateDir, "tdlib.log"),
		DataDir:        dataDir,
		TDLibDatabase:  filepath.Join(dataDir, "tdlib", "database"),
		TDLibFiles:     filepath.Join(avatarDir, "files"),
		AvatarCacheDir: filepath.Join(avatarDir, "pixels"),
	}
}

func xdgRoot(value, fallback string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return fallback
}
