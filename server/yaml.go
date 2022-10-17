package server

type Config struct {
	Context struct {
		DefaultMemory string `yaml:"default_memory"`
		IndexPage     string `yaml:"index_page"`
	}
}
