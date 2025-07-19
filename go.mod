module config

go 1.24.5

require (
	github.com/BurntSushi/toml v1.5.0
	github.com/lixenwraith/config v0.0.0-20250719015120-e02ee494d440
	github.com/mitchellh/mapstructure v1.5.0
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/mitchellh/mapstructure => github.com/go-viper/mapstructure v1.6.0
