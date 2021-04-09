package chartify

type Requirements struct {
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
}

type Dependency struct {
	Name       string `yaml:"name,omitempty"`
	Repository string `yaml:"repository,omitempty"`
	Condition  string `yaml:"condition,omitempty"`
	Alias      string `yaml:"alias,omitempty"`
	Version    string `yaml:"version,omitempty"`
}

type ChartDependency struct {
	Alias   string
	Chart   string
	Version string
}
