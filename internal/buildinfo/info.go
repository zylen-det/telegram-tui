package buildinfo

var version = "dev"

type Info struct {
	Name    string
	Version string
}

func Current() Info {
	return Info{Name: "telegram-tui", Version: version}
}
